// Package httpserver merakit server HTTP platform: router, rantai middleware, dan endpoint dasar.
package httpserver

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/bootstrap"
	"erpora/internal/platform/database"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
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
	// Bootstrap membentuk isi GET /api/bootstrap dari runtime snapshot; nil = bootstrap.Build(AppVersion, t, nil, 0).
	Bootstrap func(ctx context.Context, t tenancy.TenantID) bootstrap.Manifest
	// Dev true = mode development: SPA menampilkan halaman "frontend belum di-build" bila aset belum ada (FR-12).
	Dev bool
	// NPlusOneThreshold > 0 mengaktifkan deteksi N+1 per request, hanya bila Dev (FR-07 guardrail 5).
	NPlusOneThreshold int
	// I18n (opsional) menegosiasikan bahasa dari Accept-Language dan memasang locale serta Resolver terjemahan
	// ke context setiap request (PRD-000 FR-25); nil = tanpa i18n.
	I18n *i18n.Service
	// Authenticate membungkus route non-Public dan berjalan SETELAH tenantContext (ADR-0013 §4: klaim tid dicocokkan
	// dengan tenant context); nil = pass-through (dipakai tes router tanpa autentikasi).
	Authenticate func(http.Handler) http.Handler
	// IPLimit membungkus route ber-RateLimited() (endpoint auth/token): batas laju per IP klien, setelah tenantContext
	// dan SEBELUM Authenticate; nil = tanpa batas.
	IPLimit func(http.Handler) http.Handler
	// TrustedProxies (ERP_TRUSTED_PROXIES) adalah proxy yang boleh dipercaya melaporkan IP klien lewat X-Forwarded-For
	// (lihat resolveClientIP). Kosong (bawaan) = IP klien selalu alamat peer TCP.
	TrustedProxies []netip.Prefix
	// PrincipalLimit membungkus route ber-RateLimited() yang non-Public: batas laju per principal, SETELAH Authenticate;
	// nil = tanpa batas.
	PrincipalLimit func(http.Handler) http.Handler
}

type routeConfig struct{ noTenant, public, rateLimited bool }

// RouteOption mengubah perilaku rantai middleware sebuah route.
type RouteOption func(*routeConfig)

// WithoutTenant melewati tenantContext (untuk /healthz dan /readyz).
func WithoutTenant() RouteOption { return func(c *routeConfig) { c.noTenant = true } }

// Public melewati Authenticate (endpoint tanpa autentikasi). Tidak memengaruhi tenantContext.
func Public() RouteOption { return func(c *routeConfig) { c.public = true } }

// RateLimited menandai endpoint auth/token (login, forgot, reset, refresh, change, mfa/*; ADR-0013 §4): Deps.IPLimit
// berlaku untuk route ini (Public maupun tidak) dan Deps.PrincipalLimit bila route non-Public. Route lain tidak
// dibatasi di sini: batas per IP untuk seluruh API akan menghambat aplikasi biasa.
func RateLimited() RouteOption { return func(c *routeConfig) { c.rateLimited = true } }

// Router adalah http.Handler berbasis ServeMux dengan rantai middleware per route.
type Router struct {
	mux  *http.ServeMux
	deps Deps
	// probe memuat semua pattern ber-method yang terdaftar (handler kosong) untuk menentukan
	// method apa yang diizinkan pada sebuah path (405 + Allow), termasuk route modul di masa depan.
	probe *http.ServeMux
	// apiGate (opsional) menentukan path /api/... yang permukaan HTTP-nya disembunyikan (modul tidak enabled).
	// Diset sekali saat perakitan (SetAPIGate), sebelum server melayani request.
	apiGate func(*http.Request) bool
}

// SetAPIGate memasang gate untuk catch-all /api/: bila gate mengembalikan true, path dijawab seragam
// 404 module_not_found tanpa header Allow (FR-19.7: modul tidak aktif tidak punya permukaan HTTP).
// Gate berjalan di dalam tenantContext, jadi tenant tersedia di context request.
func (r *Router) SetAPIGate(g func(*http.Request) bool) { r.apiGate = g }

// ServeHTTP mengimplementasikan http.Handler.
// ServeHTTP menetapkan request id + header keamanan SEBELUM ServeMux, sehingga respons buatan mux
// sendiri (mis. redirect path non-kanonik) pun membawanya. Baggage inbound dibuang (tidak dipercaya).
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.Header.Del("Baggage")
	id := requestIDFor(req)
	setBaseHeaders(w.Header(), id)
	ctx := observability.WithRequestID(req.Context(), id)
	// IP klien = peer TCP, atau hop X-Forwarded-For pertama yang bukan proxy tepercaya bila peer termasuk
	// ERP_TRUSTED_PROXIES (resolveClientIP). Batas laju IP dan lockout bergantung pada ini (docs/operations/deployment.md).
	ctx = httpx.WithClientIP(ctx, resolveClientIP(req, r.deps.TrustedProxies))
	r.mux.ServeHTTP(w, req.WithContext(ctx))
}

// Handle mendaftarkan pattern dengan rantai (luar ke dalam; ADR-0013 §4):
// otelhttp -> localeEarly -> recover -> requestScope -> tenantContext -> localeContext -> IPLimit (hanya RateLimited)
// -> Authenticate (kecuali Public) -> PrincipalLimit (RateLimited non-Public) -> h.
// recover berada di dalam span otelhttp agar panic tercatat pada span aktif (lihat recoverMW).
func (r *Router) Handle(pattern string, h http.Handler, opts ...RouteOption) {
	var cfg routeConfig
	for _, o := range opts {
		o(&cfg)
	}
	if !cfg.public && cfg.rateLimited && r.deps.PrincipalLimit != nil {
		h = r.deps.PrincipalLimit(h)
	}
	if !cfg.public && r.deps.Authenticate != nil {
		h = r.deps.Authenticate(h)
	}
	if cfg.rateLimited && r.deps.IPLimit != nil {
		h = r.deps.IPLimit(h)
	}
	if r.deps.I18n != nil {
		h = localeContext(r.deps.I18n, h)
	}
	if !cfg.noTenant {
		h = tenantContext(r.deps.Resolver, r.deps.Connector, h)
	}
	h = requestScope(r.deps.Logger, pattern, r.nPlusOneThreshold(), h)
	h = recoverMW(r.deps.Logger, h)
	if r.deps.I18n != nil {
		h = localeEarly(r.deps.I18n, h)
	}
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

func (r *Router) nPlusOneThreshold() int {
	if r.deps.Dev {
		return r.deps.NPlusOneThreshold
	}
	return 0
}
