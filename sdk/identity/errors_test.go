package identity_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"erpora/sdk/identity"
)

func TestRateLimitedError(t *testing.T) {
	err := fmt.Errorf("dibungkus: %w", &identity.RateLimitedError{RetryAfter: 7 * time.Second})
	if d, ok := identity.AsRateLimited(err); !ok || d != 7*time.Second {
		t.Fatalf("AsRateLimited = %v, %v", d, ok)
	}
	if !errors.Is(err, identity.ErrRateLimited) {
		t.Error("errors.Is(ErrRateLimited) harus berlaku")
	}
	if _, ok := identity.AsRateLimited(identity.ErrInvalidCredentials); ok {
		t.Error("galat lain bukan rate limited")
	}
}

func TestPasswordPolicyError_MessageHasCodesOnly(t *testing.T) {
	err := &identity.PasswordPolicyError{Codes: []string{"too_short", "common"}}
	if got := err.Error(); !strings.Contains(got, "too_short,common") {
		t.Fatalf("Error() = %q", got)
	}
	var pe *identity.PasswordPolicyError
	if !errors.As(fmt.Errorf("x: %w", err), &pe) || len(pe.Codes) != 2 {
		t.Fatal("errors.As harus menemukan PasswordPolicyError")
	}
}

func TestDomainErrorsAreDistinct(t *testing.T) {
	all := []error{
		identity.ErrInvalidCredentials, identity.ErrWrongCurrentPassword, identity.ErrInvalidRefresh, identity.ErrOriginNotAllowed,
		identity.ErrInvalidResetToken, identity.ErrInvalidMFACode, identity.ErrMFARequired, identity.ErrMFAAlreadyEnrolled,
		identity.ErrMFANotEnrolled, identity.ErrAccountLocked, identity.ErrForbidden, identity.ErrEmailTaken, identity.ErrNotFound,
		identity.ErrInvalidInput, identity.ErrRateLimited, identity.ErrUnauthenticated,
	}
	for i, a := range all {
		for j, b := range all {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v dan %v tidak boleh saling cocok", a, b)
			}
		}
	}
}
