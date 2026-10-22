package httpserver

import (
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"erpora/internal/platform/bootstrap"
	"erpora/internal/platform/tenancy"
)

// NewRouter membuat router dengan /healthz, /readyz, /api/bootstrap, /api/ (404 JSON), dan / (SPA).
func NewRouter(d Deps) *Router {
	d = withDefaults(d)
	r := &Router{mux: http.NewServeMux(), probe: http.NewServeMux(), deps: d}

	r.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteData(w, http.StatusOK, map[string]string{"status": "ok"})
	}), WithoutTenant())

	r.Handle("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if d.Readiness != nil {
			if err := d.Readiness.Ready(req.Context()); err != nil {
				d.Logger.WarnContext(req.Context(), "readiness gagal", "error", err)
				WriteError(w, req, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
				return
			}
		}
		WriteData(w, http.StatusOK, map[string]string{"status": "ready"})
	}), WithoutTenant())

	r.Handle("GET /api/bootstrap", bootstrapHandler(d.AppVersion))

	r.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Path dikenal dengan method lain -> 405 + Allow; path tidak dikenal -> 404.
		if allowed := r.allowedMethods(req); len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed", "metode tidak diizinkan untuk endpoint ini", nil)
			return
		}
		WriteError(w, req, http.StatusNotFound, "not_found", "endpoint tidak ditemukan", nil)
	}))

	assets := d.Assets
	if assets == nil {
		assets = emptyFS{}
	}
	r.Handle("/", NewSPA(assets))
	return r
}

func bootstrapHandler(appVersion string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tid, _ := tenancy.FromContext(req.Context())
		m := bootstrap.Snapshot(appVersion, tid.String())
		etag := `"` + strconv.FormatInt(m.Revision, 10) + `"`
		w.Header().Set("ETag", etag)
		if etagMatches(req.Header[textproto.CanonicalMIMEHeaderKey("If-None-Match")], etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		WriteData(w, http.StatusOK, m)
	})
}

func etagMatches(values []string, etag string) bool {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			p := strings.TrimSpace(part)
			if p == "*" || p == etag || p == "W/"+etag {
				return true
			}
		}
	}
	return false
}
