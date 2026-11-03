package mfa

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/identity"
)

// AdministratorRole adalah key role sistem yang wajib ber-MFA (PRD-001 FR-107, Q9).
const AdministratorRole = "administrator"

const (
	secretSize = 20 // 160 bit, panjang kunci HMAC-SHA-1 yang direkomendasikan RFC 4226 §4
)

// Galat paket. Tidak ada yang memuat kode, secret, atau isi recovery code. ErrInvalidCode sengaja seragam untuk
// semua sebab kegagalan verifikasi (kode salah, kedaluwarsa, replay, format cacat, faktor tidak ada, recovery code
// salah atau sudah terpakai) sehingga klien tidak dapat membedakannya.
var (
	ErrInvalidCode     = errors.New("mfa: kode tidak valid")
	ErrLocked          = errors.New("mfa: terlalu banyak percobaan")
	ErrForbidden       = errors.New("mfa: operasi tidak diizinkan untuk pemanggil ini")
	ErrNoTenant        = errors.New("mfa: tenant tidak ada di context")
	ErrInvalidArgument = errors.New("mfa: argumen tidak valid")
	ErrAlreadyEnrolled = errors.New("mfa: faktor sudah aktif")
	ErrNotEnrolled     = errors.New("mfa: tidak ada faktor yang menunggu konfirmasi atau aktif")
)

// LockedError menyertai ErrLocked (errors.Is(err, ErrLocked)). RetryAfter > 0 berarti pemanggil harus menjawab
// 429 rate_limited dengan Retry-After; AccountLocked true bila akun terkunci (keputusan pemetaan ada di handler).
type LockedError struct {
	RetryAfter    time.Duration
	AccountLocked bool
}

func (e *LockedError) Error() string        { return ErrLocked.Error() }
func (e *LockedError) Is(target error) bool { return target == ErrLocked }

// LockState adalah keadaan lockout (FR-101, Q16) setelah satu kegagalan atau pada pra-pemeriksaan.
type LockState struct {
	PairBlocked   bool
	AccountLocked bool
	RetryAfter    time.Duration
}

func (s LockState) restricted() bool { return s.PairBlocked || s.AccountLocked || s.RetryAfter > 0 }

// Lockout dipenuhi guard.Lockout (C1.7) lewat adaptor yang dirakit di C1.10/C1.11. Percobaan verifikasi MFA
// memakai pasangan (userID, ip) yang sama dengan login sandi.
type Lockout interface {
	RegisterFailure(ctx context.Context, userID, ip string) (LockState, error)
	RegisterSuccess(ctx context.Context, userID, ip string) error
}

// LockChecker adalah perluasan opsional Lockout: pemeriksaan baca-saja keadaan pasangan (userID, ip) SEBELUM kode
// diverifikasi. Bila implementasi Lockout juga memenuhinya, Service menolak percobaan pada pasangan yang sedang
// dijeda/diblokir tanpa memeriksa kode (kode benar pun ditolak) dan tanpa menambah hitungan kegagalan. Tanpa
// ini, blokir hanya berlaku setelah gagal berikutnya sehingga tebakan tidak benar-benar dibatasi: guard (C1.7)
// sebaiknya menyediakannya.
type LockChecker interface {
	Check(ctx context.Context, userID, ip string) (LockState, error)
}

// Sealer menyegel dan membuka secret (dipenuhi *crypto.Sealer).
type Sealer interface {
	Seal(plaintext, aad []byte) ([]byte, error)
	Open(sealed, aad []byte) ([]byte, error)
}

// Store adalah himpunan metode *store.Store yang dibutuhkan paket ini (antarmuka sempit agar paket ini bebas GORM
// dan mudah diuji).
type Store interface {
	GetUser(ctx context.Context, t tenancy.TenantID, id string) (store.User, error)
	GetMFAFactor(ctx context.Context, t tenancy.TenantID, userID string) (store.MFAFactor, error)
	UpsertPendingMFAFactor(ctx context.Context, t tenancy.TenantID, userID string, secretSealed []byte, at time.Time) (bool, error)
	ConfirmMFAFactor(ctx context.Context, t tenancy.TenantID, userID string, step int64, at time.Time) (bool, error)
	AdvanceMFAStep(ctx context.Context, t tenancy.TenantID, userID string, step int64) (bool, error)
	ResetMFA(ctx context.Context, t tenancy.TenantID, userID string) error
	ReplaceRecoveryCodes(ctx context.Context, t tenancy.TenantID, userID string, hashes [][]byte, at time.Time) error
	UseRecoveryCode(ctx context.Context, t tenancy.TenantID, userID string, hash []byte, at time.Time) (bool, error)
	HasRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string) (bool, error)
}

