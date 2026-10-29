package app

import (
	"context"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/runtime"
	"go.opentelemetry.io/otel/metric"
	"log/slog"

	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/catalog"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
	uii18n "erpora/packages/ui/i18n"
	sdki18n "erpora/sdk/i18n"
)

// newI18nService membangun i18n.Service dari registri bahasa repo, katalog platform dan ui, serta katalog
// setiap modul ber-CatalogSource (namespace = slug). active melaporkan apakah modul ber-slug ns aktif untuk
// meter (opsional, nil bila tidak ada) menghitung pemakaian fallback (erp.i18n.fallback).
// tenant; nil = semua aktif (CLI). overrides (opsional, nil literal bila tidak ada) menyimpan override istilah per tenant. Katalog yang rusak secara struktural mengembalikan galat sehingga startup
// ditolak (PRD-000 FR-25, Requirement 8.1); ketidaksesuaian antar bahasa hanya dicatat sebagai peringatan.
func newI18nService(log *slog.Logger, dev bool, reg *registry.Registry, active func(tenancy.TenantID, string) bool, overrides i18n.OverrideStore, meter metric.Meter) (*i18n.Service, error) {
	locales, err := i18n.Load()
	if err != nil {
		return nil, err
	}
	sources := []i18n.Source{
		{Namespace: i18n.NamespacePlatform, FS: catalog.FS()},
		{Namespace: i18n.NamespaceUI, FS: uii18n.FS()},
	}
	for _, e := range reg.Entries() {
		if e.Catalogs == nil || e.Manifest.I18n == nil {
			continue
		}
		sources = append(sources, i18n.Source{
			Namespace: e.Manifest.Slug, Module: true, FS: e.Catalogs,
			Locales: e.Manifest.I18n.Locales, Default: e.Manifest.I18n.DefaultLocale,
		})
	}
	return i18n.New(i18n.Options{Registry: locales, Sources: sources, Active: active, Dev: dev, Log: log, Overrides: overrides, Meter: meter})
}

// moduleActive membentuk Options.Active dari status runtime: modul ber-namespace ns aktif bila statusnya enabled.
func moduleActive(reg *registry.Registry, status func(t tenancy.TenantID, key string) bool) func(tenancy.TenantID, string) bool {
	keyBySlug := map[string]string{}
	for _, e := range reg.Entries() {
		keyBySlug[e.Manifest.Slug] = e.Manifest.Key
	}
	return func(t tenancy.TenantID, ns string) bool {
		key, ok := keyBySlug[ns]
		return ok && status(t, key)
	}
}

// i18nHook mengisi runtime.Services.I18n: context hook lifecycle, command, dan seed memperoleh bahasa
// default, Resolver terjemahan terikat tenant, dan scope = slug modul (route HTTP memperolehnya dari middleware).
func i18nHook(svc *i18n.Service, reg *registry.Registry) func(context.Context, tenancy.TenantID, string) context.Context {
	slugByKey := map[string]string{}
	for _, e := range reg.Entries() {
		slugByKey[e.Manifest.Key] = e.Manifest.Slug
	}
	def := sdki18n.Locale(svc.Registry().Default)
	return func(ctx context.Context, t tenancy.TenantID, key string) context.Context {
		ctx = sdki18n.WithLocale(ctx, def)
		ctx = sdki18n.WithResolver(ctx, svc.Resolver(t))
		if slug := slugByKey[key]; slug != "" {
			ctx = sdki18n.WithScope(ctx, slug)
		}
		return ctx
	}
}

// activeFromModules membentuk Options.Active dari status efektif lifecycle: hanya modul berstatus enabled yang aktif
// (sama dengan API, yang memakai runtime snapshot). Dipakai CLI `erpora i18n override`.
func activeFromModules(mods []lifecycle.ModuleState) func(tenancy.TenantID, string) bool {
	enabled := make(map[string]bool, len(mods))
	for _, m := range mods {
		enabled[m.Slug] = m.Status == runtime.StatusEnabled
	}
	return func(_ tenancy.TenantID, ns string) bool { return enabled[ns] }
}
