package store

import (
	"time"

	"github.com/google/uuid"
)

// Status pengguna (users.status).
const (
	UserActive   = "active"
	UserDisabled = "disabled"
	UserLocked   = "locked"
)

// Status keanggotaan tenant (tenant_memberships.status).
const (
	MembershipActive    = "active"
	MembershipSuspended = "suspended"
)

// Status kunci penandatangan (signing_keys.status).
const (
	KeyPending  = "pending"
	KeyActive   = "active"
	KeyRetiring = "retiring"
	KeyRetired  = "retired"
)

// Jenis percobaan autentikasi (login_attempts.kind).
const (
	AttemptPassword = "password"
	AttemptMFA      = "mfa"
)

// ClientKind sesi (auth_sessions.client_kind).
const (
	ClientWeb     = "web"
	ClientMobile  = "mobile"
	ClientDesktop = "desktop"
)

// User adalah akun global. PasswordHash kosong untuk pengguna SSO-only. Semua waktu UTC.
type User struct {
	ID     string
	Email  string // ternormalisasi: trim + huruf kecil
	Name   string
	Status string
	// MembershipStatus adalah status keanggotaan pengguna pada tenant yang dikueri (tenant_memberships.status;
	// MembershipActive/MembershipSuspended). Hanya diisi oleh GetUser/FindUserByEmail (kueri berscope tenant).
	MembershipStatus string
	PasswordHash     string
	EmailVerifiedAt  *time.Time
	LastLoginAt      *time.Time
	LockedAt         *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NewUser adalah masukan CreateUser. Status kosong berarti active; hanya active atau disabled yang diterima.
type NewUser struct {
	Email        string
	Name         string
	PasswordHash string // PHC argon2id; kosong untuk SSO-only
	Status       string
}

// Membership adalah keanggotaan satu pengguna pada satu tenant.
type Membership struct {
	UserID    string
	Status    string
	CreatedAt time.Time
}

// Preferences adalah preferensi pengguna; nilai kosong berarti belum diatur.
type Preferences struct {
	Locale   string
	Timezone string
}

// SigningKey adalah kunci penandatangan JWT (global platform, tanpa tenant). PrivateKeySealed adalah hasil
// crypto.Sealer; store tidak pernah membukanya. Semua waktu UTC.
type SigningKey struct {
	ID               string // kid
	Status           string
	PublicKey        []byte
	PrivateKeySealed []byte
	CreatedAt        time.Time
	ActivatedAt      *time.Time
	RetiringAt       *time.Time
	RetiredAt        *time.Time
}

// Session adalah sesi per perangkat (id = klaim sid). Semua waktu UTC.
type Session struct {
	ID                string
	UserID            string
	ClientKind        string
	AuthMethod        string
	AMR               []string
	UserAgent         string
	IP                string
	CreatedAt         time.Time
	LastSeenAt        time.Time
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
	RevokeReason      string
}

// RevokedSession adalah sesi tercabut (untuk memuat daftar tercabut di memori saat startup).
type RevokedSession struct {
	ID        string
	RevokedAt time.Time
}

// RefreshToken adalah satu token refresh (hanya hash yang disimpan). ParentID kosong untuk akar family.
type RefreshToken struct {
	ID        string
	SessionID string
	FamilyID  string
	ParentID  string
	TokenHash []byte // SHA-256, 32 byte
	CreatedAt time.Time
	UsedAt    *time.Time
	ExpiresAt time.Time
}

// MFAFactor adalah faktor TOTP pengguna. SecretSealed adalah hasil crypto.Sealer.
type MFAFactor struct {
	ID           string
	UserID       string
	Type         string
	SecretSealed []byte
	ConfirmedAt  *time.Time
	LastUsedStep int64
	CreatedAt    time.Time
}

// PasswordReset adalah tautan reset sandi sekali pakai (hanya hash token yang disimpan).
type PasswordReset struct {
	ID        string
	UserID    string
	TokenHash []byte // 32 byte
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// ResetResult adalah hasil ResetPassword yang berhasil.
type ResetResult struct {
	UserID            string
	RevokedSessionIDs []string
}

// PairState adalah keadaan kegagalan satu pasangan (akun, IP): jumlah gagal sejak sukses terakhir pasangan itu.
type PairState struct {
	ConsecutiveFailures int
	LastFailureAt       *time.Time
}

// CrossIPState adalah kegagalan sebuah akun dalam satu jendela waktu, lintas IP.
type CrossIPState struct {
	Failures    int
	DistinctIPs int
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func idString(id uuid.UUID) string { return id.String() }
