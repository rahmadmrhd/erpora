package identitytest

import (
	"context"
	"testing"

	"erpora/sdk/identity"
)

func TestGuardPanicsOutsideTests(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("guard(false) harus panik")
		}
	}()
	guard(false)
}

func TestGuardAllowsInTests(t *testing.T) {
	guard(true) // tidak panik
}

func TestWithPrincipalInTest(t *testing.T) {
	ctx := WithPrincipal(t, context.Background(), identity.Principal{UserID: "u1"})
	p, ok := identity.FromContext(ctx)
	if !ok || p.UserID != "u1" {
		t.Fatalf("p=%+v ok=%v", p, ok)
	}
}
