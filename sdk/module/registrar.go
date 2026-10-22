package module

import "net/http"

// Registrar diimplementasi platform dan diberikan ke Module.Register. Modul tidak
// pernah menerima router; seluruh route didaftarkan lewat sini.
type Registrar interface {
	// Handle mendaftarkan route yang dilindungi permission (mis. "orders.read").
	Handle(method, path, permission string, h http.Handler)
	// Public mendaftarkan route tanpa permission (tetap tunduk pada status modul).
	Public(method, path string, h http.Handler)
	// Command mendaftarkan perintah CLI modul.
	Command(c Command)
}
