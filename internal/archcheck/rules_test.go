package archcheck

import (
	"os"
	"strings"
	"testing"
)

const (
	root    = "erpora"
	sdk     = root + "/sdk"
	example = root + "/modules/example"
	addon   = root + "/modules/exampleaddon"
)

// pkg membangun paket sintetis; modul pemilik diturunkan dari import path.
func pkg(path string, imports ...string) Package {
	mod := root
	switch {
	case strings.HasPrefix(path, sdk):
		mod = sdk
	case strings.HasPrefix(path, addon):
		mod = addon
	case strings.HasPrefix(path, example):
		mod = example
	}
	return Package{ImportPath: path, Module: mod, Imports: imports}
}

func repoWith(pkgs ...Package) Repo {
	return Repo{
		Root:     "x",
		Packages: pkgs,
		Mods: []GoMod{
			{Path: root, GoVersion: "1.27", Toolchain: "go1.27.3", Requires: []string{sdk, example}},
			{Path: sdk, GoVersion: "1.27", Toolchain: "go1.27.3"},
			{Path: example, GoVersion: "1.27", Toolchain: "go1.27.3", Requires: []string{sdk}},
			{Path: addon, GoVersion: "1.27", Toolchain: "go1.27.3", Requires: []string{sdk}},
		},
		Manifests: map[string]Manifest{
			example: {Key: "acme.example", Slug: "example", Dependencies: []string{"platform.core"}},
			addon:   {Key: "acme.exampleaddon", Slug: "exampleaddon", Dependencies: []string{"platform.core"}},
		},
	}
}

func rule(vs []Violation, id string) bool {
	for _, v := range vs {
		if v.Rule == id {
			return true
		}
	}
	return false
}

func assertViolation(t *testing.T, vs []Violation, id string) {
	t.Helper()
	if !rule(vs, id) {
		t.Fatalf("expected violation %s, got %+v", id, vs)
	}
}

func assertNone(t *testing.T, vs []Violation, id string) {
	t.Helper()
	if rule(vs, id) {
		t.Fatalf("unexpected violation %s: %+v", id, vs)
	}
}

func TestD2_PlatformMustNotImportModules(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(root+"/internal/platform/x", example))), "D2")
	assertNone(t, Check(repoWith(pkg(root+"/internal/platform/x", sdk))), "D2")
}

func TestD3_DomainImportsGorm(t *testing.T) {
	dom := example + "/internal/domain"
	assertViolation(t, Check(repoWith(pkg(dom, "gorm.io/gorm"))), "D3")
	assertViolation(t, Check(repoWith(pkg(dom, "net/http"))), "D3")
	assertViolation(t, Check(repoWith(pkg(dom, example+"/internal/infrastructure"))), "D3")
	assertNone(t, Check(repoWith(pkg(dom, "errors", sdk+"/module"))), "D3")
}

func TestD3b_GormOutsideAllowedPackages(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(root+"/internal/platform/settings", "gorm.io/gorm"))), "D3b")
	assertViolation(t, Check(repoWith(pkg(example+"/internal/application", "gorm.io/gorm"))), "D3b")
	for _, ok := range []string{
		example + "/internal/infrastructure/persistence",
		root + "/internal/platform/database",
		root + "/internal/platform/modules/store",
		sdk + "/data",
	} {
		assertNone(t, Check(repoWith(pkg(ok, "gorm.io/gorm"))), "D3b")
	}
}

func TestD4_ApplicationImportsInfrastructure(t *testing.T) {
	app := example + "/internal/application"
	assertViolation(t, Check(repoWith(pkg(app, example+"/internal/infrastructure"))), "D4")
	assertViolation(t, Check(repoWith(pkg(app, example+"/internal/presentation/http"))), "D4")
	assertNone(t, Check(repoWith(pkg(app, example+"/internal/domain"))), "D4")
}

func TestD5_SDKRequiresRootModule(t *testing.T) {
	r := repoWith()
	r.Mods[1].Requires = []string{root}
	assertViolation(t, Check(r), "D5")
	r.Mods[1].Requires = []string{"golang.org/x/mod"}
	assertNone(t, Check(r), "D5")
}

func TestD7_OnlyAppImportsModules(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(root+"/internal/other", example))), "D7")
	assertNone(t, Check(repoWith(pkg(root+"/internal/app", example))), "D7")
}

