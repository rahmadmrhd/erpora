package runtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

var tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

type fakeModule struct {
	m        module.Manifest
	register func(r module.Registrar) error
}

func (f fakeModule) Manifest() module.Manifest         { return f.m }
func (f fakeModule) Register(r module.Registrar) error { return f.register(r) }

func notesManifest() module.Manifest {
	doc := map[string]any{
		"schema_version": "1", "key": "acme.notes", "slug": "notes", "name": "Notes", "version": "1.0.0",
		"publisher":     map[string]string{"id": "acme", "name": "Acme"},
		"trust":         "builtin",
		"compatibility": map[string]string{"platform": "^0.1", "sdk": "^0.1"},
		"dependencies":  []map[string]any{{"module": registry.PlatformKey, "version": "^0.1", "optional": false}},
		"capabilities":  []string{"notes.items.read"},
		"migrations":    map[string]string{"strategy": "none"},
		"offline":       map[string]string{"mode": "online-only"},
	}
	b, _ := json.Marshal(doc)
	return module.MustParseManifest(b)
}

type authzFunc func(ctx context.Context, permission string) bool

func (f authzFunc) Allow(ctx context.Context, p string) bool { return f(ctx, p) }

var (
	allowAll = authzFunc(func(context.Context, string) bool { return true })
	denyAll  = authzFunc(func(context.Context, string) bool { return false })
)

type resolverFunc func(*http.Request) (tenancy.TenantID, error)

func (f resolverFunc) Resolve(r *http.Request) (tenancy.TenantID, error) { return f(r) }

type fixture struct {
	router *httpserver.Router
	snap   *Snapshot
	rec    *tracetest.SpanRecorder
	hits   *int
}

func newFixture(t *testing.T, authz Authorizer, order *[]string) fixture {
	t.Helper()
	hits := 0
	reg, err := registry.Build(registry.Options{PlatformVersion: "0.1.0", SDKVersion: "0.1.0"},
		fakeModule{m: notesManifest(), register: func(r module.Registrar) error {
			r.Handle("GET", "/items", "notes.items.read", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hits++
				ctx := req.Context()
				if id, ok := observability.RequestIDFromContext(ctx); !ok || id == "" {
					t.Error("request id tidak ada di context")
				}
				if !trace.SpanContextFromContext(ctx).IsValid() {
					t.Error("span context tidak valid")
				}
				if _, ok := tenancy.FromContext(ctx); !ok {
					t.Error("tenant tidak ada di context")
				}
				w.WriteHeader(http.StatusOK)
			}))
			r.Public("GET", "/status", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits++
				w.WriteHeader(http.StatusOK)
			}))
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	note := func(s string) {
		if order != nil {
			*order = append(*order, s)
		}
	}
	router := httpserver.NewRouter(httpserver.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tracer: tp,
		Resolver: resolverFunc(func(*http.Request) (tenancy.TenantID, error) {
			note("tenant")
			return tenantA, nil
		}),
		Connector: database.SingleTenantConnector{Default: tenantA},
		Authenticate: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				note("authn")
				next.ServeHTTP(w, r)
			})
		},
	})
	snap := NewSnapshot()
	Mount(router, reg, snap, authz)
	return fixture{router: router, snap: snap, rec: sr, hits: &hits}
}

func (f fixture) do(method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Error struct{ Code string } }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body.Error.Code
}

func TestMount_StatusGuard(t *testing.T) {
	cases := []struct {
		status     Status
		code       int
		errCode    string
		retryAfter string
	}{
		{StatusEnabled, 200, "", ""},
		{StatusInstalled, 403, "module_disabled", ""},
		{StatusDisabled, 403, "module_disabled", ""},
		{StatusAvailable, 404, "module_not_found", ""},
		{StatusUninstalled, 404, "module_not_found", ""},
		{StatusMissing, 404, "module_not_found", ""},
		{StatusFailed, 503, "module_unavailable", ""},
		{StatusIncompatible, 503, "module_unavailable", ""},
		{StatusUpgradePending, 503, "module_unavailable", "30"},
	}
	for _, tc := range cases {
		for _, target := range []string{"/api/notes/items", "/api/notes/status"} {
			t.Run(string(tc.status)+target, func(t *testing.T) {
				f := newFixture(t, allowAll, nil)
				f.snap.Set(tenantA, map[string]Status{"acme.notes": tc.status}, 1)
				rec := f.do("GET", target)
				if rec.Code != tc.code {
					t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.code, rec.Body.String())
				}
				if tc.errCode != "" && errCode(t, rec) != tc.errCode {
					t.Errorf("code = %q, want %q", errCode(t, rec), tc.errCode)
				}
				if got := rec.Header().Get("Retry-After"); got != tc.retryAfter {
					t.Errorf("Retry-After = %q, want %q", got, tc.retryAfter)
				}
				if tc.code != 200 && *f.hits != 0 {
					t.Error("handler tidak boleh dipanggil")
				}
			})
		}
	}
}

func TestMount_PermissionDeny(t *testing.T) {
	f := newFixture(t, denyAll, nil)
	f.snap.Set(tenantA, map[string]Status{"acme.notes": StatusEnabled}, 1)
	rec := f.do("GET", "/api/notes/items")
	if rec.Code != 403 || errCode(t, rec) != "permission_denied" {
		t.Fatalf("protected: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("GET", "/api/notes/status"); rec.Code != 200 {
		t.Fatalf("public: %d", rec.Code)
	}
}

func TestMount_MiddlewareOrder(t *testing.T) {
	var order []string
	f := newFixture(t, allowAll, &order)
	f.snap.Set(tenantA, map[string]Status{"acme.notes": StatusEnabled}, 1)
	if rec := f.do("GET", "/api/notes/items"); rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := strings.Join(order, ","); got != "authn,tenant" {
		t.Errorf("urutan = %q", got)
	}
	found := false
	for _, s := range f.rec.Ended() {
		for _, a := range s.Attributes() {
			if string(a.Key) == "erp.module.id" && a.Value.AsString() == "acme.notes" {
				found = true
			}
		}
	}
	if !found {
		t.Error("atribut span erp.module.id tidak ditemukan")
	}
}

func TestMount_TrailingSlashAndWrongMethod(t *testing.T) {
	f := newFixture(t, allowAll, nil)
	f.snap.Set(tenantA, map[string]Status{"acme.notes": StatusEnabled}, 1)
	rec := f.do("GET", "/api/notes/items/")
	if rec.Code != 404 || errCode(t, rec) != "not_found" || *f.hits != 0 {
		t.Fatalf("trailing slash: %d %s hits=%d", rec.Code, rec.Body.String(), *f.hits)
	}
	rec = f.do("POST", "/api/notes/items")
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("wrong method: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestSnapshot(t *testing.T) {
	s := NewSnapshot()
	if s.Status(tenantA, registry.PlatformKey) != StatusEnabled {
		t.Error("platform.core selalu enabled")
	}
	if s.Status(tenantA, "x.y") != StatusAvailable {
		t.Error("modul tak dikenal = available")
	}
	s.Set(tenantA, map[string]Status{"x.y": StatusDisabled}, 7)
	if s.Status(tenantA, "x.y") != StatusDisabled || s.Revision(tenantA) != 7 {
		t.Error("Set tidak tercermin")
	}
}
