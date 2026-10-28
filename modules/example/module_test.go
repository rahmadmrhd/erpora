package example

import (
	"bytes"
	"context"
	"io/fs"
	"testing"

	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var (
	_ module.Module          = (*Module)(nil)
	_ module.MigrationSource = (*Module)(nil)
	_ module.Installer       = (*Module)(nil)
	_ module.HealthChecker   = (*Module)(nil)
	_ module.DevSeeder       = (*Module)(nil)
)

func TestManifest_Parses(t *testing.T) {
	m := New().Manifest()
	if m.Key != "acme.example" || m.Slug != "example" || len(m.Raw) == 0 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestMigrations_Embedded(t *testing.T) {
	names, err := fs.Glob(New().Migrations(), "*.sql")
	if err != nil || len(names) != 2 || names[0] != "0001_create_example_notes.sql" || names[1] != "0002_add_example_notes_version.sql" {
		t.Fatalf("names = %v err = %v", names, err)
	}
}

func TestRegister_RoutesAndCommand(t *testing.T) {
	r := moduletest.NewRecorder()
	if err := New().Register(r); err != nil {
		t.Fatal(err)
	}
	if r.Permission("GET", "/ping") != "example.ping.read" || r.Permission("GET", "/notes") != "example.notes.read" {
		t.Errorf("routes = %+v", r.Routes)
	}
	if len(r.Commands) != 1 || r.Commands[0].Name != "recount" || r.Commands[0].Run == nil {
		t.Fatalf("commands = %+v", r.Commands)
	}
	// Tanpa konteks tenant, command gagal dengan error (bukan panic).
	if err := r.Commands[0].Run(context.Background(), module.Invocation{Out: &bytes.Buffer{}}); err == nil {
		t.Error("recount tanpa tenant harus error")
	}
}

func TestNotesContract_NoTenantContext(t *testing.T) {
	if _, err := New().Notes().Count(context.Background()); err == nil {
		t.Error("Count tanpa tenant harus error")
	}
}
