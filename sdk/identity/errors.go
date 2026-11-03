package identity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Galat domain Service dan Admin. Modul hanya boleh mengimpor sdk (D9), jadi handler modul core memetakan galat
// ini ke kode galat api-guidelines (errors.Is / errors.As); implementasi platform menerjemahkan galat internalnya ke
// nilai-nilai ini di batas Service/Admin dan tidak pernah membungkus teks internal yang dapat sampai ke klien.
//
// Anti-enumerasi: ErrInvalidCredentials sengaja seragam untuk email tak dikenal, sandi salah, akun terkunci, dan
// akun dinonaktifkan; sebab sebenarnya hanya ada di log keamanan server.
var (
	// ErrInvalidCredentials: kredensial login salah (401 `invalid_credentials`, seragam).
	ErrInvalidCredentials = errors.New("identity: kredensial tidak valid")
	// ErrWrongCurrentPassword: sandi saat ini pada ChangePassword salah.
	ErrWrongCurrentPassword = errors.New("identity: sandi saat ini salah")
	// ErrInvalidRefresh: refresh token tidak valid (tidak dikenal, kedaluwarsa, terpakai ulang, sesi tercabut); 401 generik.
	ErrInvalidRefresh = errors.New("identity: refresh token tidak valid")
	// ErrOriginNotAllowed: header Origin pada refresh hilang atau tidak diizinkan (403).
	ErrOriginNotAllowed = errors.New("identity: origin tidak diizinkan")
	// ErrInvalidResetToken: tautan reset tidak valid, kedaluwarsa, atau sudah terpakai (seragam).
	ErrInvalidResetToken = errors.New("identity: tautan reset tidak valid atau sudah tidak berlaku")
	// ErrInvalidMFACode: kode TOTP atau recovery code tidak valid (seragam untuk semua sebab).
	ErrInvalidMFACode = errors.New("identity: kode MFA tidak valid")
	// ErrMFARequired: operasi menuntut sesi yang sudah terverifikasi MFA (amr memuat otp).
	ErrMFARequired = errors.New("identity: operasi menuntut verifikasi MFA")
	// ErrMFAAlreadyEnrolled: faktor TOTP sudah aktif; reset dulu bila perlu mendaftar ulang.
	ErrMFAAlreadyEnrolled = errors.New("identity: faktor MFA sudah aktif")
	// ErrMFANotEnrolled: tidak ada faktor MFA yang menunggu konfirmasi atau aktif.
	ErrMFANotEnrolled = errors.New("identity: faktor MFA belum didaftarkan")
	// ErrAccountLocked: akun terkunci pada langkah MFA (pemanggil sudah lolos sandi, jadi tidak membuka enumerasi).
	ErrAccountLocked = errors.New("identity: akun terkunci")
	// ErrForbidden: pemanggil tidak berhak atas operasi (Admin tanpa hak administrator atau hak sistem).
	ErrForbidden = errors.New("identity: operasi tidak diizinkan")
	// ErrEmailTaken: email sudah dipakai akun lain (Admin.CreateAdmin).
	ErrEmailTaken = errors.New("identity: email sudah dipakai")
	// ErrNotFound: sasaran operasi Admin tidak ada pada tenant ini.
	ErrNotFound = errors.New("identity: tidak ditemukan")
	// ErrInvalidInput: masukan tidak memenuhi aturan bentuk (mis. email atau nama pada CreateAdmin).
	ErrInvalidInput = errors.New("identity: masukan tidak valid")
	// ErrRateLimited adalah sentinel untuk *RateLimitedError (errors.Is).
	ErrRateLimited = errors.New("identity: dibatasi laju")
)

// RateLimitedError berarti permintaan harus dijawab 429 `rate_limited` dengan header Retry-After = RetryAfter.
// Tidak membedakan sebab (IP, email, akun, atau gerbang hash penuh).
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string        { return ErrRateLimited.Error() }
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

// AsRateLimited memberi tahu apakah err (atau galat yang dibungkusnya) adalah *RateLimitedError, dan berapa lama klien
// harus menunggu.
func AsRateLimited(err error) (time.Duration, bool) {
	var rl *RateLimitedError
	if errors.As(err, &rl) {
		return rl.RetryAfter, true
	}
	return 0, false
}

// PolicyViolation menyebut satu aturan kebijakan sandi yang dilanggar (kode stabil, bukan teks bebas).
type PolicyViolation = string

// PasswordPolicyError berarti sandi baru ditolak kebijakan (FR-101). Codes memuat kode aturan yang dilanggar; tidak
// pernah memuat sandi. Handler memetakannya ke `password_policy_violation`.
type PasswordPolicyError struct{ Codes []PolicyViolation }

func (e *PasswordPolicyError) Error() string {
	return fmt.Sprintf("identity: sandi tidak memenuhi kebijakan: %s", strings.Join(e.Codes, ","))
}
