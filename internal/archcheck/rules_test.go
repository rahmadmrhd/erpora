package archcheck

import (
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
			{Path: root, GoVersion: "1.27", Toolchain: "go1.27.2", Requires: []string{sdk, example}},
			{Path: sdk, GoVersion: "1.27", Toolchain: "go1.27.2"},
			{Path: example, GoVersion: "1.27", Toolchain: "go1.27.2", Requires: []string{sdk}},
			{Path: addon, GoVersion: "1.27", Toolchain: "go1.27.2", Requires: []string{sdk}},
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
	assertNone(t, Check(r), "D8")
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
