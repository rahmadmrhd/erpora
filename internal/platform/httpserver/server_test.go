package httpserver

import (
	"bytes"
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
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"erpora/internal/platform/bootstrap"
	"erpora/internal/platform/database"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/data"
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
		Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
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

func newBootstrapRouter(t *testing.T) *Router {
	t.Helper()
	return NewRouter(Deps{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:  tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector: database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Bootstrap: func(_ context.Context, tn tenancy.TenantID) bootstrap.Manifest {
			return bootstrap.Build("9.9.9", tn, []bootstrap.ModuleInput{
				{Key: "acme.example", Slug: "example", Version: "1.0.0", Status: "enabled", OfflineMode: "online-only"},
			}, 5)
		},
	})
}

func TestBootstrap_FromDepsHook(t *testing.T) {
	r := newBootstrapRouter(t)
	rec := do(r, "GET", "/api/bootstrap", nil)
	if rec.Code != 200 || rec.Header().Get("ETag") != `"5"` {
		t.Fatalf("%d etag=%q", rec.Code, rec.Header().Get("ETag"))
	}
	d := decode(t, rec)["data"].(map[string]any)
	mods := d["modules"].([]any)
	if len(mods) != 1 || mods[0].(map[string]any)["status"] != "enabled" || d["revision"] != float64(5) {
		t.Fatalf("data %v", d)
	}
	if rec := do(r, "GET", "/api/bootstrap", map[string]string{"If-None-Match": `"5"`}); rec.Code != 304 {
		t.Fatalf("If-None-Match: %d", rec.Code)
	}
	if rec := do(r, "GET", "/api/bootstrap", map[string]string{"If-None-Match": `"4"`}); rec.Code != 200 {
		t.Fatalf("etag lama: %d", rec.Code)
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

func newLoggedRouter(t *testing.T) (*Router, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(tracetest.NewSpanRecorder()))
	return NewRouter(Deps{
		Logger:     observability.NewLogger(&buf, "text", slog.LevelDebug),
		Tracer:     tp,
		Resolver:   tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Readiness:  fakeReady{nil},
		Assets:     fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}},
		AppVersion: "9.9.9",
	}), &buf
}

func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=x dbname=x"}),
		&gorm.Config{DisableAutomaticPing: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRequestID_InLogsEmittedByHandlers(t *testing.T) {
	r, buf := newLoggedRouter(t)
	r.Handle("GET /api/test/log", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.deps.Logger.InfoContext(req.Context(), "dari handler")
		WriteData(w, 200, "ok")
	}))
	rec := do(r, "GET", "/api/test/log", map[string]string{"X-Request-Id": "rid-777"})
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	var found bool
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "dari handler") {
			found = true
			if !strings.Contains(line, "request_id=rid-777") {
				t.Fatalf("log tanpa request_id: %s", line)
			}
		}
	}
	if !found {
		t.Fatalf("log handler tidak ditemukan: %s", buf.String())
	}
}

func TestAccessLog_PanicRecordedAs500(t *testing.T) {
	r, buf := newLoggedRouter(t)
	r.Handle("GET /test/panic2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("x") }))
	do(r, "GET", "/test/panic2", nil)
	var ok bool
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "http request") && strings.Contains(line, "/test/panic2") {
			ok = true
			if !strings.Contains(line, "status=500") {
				t.Fatalf("access log: %s", line)
			}
		}
	}
	if !ok {
		t.Fatalf("access log tidak ditemukan: %s", buf.String())
	}
}

func TestServer_Timeouts(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	s := New("127.0.0.1:0", r)
	if s.ReadHeaderTimeout != 10*time.Second || s.IdleTimeout != 120*time.Second {
		t.Fatalf("timeouts: header=%v idle=%v", s.ReadHeaderTimeout, s.IdleTimeout)
	}
}

// Respons yang dibuat ServeMux sendiri (redirect path non-kanonik) tetap membawa header keamanan + request id.
func TestMuxGeneratedResponses_HaveSecurityHeaders(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	for _, p := range []string{"/a/./b", "//x", "/api/../healthz"} {
		rec := do(r, "GET", p, nil)
		if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusTemporaryRedirect && rec.Code != http.StatusPermanentRedirect {
			t.Logf("%s: code %d (bukan redirect, tetap diperiksa headernya)", p, rec.Code)
		}
		for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "X-Request-Id"} {
			if rec.Header().Get(h) == "" {
				t.Errorf("%s: header %s kosong (code %d)", p, h, rec.Code)
			}
		}
	}
}

