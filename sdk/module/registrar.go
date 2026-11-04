package module

import (
	"net/http"

	"erpora/sdk/authz"
)

// Registrar diimplementasi platform dan diberikan ke Module.Register. Modul tidak
// pernah menerima router; seluruh route didaftarkan lewat sini.
//
// Opsi per-route (batas body, klasifikasi) memakai parameter variadik pada Handle/Public agar pemanggil
// lama tidak berubah (Q26 butir 13); untuk pengujian modul gunakan moduletest.Recorder.
type Registrar interface {
	// Handle mendaftarkan route yang dilindungi permission (mis. "orders.read").
	Handle(method, path, permission string, h http.Handler, opts ...RouteOption)
	// Public mendaftarkan route tanpa permission (tetap tunduk pada status modul).
	Public(method, path string, h http.Handler, opts ...RouteOption)
	// Scope mendaftarkan scope kustom `custom:<key>` modul (PRD-001 FR-111). Key wajib berawalan slug modul
	// ("<slug>.<nama>"), unik, dan Filter/Match wajib diisi; pelanggaran menggagalkan registry
	// (invalid_scope_provider).
	Scope(p authz.ScopeProvider)
	// Command mendaftarkan perintah CLI modul.
	Command(c Command)
}
