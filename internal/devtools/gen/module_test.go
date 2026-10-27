package gen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func applyPlan(t *testing.T, root string, p Plan) {
	t.Helper()
	p.Post = nil // langkah Post (go mod tidy, bun) butuh repo penuh; dibuktikan di tests/generator
	if err := Apply(context.Background(), p, Options{Root: root}); err != nil {
		t.Fatal(err)
	}
}

func opContent(t *testing.T, ops []FileOp, path string) string {
	t.Helper()
	for _, op := range ops {
		if op.Path == path {
			return string(op.Content)
		}
	}
	t.Fatalf("op %s tidak ada; ada: %v", path, opPaths(ops))
	return ""
}

func opPaths(ops []FileOp) []string {
	out := make([]string, len(ops))
	for i, o := range ops {
		out[i] = o.Path
	}
	return out
}

func TestMakeModule_Golden(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory", Publisher: "acme", Frontend: true, Depends: []string{"acme.example"}})
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/module_inventory", plan.Ops)
	if len(plan.Post) != 4 {
		t.Errorf("Post = %d langkah, want 4 (go mod tidy, contracts:generate, bun install, biome)", len(plan.Post))
	}
}

// Review Focus 1: slug berstrip.
func TestMakeModule_Golden_Hyphenated(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Publisher: "acme", Frontend: true})
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/module_stock_opname", plan.Ops)
	checks := map[string][]string{
		"modules/stockopname/go.mod":                                        {"module erpora/modules/stockopname"},
		"modules/stockopname/module.json":                                   {`"slug": "stock-opname"`, `"key": "acme.stock-opname"`, `"stock-opname.items.read"`},
		"modules/stockopname/openapi.yaml":                                  {"/api/stock-opname/items", "x-erp-permission: stock-opname.items.read", "operationId: listStockOpnameItems"},
		"modules/stockopname/migrations/0001_create_stock_opname_items.sql": {"CREATE TABLE stock_opname_items"},
		"modules/stockopname/web/package.json":                              {`"@erpora/module-stockopname"`},
		"internal/app/modules.go":                                           {"stockopname.New(),", `"erpora/modules/stockopname"`},
		"go.work":                                                           {"./modules/stockopname"},
	}
	for path, wants := range checks {
		got := opContent(t, plan.Ops, path)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s tidak memuat %q", path, w)
			}
		}
	}
	if !strings.Contains(opContent(t, plan.Ops, "modules/stockopname/module.go"), "package stockopname") {
		t.Error("package harus stockopname")
	}
}

func TestMakeModule_RejectsCollisions(t *testing.T) {
	root := fixtureRepo(t)
	first, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Publisher: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, first)
	before := checksum(t, root)
	cases := []struct {
		name string
		opts ModuleOptions
		want string
	}{
		{"Dir sama (stockopname vs stock-opname)", ModuleOptions{Name: "stockopname"}, "stockopname"},
		{"slug example", ModuleOptions{Name: "example"}, "example"},
		{"dependensi tidak ada", ModuleOptions{Name: "billing", Depends: []string{"acme.missing"}}, "acme.missing"},
		{"slug cadangan", ModuleOptions{Name: "platform"}, "dicadangkan"},
		{"nama tidak valid", ModuleOptions{Name: "Billing"}, "tidak valid"},
	}
	for _, c := range cases {
		_, err := MakeModule(root, c.opts)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want memuat %q", c.name, err, c.want)
		}
	}
	if checksum(t, root) != before {
		t.Error("penolakan tidak boleh mengubah direktori")
	}
}

func TestMakeModule_MarkerMissing(t *testing.T) {
	root := fixtureRepo(t)
	p := filepath.Join(root, "internal", "app", "modules.go")
	data, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(string(data), "// erp:generated:modules", "")), 0o600); err != nil {
		t.Fatal(err)
	}
	before := checksum(t, root)
	_, err := MakeModule(root, ModuleOptions{Name: "inventory"})
	if !errors.Is(err, ErrMarkerNotFound) {
		t.Fatalf("err = %v, want ErrMarkerNotFound", err)
	}
	if exists(root, "modules/inventory/module.go") || checksum(t, root) != before {
		t.Error("tidak boleh ada file baru")
	}
}

func TestMakeModule_ApplyTwiceWithoutForce(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory"})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, plan)
	if _, err := MakeModule(root, ModuleOptions{Name: "inventory"}); !errors.Is(err, ErrExists) {
		t.Errorf("rencana kedua: err = %v, want ErrExists", err)
	}
	// Penerapan ulang rencana yang sama (tanpa membangun ulang) juga ditolak tanpa --force.
	plan.Post = nil
	if err := Apply(context.Background(), plan, Options{Root: root}); !errors.Is(err, ErrExists) {
		t.Errorf("Apply kedua: err = %v, want ErrExists", err)
	}
}

// Mencegah template tertinggal dari sumbernya: setiap file non-notes modules/example punya padanan di template.
func TestMakeModule_TemplateMatchesExampleShape(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory", Frontend: true, Depends: []string{"acme.example"}})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, op := range plan.Ops {
		if rel, ok := strings.CutPrefix(op.Path, "modules/inventory/"); ok {
			have[rel] = true
		}
	}
	// Padanan nama: notes -> items; contract/notes.go -> contract/doc.go; dua migrasi example -> satu migrasi awal.
	rename := map[string]string{
		"internal/application/list_notes.go":            "internal/application/list_items.go",
		"internal/application/list_notes_test.go":       "internal/application/list_items_test.go",
		"internal/domain/note.go":                       "internal/domain/item.go",
		"internal/domain/note_test.go":                  "internal/domain/item_test.go",
		"internal/infrastructure/notes_repo.go":         "internal/infrastructure/items_repo.go",
		"contract/notes.go":                             "contract/doc.go",
		"migrations/0001_create_example_notes.sql":      "migrations/0001_create_inventory_items.sql",
		"migrations/0002_add_example_notes_version.sql": "", // version/updated_at sudah ada di migrasi awal
	}
	exampleDir := filepath.Join("..", "..", "..", "modules", "example")
	err = filepath.WalkDir(exampleDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(exampleDir, p)
		rel = filepath.ToSlash(rel)
		if rel == "go.sum" {
			return nil
		}
		want := rel
		if r, ok := rename[rel]; ok {
			want = r
		}
		if want != "" && !have[want] {
			t.Errorf("file example %s tidak punya padanan template (%s); template tertinggal dari sumbernya", rel, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"deps.go", "README.md", "web/package.json", "web/tsconfig.json", "web/src/index.ts", "internal/domain/errors.go"} {
		if !have[extra] {
			t.Errorf("file %s harus dihasilkan", extra)
		}
	}
}
