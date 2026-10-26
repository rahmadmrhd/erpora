package exampleaddon

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"erpora/sdk/module"
)

var (
	_ module.Module      = (*Module)(nil)
	_ module.EnableHook  = (*Module)(nil)
	_ module.DisableHook = (*Module)(nil)
)

type fakeNotes struct{}

func (fakeNotes) Count(context.Context) (int64, error) { return 0, nil }

type recRegistrar struct{ routes map[string]string }

func (r *recRegistrar) Handle(method, path, permission string, _ http.Handler) {
	r.routes[method+" "+path] = permission
}
func (r *recRegistrar) Public(string, string, http.Handler) {}
func (r *recRegistrar) Command(module.Command)              {}

func TestManifest(t *testing.T) {
	m := New(fakeNotes{}).Manifest()
	if m.Key != "acme.example-addon" || m.Slug != "example-addon" || len(m.Raw) == 0 || len(m.Dependencies) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestRegister_SummaryRoute(t *testing.T) {
	r := &recRegistrar{routes: map[string]string{}}
	if err := New(fakeNotes{}).Register(r); err != nil {
		t.Fatal(err)
	}
	if r.routes["GET /summary"] != "example-addon.summary.read" {
		t.Errorf("routes = %v", r.routes)
	}
}

func TestHooks_LogModule(t *testing.T) {
	var buf bytes.Buffer
	env := module.Env{Logger: slog.New(slog.NewTextHandler(&buf, nil))}
	m := New(fakeNotes{})
	if err := m.OnEnable(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if err := m.OnDisable(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "erp.module.key=acme.example-addon") != 2 || !strings.Contains(out, "level=INFO") {
		t.Errorf("log = %q", out)
	}
}
