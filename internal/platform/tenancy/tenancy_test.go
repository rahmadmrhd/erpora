package tenancy

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestSingleTenantResolver_AlwaysDefault(t *testing.T) {
	id := TenantID(uuid.New())
	r := SingleTenantResolver{ID: id}
	for _, path := range []string{"/", "/api/x", "/healthz"} {
		got, err := r.Resolve(httptest.NewRequest("GET", path, nil))
		if err != nil || got != id {
			t.Fatalf("Resolve(%s) = %v, %v", path, got, err)
		}
	}
}

func TestFromContext_Missing(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("expected ok=false")
	}
}

func TestWithTenant_RoundTrip(t *testing.T) {
	id := TenantID(uuid.New())
	got, ok := FromContext(WithTenant(context.Background(), id))
	if !ok || got != id {
		t.Fatalf("got %v, %v", got, ok)
	}
}
