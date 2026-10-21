// Package httpserver merakit server HTTP platform: router, rantai middleware, dan endpoint dasar.
package httpserver

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/rahmadmrhd/erp-go/internal/platform/database"
	"github.com/rahmadmrhd/erp-go/internal/platform/tenancy"
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
}

type routeConfig struct{ noTenant bool }

// RouteOption mengubah perilaku rantai middleware sebuah route.
type RouteOption func(*routeConfig)

// WithoutTenant melewati tenantContext (untuk /healthz dan /readyz).
func WithoutTenant() RouteOption { return func(c *routeConfig) { c.noTenant = true } }

// Router adalah http.Handler berbasis ServeMux dengan rantai middleware per route.
type Router struct {
	mux  *http.ServeMux
	deps Deps
}

// ServeHTTP mengimplementasikan http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) { r.mux.ServeHTTP(w, req) }

// Handle mendaftarkan pattern dengan rantai: recover -> otelhttp -> requestScope -> tenantContext -> h.
func (r *Router) Handle(pattern string, h http.Handler, opts ...RouteOption) {
	var cfg routeConfig
	for _, o := range opts {
		o(&cfg)
	}
	if !cfg.noTenant {
		h = tenantContext(r.deps.Resolver, r.deps.Connector, h)
	}
	h = requestScope(r.deps.Logger, pattern, h)
	h = otelhttp.NewHandler(h, pattern, otelhttp.WithTracerProvider(r.deps.Tracer))
	h = recoverMW(r.deps.Logger, h)
	r.mux.Handle(pattern, h)
}

// New membuat http.Server dengan ReadHeaderTimeout 10 detik.
func New(addr string, r *Router) *http.Server {
	return &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second}
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