func TestD8_ImportNotInManifestDependencies(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(addon+"/internal/application", example+"/contract"))), "D8")
	r := repoWith(pkg(addon+"/internal/application", example+"/contract"))
	m := r.Manifests[addon]
	m.Dependencies = append(m.Dependencies, "acme.example")
	r.Manifests[addon] = m
	r.Mods[3].Requires = []string{sdk, example}
	assertNone(t, Check(r), "D8")
}

func TestD8_TestFileImportsAreChecked(t *testing.T) {
	p := pkg(addon + "/internal/application")
	p.TestImports = []string{example + "/contract"}
	assertViolation(t, Check(repoWith(p)), "D8")
	p2 := pkg(root + "/internal/platform/x")
	p2.TestImports = []string{example}
	assertViolation(t, Check(repoWith(p2)), "D2")
	p3 := pkg(root + "/internal/other")
	p3.TestImports = []string{example}
	assertViolation(t, Check(repoWith(p3)), "D7")
	// Aturan lain tetap hanya non-test: D3b tidak menilai impor test.
	p4 := pkg(example + "/internal/application")
	p4.TestImports = []string{"gorm.io/gorm"}
	assertNone(t, Check(repoWith(p4)), "D3b")
}

func TestD8_ManifestDependencyMissingFromGoModRequire(t *testing.T) {
	r := repoWith()
	m := r.Manifests[addon]
	m.Dependencies = append(m.Dependencies, "acme.example")
	r.Manifests[addon] = m
	assertViolation(t, Check(r), "D8") // addon tidak me-require example
	r.Mods[3].Requires = []string{sdk, example}
	assertNone(t, Check(r), "D8")
}

func TestD8_IndirectRequireIgnored(t *testing.T) {
	r := repoWith()
	r.Mods[3].Requires = []string{sdk, example}
	r.Mods[3].Indirect = map[string]bool{example: true}
	assertNone(t, Check(r), "D8")
}

func TestD1_ModuleMustUseOtherModuleContractOnly(t *testing.T) {
	r := repoWith(pkg(addon+"/internal/application", example))
	m := r.Manifests[addon]
	m.Dependencies = append(m.Dependencies, "acme.example")
	r.Manifests[addon] = m
	r.Mods[3].Requires = []string{sdk, example}
	assertViolation(t, Check(r), "D1")
	assertViolation(t, Check(repoWith(pkg(addon+"/x", example+"/internal/domain"))), "D1")
	r = repoWith(pkg(addon+"/internal/application", example+"/contract"))
	assertNone(t, Check(r), "D1")
	assertNone(t, Check(repoWith(pkg(example+"/internal/application", example+"/internal/domain"))), "D1")
}

func TestD3b_SDKDataOnlyFromModuleInfrastructure(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(example+"/internal/application", sdk+"/data"))), "D3b")
	assertViolation(t, Check(repoWith(pkg(example+"/internal/presentation/http", sdk+"/data"))), "D3b")
	assertNone(t, Check(repoWith(pkg(example+"/internal/infrastructure", sdk+"/data"))), "D3b")
	assertNone(t, Check(repoWith(pkg(root+"/internal/platform/settings", sdk+"/data"))), "D3b")
}

func TestD8_GoModRequireNotInManifest(t *testing.T) {
	r := repoWith()
	r.Mods[3].Requires = []string{sdk, example}
	assertViolation(t, Check(r), "D8")
	m := r.Manifests[addon]
	m.Dependencies = append(m.Dependencies, "acme.example")
	r.Manifests[addon] = m
	assertNone(t, Check(r), "D8")
}

func TestD9_ModuleRequiresRootModule(t *testing.T) {
	r := repoWith()
	r.Mods[2].Requires = []string{sdk, root}
	assertViolation(t, Check(r), "D9")
}

func TestD10_ModuleImportsChi(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(example+"/internal/presentation/http", "github.com/go-chi/chi/v5"))), "D10")
	assertViolation(t, Check(repoWith(pkg(sdk+"/http", "github.com/gin-gonic/gin"))), "D10")
	assertNone(t, Check(repoWith(pkg(root+"/internal/app", "github.com/go-chi/chi/v5"))), "D10")
}

