// Package exampleaddon adalah modul referensi kedua: add-on yang bergantung
// pada kontrak modul example (acme.example) dan tidak memiliki tabel sendiri.
package exampleaddon

import (
	"context"
	_ "embed"
	"log/slog"
	"net/http"

	"erpora/modules/example/contract"
	httpapi "erpora/modules/exampleaddon/internal/presentation/http"
	"erpora/sdk/module"
)

//go:embed module.json
var manifestJSON []byte

// Module adalah implementasi module.Module untuk acme.example-addon.
type Module struct {
	manifest module.Manifest
	handlers *httpapi.Handlers
}

// New membuat modul dengan kontrak notes milik modul example.
func New(notes contract.Notes) *Module {
	return &Module{
		manifest: module.MustParseManifest(manifestJSON),
		handlers: httpapi.New(notes),
	}
}

// Manifest mengembalikan manifest modul.
func (m *Module) Manifest() module.Manifest { return m.manifest }

// Register mendaftarkan route ringkasan.
func (m *Module) Register(r module.Registrar) error {
	r.Handle(http.MethodGet, "/summary", "example-addon.summary.read", http.HandlerFunc(m.handlers.Summary))
	return nil
}

// OnEnable mencatat log saat modul diaktifkan (idempoten: hanya log).
func (m *Module) OnEnable(ctx context.Context, env module.Env) error {
	logger(env).InfoContext(ctx, "modul diaktifkan", "module", m.manifest.Key)
	return nil
}

// OnDisable mencatat log saat modul dinonaktifkan (idempoten: hanya log).
func (m *Module) OnDisable(ctx context.Context, env module.Env) error {
	logger(env).InfoContext(ctx, "modul dinonaktifkan", "module", m.manifest.Key)
	return nil
}

// logger memakai env.Logger, atau logger default bila tidak diisi.
func logger(env module.Env) *slog.Logger {
	if env.Logger != nil {
		return env.Logger
	}
	return slog.Default()
}