// AuditEvent adalah peristiwa keamanan MFA untuk Auditor. Tidak pernah memuat kode atau secret.
type AuditEvent struct {
	Action string // mfa.enrolled, mfa.reset, mfa.recovery_regenerated, mfa.recovery_used
	Actor  string // pelaku (id pengguna atau alasan sistem "cli:core:...")
	UserID string // pengguna yang terdampak
	At     time.Time
}

// Auditor menerima peristiwa keamanan (opsional; dihubungkan ke modul audit pada task berikutnya).
type Auditor interface {
	Record(ctx context.Context, e AuditEvent)
}

// EnrollmentInfo adalah bahan pendaftaran yang ditampilkan sekali (alias tipe sdk agar redaksi log/format ikut).
type EnrollmentInfo = identity.EnrollmentInfo

// Deps adalah dependensi Service.
type Deps struct {
	Store   Store
	Sealer  Sealer
	Lockout Lockout
	Auditor Auditor          // opsional
	Issuer  string           // nama penerbit di otpauth URI, tanpa ':'
	Now     func() time.Time // opsional; default time.Now
	Logger  *slog.Logger     // opsional; default slog.Default
}

// Service adalah mesin MFA TOTP dan recovery code.
//
// IDOR: setiap metode menerima userID, tetapi pemanggil sah (Service identity C1.11) WAJIB menurunkannya dari
// Principal di context (identity.RequirePrincipal), bukan dari body/parameter klien. Sebagai pertahanan
// berlapis, bila context memuat Principal yang UserID-nya berbeda dengan userID, operasi ditolak ErrForbidden
// sebelum menyentuh store atau lockout. Tanpa Principal (alur login yang belum menerbitkan principal, CLI) userID
// dipercaya dari pemanggil. Tenant selalu dari tenancy.FromContext; pengguna di luar tenant diperlakukan seperti
// tidak ada (galat seragam atau store.ErrNotFound).
type Service struct {
	d       Deps
	totp    TOTP
	checker LockChecker
}

// New membuat Service; menolak dependensi wajib yang kosong dan Issuer yang kosong atau memuat ':'.
func New(d Deps) (*Service, error) {
	switch {
	case d.Store == nil:
		return nil, errors.New("mfa: Store wajib")
	case d.Sealer == nil:
		return nil, errors.New("mfa: Sealer wajib")
	case d.Lockout == nil:
		return nil, errors.New("mfa: Lockout wajib")
	case strings.TrimSpace(d.Issuer) == "" || strings.Contains(d.Issuer, ":"):
		return nil, errors.New("mfa: Issuer wajib dan tidak boleh memuat ':'")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	s := &Service{d: d}
	s.checker, _ = d.Lockout.(LockChecker)
	return s, nil
}

type clientIPKey struct{}

// WithClientIP memasang IP klien (hasil resolusi proxy tepercaya oleh middleware) pada context; dipakai sebagai
// bagian pasangan (akun, IP) lockout. Tanpa IP, pasangan memakai "unknown" (gagal-tertutup ke satu ember bersama).
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

func clientIP(ctx context.Context) string {
	if ip, _ := ctx.Value(clientIPKey{}).(string); ip != "" {
		return ip
	}
	return "unknown"
}

// secretAAD mengikat secret tersegel ke tujuan dan pemiliknya (sesuai anjuran store/doc.go).
func secretAAD(userID string) []byte { return []byte("user_mfa_factors|secret|" + userID) }

// prepare memeriksa userID, pemanggil (pertahanan IDOR berlapis), dan tenant.
func (s *Service) prepare(ctx context.Context, userID string) (tenancy.TenantID, error) {
	if userID == "" {
		return tenancy.TenantID{}, ErrInvalidArgument
	}
	if p, ok := identity.FromContext(ctx); ok && p.UserID != userID {
		return tenancy.TenantID{}, ErrForbidden
	}
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return tenancy.TenantID{}, ErrNoTenant
	}
	return t, nil
}