func TestD11_GoModRequireCycle(t *testing.T) {
	r := repoWith()
	r.Mods[2].Requires = []string{addon}
	r.Mods[3].Requires = []string{example}
	assertViolation(t, Check(r), "D11")
}

func TestDBOpen_OnlyDatabasePackage(t *testing.T) {
	assertViolation(t, Check(repoWith(pkg(root+"/internal/platform/settings", "gorm.io/driver/postgres"))), "DB-OPEN")
	assertViolation(t, Check(repoWith(pkg(root+"/internal/platform/settings", "github.com/jackc/pgx/v5/pgxpool"))), "DB-OPEN")
	assertNone(t, Check(repoWith(pkg(root+"/internal/platform/database", "gorm.io/driver/postgres"))), "DB-OPEN")
}

func TestDBOpen_ExtendedImports(t *testing.T) {
	for _, imp := range []string{
		"github.com/jackc/pgx/v5", "github.com/jackc/pgx/v5/stdlib", "github.com/jackc/pgx/v5/pgxpool",
		"gorm.io/driver/mysql", "gorm.io/driver/sqlite",
		"github.com/jackc/pgx/v5/pgconn", "github.com/jackc/pgx/v5/pgtype", "github.com/jackc/pgx/v4", "github.com/lib/pq",
	} {
		assertViolation(t, Check(repoWith(pkg(root+"/internal/platform/settings", imp))), "DB-OPEN")
		assertNone(t, Check(repoWith(pkg(root+"/internal/platform/database", imp))), "DB-OPEN")
	}
	// database/sql: allowlist eksplisit internal/platform/database dan internal/platform/migrate (FR-21).
	for _, p := range []string{
		example + "/internal/infrastructure", root + "/internal/platform/settings", root + "/internal/platform/modules/store",
		sdk + "/data", root + "/internal/app",
	} {
		assertViolation(t, Check(repoWith(pkg(p, "database/sql"))), "DB-OPEN")
	}
	assertViolation(t, Check(repoWith(pkg(root+"/internal/platform/settings", "database/sql/driver"))), "DB-OPEN")
	for _, p := range []string{root + "/internal/platform/migrate", root + "/internal/platform/database"} {
		assertNone(t, Check(repoWith(pkg(p, "database/sql"))), "DB-OPEN")
	}
	assertNone(t, Check(repoWith(pkg(root+"/internal/platform/database", "github.com/jackc/pgx/v5/pgconn"))), "DB-OPEN")
}

func TestD10_ExtendedRouterList(t *testing.T) {
	for _, imp := range []string{
		"github.com/uptrace/bunrouter", "github.com/dimfeld/httptreemux/v5", "goji.io/pat",
		"github.com/go-zoo/bone", "github.com/bmizerany/pat", "github.com/beego/beego/v2",
		"github.com/gofiber/fiber/v2", "github.com/go-martini/martini", "github.com/julienschmidt/httprouter",
		"github.com/gorilla/mux", "github.com/go-chi/chi/v5", "github.com/labstack/echo/v4", "github.com/gin-gonic/gin",
	} {
		assertViolation(t, Check(repoWith(pkg(example+"/internal/presentation/http", imp))), "D10")
	}
}

func TestAutoMigrate_MethodValueDetected(t *testing.T) {
	bad := pkg(root + "/internal/platform/x")
	bad.Dir, bad.GoFiles = "testdata/automigrate_methodvalue", []string{"bad.go"}
	assertViolation(t, Check(repoWith(bad)), "AUTOMIGRATE")
}

func TestAutoMigrate_InNonTestFile(t *testing.T) {
	bad := pkg(root + "/internal/platform/x")
	bad.Dir, bad.GoFiles = "testdata/automigrate_bad", []string{"bad.go"}
	assertViolation(t, Check(repoWith(bad)), "AUTOMIGRATE")

	ok := pkg(root + "/internal/platform/x")
	ok.Dir, ok.GoFiles = "testdata/automigrate_ok", []string{"ok.go"}
	assertNone(t, Check(repoWith(ok)), "AUTOMIGRATE")
}

func TestGoVersion_Mismatch(t *testing.T) {
	r := repoWith()
	r.Mods[1].GoVersion = "1.26"
	assertViolation(t, Check(r), "GOVERSION")
	r = repoWith()
	r.Mods[1].Toolchain = "go1.26.0"
	assertViolation(t, Check(r), "GOVERSION")
}

