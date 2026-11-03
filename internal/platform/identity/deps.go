package identity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/settings"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
)

// Antarmuka di bawah ini ada di sisi pemakai (Service dan Admin) agar tes unit memakai fake dan paket ini tidak
// terikat ke tipe konkret. Semuanya dipenuhi komponen sungguhan yang dirakit internal/app (lihat Deps).

// LoginVerifier memverifikasi kredensial login menjadi hasil seragam (dipenuhi *guard.Authenticator).
type LoginVerifier interface {
	Verify(ctx context.Context, in guard.LoginInput) (guard.Outcome, error)
}

// PasswordGate menjalankan hash/verifikasi argon2id lewat pembatas serentak (dipenuhi *guard.HashGate).
type PasswordGate interface {
	Verify(ctx context.Context, h *password.Hasher, plain, phc string) (ok, needsRehash bool, err error)
	Hash(ctx context.Context, h *password.Hasher, plain string) (string, error)
}

// Lockout adalah bagian guard.Lockout yang dipakai Service dan Admin. Reserve/Fail/Release adalah pembatasan tebakan
// atomik per pasangan (akun, IP) yang menggantikan Check + RegisterFailure: percobaan dicatat sebagai kegagalan SEBELUM
// verifikasi sandi sehingga permintaan serentak tidak lolos bersama-sama (PRD-001 Q16/Q19).
type Lockout interface {
	Reserve(ctx context.Context, kind, userID, ip string) (guard.Reservation, guard.State, error)
	Fail(ctx context.Context, r guard.Reservation) (guard.State, error)
	Release(ctx context.Context, r guard.Reservation) error
	RegisterSuccess(ctx context.Context, userID, ip string) error
	Unlock(ctx context.Context, userID string) error
}

// Sessions adalah bagian session.Service yang dipakai (dipenuhi *session.Service).
type Sessions interface {
	Start(ctx context.Context, in session.StartInput) (session.Session, session.RefreshToken, error)
	Rotate(ctx context.Context, rawRefresh, origin string) (session.Session, session.RefreshToken, error)
	Upgrade(ctx context.Context, userID, sessionID, addAMR string) (session.Session, session.RefreshToken, error)
	Revoke(ctx context.Context, sessionID, reason string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	MarkRevoked(ctx context.Context, sessionIDs ...string)
}

// TokenIssuer menerbitkan access token (dipenuhi *token.Issuer).
type TokenIssuer interface {
	Issue(c token.Claims) (string, error)
}

// KeyRotator membuat kunci penandatangan baru berstatus pending (dipenuhi *token.Keyring).
type KeyRotator interface {
	Rotate(ctx context.Context) (token.Key, error)
}

// MFAEngine adalah bagian mfa.Service yang dipakai (dipenuhi *mfa.Service).
type MFAEngine interface {
	BeginEnrollment(ctx context.Context, userID string) (sdkidentity.EnrollmentInfo, error)
	ConfirmEnrollment(ctx context.Context, userID, code string) ([]string, error)
	Verify(ctx context.Context, userID, code string) error
	UseRecoveryCode(ctx context.Context, userID, code string) error
	RegenerateRecoveryCodes(ctx context.Context, userID string) ([]string, error)
	Reset(ctx context.Context, actor, userID string) error
}

// MFAStatuses membaca dan membuang cache keadaan MFA (dipenuhi *MFAStateReader).
type MFAStatuses interface {
	Status(ctx context.Context, userID string) (MFAStatus, error)
	Invalidate(t tenancy.TenantID, userID string)
}

// PasswordReset menjalankan alur lupa/reset sandi (dipenuhi *reset.Service).
type PasswordReset interface {
	Request(ctx context.Context, email, locale string) error
	Complete(ctx context.Context, rawToken, newPassword string) error
}

// PublicURLSetter menyimpan setting platform.public_url (dipenuhi *settings.Service).
type PublicURLSetter interface {
	SetPublicURL(ctx context.Context, rawURL, updatedBy string) error
}

// Store adalah himpunan metode *store.Store yang dipakai Service dan Admin (paket ini bebas GORM).
type Store interface {
	GetUser(ctx context.Context, t tenancy.TenantID, id string) (store.User, error)
	FindUserByEmail(ctx context.Context, t tenancy.TenantID, email string) (store.User, error)
	CreateUser(ctx context.Context, t tenancy.TenantID, in store.NewUser) (store.User, error)
	SetUserStatus(ctx context.Context, t tenancy.TenantID, userID, status string) error
	SetPasswordHash(ctx context.Context, t tenancy.TenantID, userID, hash string) error
	ChangePassword(ctx context.Context, t tenancy.TenantID, userID, newHash, exceptSessionID string, at time.Time) ([]string, error)
	TouchLastLogin(ctx context.Context, t tenancy.TenantID, userID string, at time.Time) error
	GrantRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string, at time.Time) (bool, error)
	HasRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string) (bool, error)
	RevokeRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string) (bool, error)
	UserRoleKeys(ctx context.Context, t tenancy.TenantID, userID string) ([]string, error)
	CountUserSessions(ctx context.Context, t tenancy.TenantID, userID string) (int, error)
}

