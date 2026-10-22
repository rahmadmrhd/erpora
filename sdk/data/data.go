// Package data adalah satu-satunya jalur modul ke database: modul tidak pernah memegang pool.
package data

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrNoTenantContext dikembalikan DB bila context tidak membawa Provider (tenant).
var ErrNoTenantContext = errors.New("data: tidak ada konteks tenant")

// Provider menyediakan *gorm.DB untuk tenant yang ada di context. Diimplementasi platform.
type Provider interface {
	DB(ctx context.Context) (*gorm.DB, error)
}

type providerKey struct{}

// WithProvider menempelkan Provider ke context (dipanggil platform, bukan modul).
func WithProvider(ctx context.Context, p Provider) context.Context {
	return context.WithValue(ctx, providerKey{}, p)
}

// DB mengembalikan database tenant aktif dari context.
func DB(ctx context.Context) (*gorm.DB, error) {
	p, ok := ctx.Value(providerKey{}).(Provider)
	if !ok || p == nil {
		return nil, ErrNoTenantContext
	}
	return p.DB(ctx)
}
