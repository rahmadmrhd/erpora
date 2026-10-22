package database

import (
	"context"
	"errors"

	"erpora/internal/platform/tenancy"
)

// ErrUnknownTenant dikembalikan connector untuk tenant yang tidak dikenal.
var ErrUnknownTenant = errors.New("tenant tidak dikenal")

// TenantConnector memberi akses database untuk tenant tertentu. PRD Tenancy mengganti
// implementasinya dengan multi-pool.
type TenantConnector interface {
	For(ctx context.Context, id tenancy.TenantID) (TenantDB, error)
}

// SingleTenantConnector melayani satu tenant default dengan satu DB.
type SingleTenantConnector struct {
	Default tenancy.TenantID
	DB      TenantDB
}

// For mengembalikan DB bila id sama dengan tenant default, selain itu ErrUnknownTenant.
func (c SingleTenantConnector) For(_ context.Context, id tenancy.TenantID) (TenantDB, error) {
	if id != c.Default {
		return TenantDB{}, ErrUnknownTenant
	}
	return c.DB, nil
}
