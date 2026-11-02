package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/archcheck"
	"erpora/internal/devtools/gen"
	"erpora/internal/devtools/metadatatools/check"
)

// Task 18.3: make:module --metadata menghasilkan modul yang langsung hijau untuk metadata:check dan
// metadata:generate --check (tes ini di paket generate karena paket gen tidak boleh mengimpor pembangkit).

func copyGenRepo(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "gen", "testdata", "repo")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		b, err := os.ReadFile(p) //nolint:gosec // fixture repo
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// makeModule menerapkan make:module --metadata pada repo mini; langkah Post lain (go mod tidy, bun) dilewati,
// hanya langkah metadata:generate yang dijalankan.
func makeModule(t *testing.T, root, name string) string {
	t.Helper()
	gener := func(_ context.Context, r, dir string) error {
		res, err := GenerateWorkspace(r, Options{Modules: []string{dir}})
		for _, p := range res.Problems {
			t.Logf("temuan: %s", p)
		}
		return err
	}
	plan, err := gen.MakeModule(root, gen.ModuleOptions{Name: name, Metadata: true, MetadataGenerate: gener})
	if err != nil {
		t.Fatal(err)
	}
	var steps []gen.PostStep
	for _, s := range plan.Post {
		if strings.HasPrefix(s.Name, "metadata:generate") {
			steps = append(steps, s)
		}
	}
	if len(steps) != 1 {
		t.Fatalf("langkah metadata:generate = %d", len(steps))
	}
	plan.Post = steps
	if err := gen.Apply(context.Background(), plan, gen.Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(name, "-", "")
}

func TestMakeModuleMetadata_GreenOutOfTheBox(t *testing.T) {
	root := copyGenRepo(t)
	dir := makeModule(t, root, "stock-opname")
	mod := "modules/" + dir

	for _, f := range []string{
		mod + "/metadata/item.resource.yaml",
		mod + "/internal/domain/item_meta.gen.go", mod + "/internal/domain/meta_runtime.gen.go",
		mod + "/internal/presentation/http/item_dto.gen.go", mod + "/internal/presentation/http/meta_runtime.gen.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s tidak dihasilkan: %v", f, err)
		}
	}
	spec := readFile(t, root, mod+"/openapi.yaml")
	if !strings.Contains(spec, "x-erp-generated: true") || !strings.Contains(spec, "    ItemCreate:") || !strings.Contains(spec, "    ItemUpdate:") {
		t.Errorf("skema Item bangkitan tidak ada:\n%s", spec)
	}

	in, loadProblems := check.LoadWorkspace(root)
	rep := check.Run(in)
	for _, p := range append(loadProblems, rep.Problems...) {
		if p.Severity == check.SeverityError {
			t.Errorf("metadata:check: %s", p)
		}
	}
	if rep.Resource("stock-opname.item") == nil {
		t.Error("resource stock-opname.item tidak terdeteksi")
	}
	if _, err := GenerateWorkspace(root, Options{Check: true}); err != nil {
		t.Errorf("metadata:generate --check harus hijau: %v", err)
	}
}

func TestMakeModuleMetadata_MetadataFailureSurfaces(t *testing.T) {
	root := copyGenRepo(t)
	plan, err := gen.MakeModule(root, gen.ModuleOptions{Name: "inventory", Metadata: true,
		MetadataGenerate: func(context.Context, string, string) error { return errors.New("gagal") }})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Post[1].Run(context.Background(), root); err == nil {
		t.Error("kegagalan pembangkit harus terlihat")
	}
}

// TestMakeModuleMetadata_CompilesAndPassesGates: modul hasil dapat dikompilasi, lolos vet dan test bawaannya, dan
// bersih dari pelanggaran arsitektur D1 sampai D10 (tanpa pengecualian baru).
func TestMakeModuleMetadata_CompilesAndPassesGates(t *testing.T) {
	if testing.Short() {
		t.Skip("kompilasi modul (lambat)")
	}
	root := copyGenRepo(t)
	dir := makeModule(t, root, "inventory")
	realSDK, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "sdk"))
	if err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "modules", dir)
	gomod := filepath.Join(modDir, "go.mod")
	data, _ := os.ReadFile(gomod) //nolint:gosec // salinan sementara
	patched := strings.ReplaceAll(string(data), "=> ../../sdk", "=> "+filepath.ToSlash(realSDK))
	if err := os.WriteFile(gomod, []byte(patched), 0o600); err != nil {
		t.Fatal(err)
	}
	goTool(t, modDir, "build", "./...")
	goTool(t, modDir, "vet", "./...")
	goTool(t, modDir, "test", "./...")

	t.Setenv("GOWORK", "off")
	repo, err := archcheck.Load(modDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range archcheck.Check(repo) {
		switch v.Rule {
		case "D1", "D2", "D3", "D3b", "D4", "D7", "D9", "D10", "AUTOMIGRATE", "DB-OPEN":
			t.Errorf("pelanggaran %s di %s: %s", v.Rule, v.Package, v.Detail)
		}
	}
}
