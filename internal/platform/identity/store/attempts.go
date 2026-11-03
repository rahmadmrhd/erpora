package store

import (
	"context"
	"time"

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

// PairFailureState menghitung kegagalan berturut-turut pasangan (akun, IP): jumlah gagal setelah sukses terakhir
// pasangan itu (bila ada) beserta waktu gagal terakhir. Dasar jeda bertingkat dan pemblokiran pasangan (Q16).
func (s *Store) PairFailureState(ctx context.Context, t tenancy.TenantID, userID, ip string) (PairState, error) {
	uid, ok := parseID(userID)
	if !ok {
		return PairState{}, nil
	}
	var r struct {
		ConsecutiveFailures int        `gorm:"column:consecutive_failures"`
		LastFailureAt       *time.Time `gorm:"column:last_failure_at"`
	}
	if err := s.db(ctx).Raw(`SELECT count(*) AS consecutive_failures, max(attempted_at) AS last_failure_at
		FROM login_attempts
		WHERE tenant_id = ? AND user_id = ? AND ip = ? AND success = false
		  AND attempted_at > COALESCE((SELECT max(attempted_at) FROM login_attempts
		       WHERE tenant_id = ? AND user_id = ? AND ip = ? AND success = true), '-infinity'::timestamptz)`,
		tid(t), uid, ip, tid(t), uid, ip).Scan(&r).Error; err != nil {
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
