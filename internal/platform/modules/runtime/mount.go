package runtime

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
)

// Authorizer memutuskan apakah request pada ctx boleh memakai permission.
type Authorizer interface {
	Allow(ctx context.Context, permission string) bool
}

// Mounter dipenuhi *httpserver.Router.
type Mounter interface {
	Handle(pattern string, h http.Handler, opts ...httpserver.RouteOption)
	// SetAPIGate memasang gate permukaan HTTP modul tidak aktif (FR-19.7); wajib agar tidak terlewat.
	SetAPIGate(g func(*http.Request) bool)
}

// Mount mendaftarkan semua route modul dari registry ke router. Tiap handler
// dibungkus guard status modul, lalu pengecekan permission (kecuali Public).
// Route platform.core ikut dipasang di sini: guard selalu enabled, permission tetap berlaku.
// Rantai penuh: otel -> recover -> requestScope -> Authenticate (kecuali Public)
// -> tenantContext -> guard -> permission -> handler.
func Mount(m Mounter, reg *registry.Registry, snap *Snapshot, authz Authorizer, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	m.SetAPIGate(apiGate(reg, snap))
	for _, rt := range reg.Routes() {
		h := rt.Handler
		if !rt.Public {
			h = permissionMW(authz, rt.Permission, rt.Module, log, h)
		}
		h = guardMW(snap, rt.Module, h)
		var opts []httpserver.RouteOption
		if rt.Public {
			opts = append(opts, httpserver.Public())
		}
		m.Handle(rt.Method+" "+rt.Pattern, h, opts...)
	}
}

// apiGate menyembunyikan path /api/{slug}/... milik modul terdaftar yang statusnya bukan enabled,
// agar method lain dan sub-path tak dikenal tidak membocorkan route (FR-19.7).
func apiGate(reg *registry.Registry, snap *Snapshot) func(*http.Request) bool {
	keyBySlug := map[string]string{}
	for _, e := range reg.Entries() {
		keyBySlug[e.Manifest.Slug] = e.Manifest.Key
	}
	return func(r *http.Request) bool {
		rest, ok := strings.CutPrefix(r.URL.Path, "/api/")
		if !ok {
			return false
		}
		slug, _, _ := strings.Cut(rest, "/")
		key, ok := keyBySlug[slug]
		if !ok {
			return false
		}
		tid, _ := tenancy.FromContext(r.Context())
		return snap.Status(tid, key) != StatusEnabled
	}
}

func guardMW(snap *Snapshot, key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("erp.module.key", key))
		httpserver.SetModule(r.Context(), key)
		r = r.WithContext(observability.WithModule(r.Context(), key))
		tid, _ := tenancy.FromContext(r.Context())
		switch st := snap.Status(tid, key); st {
		case StatusEnabled:
			next.ServeHTTP(w, r)
		case StatusInstalled, StatusDisabled:
			httpserver.WriteError(w, r, http.StatusForbidden, "module_disabled", "modul tidak aktif", nil)
		case StatusFailed, StatusIncompatible:
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "module_unavailable", "modul tidak tersedia", nil)
		case StatusUpgradePending:
			w.Header().Set("Retry-After", "30")
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "module_unavailable", "modul sedang menunggu upgrade", nil)
		default: // available, uninstalled, missing, dan status tak dikenal
			httpserver.WriteError(w, r, http.StatusNotFound, "module_not_found", "modul tidak ditemukan", nil)
		}
	})
}

func permissionMW(authz Authorizer, permission, module string, log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authz == nil {
			// Tanpa Authorizer: tolak (fail closed). Penolakan oleh Authorizer dicatat Authorizer itu sendiri.
			log.WarnContext(r.Context(), "permission ditolak: authorizer tidak dikonfigurasi", "erp.module.key", module, "permission", permission)
		}
		if authz == nil || !authz.Allow(r.Context(), permission) {
			httpserver.WriteError(w, r, http.StatusForbidden, "permission_denied", "tidak memiliki izin", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