func TestDataDB_BoundToRequestContext(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	r.deps.Connector = database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}}
	type ctxKey struct{}
	var stmtCtx, reqCtx context.Context
	r.Handle("GET /api/test/dbctx", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqCtx = req.Context()
		db, err := data.DB(req.Context())
		if err != nil {
			t.Error(err)
			WriteData(w, 500, nil)
			return
		}
		stmtCtx = db.Statement.Context
		WriteData(w, 200, "ok")
	}))
	do(r, "GET", "/api/test/dbctx", nil)
	if reqCtx == nil || stmtCtx != reqCtx {
		t.Fatalf("Statement.Context bukan context request")
	}
}

// NV1: traceparent dari klien tidak boleh menjadi parent span server (endpoint publik):
// trace id baru, konteks klien hanya sebagai link; baggage inbound dibuang.
func TestPublicEndpoint_ClientTraceparentIsLinkOnly(t *testing.T) {
	r, sr := newTestRouter(t, nil)
	var bag int
	r.Handle("GET /api/test/bag", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		bag = baggage.FromContext(req.Context()).Len()
		WriteData(w, 200, "ok")
	}))
	const clientTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
	const clientSpan = "00f067aa0ba902b7"
	hdr := map[string]string{
		"traceparent": "00-" + clientTrace + "-" + clientSpan + "-01",
		"baggage":     "userId=123,secret=abc",
	}

	rec := do(r, "GET", "/api/unknown", hdr)
	tid, _ := decode(t, rec)["error"].(map[string]any)["traceId"].(string)
	if tid == "" || tid == clientTrace {
		t.Fatalf("traceId envelope = %q (harus trace id server)", tid)
	}
	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	s := spans[0]
	if s.SpanContext().TraceID().String() == clientTrace || s.SpanContext().TraceID().String() != tid {
		t.Fatalf("span trace id = %s, envelope = %s", s.SpanContext().TraceID(), tid)
	}
	if s.Parent().IsValid() {
		t.Fatalf("span punya parent dari klien: %v", s.Parent())
	}
	links := s.Links()
	if len(links) != 1 || links[0].SpanContext.TraceID().String() != clientTrace ||
		links[0].SpanContext.SpanID() != mustSpanID(t, clientSpan) {
		t.Fatalf("links = %+v", links)
	}

	do(r, "GET", "/api/test/bag", hdr)
	if bag != 0 {
		t.Fatalf("baggage inbound terbawa ke context: %d anggota", bag)
	}
}

func mustSpanID(t *testing.T, s string) trace.SpanID {
	t.Helper()
	id, err := trace.SpanIDFromHex(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Finding 3: panic ditangani di dalam span aktif: log membawa trace id, span ditandai Error.
func TestPanic_RecordedOnActiveSpanWithTraceIDsInLog(t *testing.T) {
	var buf bytes.Buffer
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	r := NewRouter(Deps{
		Logger:    observability.NewLogger(&buf, "text", slog.LevelDebug),
		Tracer:    tp,
		Resolver:  tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector: database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:    fstest.MapFS{"index.html": {Data: []byte("x")}},
	})
	r.Handle("GET /test/panic3", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") }))
	rec := do(r, "GET", "/test/panic3", nil)
	if rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	s := spans[0]
	if s.Status().Code != codes.Error {
		t.Fatalf("status span = %v, want Error", s.Status())
	}
	var hasException bool
	for _, ev := range s.Events() {
		if ev.Name == "exception" {
			hasException = true
		}
	}
	if !hasException {
		t.Fatalf("span tanpa event exception: %+v", s.Events())
	}
	tid := s.SpanContext().TraceID().String()
	var logged bool
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "panic dalam handler") {
			logged = true
			if !strings.Contains(line, "trace_id="+tid) {
				t.Fatalf("log panic tanpa trace_id server: %s", line)
			}
		}
	}
	if !logged {
		t.Fatalf("log panic tidak ada: %s", buf.String())
	}
	e := decode(t, rec)["error"].(map[string]any)
	if e["traceId"] != tid {
		t.Fatalf("envelope traceId = %v, want %s", e["traceId"], tid)
	}
}

// Finding 4: panic SETELAH response mulai ditulis harus memutus koneksi, bukan selesai bersih.
func TestPanic_AfterResponseStarted_AbortsConnection(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	r.Handle("GET /test/partial", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		_ = http.NewResponseController(w).Flush()
		panic("setelah menulis")
	}))
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/test/partial")
	if err != nil {
		return // koneksi putus sebelum header terbaca: juga perilaku abort yang benar
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Fatalf("klien melihat respons 200 bersih (%q); seharusnya koneksi diputus", body)
	}
}

