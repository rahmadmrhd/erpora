// Package tenancy memuat seam multi-tenant: identitas tenant di context dan Resolver request.
package tenancy

import (
	"context"

	"github.com/google/uuid"
)

// TenantID adalah identitas tenant (UUID).
type TenantID uuid.UUID

// String mengembalikan representasi UUID.
func (t TenantID) String() string { return uuid.UUID(t).String() }

type ctxKey struct{}

// WithTenant menyimpan tenant aktif di context.
func WithTenant(ctx context.Context, id TenantID) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext mengambil tenant aktif dari context.
func FromContext(ctx context.Context) (TenantID, bool) {
	id, ok := ctx.Value(ctxKey{}).(TenantID)
	return id, ok
}
