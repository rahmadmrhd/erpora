package identity

import (
	"context"
	"time"
)

// Pola metadata request: setiap input yang berasal dari jaringan membawa ClientIP dan UserAgent
// sebagai field bertag json:"-". Handler mengisinya dari request (alamat peer tepercaya, header
// User-Agent) sehingga klien tidak dapat memalsukannya lewat body; Service memakainya untuk rate
// limit, lockout, dan audit. Locale (ForgotInput) mengikuti pola yang sama.

// LoginInput adalah masukan Service.Login.
type LoginInput struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	ClientIP  string `json:"-"`
	UserAgent string `json:"-"`
}

// MFAState adalah keadaan MFA pada hasil login: apakah akses penuh sudah diberikan atau masih ada
// langkah MFA. Membedakan "belum mendaftar" dari "perlu verifikasi" agar klien memilih layar yang benar.
type MFAState string

const (
	// MFANone: tidak ada langkah MFA tersisa; token berlaku penuh.
	MFANone MFAState = "none"
	// MFAEnrollRequired: pengguna wajib MFA (mis. Administrator) tetapi belum mendaftarkan TOTP.
	MFAEnrollRequired MFAState = "enroll_required"
	// MFAVerifyRequired: pengguna sudah mendaftar TOTP dan harus memverifikasi kode.
	MFAVerifyRequired MFAState = "verify_required"
)

// LoginResult adalah hasil login/refresh/verifikasi MFA.
//
// RefreshToken dan RefreshExpiresAt bertag json:"-" dengan sengaja: refresh token hanya boleh sampai
// ke klien lewat cookie HttpOnly yang dipasang handler (RefreshExpiresAt menentukan Max-Age cookie),
// tidak pernah lewat body JSON yang terbaca skrip dan bisa masuk log. Dengan tag ini json.Marshal tidak
// dapat membocorkannya meski struct ini tak sengaja diserialisasi utuh.
//
// Keputusan M-10: bool `mfaRequired` diganti `mfaState` (tiga keadaan); kontrak core belum terbit
// sehingga tidak ada breaking change. MFARequired() tersedia sebagai kemudahan di Go, tidak diserialisasi.
type LoginResult struct {
	AccessToken      string    `json:"accessToken"`
	ExpiresIn        int       `json:"expiresIn"` // detik
	MFAState         MFAState  `json:"mfaState"`
	RefreshToken     string    `json:"-"`
	RefreshExpiresAt time.Time `json:"-"`
}

// MFARequired melaporkan apakah masih ada langkah MFA (enroll atau verify); nilai nol berarti tidak.
func (r LoginResult) MFARequired() bool {
	return r.MFAState == MFAEnrollRequired || r.MFAState == MFAVerifyRequired
}

// RefreshInput adalah masukan Service.Refresh. RefreshToken dibaca handler dari cookie, bukan body.
type RefreshInput struct {
	RefreshToken string `json:"-"`
	ClientIP     string `json:"-"`
	UserAgent    string `json:"-"`
}

// ForgotInput adalah masukan Service.ForgotPassword. Locale (bahasa email) dari Accept-Language.
type ForgotInput struct {
	Email     string `json:"email"`
	Locale    string `json:"-"`
	ClientIP  string `json:"-"`
	UserAgent string `json:"-"`
}

// ResetInput adalah masukan Service.ResetPassword (tautan sekali pakai dari email).
type ResetInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
	ClientIP    string `json:"-"`
	UserAgent   string `json:"-"`
}

// ChangeInput adalah masukan Service.ChangePassword; pengguna diambil dari Principal di context.
type ChangeInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ClientIP        string `json:"-"`
	UserAgent       string `json:"-"`
}

// EnrollmentInfo adalah bahan pendaftaran TOTP yang ditampilkan sekali ke pengguna.
type EnrollmentInfo struct {
	Secret     string `json:"secret"`     // base32
	OTPAuthURI string `json:"otpauthUri"` // untuk QR code; memuat secret
}

// RecoveryCodes adalah recovery code sekali pakai; plaintext hanya ditampilkan sekali saat dibuat.
type RecoveryCodes struct {
	Codes []string `json:"codes"`
}

// ConfirmMFAInput adalah masukan Service.ConfirmMFA (kode TOTP pertama untuk mengaktifkan MFA).
type ConfirmMFAInput struct {
	Code      string `json:"code"`
	ClientIP  string `json:"-"`
	UserAgent string `json:"-"`
}

// VerifyInput adalah masukan Service.VerifyMFA pada sesi yang menunggu verifikasi MFA (Principal di
// context). Recovery true berarti Code adalah recovery code, bukan kode TOTP.
type VerifyInput struct {
	Code      string `json:"code"`
	Recovery  bool   `json:"recovery,omitempty"`
	ClientIP  string `json:"-"`
	UserAgent string `json:"-"`
}

// Service adalah mesin identitas yang dipakai handler modul core. Diimplementasi platform
// (internal/platform/identity) dan dipasang di context lewat WithService (dependency inversion D5/D9).
// Kegagalan autentikasi tidak membedakan sebab ke pemanggil (anti-enumerasi); implementasi
// mengembalikan error domain yang dipetakan handler ke kode galat api-guidelines.
//
// Metode tanpa parameter pengguna/sesi (Logout, ChangePassword, MFA) menurunkan pengguna dan sesi dari
// Principal di ctx lewat RequirePrincipal; tanpa Principal mengembalikan ErrUnauthenticated. Pemanggil
// tidak pernah menyebut sesi atau pengguna target, sehingga tidak ada jalur IDOR.
type Service interface {
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
	Refresh(ctx context.Context, in RefreshInput) (LoginResult, error)
	Logout(ctx context.Context) error
	ForgotPassword(ctx context.Context, in ForgotInput) error
	ResetPassword(ctx context.Context, in ResetInput) error
	ChangePassword(ctx context.Context, in ChangeInput) error
	BeginMFAEnrollment(ctx context.Context) (EnrollmentInfo, error)
	ConfirmMFA(ctx context.Context, in ConfirmMFAInput) (RecoveryCodes, error)
	VerifyMFA(ctx context.Context, in VerifyInput) (LoginResult, error)
	RegenerateRecoveryCodes(ctx context.Context) (RecoveryCodes, error)
}

type serviceKey struct{}

// serviceBox: lihat principalBox; mencegah pemalsuan dan pembacaan Service lewat kunci tiruan refleksi.
type serviceBox struct {
	s  Service
	ok bool
}

// WithService memasang s di ctx; dipanggil platform saat merakit modul.
func WithService(ctx context.Context, s Service) context.Context {
	return context.WithValue(ctx, serviceKey{}, &serviceBox{s: s, ok: true})
}

// ServiceFrom mengembalikan Service di ctx; ok false bila tidak ada, nil, nil bertipe, atau nilai di
// bawah kunci itu bukan kotak sah buatan WithService.
func ServiceFrom(ctx context.Context) (Service, bool) {
	b, ok := ctx.Value(serviceKey{}).(*serviceBox)
	if !ok || b == nil || !b.ok || isNil(b.s) {
		return nil, false
	}
	return b.s, true
}
