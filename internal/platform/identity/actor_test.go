package identity

import (
	"context"
	"testing"

	sdkidentity "erpora/sdk/identity"
)

func TestActorFromContext(t *testing.T) {
	ctx := sdkidentity.WithPrincipal(context.Background(), sdkidentity.Principal{UserID: "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", Kind: sdkidentity.KindHuman})
	got, ok := ActorFromContext(ctx)
	if !ok || got != "user:0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b" {
		t.Fatalf("ActorFromContext = %q, %v", got, ok)
	}
}

// Tanpa Principal (atau Principal tanpa UserID) tidak ada aktor: pemanggil wajib gagal tertutup, tidak ada nilai bawaan.
func TestActorFromContext_FailClosed(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"tanpa principal": context.Background(),
		"tanpa user id":   sdkidentity.WithPrincipal(context.Background(), sdkidentity.Principal{Kind: sdkidentity.KindHuman}),
	} {
		if got, ok := ActorFromContext(ctx); ok || got != "" {
			t.Errorf("%s: ActorFromContext = %q, %v; want kosong dan false", name, got, ok)
		}
	}
}
