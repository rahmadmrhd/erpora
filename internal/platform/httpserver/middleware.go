package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/rahmadmrhd/erp-go/internal/platform/database"
	"github.com/rahmadmrhd/erp-go/internal/platform/tenancy"
	"github.com/rahmadmrhd/erp-go/sdk/data"
)

const (
	headerRequestID = "X-Request-Id"
	csp             = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// requestMeta dibagi antara middleware luar (recover) dan dalam (requestID) agar error panic
// tetap membawa traceId.
type requestMeta struct{ traceID string }

type metaKey struct{}

func metaFrom(ctx context.Context) *requestMeta {
	m, _ := ctx.Value(metaKey{}).(*requestMeta)
	return m
}

// recoverMW mengubah panic menjadi 500 internal_error tanpa membocorkan pesan panic.
func recoverMW(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), metaKey{}, &requestMeta{}))
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			log.ErrorContext(r.Context(), "panic dalam handler",
				"panic", rec, "stack", string(debug.Stack()),
				"request_id", w.Header().Get(headerRequestID))
			if !sw.wrote {
				WriteError(w, r, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server", nil)
			}
		}()
		next.ServeHTTP(sw, r)
	})
}

// requestScope menetapkan X-Request-Id, security headers, dan access log.
func requestScope(log *slog.Logger, pattern string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if !requestIDPattern.MatchString(id) {
			if u, err := uuid.NewV7(); err == nil {
				id = u.String()
			} else {
				id = uuid.NewString()
			}
		}
		h := w.Header()
		h.Set(headerRequestID, id)
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		if m := metaFrom(r.Context()); m != nil {
			if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
				m.traceID = sc.TraceID().String()
			}
		}

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			log.InfoContext(r.Context(), "http request",
				"method", r.Method, "route", pattern, "path", r.URL.Path,
				"status", status, "duration_ms", time.Since(start).Milliseconds(),
				"request_id", id)
		}()
		next.ServeHTTP(sw, r)
	})
}

// tenantContext menentukan tenant lewat Resolver dan memasang tenant + data.Provider ke context.
func tenantContext(res tenancy.Resolver, conn database.TenantConnector, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := res.Resolve(r)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "tenant_unresolved", "tenant tidak dapat ditentukan", nil)
			return
		}
		ctx := tenancy.WithTenant(r.Context(), id)
		ctx = data.WithProvider(ctx, connectorProvider{conn: conn})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// connectorProvider mengadaptasi TenantConnector menjadi data.Provider berdasarkan tenant di ctx.
type connectorProvider struct{ conn database.TenantConnector }

func (p connectorProvider) DB(ctx context.Context) (*database.DB, error) {
	id, ok := tenancy.FromContext(ctx)
	if !ok {
		return nil, data.ErrNoTenantContext
	}
	tdb, err := p.conn.For(ctx, id)
	if err != nil {
		return nil, err
	}
	return tdb.DB, nil
}

// statusWriter mencatat status dan apakah header sudah ditulis.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap memungkinkan http.ResponseController mengakses writer asli (Flush, dst).
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
