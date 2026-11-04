package identity

import (
	"errors"
	"fmt"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/settings"
	sdkidentity "erpora/sdk/identity"
)

// mapErr menerjemahkan galat komponen platform ke galat domain sdk/identity di batas Service/Admin. Galat yang tak
// dikenal dibungkus dengan konteks operasi (op) dan diteruskan sebagai kegagalan infrastruktur: handler menjawab 500
// generik, sehingga teks internal tidak pernah sampai ke klien.
func mapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	// Sudah galat domain (mis. dari fungsi bantu di paket ini): teruskan.
	for _, d := range []error{
		sdkidentity.ErrInvalidCredentials, sdkidentity.ErrWrongCurrentPassword, sdkidentity.ErrInvalidRefresh,
		sdkidentity.ErrOriginNotAllowed, sdkidentity.ErrInvalidResetToken, sdkidentity.ErrInvalidMFACode,
		sdkidentity.ErrMFARequired, sdkidentity.ErrMFAAlreadyEnrolled, sdkidentity.ErrMFANotEnrolled,
		sdkidentity.ErrAccountLocked, sdkidentity.ErrForbidden, sdkidentity.ErrEmailTaken, sdkidentity.ErrNotFound,
		sdkidentity.ErrInvalidInput, sdkidentity.ErrUnauthenticated, sdkidentity.ErrRateLimited,
	} {
		if errors.Is(err, d) {
			return err
		}
	}
	if retry, ok := guard.AsRateLimited(err); ok {
		return &sdkidentity.RateLimitedError{RetryAfter: retry}
	}
	var locked *mfa.LockedError
	if errors.As(err, &locked) {
		if locked.AccountLocked {
			return sdkidentity.ErrAccountLocked
		}
		return &sdkidentity.RateLimitedError{RetryAfter: locked.RetryAfter}
	}
	var pol *reset.PolicyError
	if errors.As(err, &pol) {
		return policyError(pol.Violations)
	}
	switch {
	case errors.Is(err, session.ErrInvalidRefresh):
		return sdkidentity.ErrInvalidRefresh
	case errors.Is(err, session.ErrOriginNotAllowed):
		return sdkidentity.ErrOriginNotAllowed
	case errors.Is(err, session.ErrSessionNotFound), errors.Is(err, store.ErrNotFound):
		return sdkidentity.ErrNotFound
	case errors.Is(err, reset.ErrInvalidToken):
		return sdkidentity.ErrInvalidResetToken
	case errors.Is(err, mfa.ErrInvalidCode):
		return sdkidentity.ErrInvalidMFACode
	case errors.Is(err, mfa.ErrAlreadyEnrolled):
		return sdkidentity.ErrMFAAlreadyEnrolled
	case errors.Is(err, mfa.ErrNotEnrolled):
		return sdkidentity.ErrMFANotEnrolled
	case errors.Is(err, mfa.ErrForbidden):
		return sdkidentity.ErrForbidden
	case errors.Is(err, mfa.ErrInvalidArgument), errors.Is(err, store.ErrInvalidArgument), errors.Is(err, session.ErrInvalidInput),
		errors.Is(err, settings.ErrInvalidURL):
		// Pesan galat settings/store tidak pernah memuat masukan mentah (kontrak paketnya), jadi aman disertakan.
		return fmt.Errorf("%w: %v", sdkidentity.ErrInvalidInput, err)
	case errors.Is(err, store.ErrEmailTaken):
		return sdkidentity.ErrEmailTaken
	}
	return fmt.Errorf("identity: %s: %w", op, err)
}

// policyError mengubah pelanggaran kebijakan sandi menjadi galat domain (hanya kode aturan, tanpa sandi).
func policyError(vs []password.Violation) error {
	codes := make([]string, len(vs))
	for i, v := range vs {
		codes[i] = string(v.Code)
	}
	return &sdkidentity.PasswordPolicyError{Codes: codes}
}

// mapPrincipalErr adalah mapErr untuk metode yang terikat Principal (ChangePassword, MFA, ...): pengguna, keanggotaan,
// atau sesi milik Principal yang sudah tidak ada walau token masih berlaku berarti sesi tak lagi berlaku, jadi
// not-found dijawab ErrUnauthenticated (401), bukan ErrNotFound. ErrNotFound tetap milik operasi lain (mis. Admin pada
// sasaran tak dikenal; record di luar scope tetap 404 per FR-119).
func mapPrincipalErr(op string, err error) error {
	m := mapErr(op, err)
	if errors.Is(m, sdkidentity.ErrNotFound) {
		return sdkidentity.ErrUnauthenticated
	}
	return m
}
