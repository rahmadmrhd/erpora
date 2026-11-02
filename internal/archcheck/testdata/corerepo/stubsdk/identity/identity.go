// Package identity adalah stub minimal SDK untuk fixture arch test IDENT.
package identity

import "context"

// Principal, Service, dan Admin meniru tipe asli.
type (
	Principal struct{ UserID string }
	Service   interface {
		Logout(ctx context.Context, sessionID string) error
	}
	Admin struct{}
)

// WithPrincipal, WithService, dan WithAdmin meniru pemasang konteks milik platform.
func WithPrincipal(ctx context.Context, p Principal) context.Context { return ctx }

// WithService meniru signature asli.
func WithService(ctx context.Context, s Service) context.Context { return ctx }

// WithAdmin meniru signature asli.
func WithAdmin(ctx context.Context, a Admin) context.Context { return ctx }

// FromContext adalah pembaca yang sah dipakai semua modul; ServiceFrom dan AdminFrom hanya modul core.
func FromContext(ctx context.Context) (Principal, bool) { return Principal{}, false }

// ServiceFrom meniru signature asli.
func ServiceFrom(ctx context.Context) (Service, bool) { return nil, false }

// AdminFrom meniru signature asli; hanya modul core yang boleh membaca Admin dari context.
func AdminFrom(ctx context.Context) (Admin, bool) { return Admin{}, false }
