package module

import "net/http"

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
	// Command mendaftarkan perintah CLI modul.
	Command(c Command)
}
