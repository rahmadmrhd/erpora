package runtime

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/database"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/data"
	"erpora/sdk/module"
)

// ModuleContext menyiapkan ctx + Env untuk hook lifecycle dan command modul: tenant, data.Provider,
// erp.module.key (context dan atribut span), hak sistem teraudit bila systemReason != "", dan
// Env.Logger = base.With("erp.module.key", key), serta layanan platform (masker field, auditor ekspor) dari svc. Modul tidak pernah memegang pool.
func ModuleContext(ctx context.Context, t tenancy.TenantID, key string, conn database.TenantConnector, base *slog.Logger, systemReason string, svc Services) (context.Context, module.Env) {
	if base == nil {
		base = slog.Default()
	}
	ctx = tenancy.WithTenant(ctx, t)
	ctx = data.WithProvider(ctx, database.DataProvider(conn))
	ctx = observability.WithModule(ctx, key)
	ctx = svc.install(ctx)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("erp.module.key", key))
	if systemReason != "" {
		ctx = authz.AsSystem(ctx, systemReason)
		base.InfoContext(ctx, "hak sistem dipakai", "system_reason", systemReason, "erp.module.key", key)
	}
	return ctx, module.Env{TenantID: t.String(), Logger: base.With("erp.module.key", key)}
}
