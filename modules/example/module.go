// Package example adalah modul referensi ERP untuk memvalidasi SDK modul dan
// scaffolding platform: tata letak modul, manifest, rute, migrasi, hook
// lifecycle, command, dan kontrak untuk modul lain.
package example

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"erpora/modules/example/contract"
	"erpora/modules/example/internal/application"
	"erpora/modules/example/internal/infrastructure"
	httpapi "erpora/modules/example/internal/presentation/http"
	"erpora/sdk/module"
)

//go:embed module.json
var manifestJSON []byte

//go:embed openapi.yaml
var openAPI []byte

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed i18n/*.json
var catalogsFS embed.FS

// Module adalah implementasi module.Module untuk acme.example.
type Module struct {
	manifest module.Manifest
	svc      *application.Service
	handlers *httpapi.Handlers
}

// New merakit modul beserta dependensinya.
func New() *Module {
	svc := application.NewService(infrastructure.NewNotesRepo())
	return &Module{
		manifest: module.MustParseManifest(manifestJSON),
		svc:      svc,
		handlers: httpapi.New(svc),
	}
}

// OpenAPI mengembalikan kontrak API modul (module.ContractSource; PRD-000 FR-23.1).
func (m *Module) OpenAPI() []byte { return openAPI }

// Manifest mengembalikan manifest modul.
func (m *Module) Manifest() module.Manifest { return m.manifest }

// Register mendaftarkan route dan command.
func (m *Module) Register(r module.Registrar) error {
	r.Handle(http.MethodGet, "/ping", "example.ping.read", http.HandlerFunc(m.handlers.Ping))
	r.Handle(http.MethodGet, "/notes", "example.notes.read", http.HandlerFunc(m.handlers.ListNotes))
	// erp:generated:routes
	r.Command(module.Command{
		Name:  "recount",
		Short: "Hitung jumlah notes",
		Run: func(ctx context.Context, inv module.Invocation) error {
			n, err := m.svc.Count(ctx)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(inv.Out, "notes: %d\n", n)
			return err
		},
	})
	// erp:generated:commands
	return nil
}

// Migrations mengembalikan migrasi goose modul.
func (m *Module) Migrations() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		panic(err) // direktori di-embed saat kompilasi; tidak mungkin gagal
	}
	return sub
}

// Catalogs mengembalikan katalog terjemahan modul, satu berkas "<locale>.json" per bahasa (module.CatalogSource; PRD-000 FR-25).
func (m *Module) Catalogs() fs.FS {
	sub, err := fs.Sub(catalogsFS, "i18n")
	if err != nil {
		panic(err) // direktori di-embed saat kompilasi; tidak mungkin gagal
	}
	return sub
}

// Install menyemai 3 notes bila tabel kosong (idempoten).
func (m *Module) Install(ctx context.Context, _ module.Env) error { return m.svc.SeedInitial(ctx) }

// SeedDev menambah notes contoh untuk pengembangan (idempoten).
func (m *Module) SeedDev(ctx context.Context, _ module.Env) error { return m.svc.SeedDev(ctx) }

// Health memastikan tabel notes dapat dibaca.
func (m *Module) Health(ctx context.Context, _ module.Env) error { return m.svc.Health(ctx) }

// Notes mengembalikan kontrak publik untuk modul lain.
func (m *Module) Notes() contract.Notes { return notesContract{svc: m.svc} }

// notesContract mengimplementasi contract.Notes; agregat tenant aktif dari context.
type notesContract struct{ svc *application.Service }

func (c notesContract) Count(ctx context.Context) (int64, error) { return c.svc.Count(ctx) }
