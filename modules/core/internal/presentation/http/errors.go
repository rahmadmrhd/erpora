package httpapi

import (
	"errors"
	"net/http"

	"erpora/sdk/httpx"
	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/identity"
)

// Kode galat modul core. Yang dimiliki platform (unauthenticated, permission_denied, mfa_required, rate_limited,
// validation_failed, bad_request, conflict, invalid_state, internal_error) tidak diulang di sini.
const (
	// CodeInvalidCredentials: login gagal; seragam untuk email tak dikenal, sandi salah, akun terkunci, dan akun
	// nonaktif (anti-enumerasi; FR-101).
	CodeInvalidCredentials = "invalid_credentials" //nolint:gosec // G101 salah duga: kode galat API, bukan kredensial
	// CodeAccountLocked: akun terkunci pada langkah setelah sandi lolos (MFA); tidak membuka enumerasi karena pemanggil
	// sudah membuktikan sandi.
	CodeAccountLocked = "account_locked"
	// CodePasswordPolicyViolation: sandi baru ditolak kebijakan FR-101; details memuat kode aturan, bukan daftar sandi.
	CodePasswordPolicyViolation = "password_policy_violation"
)

// respondError memetakan galat identity.Service/Admin (sdk/identity/errors.go) ke amplop galat api-guidelines §8.
// Pesan dan details tidak pernah memuat masukan pengguna (sandi, token, kode) maupun sebab internal; galat tak dikenal
// dijawab 500 generik (httpx.RespondError mencatat sebabnya di log server dengan requestId).
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	if d, ok := identity.AsRateLimited(err); ok {
		httpx.TooManyRequests(w, r, d)
		return
	}
	var policy *identity.PasswordPolicyError
	switch {
	case errors.As(err, &policy):
		details := make([]httpx.Detail, 0, len(policy.Codes))
		for _, c := range policy.Codes {
			details = append(details, httpx.Detail{Field: "newPassword", Code: c, Message: policyMessage(r, c)})
		}
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, CodePasswordPolicyViolation, "Sandi baru tidak memenuhi kebijakan.", details)
	case errors.Is(err, identity.ErrInvalidCredentials):
		httpx.WriteError(w, r, http.StatusUnauthorized, CodeInvalidCredentials, "Email atau sandi salah.", nil)
	case errors.Is(err, identity.ErrUnauthenticated), errors.Is(err, identity.ErrInvalidRefresh):
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "Autentikasi diperlukan.", nil)
	case errors.Is(err, identity.ErrOriginNotAllowed), errors.Is(err, identity.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "permission_denied", "Anda tidak memiliki izin untuk aksi ini.", nil)
	case errors.Is(err, identity.ErrMFARequired):
		httpx.WriteError(w, r, http.StatusForbidden, "mfa_required", "Verifikasi dua langkah diperlukan.", nil)
	case errors.Is(err, identity.ErrAccountLocked):
		httpx.WriteError(w, r, http.StatusForbidden, CodeAccountLocked, "Akun terkunci.", nil)
	case errors.Is(err, identity.ErrWrongCurrentPassword):
		validationFailed(w, r, "currentPassword", "wrong_current_password", detailMessage(r, "detail.wrong_current_password", "Sandi saat ini salah."))
	case errors.Is(err, identity.ErrInvalidResetToken):
		validationFailed(w, r, "token", "invalid_token", detailMessage(r, "detail.invalid_token", "Tautan tidak valid atau sudah tidak berlaku."))
	case errors.Is(err, identity.ErrInvalidMFACode):
		validationFailed(w, r, "code", "invalid_code", detailMessage(r, "detail.invalid_code", "Kode tidak valid."))
	case errors.Is(err, identity.ErrInvalidInput):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.", nil)
	case errors.Is(err, identity.ErrMFAAlreadyEnrolled):
		httpx.WriteError(w, r, http.StatusConflict, "conflict", "Verifikasi dua langkah sudah aktif.", nil)
	case errors.Is(err, identity.ErrMFANotEnrolled):
		httpx.WriteError(w, r, http.StatusConflict, "invalid_state", "Verifikasi dua langkah belum didaftarkan.", nil)
	default:
		httpx.RespondError(w, r, err)
	}
}

func validationFailed(w http.ResponseWriter, r *http.Request, field, code, message string) {
	httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.",
		[]httpx.Detail{{Field: field, Code: code, Message: message}})
}

// policyMessage menerjemahkan satu kode pelanggaran kebijakan sandi; kode tak dikenal memakai pesan generik.
func policyMessage(r *http.Request, code string) string {
	switch code {
	case "too_short", "too_long", "common":
		return detailMessage(r, "password.violation."+code, "Sandi tidak memenuhi kebijakan.")
	}
	return detailMessage(r, "password.violation.unknown", "Sandi tidak memenuhi kebijakan.")
}

// detailMessage mencari terjemahan di katalog modul (namespace core dari konteks request); bila penerjemah tidak ada
// atau kunci tidak ditemukan, teks bawaan dipakai.
func detailMessage(r *http.Request, key, fallback string) string {
	if s, ok := sdki18n.Lookup(r.Context(), key); ok {
		return s
	}
	return fallback
}
