package runtime

import (
	"context"

	"erpora/sdk/authz"
	"erpora/sdk/export"
)

// Services adalah layanan platform yang dipasang ke context modul: masker field dan auditor ekspor.
type Services struct {
	Masker  authz.FieldMasker
	Auditor export.Auditor
}

// install memasang layanan yang tidak nil ke ctx.
func (s Services) install(ctx context.Context) context.Context {
	if s.Masker != nil {
		ctx = authz.WithFieldMasker(ctx, s.Masker)
	}
	if s.Auditor != nil {
		ctx = export.WithAuditor(ctx, s.Auditor)
	}
	return ctx
}

// WithServices memasang Services pada setiap route modul.
func WithServices(s Services) MountOption {
	return func(c *mountConfig) { c.services = s }
}
