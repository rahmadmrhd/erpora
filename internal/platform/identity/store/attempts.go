package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
)

// RecordLoginAttempt mencatat satu percobaan autentikasi (kind: AttemptPassword atau AttemptMFA) untuk pengguna
// yang dikenal. ErrNotFound bila pengguna bukan anggota tenant t. Catat sukses hanya ketika autentikasi selesai
// penuh (setelah MFA bila ada), karena sukses mereset hitungan pasangan (akun, IP).
func (s *Store) RecordLoginAttempt(ctx context.Context, t tenancy.TenantID, userID, ip, kind string, success bool, at time.Time) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	if (kind != AttemptPassword && kind != AttemptMFA) || len(ip) < 1 || len(ip) > 45 {
		return ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `INSERT INTO login_attempts (tenant_id, user_id, ip, kind, success, attempted_at)
		SELECT m.tenant_id, m.user_id, ?::text, ?::text, ?::boolean, ?::timestamptz FROM tenant_memberships m
		WHERE m.tenant_id = ? AND m.user_id = ?`, ip, kind, success, at.UTC(), tid(t), uid)
	if err != nil {
		return wrap("catat percobaan login", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PairFailureState menghitung kegagalan berturut-turut pasangan (akun, IP) tanpa batas waktu: jumlah gagal setelah
// sukses terakhir pasangan itu (bila ada) beserta waktu gagal terakhir. Sama dengan PairFailureStateSince dengan since
// nol. Dasar jeda bertingkat dan pemblokiran pasangan (Q16).
func (s *Store) PairFailureState(ctx context.Context, t tenancy.TenantID, userID, ip string) (PairState, error) {
	return s.PairFailureStateSince(ctx, t, userID, ip, time.Time{})
}

// PairFailureStateSince seperti PairFailureState tetapi hanya menghitung kegagalan dengan waktu >= since
// (keputusan pemilik produk 3 Nov 2026: hanya 24 jam terakhir yang dihitung; kegagalan lebih tua diabaikan). Sukses
// terakhir pasangan tetap mereset hitungan, berapa pun usianya. since nol berarti tanpa batas.
func (s *Store) PairFailureStateSince(ctx context.Context, t tenancy.TenantID, userID, ip string, since time.Time) (PairState, error) {
	uid, ok := parseID(userID)
	if !ok {
		return PairState{}, nil
	}
	return pairState(s.db(ctx), t, uid, ip, since)
}

// pairState adalah kueri keadaan pasangan; db dapat berupa koneksi biasa atau transaksi (ReserveLoginAttempt).
func pairState(db *gorm.DB, t tenancy.TenantID, uid uuid.UUID, ip string, since time.Time) (PairState, error) {
	lower := "'-infinity'::timestamptz"
	args := []any{tid(t), uid, ip}
	if !since.IsZero() {
		lower = "?::timestamptz"
		args = append(args, since.UTC())
	}
	args = append(args, tid(t), uid, ip)
	var r struct {
		ConsecutiveFailures int        `gorm:"column:consecutive_failures"`
		LastFailureAt       *time.Time `gorm:"column:last_failure_at"`
	}
	if err := db.Raw(`SELECT count(*) AS consecutive_failures, max(attempted_at) AS last_failure_at
		FROM login_attempts
		WHERE tenant_id = ? AND user_id = ? AND ip = ? AND success = false AND attempted_at >= `+lower+`
		  AND attempted_at > COALESCE((SELECT max(attempted_at) FROM login_attempts
		       WHERE tenant_id = ? AND user_id = ? AND ip = ? AND success = true), '-infinity'::timestamptz)`,
		args...).Scan(&r).Error; err != nil {
		return PairState{}, wrap("hitung kegagalan pasangan", err)
	}
	return PairState{ConsecutiveFailures: r.ConsecutiveFailures, LastFailureAt: utcPtr(r.LastFailureAt)}, nil
}

// CrossIPFailures menghitung semua kegagalan akun sejak since beserta jumlah IP berbeda (ambang kunci akun lintas
// IP, Q16). Sukses dari IP mana pun tidak mereset hitungan ini.
func (s *Store) CrossIPFailures(ctx context.Context, t tenancy.TenantID, userID string, since time.Time) (CrossIPState, error) {
	uid, ok := parseID(userID)
	if !ok {
		return CrossIPState{}, nil
	}
	var r struct {
		Failures    int `gorm:"column:failures"`
		DistinctIPs int `gorm:"column:distinct_ips"`
	}
	if err := s.db(ctx).Raw(`SELECT count(*) AS failures, count(DISTINCT ip) AS distinct_ips FROM login_attempts
		WHERE tenant_id = ? AND user_id = ? AND success = false AND attempted_at >= ?`, tid(t), uid, since.UTC()).Scan(&r).Error; err != nil {
		return CrossIPState{}, wrap("hitung kegagalan lintas IP", err)
	}
	return CrossIPState{Failures: r.Failures, DistinctIPs: r.DistinctIPs}, nil
}

// PruneLoginAttempts menghapus catatan percobaan tenant t yang lebih lama dari before (pemeliharaan retensi).
func (s *Store) PruneLoginAttempts(ctx context.Context, t tenancy.TenantID, before time.Time) (int64, error) {
	n, err := exec(s.db(ctx), `DELETE FROM login_attempts WHERE tenant_id = ? AND attempted_at < ?`, tid(t), before.UTC())
	if err != nil {
		return 0, wrap("bersihkan percobaan login", err)
	}
	return n, nil
}

// advisoryClassLoginPair adalah kelas kunci advisory (argumen pertama bentuk dua-int) untuk serialisasi per pasangan
// (tenant, akun, IP); kelas tersendiri agar tidak bertabrakan dengan kunci advisory lain (mis. goose).
const advisoryClassLoginPair int32 = 0x4C504152 // "LPAR"

// ReserveLoginAttempt memeriksa dan mereservasi satu percobaan autentikasi untuk pasangan (userID, ip) secara ATOMIK
// lintas instance (PRD-001 Q16/Q19, ADR-0013 section 5): dalam satu transaksi singkat ia mengambil kunci advisory
// transaksional per pasangan, membaca keadaan pasangan (kegagalan >= since sejak sukses terakhir), memanggil admit
// dengan keadaan itu, dan HANYA bila admit mengembalikan true menyisipkan satu baris kegagalan (kind: AttemptPassword
// atau AttemptMFA) pada waktu at. Kunci dilepas saat commit, SEBELUM pemanggil memverifikasi kredensial (argon2/TOTP),
// sehingga tidak ada koneksi atau kunci yang ditahan selama hashing dan tidak ada deadlock. Akibatnya permintaan
// serentak saling melihat reservasi satu sama lain: hanya sebanyak yang diizinkan jadwal jeda yang lolos ke verifikasi.
//
// Pemanggil WAJIB menyelesaikan reservasi: gagal -> biarkan baris (sudah tercatat sebagai kegagalan); selain gagal
// (sukses, dibatasi laju, galat infrastruktur) -> ReleaseLoginAttempt. Mengembalikan keadaan pasangan SEBELUM
// reservasi, apakah diterima, dan id baris reservasi (0 bila ditolak). admit dipanggil di dalam transaksi: harus
// murni (tanpa I/O). ErrNotFound bila pengguna bukan anggota tenant (setelah kunci diambil dan keadaan dibaca, agar
// jejak database sama dengan pengguna nyata: dipakai pemanggil untuk operasi tiruan).
func (s *Store) ReserveLoginAttempt(ctx context.Context, t tenancy.TenantID, userID, ip, kind string, at, since time.Time, admit func(PairState) bool) (PairState, bool, int64, error) {
	uid, ok := parseID(userID)
	if !ok {
		return PairState{}, false, 0, ErrNotFound
	}
	if (kind != AttemptPassword && kind != AttemptMFA) || len(ip) < 1 || len(ip) > 45 || admit == nil {
		return PairState{}, false, 0, ErrInvalidArgument
	}
	var (
		before   PairState
		admitted bool
		id       int64
	)
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?::int, hashtext(?))`, advisoryClassLoginPair, t.String()+"|"+uid.String()+"|"+ip).Error; err != nil {
			return err
		}
		var err error
		if before, err = pairState(tx, t, uid, ip, since); err != nil {
			return err
		}
		if admitted = admit(before); !admitted {
			// Ditolak: tidak menulis apa pun.
			return nil
		}
		return tx.Raw(`INSERT INTO login_attempts (tenant_id, user_id, ip, kind, success, attempted_at)
			SELECT m.tenant_id, m.user_id, ?::text, ?::text, false, ?::timestamptz FROM tenant_memberships m
			WHERE m.tenant_id = ? AND m.user_id = ? RETURNING id`, ip, kind, at.UTC(), tid(t), uid).Scan(&id).Error
	})
	if err != nil {
		return PairState{}, false, 0, wrap("reservasi percobaan login", err)
	}
	if admitted && id == 0 {
		return before, false, 0, ErrNotFound
	}
	return before, admitted, id, nil
}

// ReleaseLoginAttempt menghapus baris reservasi (kegagalan) id milik (tenant, pengguna): dipanggil ketika percobaan
// ternyata bukan kegagalan (sandi/kode benar, dibatasi laju, atau galat infrastruktur). Idempoten: baris yang sudah
// tidak ada bukan galat. Hanya baris success=false milik pengguna itu yang dapat dihapus.
func (s *Store) ReleaseLoginAttempt(ctx context.Context, t tenancy.TenantID, userID string, id int64) error {
	uid, ok := parseID(userID)
	if !ok {
		return nil
	}
	if _, err := exec(s.db(ctx), `DELETE FROM login_attempts WHERE id = ? AND tenant_id = ? AND user_id = ? AND success = false`, id, tid(t), uid); err != nil {
		return wrap("lepas reservasi percobaan login", err)
	}
	return nil
}