// Finding 8: method salah pada path yang dikenal -> 405 + Allow; path tidak dikenal tetap 404.
func TestWrongMethodOnKnownAPIPath_405WithAllow(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	r.Handle("GET /api/test/items/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { WriteData(w, 200, "ok") }))
	r.Handle("POST /api/test/items/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { WriteData(w, 200, "ok") }))

	rec := do(r, "POST", "/api/bootstrap", nil)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST /api/bootstrap: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	if decode(t, rec)["error"].(map[string]any)["code"] != "method_not_allowed" {
		t.Fatalf("%s", rec.Body)
	}

	rec = do(r, "DELETE", "/api/test/items/42", nil)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD, POST" {
		t.Fatalf("DELETE items: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}

	rec = do(r, "POST", "/api/unknown", nil)
	if rec.Code != 404 || rec.Header().Get("Allow") != "" {
		t.Fatalf("unknown path: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	if decode(t, rec)["error"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("%s", rec.Body)
	}
}

// Nilai panic (bisa memuat rahasia) tidak boleh masuk log; hanya tipe + stack.
func TestPanic_ValueNotLogged(t *testing.T) {
	var buf bytes.Buffer
	r := NewRouter(Deps{
		Logger:    observability.NewLogger(&buf, "text", slog.LevelDebug),
		Resolver:  tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector: database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:    fstest.MapFS{"index.html": {Data: []byte("x")}},
	})
	r.Handle("GET /test/panic-secret", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("password=SuperRahasia123")
	}))
	if rec := do(r, "GET", "/test/panic-secret", nil); rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
	out := buf.String()
	if strings.Contains(out, "SuperRahasia123") {
		t.Fatalf("nilai panic bocor ke log: %s", out)
	}
	if !strings.Contains(out, "panic_type=string") || !strings.Contains(out, "stack=") {
		t.Fatalf("log panic tanpa tipe/stack: %s", out)
	}
}

func TestServer_ErrorLogRoutedToSlog(t *testing.T) {
	var buf bytes.Buffer
	r := NewRouter(Deps{
		Logger:    observability.NewLogger(&buf, "text", slog.LevelDebug),
		Resolver:  tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector: database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:    fstest.MapFS{"index.html": {Data: []byte("x")}},
	})
	s := New("127.0.0.1:0", r)
	if s.ErrorLog == nil {
		t.Fatal("ErrorLog harus diset ke slog")
	}
	s.ErrorLog.Print("x-error-internal")
	out := buf.String()
	if !strings.Contains(out, "x-error-internal") || !strings.Contains(out, "level=WARN") {
		t.Fatalf("ErrorLog harus menulis WARN ke slog: %q", out)
	}
}

type failResolver struct{}

func (failResolver) Resolve(*http.Request) (tenancy.TenantID, error) {
	return tenancy.TenantID{}, errors.New("tidak bisa menentukan tenant")
}

func TestTenantContext_ResolverErrorIs400(t *testing.T) {
	r := NewRouter(Deps{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:  failResolver{},
		Connector: database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:    fstest.MapFS{"index.html": {Data: []byte("x")}},
	})
	r.Handle("GET /test/tenant", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	rec := do(r, "GET", "/test/tenant", nil)
	if rec.Code != 400 {
		t.Fatalf("code %d: %s", rec.Code, rec.Body)
	}
	e := decode(t, rec)["error"].(map[string]any)
	if e["code"] != "tenant_unresolved" {
		t.Fatalf("error = %v", e)
	}
}

func TestConnectorProvider_NonDefaultTenantErrors(t *testing.T) {
	p := database.DataProvider(database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}})
	other := tenancy.TenantID(uuid.New())
	if _, err := p.DB(tenancy.WithTenant(context.Background(), other)); !errors.Is(err, database.ErrUnknownTenant) {
		t.Fatalf("err = %v, want ErrUnknownTenant", err)
	}
	if _, err := p.DB(context.Background()); !errors.Is(err, data.ErrNoTenantContext) {
		t.Fatalf("tanpa tenant: err = %v", err)
	}
}

// FR-07 guardrail 5: requestScope memasang counter N+1 hanya di development.
func TestNPlusOne_RequestScopeWarnsOnlyInDev(t *testing.T) {
	for _, tc := range []struct {
		dev  bool
		want int
	}{{true, 1}, {false, 0}} {
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		gl := database.NewGormLogger(log, 0)
		r := NewRouter(Deps{
			Logger:    log,
			Resolver:  tenancy.SingleTenantResolver{ID: defaultTenant},
			Connector: database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
			Dev:       tc.dev, NPlusOneThreshold: 2,
		})
		r.Handle("GET /api/test/n1", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			for range 3 {
				gl.Trace(req.Context(), time.Now(), func() (string, int64) { return "SELECT 1 WHERE id = $1", 1 }, nil)
			}
			WriteData(w, 200, "ok")
		}))
		if rec := do(r, "GET", "/api/test/n1", nil); rec.Code != 200 {
			t.Fatalf("code %d", rec.Code)
		}
		if n := strings.Count(buf.String(), "indikasi N+1"); n != tc.want {
			t.Errorf("dev=%v: warnings = %d, want %d: %s", tc.dev, n, tc.want, buf.String())
		}
	}
}
