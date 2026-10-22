package database

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/tenancy"
)

func TestSingleTenantConnector_ReturnsDB(t *testing.T) {
	id := tenancy.TenantID(uuid.New())
	c := SingleTenantConnector{Default: id, DB: TenantDB{}}
	if _, err := c.For(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

func TestSingleTenantConnector_RejectsOtherTenant(t *testing.T) {
	c := SingleTenantConnector{Default: tenancy.TenantID(uuid.New())}
	_, err := c.For(context.Background(), tenancy.TenantID(uuid.New()))
	if !errors.Is(err, ErrUnknownTenant) {
		t.Fatalf("err = %v", err)
	}
}
