// Package core adalah modul Core (erp.core, required; PRD-001): identitas, autentikasi, dan kelak RBAC, settings, serta
// audit. Pada C1.11 modul ini berisi API autentikasi (login, refresh, logout, reset sandi, MFA). Handlernya tipis dan
// memanggil identity.Service yang dipasang platform pada context modul ini saja (ADR-0013 §7a butir 5); mesin
// identitasnya berada di platform, bukan di modul.
package core

import (
	"embed"
	"io/fs"
	"net/http"

	httpapi "erpora/modules/core/internal/presentation/http"
	"erpora/sdk/module"
)

//go:embed module.json
var manifestJSON []byte

//go:embed openapi.yaml
var openAPI []byte

//go:embed i18n/*.json
var catalogsFS embed.FS

// Module adalah implementasi module.Module untuk erp.core.
type Module struct {
	manifest module.Manifest
	handlers *httpapi.Handlers
}

// New merakit modul.
func New() *Module {
	return &Module{manifest: module.MustParseManifest(manifestJSON), handlers: httpapi.New()}
}

// Manifest mengembalikan manifest modul.
func (m *Module) Manifest() module.Manifest { return m.manifest }

// OpenAPI mengembalikan kontrak API modul (module.ContractSource; PRD-000 FR-23.1).
func (m *Module) OpenAPI() []byte { return openAPI }

// Catalogs mengembalikan katalog terjemahan modul, satu berkas "<locale>.json" per bahasa (module.CatalogSource; PRD-000 FR-25).
func (m *Module) Catalogs() fs.FS {
	sub, err := fs.Sub(catalogsFS, "i18n")
	if err != nil {
		panic(err) // direktori di-embed saat kompilasi; tidak mungkin gagal
	}
	return sub
}

// Register mendaftarkan route autentikasi. Path dan method persis ini juga menjadi kunci daftar route yang boleh
// dipanggil sesi menunggu MFA di platform (ADR-0013 §4; runtime/authroutes.go), jadi mengubahnya berarti mengubah
// aturan keamanan: mfa/verify, mfa/recovery, dan logout untuk sesi yang menunggu verifikasi; mfa/enroll, mfa/confirm,
// dan logout untuk administrator tanpa faktor.
//
// Aturan publik vs terautentikasi (PRD-001 Q29): login, refresh (berbasis cookie), password/forgot, dan password/reset
// publik (tanpa Principal). logout, password/change, dan semua mfa/* milik pengguna sendiri: terautentikasi tanpa
// permission (module.Authenticated). Semua route auth dikenai batas laju per IP oleh platform (runtime.isAuthEndpoint)
// dan membawa Cache-Control: no-store.
func (m *Module) Register(r module.Registrar) error {
	h := m.handlers
	secret := module.Classify(module.ClassHighlyConfidential) // respons memuat token/rahasia/recovery code: no-store oleh platform
	small := module.MaxBodyBytes(4 << 10)                     // body auth kecil (email, sandi, kode)
	pub := func(path string, fn http.HandlerFunc, opts ...module.RouteOption) {
		r.Public(http.MethodPost, path, httpapi.NoStore(fn), append([]module.RouteOption{small}, opts...)...)
	}
	own := func(path string, fn http.HandlerFunc, opts ...module.RouteOption) {
		r.Handle(http.MethodPost, path, "", httpapi.NoStore(fn), append([]module.RouteOption{small, module.Authenticated()}, opts...)...)
	}

	pub("/auth/login", h.Login, secret)
	pub("/auth/refresh", h.Refresh, secret)
	pub("/auth/password/forgot", h.ForgotPassword)
	pub("/auth/password/reset", h.ResetPassword)

	own("/auth/logout", h.Logout)
	own("/auth/password/change", h.ChangePassword)
	own("/auth/mfa/enroll", h.EnrollMFA, secret)
	own("/auth/mfa/confirm", h.ConfirmMFA, secret)
	own("/auth/mfa/verify", h.VerifyMFA, secret)
	own("/auth/mfa/recovery", h.RecoverMFA, secret)
	own("/auth/mfa/recovery-codes", h.RegenerateRecoveryCodes, secret, module.RequireMFA())
	// erp:generated:routes
	// erp:generated:commands
	return nil
}
