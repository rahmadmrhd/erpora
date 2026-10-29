package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/database"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/data"
	sdki18n "erpora/sdk/i18n"
)

const (
	headerRequestID = "X-Request-Id"
	csp             = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// recoverMW berjalan DI DALAM span otelhttp: panic dicatat pada span aktif (event exception +
// status Error) dan log membawa trace_id/span_id. Bila respons belum ditulis, kirim 500 internal_error
// (tanpa pesan panic). Bila respons SUDAH mulai ditulis, koneksi diputus dengan http.ErrAbortHandler
// agar klien tidak mengira respons yang terpotong itu utuh.
func recoverMW(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			span := trace.SpanFromContext(r.Context())
			// Hanya tipe panic yang dicatat di span (nilai panic bisa memuat data).
			span.RecordError(fmt.Errorf("panic dalam handler: %T", rec))
			span.SetStatus(codes.Error, "panic")
			// Nilai panic tidak dicatat (bisa memuat rahasia): hanya tipe + stack, sama seperti span.
			log.ErrorContext(r.Context(), "panic dalam handler",
				"panic_type", fmt.Sprintf("%T", rec), "stack", string(debug.Stack()))
			if sw.wrote {
				panic(http.ErrAbortHandler)
			}
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server", nil)
		}()
		next.ServeHTTP(sw, r)
	})
}

// requestScope menetapkan X-Request-Id, security headers, dan access log.
func requestScope(log *slog.Logger, pattern string, nPlusOne int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		meta := &requestMeta{}
		r = r.WithContext(database.WithQueryCounter(context.WithValue(r.Context(), requestMetaKey{}, meta), nPlusOne))
		completed := false // tetap false bila handler panic (recoverMW menulis 500 sesudahnya)
		defer func() {
			status := sw.status
			switch {
			case !completed:
				status = http.StatusInternalServerError
			case status == 0:
				status = http.StatusOK
			}
			attrs := []any{
				"method", r.Method, "route", pattern, "path", r.URL.Path,
				"status", status, "duration_ms", time.Since(start).Milliseconds(),
			}
			if meta.module != "" {
				attrs = append(attrs, "erp.module.key", meta.module)
			}
			log.InfoContext(r.Context(), "http request", attrs...)
		}()
		next.ServeHTTP(sw, r)
		completed = true
	})
}

// requestMeta menampung atribut yang diisi handler di dalam rantai dan dibaca access log.
type requestMeta struct{ module string }

type requestMetaKey struct{}

// SetModule menandai key modul pemilik request agar access log memuat atribut module.
// Tanpa requestScope di context, pemanggilan ini tidak berpengaruh.
func SetModule(ctx context.Context, key string) {
	if m, ok := ctx.Value(requestMetaKey{}).(*requestMeta); ok {
		m.module = key
	}
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
		ctx = data.WithProvider(ctx, database.DataProvider(conn))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

// requestIDFor memakai X-Request-Id masuk bila valid, selain itu membuat UUIDv7.
func requestIDFor(r *http.Request) string {
	if id := r.Header.Get(headerRequestID); requestIDPattern.MatchString(id) {
		return id
	}
	if u, err := uuid.NewV7(); err == nil {
		return u.String()
	}
	return uuid.NewString()
}

// setBaseHeaders memasang X-Request-Id dan header keamanan pada setiap respons.
func setBaseHeaders(h http.Header, id string) {
	h.Set(headerRequestID, id)
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
}

// localeContext menegosiasikan bahasa dari Accept-Language terhadap registri bahasa (hasil selalu anggota
// registri; Requirement 3.4) dan memasang locale serta Resolver terjemahan (terikat tenant) ke context.
// Berjalan SETELAH tenantContext. Sebelum PRD-001 tidak ada preferensi pengguna; setelah itu urutannya
// preferensi pengguna, Accept-Language, default (Requirement 3.1) cukup mengubah titik ini.
func localeContext(svc *i18n.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc, ok := sdki18n.LocaleFrom(r.Context())
		if !ok {
			loc = sdki18n.Locale(svc.Registry().Negotiate(r.Header.Get("Accept-Language")))
		}
		t, _ := tenancy.FromContext(r.Context())
		ctx := sdki18n.WithLocale(r.Context(), loc)
		ctx = sdki18n.WithResolver(ctx, svc.Resolver(t))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// localeEarly memasang locale hasil negosiasi dan Resolver HANYA-BAWAAN paling awal di rantai (di luar recover,
// requestScope, Authenticate, dan tenantContext) supaya galat yang ditulis di sana (500 internal_error,
// tenant_unresolved, penolakan Authenticate) ikut terlokalisasi (Requirement 4.4). Hanya butuh Accept-Language dan
// registri; tidak membaca override karena tenant belum dikenal. localeContext kemudian mengganti Resolver dengan
// yang terikat tenant.
func localeEarly(svc *i18n.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := sdki18n.Locale(svc.Registry().Negotiate(r.Header.Get("Accept-Language")))
		ctx := sdki18n.WithLocale(r.Context(), loc)
		ctx = sdki18n.WithResolver(ctx, svc.BuiltinResolver())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
