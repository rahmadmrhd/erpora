package inventory

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"

	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var (
	_ module.Module          = (*Module)(nil)
	_ module.MigrationSource = (*Module)(nil)
	_ module.ContractSource  = (*Module)(nil)
	_ module.Installer       = (*Module)(nil)
	_ module.HealthChecker   = (*Module)(nil)
	_ module.DevSeeder       = (*Module)(nil)
)

func TestManifest_Parses(t *testing.T) {
	m := New().Manifest()
	if m.Key != "acme.inventory" || m.Slug != "inventory" || len(m.Raw) == 0 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestMigrations_Embedded(t *testing.T) {
	names, err := fs.Glob(New().Migrations(), "*.sql")
	if err != nil || len(names) != 1 || names[0] != "0001_create_inventory_items.sql" {
		t.Fatalf("names = %v err = %v", names, err)
	}
}

// Route, permission, dan command yang didaftarkan harus sesuai manifest (capabilities).
func TestRegister_RoutesAndCommand(t *testing.T) {
	r := moduletest.NewRecorder()
	if err := New().Register(r); err != nil {
		t.Fatal(err)
	}
	if r.Permission("GET", "/items") != "inventory.items.read" {
		t.Errorf("routes = %+v", r.Routes)
	}
	caps := map[string]bool{}
	for _, c := range New().Manifest().Capabilities {
		caps[c] = true
	}
	for _, rt := range r.Routes {
		if !rt.Public && !caps[rt.Permission] {
			t.Errorf("permission %q tidak ada di capabilities manifest", rt.Permission)
		}
	}
	if len(r.Commands) != 1 || r.Commands[0].Name != "recount" || r.Commands[0].Run == nil {
		t.Fatalf("commands = %+v", r.Commands)
	}
	// Tanpa konteks tenant, command gagal dengan error (bukan panic).
	if err := r.Commands[0].Run(context.Background(), module.Invocation{Out: &bytes.Buffer{}}); err == nil {
		t.Error("recount tanpa tenant harus error")
	}
}

// Kontrak OpenAPI di-embed; kesesuaiannya dengan route diperiksa oleh test di internal/app (modul tidak boleh
// mengimpor paket platform, aturan D9).
func TestOpenAPI_Embedded(t *testing.T) {
	doc := string(New().OpenAPI())
	if !strings.HasPrefix(doc, "openapi: 3.1.0") {
		t.Errorf("openapi.yaml harus diawali \"openapi: 3.1.0\"")
	}
	for _, want := range []string{"/api/inventory/items", "x-erp-permission: inventory.items.read"} {
		if !strings.Contains(doc, want) {
			t.Errorf("openapi.yaml tidak memuat %q", want)
		}
	}
}

// Generator make:* menyisipkan registrasi di atas penanda; penanda tidak boleh hilang dari module.go.
func TestModuleGo_GeneratorMarkers(t *testing.T) {
	src, err := os.ReadFile("module.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"// erp:generated:routes", "// erp:generated:commands"} {
		if strings.Count(string(src), marker) != 1 {
			t.Errorf("penanda %q harus tepat satu di module.go", marker)
		}
	}
}

func TestCount_NoTenantContext(t *testing.T) {
	if _, err := New().svc.Count(context.Background()); err == nil {
		t.Error("Count tanpa tenant harus error")
	}
}