// BeginEnrollment membuat secret TOTP acak 20 byte, menyimpannya tersegel (AAD memuat tujuan dan userID) sebagai
// faktor pending (belum aktif sampai ConfirmEnrollment), dan mengembalikan secret base32 dan otpauth URI sekali
// saja. Pemanggilan ulang saat masih pending mengganti secret; ErrAlreadyEnrolled bila faktor sudah aktif (reset
// dulu lewat Reset).
func (s *Service) BeginEnrollment(ctx context.Context, userID string) (EnrollmentInfo, error) {
	t, err := s.prepare(ctx, userID)
	if err != nil {
		return EnrollmentInfo{}, err
	}
	user, err := s.d.Store.GetUser(ctx, t, userID)
	if err != nil {
		return EnrollmentInfo{}, fmt.Errorf("mfa: muat pengguna: %w", err)
	}
	secret := make([]byte, secretSize)
	if _, err := rand.Read(secret); err != nil {
		return EnrollmentInfo{}, fmt.Errorf("mfa: bangkitkan secret: %w", err)
	}
	sealed, err := s.d.Sealer.Seal(secret, secretAAD(userID))
	if err != nil {
		return EnrollmentInfo{}, fmt.Errorf("mfa: segel secret: %w", err)
	}
	ok, err := s.d.Store.UpsertPendingMFAFactor(ctx, t, userID, sealed, s.d.Now())
	if err != nil {
		return EnrollmentInfo{}, fmt.Errorf("mfa: simpan faktor: %w", err)
	}
	if !ok {
		return EnrollmentInfo{}, ErrAlreadyEnrolled
	}
	b32 := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	clear(secret)
	return EnrollmentInfo{Secret: b32, OTPAuthURI: otpauthURI(s.d.Issuer, user.Email, b32)}, nil
}

// otpauthURI membangun URI Key URI Format (Google Authenticator): label "Issuer:akun" dan parameter secret,
// issuer, algorithm=SHA1, digits=6, period=30.
func otpauthURI(issuer, account, secretB32 string) string {
	q := url.Values{}
	q.Set("secret", secretB32)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(Period))
	return "otpauth://totp/" + url.PathEscape(issuer) + ":" + url.PathEscape(account) +
		"?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

// ConfirmEnrollment mengaktifkan faktor pending bila code (kode TOTP pertama) benar, lalu menerbitkan
// RecoveryCodeCount recovery code yang ditampilkan sekali (hanya hash yang disimpan). Langkah kode konfirmasi
// menjadi last_used_step sehingga kode itu tidak dapat dipakai ulang. Kode salah: ErrInvalidCode, faktor tetap
// tidak aktif dan recovery code tidak berubah (dicatat sebagai kegagalan lockout). ErrNotEnrolled bila tidak ada
// pendaftaran pending; ErrAlreadyEnrolled bila sudah aktif. Bila penyimpanan recovery code gagal setelah faktor
// aktif, galat dikembalikan dan pengguna menerbitkan kode lewat RegenerateRecoveryCodes. Keberhasilan konfirmasi
// tidak dicatat sebagai RegisterSuccess lockout (autentikasi login belum selesai oleh langkah ini).
func (s *Service) ConfirmEnrollment(ctx context.Context, userID, code string) ([]string, error) {
	t, err := s.prepare(ctx, userID)
	if err != nil {
		return nil, err
	}
	ip := clientIP(ctx)
	if err := s.precheck(ctx, userID, ip); err != nil {
		return nil, err
	}
	f, err := s.d.Store.GetMFAFactor(ctx, t, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotEnrolled
	}
	if err != nil {
		return nil, fmt.Errorf("mfa: muat faktor: %w", err)
	}
	if f.ConfirmedAt != nil {
		return nil, ErrAlreadyEnrolled
	}
	secret, err := s.openSecret(f, userID)
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	step, ok := s.totp.Verify(secret, code, s.d.Now(), f.LastUsedStep)
	if ok {
		ok, err = s.d.Store.ConfirmMFAFactor(ctx, t, userID, step, s.d.Now())
		if err != nil {
			return nil, fmt.Errorf("mfa: konfirmasi faktor: %w", err)
		}
	}
	if !ok {
		return nil, s.failure(ctx, userID, ip)
	}
	codes, err := s.issueRecoveryCodes(ctx, t, userID)
	if err != nil {
		return nil, err
	}
	s.record(ctx, "mfa.enrolled", userID, userID)
	return codes, nil
}

