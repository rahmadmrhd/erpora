// Package example (fixture generator).
package example

import (
	"net/http"

	"erpora/sdk/module"
)

// Module (fixture).
type Module struct{}

// Register (fixture).
func (m *Module) Register(r module.Registrar) error {
	r.Handle(http.MethodGet, "/ping", "example.ping.read", http.NotFoundHandler())
	// erp:generated:routes
	r.Command(module.Command{Name: "recount", Short: "Hitung", Run: nil})
	// erp:generated:commands
	return nil
}
