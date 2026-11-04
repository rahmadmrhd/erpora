package store

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
	sdkmail "erpora/sdk/mail"
)

type userRow struct {
	ID               uuid.UUID  `gorm:"column:id"`
	Email            string     `gorm:"column:email"`
	Name             string     `gorm:"column:name"`
	Status           string     `gorm:"column:status"`
	MembershipStatus string     `gorm:"column:membership_status"`
	PasswordHash     *string    `gorm:"column:password_hash"`
	EmailVerifiedAt  *time.Time `gorm:"column:email_verified_at"`
	LastLoginAt      *time.Time `gorm:"column:last_login_at"`
	LockedAt         *time.Time `gorm:"column:locked_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (userRow) TableName() string { return "users" }

func (r userRow) user() User {
	u := User{
		ID: idString(r.ID), Email: r.Email, Name: r.Name, Status: r.Status, MembershipStatus: r.MembershipStatus,
		EmailVerifiedAt: utcPtr(r.EmailVerifiedAt), LastLoginAt: utcPtr(r.LastLoginAt), LockedAt: utcPtr(r.LockedAt),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.PasswordHash != nil {
		u.PasswordHash = *r.PasswordHash
	}
	return u
}

type membershipRow struct {
	UserID    uuid.UUID `gorm:"column:user_id"`
	Status    string    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (membershipRow) TableName() string { return "tenant_memberships" }

// Pengguna hanya terlihat lewat keanggotaan tenant; kueri berikut selalu menggabungkan tenant_memberships.
const userByMember = `SELECT u.*, m.status AS membership_status FROM users u JOIN tenant_memberships m ON m.user_id = u.id AND m.tenant_id = ? WHERE `

// validEmail menerima alamat yang dapat dikirimi: batas panjang CHECK database DAN aturan ketat sdk/mail (alamat polos
// ASCII lokal@domain.tld), sehingga akun yang tidak dapat menerima surat reset sandi tidak dapat dibuat. Hanya jalur tulis;
// data lama tidak dimigrasi (alur reset menangani alamat lama yang tak dapat dikirimi tanpa menghanguskan tautan).
func validEmail(e string) bool {
	if utf8.RuneCountInString(e) < 3 || utf8.RuneCountInString(e) > 254 || strings.Index(e, "@") <= 0 {
		return false
	}
	return sdkmail.Validate(sdkmail.Message{To: []string{e}, Subject: "x", Text: "x"}) == nil
}

// CreateUser membuat akun dan keanggotaannya pada tenant t secara atomik. Email dinormalisasi; email yang sudah ada
// (di tenant mana pun: identitas global) menghasilkan ErrEmailTaken, juga pada pembuatan serentak.
func (s *Store) CreateUser(ctx context.Context, t tenancy.TenantID, in NewUser) (User, error) {
	email, name := NormalizeEmail(in.Email), strings.TrimSpace(in.Name)
	status := in.Status
	if status == "" {
		status = UserActive
	}
	if !validEmail(email) || name == "" || utf8.RuneCountInString(name) > 200 || len(in.PasswordHash) > 512 ||
		(status != UserActive && status != UserDisabled) {
		return User{}, ErrInvalidArgument
	}
	id, err := uuid.NewV7()
	if err != nil {
		return User{}, wrap("buat id user", err)
	}
	var ph any
	if in.PasswordHash != "" {
		ph = in.PasswordHash
	}
	at := nowUTC()
	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := exec(tx, `INSERT INTO users (id, email, name, status, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (email) DO NOTHING`, id, email, name, status, ph, at, at)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrEmailTaken
		}
		_, err = exec(tx, `INSERT INTO tenant_memberships (tenant_id, user_id, status, created_at) VALUES (?, ?, 'active', ?)`, tid(t), id, at)
		return err
	})
	if err != nil {
		if err == ErrEmailTaken {
			return User{}, ErrEmailTaken
		}
		return User{}, wrap("buat user", err)
	}
	return s.GetUser(ctx, t, idString(id))
}

// GetUser memuat pengguna anggota tenant t; selain itu ErrNotFound (termasuk ID bukan UUID).
func (s *Store) GetUser(ctx context.Context, t tenancy.TenantID, id string) (User, error) {
	uid, ok := parseID(id)
	if !ok {
		return User{}, ErrNotFound
	}
	return s.oneUser(ctx, userByMember+`u.id = ?`, tid(t), uid)
}

// FindUserByEmail memuat pengguna anggota tenant t menurut email (dinormalisasi); selain itu ErrNotFound.
func (s *Store) FindUserByEmail(ctx context.Context, t tenancy.TenantID, email string) (User, error) {
	return s.oneUser(ctx, userByMember+`u.email = ?`, tid(t), NormalizeEmail(email))
}

func (s *Store) oneUser(ctx context.Context, q string, args ...any) (User, error) {
	var r userRow
	res := s.db(ctx).Raw(q, args...).Scan(&r)
	if res.Error != nil {
		return User{}, wrap("muat user", res.Error)
	}
	if res.RowsAffected == 0 {
		return User{}, ErrNotFound
	}
	return r.user(), nil
}

// GetMembership memuat keanggotaan pengguna pada tenant t; ErrNotFound bila bukan anggota.
func (s *Store) GetMembership(ctx context.Context, t tenancy.TenantID, userID string) (Membership, error) {
	uid, ok := parseID(userID)
	if !ok {
		return Membership{}, ErrNotFound
	}
	var r membershipRow
	res := s.db(ctx).Raw(`SELECT user_id, status, created_at FROM tenant_memberships WHERE tenant_id = ? AND user_id = ?`, tid(t), uid).Scan(&r)
	if res.Error != nil {
		return Membership{}, wrap("muat keanggotaan", res.Error)
	}
	if res.RowsAffected == 0 {
		return Membership{}, ErrNotFound
	}
	return Membership{UserID: idString(r.UserID), Status: r.Status, CreatedAt: r.CreatedAt.UTC()}, nil
}

// AddMembership menjadikan pengguna yang sudah ada anggota tenant t. created=false bila sudah menjadi anggota;
// ErrNotFound bila pengguna tidak ada.
func (s *Store) AddMembership(ctx context.Context, t tenancy.TenantID, userID, status string) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, ErrNotFound
	}
	if status != MembershipActive && status != MembershipSuspended {
		return false, ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `INSERT INTO tenant_memberships (tenant_id, user_id, status, created_at)
		SELECT ?::uuid, u.id, ?::text, ?::timestamptz FROM users u WHERE u.id = ? ON CONFLICT DO NOTHING`, tid(t), status, nowUTC(), uid)
	if err != nil {
		return false, wrap("tambah keanggotaan", err)
	}
	if n == 1 {
		return true, nil
	}
	var exists int64
	if err := s.db(ctx).Raw(`SELECT count(*) FROM users WHERE id = ?`, uid).Scan(&exists).Error; err != nil {
		return false, wrap("tambah keanggotaan", err)
	}
	if exists == 0 {
		return false, ErrNotFound
	}
	return false, nil
}

// SetPasswordHash mengganti hash sandi (PHC). Pencabutan sesi adalah urusan pemanggil (lihat ChangePassword dan
// ResetPassword untuk jalur atomik).
func (s *Store) SetPasswordHash(ctx context.Context, t tenancy.TenantID, userID, hash string) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	if hash == "" || len(hash) > 512 {
		return ErrInvalidArgument
	}
	// urutan placeholder: set, lalu id, lalu tenant (memberExists memakai tenant di posisi pertamanya)
	n, err := exec(s.db(ctx), `UPDATE users SET password_hash = ?, updated_at = now() WHERE id = ? AND `+memberExists("users.id"), hash, uid, tid(t))
	if err != nil {
		return wrap("ganti hash sandi", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RehashPassword menyimpan hash dengan parameter terbaru setelah login, HANYA bila password_hash masih sama dengan
// oldHash (compare-and-set): reset/ganti sandi yang selesai di antara verifikasi dan rehash tidak boleh ditimpa hash
// turunan sandi lama. false (tanpa galat) bila hash sudah berubah atau pengguna bukan anggota tenant t.
func (s *Store) RehashPassword(ctx context.Context, t tenancy.TenantID, userID, oldHash, newHash string) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	if oldHash == "" || newHash == "" || len(oldHash) > 512 || len(newHash) > 512 {
		return false, ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `UPDATE users SET password_hash = ?, updated_at = now() WHERE id = ? AND password_hash = ? AND `+memberExists("users.id"), newHash, uid, oldHash, tid(t))
	if err != nil {
		return false, wrap("hash ulang sandi", err)
	}
	return n == 1, nil
}

// TouchLastLogin mencatat waktu login berhasil terakhir.
func (s *Store) TouchLastLogin(ctx context.Context, t tenancy.TenantID, userID string, at time.Time) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	n, err := exec(s.db(ctx), `UPDATE users SET last_login_at = ?, updated_at = now() WHERE id = ? AND `+memberExists("users.id"), at.UTC(), uid, tid(t))
	if err != nil {
		return wrap("catat login terakhir", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserStatus menetapkan status active atau disabled (tanpa syarat). Status locked hanya lewat LockUser, dan
// pembukaan kunci lewat UnlockUser atau ResetPassword (keduanya menghapus catatan percobaan login).
func (s *Store) SetUserStatus(ctx context.Context, t tenancy.TenantID, userID, status string) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	if status != UserActive && status != UserDisabled {
		return ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `UPDATE users SET status = ?, locked_at = NULL, updated_at = now() WHERE id = ? AND `+memberExists("users.id"), status, uid, tid(t))
	if err != nil {
		return wrap("ubah status user", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// LockUser mengunci akun active (bersyarat atomik); false bila akun tidak berstatus active (sudah locked/disabled)
// atau bukan anggota tenant t.
func (s *Store) LockUser(ctx context.Context, t tenancy.TenantID, userID string, at time.Time) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	n, err := exec(s.db(ctx), `UPDATE users SET status = 'locked', locked_at = ?, updated_at = now() WHERE id = ? AND status = 'active' AND `+memberExists("users.id"), at.UTC(), uid, tid(t))
	if err != nil {
		return false, wrap("kunci user", err)
	}
	return n == 1, nil
}

// UnlockUser membuka akun locked (bila ada) dan SELALU menghapus catatan percobaan login pengguna itu pada tenant t,
// sehingga jeda/blokir per pasangan (akun, IP) pada akun yang masih active juga hilang dan jendela lintas IP tidak
// langsung mengunci ulang. Idempoten. Mengembalikan true hanya bila status berubah dari locked ke active; false
// (tanpa galat) bila akun tidak locked, tidak ada, atau bukan anggota tenant t (catatan tenant lain tidak disentuh).
func (s *Store) UnlockUser(ctx context.Context, t tenancy.TenantID, userID string) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	unlocked := false
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := exec(tx, `UPDATE users SET status = 'active', locked_at = NULL, updated_at = now() WHERE id = ? AND status = 'locked' AND `+memberExists("users.id"), uid, tid(t))
		if err != nil {
			return err
		}
		unlocked = n > 0
		_, err = exec(tx, `DELETE FROM login_attempts WHERE tenant_id = ? AND user_id = ?`, tid(t), uid)
		return err
	})
	if err != nil {
		return false, wrap("buka kunci user", err)
	}
	return unlocked, nil
}

// GetPreferences memuat preferensi pengguna; kosong bila belum diatur, ErrNotFound bila bukan anggota tenant t.
func (s *Store) GetPreferences(ctx context.Context, t tenancy.TenantID, userID string) (Preferences, error) {
	uid, ok := parseID(userID)
	if !ok {
		return Preferences{}, ErrNotFound
	}
	var r struct {
		Locale   string `gorm:"column:locale"`
		Timezone string `gorm:"column:timezone"`
	}
	res := s.db(ctx).Raw(`SELECT COALESCE(p.locale, '') AS locale, COALESCE(p.timezone, '') AS timezone
		FROM tenant_memberships m LEFT JOIN user_preferences p ON p.user_id = m.user_id
		WHERE m.tenant_id = ? AND m.user_id = ?`, tid(t), uid).Scan(&r)
	if res.Error != nil {
		return Preferences{}, wrap("muat preferensi", res.Error)
	}
	if res.RowsAffected == 0 {
		return Preferences{}, ErrNotFound
	}
	return Preferences{Locale: r.Locale, Timezone: r.Timezone}, nil
}

// UpsertPreferences menyimpan preferensi; ErrNotFound bila bukan anggota tenant t.
func (s *Store) UpsertPreferences(ctx context.Context, t tenancy.TenantID, userID, locale, timezone string) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	if len(locale) > 35 || len(timezone) > 64 {
		return ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `INSERT INTO user_preferences (user_id, locale, timezone, updated_at)
		SELECT m.user_id, ?::text, ?::text, ?::timestamptz FROM tenant_memberships m WHERE m.tenant_id = ? AND m.user_id = ?
		ON CONFLICT (user_id) DO UPDATE SET locale = EXCLUDED.locale, timezone = EXCLUDED.timezone, updated_at = EXCLUDED.updated_at`,
		locale, timezone, nowUTC(), tid(t), uid)
	if err != nil {
		return wrap("simpan preferensi", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// emailLockTimeout membatasi lama menunggu WithEmailLock; pemegang yang macet tidak boleh menahan run lain tanpa batas.
const emailLockTimeout = "30s"

// emailLockFnTimeout membatasi fn bila ctx pemanggil tanpa deadline. Isi fn yang wajar (mis. alur CreateAdmin) hanya
// beberapa pernyataan SQL di control dan tenant DB (hash sandi dihitung di luar kunci), jauh di bawah satu detik;
// 30 detik sengaja sama dengan emailLockTimeout supaya pemegang yang macet tidak menahan penunggu lebih lama
// daripada batas tunggu penunggu itu sendiri. Deadline pemanggil yang lebih ketat tetap dipakai.
const emailLockFnTimeout = 30 * time.Second

// WithEmailLock menjalankan fn selama memegang advisory lock transaksi PostgreSQL untuk email ternormalisasi, sehingga
// dua alur yang menyentuh email yang sama (mis. dua `core:create-admin` serentak) berjalan bergantian, bukan saling
// menyisip. users.email unik secara GLOBAL (lintas tenant), jadi kuncinya hanya berasal dari email: kunci per
// (tenant, email) tidak akan menyerialkan dua tenant yang berebut email yang sama.
//
// Lock dipegang oleh satu transaksi khusus di control DB selama fn berjalan; fn memakai koneksi lain (store ini)
// dan tidak berada di dalam transaksi itu, jadi pool control DB WAJIB berisi minimal 2 koneksi per pemanggil serentak
// (satu untuk lock, satu untuk fn); pool yang dibatasi lebih kecil dari jumlah pemanggil serentak dapat menemui
// kebuntuan sampai lock_timeout/deadline. Lock dilepas otomatis saat transaksi berakhir (commit, rollback, atau
// koneksi putus), jadi tidak ada lock yatim. Menunggu lebih dari emailLockTimeout gagal (lock_timeout).
//
// Transaksi hanya menahan lock; tidak ada yang perlu di-commit. Karena itu galat fn dikembalikan apa adanya, dan bila
// fn sukses tetapi commit gagal (Ctrl+C, idle_in_transaction_session_timeout, koneksi putus) hasilnya tetap nil:
// efek fn sudah terjadi dan melaporkan gagal menyesatkan; kejadian dicatat sebagai peringatan (tanpa data pribadi).
// Konsekuensinya serialisasi bersifat best-effort bila server memutus koneksi pemegang di tengah fn: lock lepas lebih
// awal dan run lain dapat masuk sebelum fn selesai. Bila ctx pemanggil tanpa deadline, fn dijalankan dengan
// batas emailLockFnTimeout. Email kosong: ErrInvalidArgument.
func (s *Store) WithEmailLock(ctx context.Context, email string, fn func(ctx context.Context) error) error {
	email = NormalizeEmail(email)
	if email == "" {
		return ErrInvalidArgument
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, emailLockFnTimeout)
		defer cancel()
	}
	var (
		ran   bool
		fnErr error
	)
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := exec(tx, "SET LOCAL lock_timeout = '"+emailLockTimeout+"'"); err != nil {
			return err
		}
		// hashtextextended memberi bigint 64-bit dari teks; awalan memisahkan ruang kunci dari pemakai advisory lock lain.
		if _, err := exec(tx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "erpora:identity:email:"+email); err != nil {
			return err
		}
		ran = true
		fnErr = fn(ctx)
		return nil // commit: hanya untuk melepas lock
	})
	switch {
	case !ran:
		return wrap("kunci email", err) // lock tidak pernah didapat: fn tidak berjalan
	case fnErr != nil:
		return fnErr
	case err != nil:
		slog.WarnContext(ctx, "identity: transaksi kunci email berakhir dengan galat setelah operasi sukses; serialisasi mungkin lepas lebih awal", "error", err)
	}
	return nil
}
