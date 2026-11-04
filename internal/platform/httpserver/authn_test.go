package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
)

type resolverFunc func(*http.Request) (tenancy.TenantID, error)

func (f resolverFunc) Resolve(r *http.Request) (tenancy.TenantID, error) { return f(r) }

// ADR-0013 §4: tenantContext berjalan SEBELUM Authenticate (klaim tid dicocokkan dengan tenant context). Urutan ini
// sebelumnya terbalik (authn,tenant); diubah sengaja bersama C1.10.
func TestAuthenticate_RunsAfterTenantAndSkippedForPublic(t *testing.T) {
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
	if got := strings.Join(order, ","); got != "tenant,authn,handler" {
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

func TestRateLimited_ChainOrderAndScope(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	d := Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver: resolverFunc(func(*http.Request) (tenancy.TenantID, error) {
			order = append(order, "tenant")
			return defaultTenant, nil
		}),
		Connector:      database.SingleTenantConnector{Default: defaultTenant},
		Authenticate:   mw("authn"),
		IPLimit:        mw("ip"),
		PrincipalLimit: mw("principal"),
	}
	r := NewRouter(d)
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") })
	r.Handle("POST /api/auth/priv", ok, RateLimited())
	r.Handle("POST /api/auth/open", ok, Public(), RateLimited())
	r.Handle("GET /api/plain", ok)

	cases := []struct{ method, path, want string }{
		{"POST", "/api/auth/priv", "tenant,ip,authn,principal,handler"},
		{"POST", "/api/auth/open", "tenant,ip,handler"}, // Public: tanpa Authenticate dan batas principal
		{"GET", "/api/plain", "tenant,authn,handler"},   // bukan RateLimited: tanpa batas laju sama sekali
	}
	for _, c := range cases {
		order = nil
		do(r, c.method, c.path, nil)
		if got := strings.Join(order, ","); got != c.want {
			t.Errorf("%s %s: urutan = %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

func TestRouter_ClientIPIsPeerAddressNotForwardedHeader(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	var got string
	r.Handle("GET /api/ip", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got = httpx.ClientIP(req.Context())
	}), Public())
	req := httptest.NewRequest(http.MethodGet, "/api/ip", nil)
	req.RemoteAddr = "198.51.100.9:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.Header.Set("X-Real-IP", "203.0.113.2")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.9" {
		t.Fatalf("ClientIP = %q, want alamat peer", got)
	}
}

// ADR-0013 §1: semua respons endpoint auth (RateLimited) ber-Cache-Control: no-store, termasuk yang dibuat platform
// sebelum handler (tenant tak terselesaikan, 429 IPLimit, 401 Authenticate, 429 PrincipalLimit) dan panic. Route yang
// bukan RateLimited tidak dipengaruhi.
func TestRateLimited_PlatformResponsesAreNoStore(t *testing.T) {
	reject := func(status int, code string) func(http.Handler) http.Handler {
		return func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				WriteError(w, r, status, code, "ditolak", nil)
			})
		}
	}
	pass := func(next http.Handler) http.Handler { return next }
	build := func(ip, authn, principal func(http.Handler) http.Handler, resolveErr bool) *Router {
		return NewRouter(Deps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			Resolver: resolverFunc(func(*http.Request) (tenancy.TenantID, error) {
				if resolveErr {
					return tenancy.TenantID{}, http.ErrNoLocation
				}
				return defaultTenant, nil
			}),
			Connector:      database.SingleTenantConnector{Default: defaultTenant},
			Authenticate:   authn,
			IPLimit:        ip,
			PrincipalLimit: principal,
		})
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("uji") })

	cases := []struct {
		name string
		r    *Router
		path string
		want int
	}{
		{"429 IPLimit", build(reject(http.StatusTooManyRequests, "rate_limited"), pass, pass, false), "/api/auth/priv", http.StatusTooManyRequests},
		{"401 Authenticate", build(pass, reject(http.StatusUnauthorized, "unauthenticated"), pass, false), "/api/auth/priv", http.StatusUnauthorized},
		{"429 PrincipalLimit", build(pass, pass, reject(http.StatusTooManyRequests, "rate_limited"), false), "/api/auth/priv", http.StatusTooManyRequests},
		{"tenant tak terselesaikan", build(pass, pass, pass, true), "/api/auth/priv", http.StatusBadRequest},
		{"Public: 429 IPLimit", build(reject(http.StatusTooManyRequests, "rate_limited"), pass, pass, false), "/api/auth/open", http.StatusTooManyRequests},
		{"panic handler", build(pass, pass, pass, false), "/api/auth/boom", http.StatusInternalServerError},
		{"sukses", build(pass, pass, pass, false), "/api/auth/priv", http.StatusNoContent},
	}
	for _, c := range cases {
		c.r.Handle("POST /api/auth/priv", ok, RateLimited())
		c.r.Handle("POST /api/auth/open", ok, Public(), RateLimited())
		c.r.Handle("POST /api/auth/boom", boom, Public(), RateLimited())
		rec := do(c.r, http.MethodPost, c.path, nil)
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, rec.Code, c.want)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", c.name, cc)
		}
	}

	// Bukan RateLimited: tidak ditandai.
	r := build(pass, reject(http.StatusUnauthorized, "unauthenticated"), pass, false)
	r.Handle("GET /api/plain", ok)
	if cc := do(r, http.MethodGet, "/api/plain", nil).Header().Get("Cache-Control"); cc != "" {
		t.Errorf("route non-RateLimited: Cache-Control = %q, want kosong", cc)
	}
}
