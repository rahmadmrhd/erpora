package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

type resolverFunc func(*http.Request) (tenancy.TenantID, error)

func (f resolverFunc) Resolve(r *http.Request) (tenancy.TenantID, error) { return f(r) }

func TestAuthenticate_RunsBeforeTenantAndSkippedForPublic(t *testing.T) {
	var order []string
	d := Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver: resolverFunc(func(*http.Request) (tenancy.TenantID, error) {
			order = append(order, "tenant")
			return defaultTenant, nil
		}),
		Connector: database.SingleTenantConnector{Default: defaultTenant},
		Authenticate: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "authn")
				next.ServeHTTP(w, r)
			})
		},
	}
	r := NewRouter(d)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { order = append(order, "handler") })
	r.Handle("GET /api/private", ok)
	r.Handle("GET /api/open", ok, Public())

	do(r, http.MethodGet, "/api/private", nil)
	if got := strings.Join(order, ","); got != "authn,tenant,handler" {
		t.Fatalf("urutan route terproteksi = %q", got)
	}
	order = nil
	do(r, http.MethodGet, "/api/open", nil)
	if got := strings.Join(order, ","); got != "tenant,handler" {
		t.Fatalf("urutan route Public = %q", got)
	}
}

func TestAuthenticate_NilIsPassThrough(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	r.Handle("GET /api/x", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	if rec := do(r, http.MethodGet, "/api/x", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}
