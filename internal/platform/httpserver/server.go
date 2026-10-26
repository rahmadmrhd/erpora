// Package httpserver merakit server HTTP platform: router, rantai middleware, dan endpoint dasar.
package httpserver

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/database"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
)

// ReadinessChecker memeriksa kesiapan layanan; nil = siap. Implementasi WAJIB read-only.
type ReadinessChecker interface {
	Ready(ctx context.Context) error
}

// Deps adalah dependensi router.
type Deps struct {
	Logger     *slog.Logger
	Tracer     trace.TracerProvider
	Resolver   tenancy.Resolver
	Connector  database.TenantConnector
	Readiness  ReadinessChecker
	Assets     fs.FS
	AppVersion string
	// Dev true = mode development: SPA menampilkan halaman "frontend belum di-build" bila aset belum ada (FR-12).
	Dev bool
	// Authenticate membungkus route non-Public; nil = pass-through (autentikasi nyata datang dari PRD-001).
	Authenticate func(http.Handler) http.Handler
}

type routeConfig struct{ noTenant, public bool }

// RouteOption mengubah perilaku rantai middleware sebuah route.
type RouteOption func(*routeConfig)

// WithoutTenant melewati tenantContext (untuk /healthz dan /readyz).
func WithoutTenant() RouteOption { return func(c *routeConfig) { c.noTenant = true } }

// Public melewati Authenticate (endpoint tanpa autentikasi). Tidak memengaruhi tenantContext.
func Public() RouteOption { return func(c *routeConfig) { c.public = true } }

// Router adalah http.Handler berbasis ServeMux dengan rantai middleware per route.
type Router struct {
	mux  *http.ServeMux
	deps Deps
	// probe memuat semua pattern ber-method yang terdaftar (handler kosong) untuk menentukan
	// method apa yang diizinkan pada sebuah path (405 + Allow), termasuk route modul di masa depan.
	probe *http.ServeMux
}

// ServeHTTP mengimplementasikan http.Handler.
// ServeHTTP menetapkan request id + header keamanan SEBELUM ServeMux, sehingga respons buatan mux
// sendiri (mis. redirect path non-kanonik) pun membawanya. Baggage inbound dibuang (tidak dipercaya).
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.Header.Del("Baggage")
	id := requestIDFor(req)
	setBaseHeaders(w.Header(), id)
	req = req.WithContext(observability.WithRequestID(req.Context(), id))
	r.mux.ServeHTTP(w, req)
}

// Handle mendaftarkan pattern dengan rantai:
// otelhttp -> recover -> requestScope -> Authenticate (kecuali Public) -> tenantContext -> h.
// recover berada di dalam span otelhttp agar panic tercatat pada span aktif (lihat recoverMW).
func (r *Router) Handle(pattern string, h http.Handler, opts ...RouteOption) {
	var cfg routeConfig
	for _, o := range opts {
		o(&cfg)
	}
	if !cfg.noTenant {
		h = tenantContext(r.deps.Resolver, r.deps.Connector, h)
	}
	if !cfg.public && r.deps.Authenticate != nil {
		h = r.deps.Authenticate(h)
	}
	h = requestScope(r.deps.Logger, pattern, h)
	h = recoverMW(r.deps.Logger, h)
	h = otelhttp.NewHandler(h, pattern,
		otelhttp.WithTracerProvider(r.deps.Tracer),
		// Endpoint publik: traceparent klien tidak menjadi parent (hanya link pada span root baru).
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }),
		// Hanya W3C trace context yang diekstrak; baggage inbound tidak dipercaya.
		otelhttp.WithPropagators(propagation.TraceContext{}),
	)
	r.mux.Handle(pattern, h)
	if hasMethod(pattern) {
		r.probe.Handle(pattern, http.NotFoundHandler())
	}
}

// hasMethod: pattern berbentuk "METHOD /path".
func hasMethod(pattern string) bool {
	m, _, ok := strings.Cut(pattern, " ")
	return ok && m != "" && !strings.Contains(m, "/")
}

var probeMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}

// allowedMethods mengembalikan method (terurut seperti probeMethods) yang punya route terdaftar untuk path request.
func (r *Router) allowedMethods(req *http.Request) []string {
	var out []string
	for _, m := range probeMethods {
		probeReq := &http.Request{Method: m, URL: req.URL, Host: req.Host, Header: http.Header{}}
		if _, pat := r.probe.Handler(probeReq); pat != "" {
			out = append(out, m)
		}
	}
	return out
}

// New membuat http.Server dengan ReadHeaderTimeout 10 detik, IdleTimeout 120 detik, dan ErrorLog ke slog (WARN).
func New(addr string, r *Router) *http.Server {
	return &http.Server{
		Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		// Log internal net/http (mis. error TLS/handshake) lewat slog proses, bukan stderr mentah.
		ErrorLog: slog.NewLogLogger(r.deps.Logger.Handler(), slog.LevelWarn),
	}
}

func withDefaults(d Deps) Deps {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Tracer == nil {
		d.Tracer = noop.NewTracerProvider()
	}
	return d
}
