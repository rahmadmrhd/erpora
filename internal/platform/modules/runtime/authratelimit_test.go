package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/identity"
	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/modules/modtest"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
	"erpora/sdk/module"
)

func coreManifest() module.Manifest {
	doc := map[string]any{
		"schema_version": "1", "key": CoreModuleKey, "slug": "core", "name": "Core", "version": "1.0.0",
		"publisher":     map[string]string{"id": "erpora", "name": "ERPora"},
		"trust":         "builtin",
		"compatibility": map[string]string{"platform": "^0.1", "sdk": "^0.1"},
		"dependencies":  []map[string]any{{"module": registry.PlatformKey, "version": "^0.1", "optional": false}},
		"capabilities":  []string{"core.users.read"},
		"resources":     modtest.Resources("core.users.read"),
		"migrations":    map[string]string{"strategy": "none"},
		"offline":       map[string]string{"mode": "online-only"},
	}
	b, _ := json.Marshal(doc)
	return module.MustParseManifest(b)
}

// Rantai batas laju (ADR-0013 section 4, NFR-105) HARUS terpasang pada route auth core: IPLimit untuk semua (juga Public),
// PrincipalLimit hanya untuk yang terautentikasi; route non-auth tidak dibatasi.
func TestMount_RateLimitsCoreAuthRoutes(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	reg, err := registry.Build(registry.Options{
		PlatformVersion: "0.1.0", SDKVersion: "0.1.0", PlatformResources: modtest.Resources("platform.modules.read"),
		Platform: func(r module.Registrar) error { return nil },
	}, fakeModule{m: coreManifest(), register: func(r module.Registrar) error {
		r.Public("POST", "/auth/login", ok)
		r.Public("POST", "/auth/refresh", ok)
		r.Handle("POST", "/auth/mfa/verify", "core.users.read", ok)
		r.Handle("GET", "/users", "core.users.read", ok)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ipl, err := guard.NewIPLimiter(guard.LimiterConfig{Burst: 2, Every: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	var principalCalls atomic.Int32
	router := httpserver.NewRouter(httpserver.Deps{
		Logger: quietLog(), Tracer: sdktrace.NewTracerProvider(),
		Resolver:  resolverFunc(func(*http.Request) (tenancy.TenantID, error) { return tenantA, nil }),
		Connector: database.SingleTenantConnector{Default: tenantA},
		IPLimit:   identity.IPLimit(ipl),
		Authenticate: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := sdkidentity.WithPrincipal(r.Context(), sdkidentity.Principal{UserID: "u1", Kind: sdkidentity.KindHuman, AMR: []string{"pwd", "otp"}})
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		},
		PrincipalLimit: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				principalCalls.Add(1)
				next.ServeHTTP(w, r)
			})
		},
	})
	snap := NewSnapshot()
	snap.Apply(tenantA, map[string]Status{CoreModuleKey: StatusEnabled}, 1)
	Mount(router, reg, snap, allowAll, quietLog())

	do := func(method, path, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	// Route non-auth tidak pernah dibatasi IP.
	for range 6 {
		if rec := do("GET", "/api/core/users", "203.0.113.1"); rec.Code != http.StatusOK {
			t.Fatalf("/users = %d", rec.Code)
		}
	}
	if principalCalls.Load() != 0 {
		t.Fatalf("PrincipalLimit terpasang pada route non-auth: %d", principalCalls.Load())
	}
	// Route auth Public (login): batas IP berlaku (burst 2 per IP, dibagi bersama seluruh route auth); tanpa batas principal.
	for i := range 2 {
		if rec := do("POST", "/api/core/auth/login", "203.0.113.7"); rec.Code != http.StatusOK {
			t.Fatalf("login #%d = %d %s", i+1, rec.Code, rec.Body)
		}
	}
	rec := do("POST", "/api/core/auth/login", "203.0.113.7")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("login melewati batas IP = %d %s, want 429", rec.Code, rec.Body)
	}
	if ra, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || ra < 1 {
		t.Errorf("Retry-After = %q, want bilangan bulat >= 1", rec.Header().Get("Retry-After"))
	}
	if principalCalls.Load() != 0 {
		t.Errorf("route Public tidak boleh melewati PrincipalLimit: %d", principalCalls.Load())
	}
	// Route auth Public lain pada IP yang sama juga terkena (satu ember per IP); IP lain bersih.
	if rec := do("POST", "/api/core/auth/refresh", "203.0.113.7"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("refresh dari IP yang sama = %d, want 429", rec.Code)
	}
	// Route auth terautentikasi: IP limit + principal limit.
	if rec := do("POST", "/api/core/auth/mfa/verify", "203.0.113.8"); rec.Code != http.StatusOK {
		t.Fatalf("mfa/verify IP lain = %d %s", rec.Code, rec.Body)
	}
	if principalCalls.Load() != 1 {
		t.Errorf("PrincipalLimit pada mfa/verify = %d, want 1", principalCalls.Load())
	}
	do("POST", "/api/core/auth/mfa/verify", "203.0.113.8")
	if rec := do("POST", "/api/core/auth/mfa/verify", "203.0.113.8"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("mfa/verify melewati batas IP = %d, want 429", rec.Code)
	}
}