// Verify memeriksa kode TOTP untuk userID (MFA login). Sukses hanya bila kode cocok pada langkah > last_used_step
// DAN pembaruan atomik bersyarat store.AdvanceMFAStep menang: kode yang sama, kode langkah lama, dan verifikasi
// serentak ditolak (tepat satu sukses). Semua kegagalan mengembalikan ErrInvalidCode yang seragam dan dicatat ke
// Lockout; pasangan yang dijeda/diblokir mengembalikan *LockedError (ErrLocked).
func (s *Service) Verify(ctx context.Context, userID, code string) error {
	t, err := s.prepare(ctx, userID)
	if err != nil {
		return err
	}
	ip := clientIP(ctx)
	if err := s.precheck(ctx, userID, ip); err != nil {
		return err
	}
	ok, err := s.checkTOTP(ctx, t, userID, code)
	if err != nil {
		return err
	}
	return s.finish(ctx, userID, ip, ok)
}

func (s *Service) checkTOTP(ctx context.Context, t tenancy.TenantID, userID, code string) (bool, error) {
	f, err := s.d.Store.GetMFAFactor(ctx, t, userID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mfa: muat faktor: %w", err)
	}
	if f.ConfirmedAt == nil {
		return false, nil
	}
	secret, err := s.openSecret(f, userID)
	if err != nil {
		return false, err
	}
	defer clear(secret)
	step, ok := s.totp.Verify(secret, code, s.d.Now(), f.LastUsedStep)
	if !ok {
		return false, nil
	}
	won, err := s.d.Store.AdvanceMFAStep(ctx, t, userID, step)
	if err != nil {
		return false, fmt.Errorf("mfa: majukan step: %w", err)
	}
	return won, nil
}

func (s *Service) openSecret(f store.MFAFactor, userID string) ([]byte, error) {
	secret, err := s.d.Sealer.Open(f.SecretSealed, secretAAD(userID))
	if err != nil {
		return nil, fmt.Errorf("mfa: buka secret: %w", err)
	}
	return secret, nil
}

// UseRecoveryCode memakai satu recovery code (sekali pakai, atomik di store). Masukan dinormalisasi: huruf
// besar/kecil, tanda hubung, dan spasi diabaikan; format yang cacat dihitung sebagai kegagalan biasa. Galat dan
// lockout sama dengan Verify.
func (s *Service) UseRecoveryCode(ctx context.Context, userID, code string) error {
	t, err := s.prepare(ctx, userID)
	if err != nil {
		return err
	}
	ip := clientIP(ctx)
	if err := s.precheck(ctx, userID, ip); err != nil {
		return err
	}
	var ok bool
	if canon, valid := normalizeRecoveryCode(code); valid {
		ok, err = s.d.Store.UseRecoveryCode(ctx, t, userID, hashRecoveryCode(userID, canon), s.d.Now())
		if err != nil {
			return fmt.Errorf("mfa: pakai recovery code: %w", err)
		}
	}
	if err := s.finish(ctx, userID, ip, ok); err != nil {
		return err
	}
	s.record(ctx, "mfa.recovery_used", userID, userID)
	return nil
}

// RegenerateRecoveryCodes menerbitkan set recovery code baru dan mencabut seluruh kode lama (termasuk yang belum
// terpakai). Hanya untuk faktor yang aktif (ErrNotEnrolled bila tidak). Pemanggil WAJIB memastikan sesi sudah
// terverifikasi MFA (amr memuat otp) sebelum memanggil.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	t, err := s.prepare(ctx, userID)
	if err != nil {
		return nil, err
	}
	f, err := s.d.Store.GetMFAFactor(ctx, t, userID)
	if errors.Is(err, store.ErrNotFound) || err == nil && f.ConfirmedAt == nil {
		return nil, ErrNotEnrolled
	}
	if err != nil {
		return nil, fmt.Errorf("mfa: muat faktor: %w", err)
	}
	codes, err := s.issueRecoveryCodes(ctx, t, userID)
	if err != nil {
		return nil, err
	}
	s.record(ctx, "mfa.recovery_regenerated", userID, userID)
	return codes, nil
}

func (s *Service) issueRecoveryCodes(ctx context.Context, t tenancy.TenantID, userID string) ([]string, error) {
	codes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	hashes := make([][]byte, 0, len(codes))
	for _, c := range codes {
		canon, _ := normalizeRecoveryCode(c)
		hashes = append(hashes, hashRecoveryCode(userID, canon))
	}
	if err := s.d.Store.ReplaceRecoveryCodes(ctx, t, userID, hashes, s.d.Now()); err != nil {
		return nil, fmt.Errorf("mfa: simpan recovery code: %w", err)
	}
	return codes, nil
}

