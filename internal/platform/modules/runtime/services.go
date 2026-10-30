package runtime

import (
	"context"

	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/export"
)

// Services adalah layanan platform yang dipasang ke context modul: masker field dan auditor ekspor.
type Services struct {
	Masker  authz.FieldMasker
	Auditor export.Auditor
	// I18n (opsional) memasang locale, Resolver terjemahan, dan scope namespace modul pada context hook
	// lifecycle, command, dan seed (PRD-000 FR-25); route HTTP memperolehnya dari middleware platform.
	I18n func(ctx context.Context, t tenancy.TenantID, moduleKey string) context.Context
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

// installI18n memasang i18n untuk hook lifecycle, command, dan seed (route HTTP memperolehnya dari middleware).
func (s Services) installI18n(ctx context.Context, t tenancy.TenantID, moduleKey string) context.Context {
	if s.I18n != nil {
		ctx = s.I18n(ctx, t, moduleKey)
	}
	return ctx
}

// WithServices memasang Services pada setiap route modul.
func WithServices(s Services) MountOption {
	return func(c *mountConfig) { c.services = s }
}
