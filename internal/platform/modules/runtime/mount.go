package runtime

import (
	"context"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
)

// Authorizer memutuskan apakah request pada ctx boleh memakai permission.
type Authorizer interface {
	Allow(ctx context.Context, permission string) bool
}

// Mounter dipenuhi *httpserver.Router.
type Mounter interface {
	Handle(pattern string, h http.Handler, opts ...httpserver.RouteOption)
}

// Mount mendaftarkan semua route modul dari registry ke router. Tiap handler
// dibungkus guard status modul, lalu pengecekan permission (kecuali Public).
// Rantai penuh: otel -> recover -> requestScope -> Authenticate (kecuali Public)
// -> tenantContext -> guard -> permission -> handler.
func Mount(m Mounter, reg *registry.Registry, snap *Snapshot, authz Authorizer) {
	for _, rt := range reg.Routes() {
		if rt.Module == registry.PlatformKey {
			continue // route platform dipasang oleh perakit aplikasi
		}
		h := rt.Handler
		if !rt.Public {
			h = permissionMW(authz, rt.Permission, rt.Module, h)
		}
		h = guardMW(snap, rt.Module, h)
		var opts []httpserver.RouteOption
		if rt.Public {
			opts = append(opts, httpserver.Public())
		}
		m.Handle(rt.Method+" "+rt.Pattern, h, opts...)
	}
}

func guardMW(snap *Snapshot, key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("erp.module.id", key))
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

func permissionMW(authz Authorizer, permission, module string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authz == nil || !authz.Allow(r.Context(), permission) {
			slog.WarnContext(r.Context(), "permission ditolak", "module", module, "permission", permission)
			httpserver.WriteError(w, r, http.StatusForbidden, "permission_denied", "tidak memiliki izin", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