// AuditEvent adalah peristiwa keamanan operasi Admin untuk Auditor. Tidak pernah memuat email, sandi, atau secret:
// hanya id pengguna dan pelaku. Audit sungguhan (tabel audit) adalah C3; sampai itu Auditor opsional dan setiap
// peristiwa tetap dicatat lewat slog terstruktur.
type AuditEvent struct {
	Action string    // mis. identity.admin_created, identity.signing_key_rotated, identity.account_unlocked
	Actor  string    // id pengguna administrator, atau alasan sistem "cli:core:..."
	Target string    // id pengguna atau kid kunci yang terdampak; kosong bila tidak ada
	At     time.Time // waktu peristiwa
}

// Auditor menerima peristiwa keamanan (opsional; dihubungkan ke modul audit pada C3).
type Auditor interface {
	Record(ctx context.Context, e AuditEvent)
}

// Deps adalah dependensi NewService dan NewAdmin. Field wajib berbeda per konstruktor (lihat masing-masing).
type Deps struct {
	Login    LoginVerifier    // Service
	Gate     PasswordGate     // Service, Admin
	Hasher   *password.Hasher // Service, Admin (parameter argon2 dari config)
	Lockout  Lockout          // Service, Admin
	Sessions Sessions         // Service, Admin
	Issuer   TokenIssuer      // Service
	Keys     KeyRotator       // Admin
	MFA      MFAEngine        // Service, Admin
	MFAState MFAStatuses      // Service, Admin
	Reset    PasswordReset    // Service
	Settings PublicURLSetter  // Admin
	Store    Store            // Service, Admin

	Auditor Auditor          // opsional (Admin)
	Now     func() time.Time // opsional; default time.Now
	Log     *slog.Logger     // opsional; default slog.Default
}

func (d Deps) clock() func() time.Time {
	if d.Now != nil {
		return d.Now
	}
	return time.Now
}

func (d Deps) logger() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// need mengembalikan galat untuk dependensi wajib pertama yang kosong; pasangan (nama field, ada).
type need struct {
	name string
	ok   bool
}

func require(kind string, deps ...need) error {
	for _, d := range deps {
		if !d.ok {
			return errors.New("identity: " + kind + ": Deps." + d.name + " wajib")
		}
	}
	return nil
}

// Komponen sungguhan memenuhi antarmuka di atas (gagal kompilasi bila salah satunya berubah bentuk).
var (
	_ LoginVerifier   = (*guard.Authenticator)(nil)
	_ PasswordGate    = (*guard.HashGate)(nil)
	_ Lockout         = (*guard.Lockout)(nil)
	_ Sessions        = (*session.Service)(nil)
	_ TokenIssuer     = (*token.Issuer)(nil)
	_ KeyRotator      = (*token.Keyring)(nil)
	_ MFAEngine       = (*mfa.Service)(nil)
	_ MFAStatuses     = (*MFAStateReader)(nil)
	_ PasswordReset   = (*reset.Service)(nil)
	_ PublicURLSetter = (*settings.Service)(nil)
	_ Store           = (*store.Store)(nil)
)
