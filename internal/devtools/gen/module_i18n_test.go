package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/devtools/i18ntools"
)

// Berkas i18n hasil make:module harus identik dengan keluaran i18n:generate dan lolos i18n:check tanpa temuan
// (Requirement 8.9 dan 1.8): jaminan bahwa web:check (i18n:generate --check) dan lint (i18n:check) hijau untuk modul baru.
func TestMakeModule_I18nOutputMatchesGeneratorAndCheck(t *testing.T) {
	realRoot := filepath.Join("..", "..", "..")
	for _, name := range []string{"inventory", "stock-opname"} {
		t.Run(name, func(t *testing.T) {
			root := fixtureRepo(t)
			plan, err := MakeModule(root, ModuleOptions{Name: name, Frontend: true})
			if err != nil {
				t.Fatal(err)
			}
			dir := strings.ReplaceAll(name, "-", "")

			// Repo mini: registri dan katalog platform/ui nyata + berkas i18n/manifest/keys hasil plan.
			tmp := t.TempDir()
			for _, rel := range []string{
				i18ntools.RegistryPath,
				i18ntools.PlatformCatalogDir + "/id.json", i18ntools.PlatformCatalogDir + "/en.json",
				i18ntools.UICatalogDir + "/id.json", i18ntools.UICatalogDir + "/en.json",
			} {
				data, err := os.ReadFile(filepath.Join(realRoot, filepath.FromSlash(rel))) //nolint:gosec // path tetap di repo
				if err != nil {
					t.Fatal(err)
				}
				writeTemp(t, tmp, rel, data)
			}
			for _, op := range plan.Ops {
				if strings.HasPrefix(op.Path, "modules/"+dir+"/") && op.Kind == OpCreate {
					writeTemp(t, tmp, op.Path, op.Content)
				}
			}

			rep, err := i18ntools.Check(tmp, i18ntools.CheckOptions{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Findings) != 0 {
				t.Fatalf("i18n:check --strict atas modul baru: %+v", rep.Findings)
			}
			files, err := i18ntools.Generate(tmp)
			if err != nil {
				t.Fatal(err)
			}
			keysPath := "modules/" + dir + "/web/src/i18n/keys.gen.ts"
			var got string
			for _, f := range files {
				if f.Path == keysPath {
					got = string(f.Content)
				}
			}
			if got == "" {
				t.Fatalf("i18n:generate tidak membangkitkan %s", keysPath)
			}
			if want := opContent(t, plan.Ops, keysPath); want != got {
				t.Errorf("keys.gen.ts template berbeda dari keluaran i18n:generate:\n--- template\n%s\n--- generator\n%s", want, got)
			}
		})
	}
}

func writeTemp(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Modul tanpa --frontend tetap membawa katalog dan deklarasi i18n, tetapi tidak membuat berkas web.
func TestMakeModule_BackendOnlyHasCatalogsNoWeb(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory"})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, p := range opPaths(plan.Ops) {
		have[p] = true
	}
	for _, p := range []string{"modules/inventory/i18n/id.json", "modules/inventory/i18n/en.json"} {
		if !have[p] {
			t.Errorf("%s harus dihasilkan", p)
		}
	}
	for p := range have {
		if strings.HasPrefix(p, "modules/inventory/web/") {
			t.Errorf("modul tanpa --frontend tidak boleh menghasilkan %s", p)
		}
	}
	if m := opContent(t, plan.Ops, "modules/inventory/module.json"); !strings.Contains(m, `"i18n": { "locales": ["id", "en"], "defaultLocale": "id" }`) {
		t.Errorf("module.json tanpa bagian i18n:\n%s", m)
	}
	if g := opContent(t, plan.Ops, "modules/inventory/module.go"); !strings.Contains(g, "//go:embed i18n/*.json") || !strings.Contains(g, "func (m *Module) Catalogs() fs.FS") {
		t.Errorf("module.go tanpa embed/Catalogs:\n%s", g)
	}
}

// make:module --frontend menyisipkan frontendModule (bukan moduleInfo) ke web/src/app/modules.ts (M3a 13.4).
func TestMakeModule_FrontendRegistersFrontendModule(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Frontend: true})
	if err != nil {
		t.Fatal(err)
	}
	ts := opContent(t, plan.Ops, "web/src/app/modules.ts")
	if !strings.Contains(ts, `import { frontendModule as stockOpnameModule } from "@erpora/module-stockopname";`) || !strings.Contains(ts, "  stockOpnameModule,\n") {
		t.Errorf("modules.ts:\n%s", ts)
	}
	if strings.Contains(ts, "moduleInfo") {
		t.Errorf("modules.ts tidak boleh memuat moduleInfo:\n%s", ts)
	}
	idx := opContent(t, plan.Ops, "modules/stockopname/web/src/index.ts")
	for _, want := range []string{`export * as api from "../api/contract.gen";`, `export { moduleInfo } from "./info";`, `export { frontendModule } from "./module";`} {
		if !strings.Contains(idx, want) {
			t.Errorf("index.ts tidak memuat %q:\n%s", want, idx)
		}
	}
}
