// Package inventory adalah modul Inventory (acme.inventory), dibuat oleh `erpora make:module`.
// Kerangkanya diturunkan dari modul referensi example: manifest, route berpagination, migrasi, hook lifecycle,
// command, dan kontrak OpenAPI. Registrasi tambahan disisipkan generator di atas penanda `erp:generated:*`.
package inventory

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"erpora/modules/inventory/internal/application"
	"erpora/modules/inventory/internal/infrastructure"
	"erpora/modules/inventory/internal/presentation/cli"
	httpapi "erpora/modules/inventory/internal/presentation/http"
	"erpora/sdk/module"
)

//go:embed module.json
var manifestJSON []byte

//go:embed openapi.yaml
var openAPI []byte

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Module adalah implementasi module.Module untuk acme.inventory.
type Module struct {
	manifest module.Manifest
	svc      *application.Service
	handlers *httpapi.Handlers
}

// New merakit modul beserta dependensinya.
func New() *Module {
	svc := application.NewService(infrastructure.NewItemsRepo())
	return &Module{
		manifest: module.MustParseManifest(manifestJSON),
		svc:      svc,
		handlers: httpapi.New(svc),
	}
}

// Manifest mengembalikan manifest modul.
func (m *Module) Manifest() module.Manifest { return m.manifest }

// OpenAPI mengembalikan kontrak API modul (module.ContractSource; PRD-000 FR-23.1).
func (m *Module) OpenAPI() []byte { return openAPI }

// Register mendaftarkan route dan command.
func (m *Module) Register(r module.Registrar) error {
	r.Handle(http.MethodGet, "/items", "inventory.items.read", http.HandlerFunc(m.handlers.ListItems),
		module.Classify(module.ClassInternal))
	// erp:generated:routes
	r.Command(module.Command{
		Name:  "recount",
		Short: "Hitung jumlah items",
		Run: func(ctx context.Context, inv module.Invocation) error {
			n, err := m.svc.Count(ctx)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(inv.Out, "items: %d\n", n)
			return err
		},
	})
	r.Command(cli.SyncStockCommand())
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

// Install menyemai items awal bila tabel kosong (idempoten).
func (m *Module) Install(ctx context.Context, _ module.Env) error { return m.svc.SeedInitial(ctx) }

// SeedDev menambah items contoh untuk pengembangan (idempoten).
func (m *Module) SeedDev(ctx context.Context, _ module.Env) error { return m.svc.SeedDev(ctx) }

// Health memastikan tabel items dapat dibaca.
func (m *Module) Health(ctx context.Context, _ module.Env) error { return m.svc.Health(ctx) }
