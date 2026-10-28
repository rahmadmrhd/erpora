package database

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/tenancy"
	"erpora/sdk/data"
)

func TestDataProvider_NoTenantInContext(t *testing.T) {
	p := DataProvider(SingleTenantConnector{})
	if _, err := p.DB(context.Background()); !errors.Is(err, data.ErrNoTenantContext) {
		t.Fatalf("err = %v, want ErrNoTenantContext", err)
	}
}

func TestDataProvider_UnknownTenant(t *testing.T) {
	p := DataProvider(SingleTenantConnector{Default: tenancy.TenantID(uuid.New())})
	ctx := tenancy.WithTenant(context.Background(), tenancy.TenantID(uuid.New()))
	if _, err := p.DB(ctx); !errors.Is(err, ErrUnknownTenant) {
		t.Fatalf("err = %v, want ErrUnknownTenant", err)
	}
}

func TestDataProvider_TenantDBNotReady(t *testing.T) {
	id := tenancy.TenantID(uuid.New())
	p := DataProvider(SingleTenantConnector{Default: id})
	if _, err := p.DB(tenancy.WithTenant(context.Background(), id)); err == nil {
		t.Fatal("DB nil harus error")
	}
}
