package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rahmadmrhd/erp-go/internal/platform/database"
	"github.com/rahmadmrhd/erp-go/internal/platform/tenancy"
	"github.com/rahmadmrhd/erp-go/sdk/data"
)

type fakeReady struct{ err error }

func (f fakeReady) Ready(context.Context) error { return f.err }

var defaultTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

func newTestRouter(t *testing.T, ready error) (*Router, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	return NewRouter(Deps{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tracer:     tp,
		Resolver:   tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{}},
		Readiness:  fakeReady{ready},
		Assets:     fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}},
		AppVersion: "9.9.9",
	}), sr
}

func do(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return m
}

func TestHealthz_OK(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	rec := do(r, "GET", "/healthz", nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"data":{"status":"ok"}}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestReadyz_NotReady(t *testing.T) {
	r, _ := newTestRouter(t, errors.New("pending migrations: secret detail"))
	rec := do(r, "GET", "/readyz", nil)
	if rec.Code != 503 {
		t.Fatalf("code %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret detail") {
		t.Fatal("internal detail leaked")
	}
	if decode(t, rec)["error"].(map[string]any)["code"] != "not_ready" {
		t.Fatalf("%s", rec.Body)
	}
}

func TestReadyz_Ready(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	if rec := do(r, "GET", "/readyz", nil); rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestBootstrap_Envelope(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	rec := do(r, "GET", "/api/bootstrap", nil)
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	d := decode(t, rec)["data"].(map[string]any)
	if d["appVersion"] != "9.9.9" || d["tenantId"] != defaultTenant.String() || d["revision"] != float64(0) {
		t.Fatalf("data %v", d)
	}
	for _, k := range []string{"modules", "permissions"} {
		a, ok := d[k].([]any)
		if !ok || len(a) != 0 {
			t.Fatalf("%s = %#v", k, d[k])
		}
	}
	if rec.Header().Get("ETag") != `"0"` {
		t.Fatalf("etag %q", rec.Header().Get("ETag"))
	}
}

func TestBootstrap_NotModified(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	rec := do(r, "GET", "/api/bootstrap", map[string]string{"If-None-Match": `"0"`})
	if rec.Code != 304 || rec.Body.Len() != 0 {
		t.Fatalf("%d %q", rec.Code, rec.Body)
	}
}

func TestUnknownAPI_JSON404(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	rec := do(r, "GET", "/api/unknown", nil)
	if rec.Code != 404 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if decode(t, rec)["error"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("%s", rec.Body)
	}
}

func TestRequestID_GeneratedAndPropagated(t *testing.T) {
	r, _ := newTestRouter(t, nil)

	rec := do(r, "GET", "/api/unknown", nil)
	id := rec.Header().Get("X-Request-Id")
	u, err := uuid.Parse(id)
	if err != nil || u.Version() != 7 {
		t.Fatalf("generated id %q", id)
	}
	if decode(t, rec)["error"].(map[string]any)["requestId"] != id {
		t.Fatalf("error.requestId mismatch: %s", rec.Body)
	}

	rec = do(r, "GET", "/healthz", map[string]string{"X-Request-Id": "abc-123"})
	if rec.Header().Get("X-Request-Id") != "abc-123" {
		t.Fatalf("got %q", rec.Header().Get("X-Request-Id"))
	}

	rec = do(r, "GET", "/healthz", map[string]string{"X-Request-Id": "bad id with spaces"})
	if got := rec.Header().Get("X-Request-Id"); got == "bad id with spaces" || !regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`).MatchString(got) {
		t.Fatalf("got %q", got)
	}
	rec = do(r, "GET", "/healthz", map[string]string{"X-Request-Id": strings.Repeat("a", 129)})
	if rec.Header().Get("X-Request-Id") == strings.Repeat("a", 129) {
		t.Fatal("too-long id accepted")
	}
}

func TestSecurityHeaders(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
	}
	for _, p := range []string{"/healthz", "/api/unknown", "/", "/api/bootstrap"} {
		rec := do(r, "GET", p, nil)
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("%s %s = %q", p, k, got)
			}
		}
	}
}

func TestPanic_Returns500Envelope(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	r.Handle("GET /test/panic", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom-secret") }))
	rec := do(r, "GET", "/test/panic", nil)
	if rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom-secret") {
		t.Fatal("panic message leaked")
	}
	e := decode(t, rec)["error"].(map[string]any)
	if e["code"] != "internal_error" || e["requestId"] != rec.Header().Get("X-Request-Id") || e["requestId"] == "" {
		t.Fatalf("%v", e)
	}
}

func TestSpanName_UsesRoutePattern(t *testing.T) {
	r, sr := newTestRouter(t, nil)
	r.Handle("GET /api/test/items/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { WriteData(w, 200, "ok") }))
	do(r, "GET", "/api/test/items/42", nil)
	var names []string
	for _, s := range sr.Ended() {
		names = append(names, s.Name())
	}
	if len(names) != 1 || names[0] != "GET /api/test/items/{id}" {
		t.Fatalf("spans %v", names)
	}
}

func TestTenantContext_InjectedForAPI(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	var gotTenant tenancy.TenantID
	var okTenant bool
	var dbErr error
	r.Handle("GET /api/test/tenant", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotTenant, okTenant = tenancy.FromContext(req.Context())
		_, dbErr = data.DB(req.Context())
		WriteData(w, 200, "ok")
	}))
	do(r, "GET", "/api/test/tenant", nil)
	if !okTenant || gotTenant != defaultTenant || dbErr != nil {
		t.Fatalf("tenant=%v ok=%v dbErr=%v", gotTenant, okTenant, dbErr)
	}
}

func TestWithoutTenant_SkipsTenantContext(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	var ok bool
	r.Handle("GET /test/notenant", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, ok = tenancy.FromContext(req.Context())
		WriteData(w, 200, "ok")
	}), WithoutTenant())
	do(r, "GET", "/test/notenant", nil)
	if ok {
		t.Fatal("tenant must not be injected")
	}
}