// Reset menghapus faktor TOTP dan seluruh recovery code userID (hanya pemanggil berwenang: admin atau
// core:reset-mfa) dan mencatat actor ke Auditor dan log. actor wajib (id pengguna admin atau alasan sistem
// "cli:core:reset-mfa"). Pertahanan berlapis: bila context memuat Principal, actor harus sama dengan Principal.UserID,
// Principal wajib memegang role administrator, dan tidak boleh mereset dirinya sendiri (ErrForbidden); tanpa
// Principal (CLI/hak sistem) otorisasi sepenuhnya ada pada pemanggil. Otorisasi permission pemanggil (mis.
// core:user:reset-mfa) tetap tanggung jawab lapisan atasnya.
func (s *Service) Reset(ctx context.Context, actor, userID string) error {
	if strings.TrimSpace(actor) == "" || userID == "" {
		return ErrInvalidArgument
	}
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return ErrNoTenant
	}
	if p, has := identity.FromContext(ctx); has {
		if p.UserID == "" || p.UserID != actor || actor == userID {
			return ErrForbidden
		}
		isAdmin, err := s.d.Store.HasRole(ctx, t, p.UserID, AdministratorRole)
		if err != nil {
			return fmt.Errorf("mfa: periksa role: %w", err)
		}
		if !isAdmin {
			return ErrForbidden
		}
	}
	if err := s.d.Store.ResetMFA(ctx, t, userID); err != nil {
		return fmt.Errorf("mfa: reset: %w", err)
	}
	s.record(ctx, "mfa.reset", actor, userID)
	return nil
}

// Required melaporkan apakah userID wajib melewati MFA: true bila memegang role administrator (wajib mendaftar,
// FR-107) atau sudah punya faktor aktif (tidak boleh lolos hanya dengan sandi, ADR-0013 section 4). Hanya baca.
func (s *Service) Required(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, ErrInvalidArgument
	}
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return false, ErrNoTenant
	}
	f, err := s.d.Store.GetMFAFactor(ctx, t, userID)
	switch {
	case err == nil && f.ConfirmedAt != nil:
		return true, nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return false, fmt.Errorf("mfa: muat faktor: %w", err)
	}
	isAdmin, err := s.d.Store.HasRole(ctx, t, userID, AdministratorRole)
	if err != nil {
		return false, fmt.Errorf("mfa: periksa role: %w", err)
	}
	return isAdmin, nil
}

// precheck menolak percobaan pada pasangan yang sedang dijeda/diblokir, bila Lockout menyediakan LockChecker.
func (s *Service) precheck(ctx context.Context, userID, ip string) error {
	if s.checker == nil {
		return nil
	}
	st, err := s.checker.Check(ctx, userID, ip)
	if err != nil {
		return fmt.Errorf("mfa: periksa lockout: %w", err)
	}
	if st.restricted() {
		return &LockedError{RetryAfter: st.RetryAfter, AccountLocked: st.AccountLocked}
	}
	return nil
}

// finish mencatat hasil percobaan ke Lockout. Kegagalan -> failure(); sukses -> RegisterSuccess (galatnya hanya
// dilog: autentikasi sudah sah dan step sudah terpakai).
func (s *Service) finish(ctx context.Context, userID, ip string, ok bool) error {
	if !ok {
		return s.failure(ctx, userID, ip)
	}
	if err := s.d.Lockout.RegisterSuccess(ctx, userID, ip); err != nil {
		s.d.Logger.ErrorContext(ctx, "mfa: gagal mencatat keberhasilan ke lockout", "userId", userID, "error", err)
	}
	return nil
}

// failure mencatat satu kegagalan dan mengembalikan galat seragam (atau LockedError bila kini dijeda/diblokir).
// Galat Lockout gagal-tertutup (dikembalikan sebagai galat, bukan sukses).
func (s *Service) failure(ctx context.Context, userID, ip string) error {
	st, err := s.d.Lockout.RegisterFailure(ctx, userID, ip)
	if err != nil {
		return fmt.Errorf("mfa: catat kegagalan: %w", err)
	}
	if st.restricted() {
		return &LockedError{RetryAfter: st.RetryAfter, AccountLocked: st.AccountLocked}
	}
	return ErrInvalidCode
}

func (s *Service) record(ctx context.Context, action, actor, userID string) {
	s.d.Logger.InfoContext(ctx, "mfa: peristiwa keamanan", "action", action, "actor", actor, "userId", userID)
	if s.d.Auditor != nil {
		s.d.Auditor.Record(ctx, AuditEvent{Action: action, Actor: actor, UserID: userID, At: s.d.Now()})
	}
}
