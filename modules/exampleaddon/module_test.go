package exampleaddon

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var (
	_ module.Module         = (*Module)(nil)
	_ module.ContractSource = (*Module)(nil)
	_ module.EnableHook     = (*Module)(nil)
	_ module.DisableHook    = (*Module)(nil)
)

type fakeNotes struct{}

func (fakeNotes) Count(context.Context) (int64, error) { return 0, nil }

// Kontrak OpenAPI modul di-embed; keabsahan dan kesesuaiannya dengan route diperiksa di internal/app
// (modul tidak boleh mengimpor paket platform).
func TestOpenAPI_Embedded(t *testing.T) {
	doc := string(New(fakeNotes{}).OpenAPI())
	for _, want := range []string{"openapi: 3.1.0", "/api/example-addon/summary", "x-erp-permission: example-addon.summary.read", "erp:example#/components/schemas/NoteCount"} {
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
