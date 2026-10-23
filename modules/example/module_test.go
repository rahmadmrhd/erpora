package example

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"testing"

	"erpora/sdk/module"
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
	if err != nil || len(names) != 1 || names[0] != "0001_create_example_notes.sql" {
		t.Fatalf("names = %v err = %v", names, err)
	}
}

type recRegistrar struct {
	routes   map[string]string // "METHOD path" -> permission
	commands []module.Command
}

func (r *recRegistrar) Handle(method, path, permission string, _ http.Handler) {
	r.routes[method+" "+path] = permission
}
func (r *recRegistrar) Public(string, string, http.Handler) {}
func (r *recRegistrar) Command(c module.Command)            { r.commands = append(r.commands, c) }

func TestRegister_RoutesAndCommand(t *testing.T) {
	r := &recRegistrar{routes: map[string]string{}}
	if err := New().Register(r); err != nil {
		t.Fatal(err)
	}
	if r.routes["GET /ping"] != "example.ping.read" || r.routes["GET /notes"] != "example.notes.read" {
		t.Errorf("routes = %v", r.routes)
	}
	if len(r.commands) != 1 || r.commands[0].Name != "recount" || r.commands[0].Run == nil {
		t.Fatalf("commands = %+v", r.commands)
	}
	// Tanpa konteks tenant, command gagal dengan error (bukan panic).
	if err := r.commands[0].Run(context.Background(), module.Invocation{Out: &bytes.Buffer{}}); err == nil {
		t.Error("recount tanpa tenant harus error")
	}
}

func TestNotesContract_NoTenantContext(t *testing.T) {
	if _, err := New().Notes().Count(context.Background()); err == nil {
		t.Error("Count tanpa tenant harus error")
	}
}
