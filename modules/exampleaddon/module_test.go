package exampleaddon

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var (
	_ module.Module      = (*Module)(nil)
	_ module.EnableHook  = (*Module)(nil)
	_ module.DisableHook = (*Module)(nil)
)

type fakeNotes struct{}

func (fakeNotes) Count(context.Context) (int64, error) { return 0, nil }

func TestManifest(t *testing.T) {
	m := New(fakeNotes{}).Manifest()
	if m.Key != "acme.example-addon" || m.Slug != "example-addon" || len(m.Raw) == 0 || len(m.Dependencies) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestRegister_SummaryRoute(t *testing.T) {
	r := moduletest.NewRecorder()
	if err := New(fakeNotes{}).Register(r); err != nil {
		t.Fatal(err)
	}
	if r.Permission("GET", "/summary") != "example-addon.summary.read" {
		t.Errorf("routes = %+v", r.Routes)
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
