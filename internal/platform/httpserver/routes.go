package httpserver

import (
	"net/http"
	"net/textproto"
	"strings"

	"erpora/internal/platform/bootstrap"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
)

// BootstrapPath adalah path route bootstrap platform (GET, wajib autentikasi; PRD-001 FR-106). Satu-satunya sumber:
// dipakai pendaftaran route di bawah dan test kesesuaian route ↔ kontrak (internal/app), yang mencocokkannya dengan
// kontrak platform.
const BootstrapPath = "/api/bootstrap"

// authenticatedRoute adalah route platform yang didaftarkan langsung di router (di luar registry modul), bukan Public(),
// dan hanya mewajibkan Principal terautentikasi tanpa permission tertentu (kontrak `x-erp-authenticated`; PRD-001 Q29).
type authenticatedRoute struct {
	pattern string
	handler func(Deps) http.Handler
}

// authenticatedRoutes adalah SATU-SATUNYA tabel route semacam itu: NewRouter mendaftarkan route dari tabel ini, dan
// AuthenticatedRoutes (dipakai tests/security/authenticated_routes_test.go terhadap allowlist authenticated_routes.txt)
// diturunkan darinya, sehingga route baru tidak dapat terdaftar tanpa ikut tampil di tes keamanan. Route Public()
// (healthz, readyz, /api/, SPA) bukan bagian tabel ini.
//
// Bootstrap: sengaja TANPA aturan mfa_required (keputusan pemilik produk 3 Nov 2026, PRD-001 Q29 butir 2, ADR-0013
// section 4, FR-106): UI membutuhkan bootstrap untuk layar enroll/verify MFA, dan isinya hanya data pengguna itu sendiri.
var authenticatedRoutes = []authenticatedRoute{
	{pattern: "GET " + BootstrapPath, handler: bootstrapHandler},
}

// AuthenticatedRoutes mengembalikan pola "METHOD /path" route terautentikasi-tanpa-permission yang didaftarkan platform
// langsung di router (salinan baru pada tiap pemanggilan).
func AuthenticatedRoutes() []string {
	out := make([]string, 0, len(authenticatedRoutes))
	for _, ar := range authenticatedRoutes {
		out = append(out, ar.pattern)
	}
	return out
}

// NewRouter membuat router dengan /healthz, /readyz, /api/bootstrap, /api/ (404 JSON), dan / (SPA).
func NewRouter(d Deps) *Router {
	d = withDefaults(d)
	r := &Router{mux: http.NewServeMux(), probe: http.NewServeMux(), deps: d}

	r.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteData(w, http.StatusOK, map[string]string{"status": "ok"})
	}), WithoutTenant(), Public())

	r.Handle("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if d.Readiness != nil {
			if err := d.Readiness.Ready(req.Context()); err != nil {
				d.Logger.WarnContext(req.Context(), "readiness gagal", "error", err)
				WriteError(w, req, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
				return
			}
		}
		WriteData(w, http.StatusOK, map[string]string{"status": "ready"})
	}), WithoutTenant(), Public())

	// Bukan Public(): hanya dilayani setelah Authenticate (lihat authenticatedRoutes untuk alasan tanpa mfa_required).
	for _, ar := range authenticatedRoutes {
		r.Handle(ar.pattern, ar.handler(d))
	}

	r.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.apiGate != nil && r.apiGate(req) {
			WriteError(w, req, http.StatusNotFound, "module_not_found", "modul tidak ditemukan", nil)
			return
		}
		// Path dikenal dengan method lain -> 405 + Allow; path tidak dikenal -> 404.
		if allowed := r.allowedMethods(req); len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed", "metode tidak diizinkan untuk endpoint ini", nil)
			return
		}
		WriteError(w, req, http.StatusNotFound, "not_found", "endpoint tidak ditemukan", nil)
	}), Public())

	assets := d.Assets
	if assets == nil {
		assets = emptyFS{}
	}
	r.Handle("/", NewSPA(assets, d.Dev), Public())
	return r
}

// bootstrapHandler melayani GET /api/bootstrap untuk Principal request (PRD-001 FR-106). Respons SELALU milik Principal
// itu: identitas hanya dari context (dipasang Authenticate dari token), tidak pernah dari header, query, atau body.
// Karena isinya per pengguna, respons `private` dan `Vary: Authorization` (cache bersama tidak boleh menyajikannya ke
// pengguna lain) dan ETag-nya memuat identitas pengguna (Manifest.ETag).
//
// Gagal tertutup: tanpa Principal 401 (pertahanan berlapis; Authenticate sudah menolaknya), dan manifest tanpa user
// (sumber tidak menemukan pengguna aktif untuk Principal) juga 401. Manifest yang user-nya bukan Principal adalah
// bug perakitan: 500 tanpa memuat isinya.
func bootstrapHandler(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		p, err := sdkidentity.RequirePrincipal(ctx)
		if err != nil {
			WriteError(w, req, http.StatusUnauthorized, "unauthenticated", "autentikasi diperlukan", nil)
			return
		}
		// PRD-001 Q29 butir 2 hanya mengecualikan aturan mfa_required; scope token tetap berlaku. Bootstrap memuat data
		// pengguna, bukan sumber daya yang di-scope-kan, jadi token ber-scope terbatas ditolak.
		if len(p.TokenScope) > 0 {
			WriteError(w, req, http.StatusForbidden, "permission_denied", "token ber-scope terbatas tidak dapat membaca bootstrap", nil)
			return
		}
		tid, _ := tenancy.FromContext(ctx)
		var m bootstrap.Manifest
		if d.Bootstrap != nil {
			m = d.Bootstrap(ctx, tid)
		} else {
			m = bootstrap.Build(d.AppVersion, tid, nil, 0)
			m.User = &bootstrap.User{ID: p.UserID}
		}
		if m.Unavailable {
			WriteError(w, req, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
			return
		}
		if m.User == nil {
			WriteError(w, req, http.StatusUnauthorized, "unauthenticated", "autentikasi diperlukan", nil)
			return
		}
		if m.User.ID != p.UserID {
			d.Logger.ErrorContext(ctx, "bootstrap: user manifest bukan principal request; respons dibatalkan")
			WriteError(w, req, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server", nil)
			return
		}
		h := w.Header()
		h.Set("Cache-Control", "private, no-cache")
		h.Set("Vary", "Authorization")
		etag := m.ETag()
		h.Set("ETag", etag)
		if etagMatches(req.Header[textproto.CanonicalMIMEHeaderKey("If-None-Match")], etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		WriteData(w, http.StatusOK, m)
	})
}

// etagMatches mengevaluasi If-None-Match dengan perbandingan LEMAH (RFC 9110 section 8.8.3.2): awalan W/ di
// kedua sisi diabaikan, sehingga "x" cocok dengan W/"x". "*" cocok dengan apa pun.
func etagMatches(values []string, etag string) bool {
	want := strings.TrimPrefix(etag, "W/")
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			p := strings.TrimSpace(part)
			if p == "*" || strings.TrimPrefix(p, "W/") == want {
				return true
			}
		}
	}
	return false
}

// CheckETag memasang header ETag dan, bila If-None-Match cocok, menjawab 304 lalu mengembalikan true
// (handler harus berhenti). etag sudah berbentuk kutip, mis. `"7"`.
func CheckETag(w http.ResponseWriter, r *http.Request, etag string) bool {
	w.Header().Set("ETag", etag)
	if etagMatches(r.Header[textproto.CanonicalMIMEHeaderKey("If-None-Match")], etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}