func TestGoVersion_ExactValues(t *testing.T) {
	r := repoWith()
	for i := range r.Mods {
		r.Mods[i].GoVersion, r.Mods[i].Toolchain = "1.28", "go1.28.0" // seragam tetapi bukan nilai wajib
	}
	assertViolation(t, Check(r), "GOVERSION")
	r = repoWith()
	r.Work = &Work{Enabled: true, GoVersion: "1.27", Toolchain: "go1.27.2"}
	assertViolation(t, Check(r), "GOVERSION")
	r.Work = &Work{Enabled: true, GoVersion: "1.26", Toolchain: "go1.27.3"}
	assertViolation(t, Check(r), "GOVERSION")
	r.Work = &Work{Enabled: true, GoVersion: "1.27", Toolchain: "go1.27.3"}
	assertNone(t, Check(r), "GOVERSION")
}

func TestGoWork_CompletenessAndEnabled(t *testing.T) {
	r := repoWith()
	r.Work = &Work{Enabled: true, GoVersion: "1.27", Toolchain: "go1.27.3",
		Use: []string{"/r", "/r/sdk"}, ModDirs: []string{"/r/sdk", "/r/modules/baru"}}
	vs := Check(r)
	assertViolation(t, vs, "GOWORK")
	if !strings.Contains(vs[0].Detail+vs[0].Package, "baru") {
		t.Fatalf("pesan harus menyebut modul yang hilang: %+v", vs)
	}
	r.Work.Use = append(r.Work.Use, "/r/modules/baru")
	assertNone(t, Check(r), "GOWORK")

	r.Work = &Work{Enabled: false}
	vs = Check(r)
	assertViolation(t, vs, "GOWORK")
	if !strings.Contains(vs[0].Detail, "workspace") {
		t.Fatalf("pesan kurang jelas: %+v", vs)
	}
}

func TestF2_WebDependenciesMustMatchManifest(t *testing.T) {
	r := repoWith()
	m := r.Manifests[addon]
	m.Dependencies = append(m.Dependencies, "acme.example")
	r.Manifests[addon] = m
	r.Mods[3].Requires = []string{sdk, example}

	r.Web = map[string]WebPackage{addon: {Name: "@erpora/module-exampleaddon", ModuleDeps: []string{"@erpora/module-example"}}}
	assertNone(t, Check(r), "F2")

	// dependency package.json yang tidak ada di manifest
	r.Web = map[string]WebPackage{addon: {ModuleDeps: []string{"@erpora/module-example", "@erpora/module-lain"}}}
	assertViolation(t, Check(r), "F2")
	// dependency manifest yang tidak ada di package.json
	r.Web = map[string]WebPackage{addon: {ModuleDeps: nil}}
	assertViolation(t, Check(r), "F2")
}

func TestParseWebPackage_Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/web_f2/package.json")
	if err != nil {
		t.Fatal(err)
	}
	wp, ok, err := parseWebPackage(data)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	want := []string{"@erpora/module-example", "@erpora/module-lain"}
	if strings.Join(wp.ModuleDeps, ",") != strings.Join(want, ",") {
		t.Fatalf("ModuleDeps = %v, want %v", wp.ModuleDeps, want)
	}
	// Fixture ini melanggar F2 terhadap manifest exampleaddon yang hanya bergantung pada acme.example.
	r := repoWith()
	m := r.Manifests[addon]
	m.Dependencies = append(m.Dependencies, "acme.example")
	r.Manifests[addon] = m
	r.Mods[3].Requires = []string{sdk, example}
	r.Web = map[string]WebPackage{addon: wp}
	assertViolation(t, Check(r), "F2")
}

func TestCheck_CleanRepoHasNoViolations(t *testing.T) {
	r := repoWith(
		pkg(root+"/internal/app", example),
		pkg(root+"/internal/platform/database", "gorm.io/driver/postgres", "gorm.io/gorm"),
		pkg(example+"/internal/domain", "errors"),
		pkg(example+"/internal/application", example+"/internal/domain"),
		pkg(example+"/internal/infrastructure/persistence", "gorm.io/gorm"),
		pkg(sdk+"/module", "context"),
	)
	if vs := Check(r); len(vs) != 0 {
		t.Fatalf("expected none, got %+v", vs)
	}
}
