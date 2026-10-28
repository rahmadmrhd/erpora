package registry

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"

	"erpora/sdk/module"
)

const (
	testPlatformVersion = "0.1.0"
	testSDKVersion      = "0.1.0"
)

func testOpts() Options {
	return Options{PlatformVersion: testPlatformVersion, SDKVersion: testSDKVersion}
}

// fakeModule adalah module.Module uji dengan fungsi register yang dapat diatur.
type fakeModule struct {
	m        module.Manifest
	register func(r module.Registrar) error
}

func (f fakeModule) Manifest() module.Manifest { return f.m }
func (f fakeModule) Register(r module.Registrar) error {
	if f.register == nil {
		return nil
	}
	return f.register(r)
}

type manifestSpec struct {
	key, slug, version string
	caps               []string
	deps               []module.Dependency
	strategy           string // default "none"
}

// newFake membuat fakeModule dari manifest JSON inline yang valid terhadap schema.
func newFake(s manifestSpec, register func(r module.Registrar) error) fakeModule {
	if s.strategy == "" {
		s.strategy = "none"
	}
	if s.version == "" {
		s.version = "1.0.0"
	}
	if s.caps == nil {
		s.caps = []string{}
	}
	if s.deps == nil {
		s.deps = []module.Dependency{{Module: PlatformKey, Version: "^0.1"}}
	}
	doc := map[string]any{
		"schema_version": "1", "key": s.key, "slug": s.slug, "name": s.slug, "version": s.version,
		"publisher":     map[string]string{"id": "acme", "name": "Acme"},
		"trust":         "builtin",
		"compatibility": map[string]string{"platform": "^0.1", "sdk": "^0.1"},
		"dependencies":  s.deps,
		"capabilities":  s.caps,
		"migrations":    map[string]string{"strategy": s.strategy},
		"offline":       map[string]string{"mode": "online-only"},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return fakeModule{m: module.MustParseManifest(b), register: register}
}

func notes(register func(r module.Registrar) error) fakeModule {
	return newFake(manifestSpec{key: "acme.notes", slug: "notes", caps: []string{"notes.items.read"}}, register)
}

var okHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func TestBuild_RoutesPrefixedBySlug(t *testing.T) {
	reg, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Handle("GET", "/items", "notes.items.read", okHandler)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	rs := reg.Routes()
	if len(rs) != 1 {
		t.Fatalf("routes = %+v", rs)
	}
	got := rs[0]
	if got.Module != "acme.notes" || got.Method != "GET" || got.Pattern != "/api/notes/items" || got.Permission != "notes.items.read" || got.Public {
		t.Errorf("route = %+v", got)
	}
}

func TestBuild_PublicRoute(t *testing.T) {
	reg, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Public("GET", "/status", okHandler)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Routes()[0]
	if !got.Public || got.Permission != "" || got.Pattern != "/api/notes/status" {
		t.Errorf("route = %+v", got)
	}
}

func TestBuild_CommandsPrefixed(t *testing.T) {
	reg, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Command(module.Command{Name: "recount", Run: func(context.Context, module.Invocation) error { return nil }})
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cs := reg.Commands(); len(cs) != 1 || cs[0].Name != "notes:recount" || cs[0].Module != "acme.notes" {
		t.Errorf("commands = %+v", cs)
	}
}

func TestBuild_PlatformCoreAlwaysPresent(t *testing.T) {
	reg, err := Build(testOpts())
	if err != nil {
		t.Fatal(err)
	}
	e, ok := reg.Entry(PlatformKey)
	if !ok || e.Manifest.Version != testPlatformVersion || e.Module != nil {
		t.Fatalf("entry = %+v ok=%v", e, ok)
	}
	if reg.Entries()[0].Manifest.Key != PlatformKey {
		t.Errorf("Entries()[0] = %q", reg.Entries()[0].Manifest.Key)
	}
}

func TestBuild_RegisterErrorReported(t *testing.T) {
	_, err := Build(testOpts(), notes(func(module.Registrar) error { return errors.New("boom") }))
	var be *Error
	if !errors.As(err, &be) {
		t.Fatalf("err = %v", err)
	}
	p, ok := findProblem(be.Problems, "register_failed")
	if !ok || p.Module != "acme.notes" {
		t.Fatalf("problems = %+v", be.Problems)
	}
}

func TestBuild_RegistersInTopologicalOrder(t *testing.T) {
	var order []string
	rec := func(name string) func(module.Registrar) error {
		return func(module.Registrar) error { order = append(order, name); return nil }
	}
	a := newFake(manifestSpec{key: "acme.a", slug: "a", deps: []module.Dependency{{Module: "acme.b", Version: "^1"}, {Module: PlatformKey, Version: "^0.1"}}}, rec("acme.a"))
	b := newFake(manifestSpec{key: "acme.b", slug: "b"}, rec("acme.b"))
	if _, err := Build(testOpts(), a, b); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "acme.b" {
		t.Errorf("urutan register = %v", order)
	}
}

func findProblem(ps []Problem, code string) (Problem, bool) {
	for _, p := range ps {
		if p.Code == code {
			return p, true
		}
	}
	return Problem{}, false
}

// migratingModule menambahkan module.MigrationSource pada fakeModule.
type migratingModule struct{ fakeModule }

func (migratingModule) Migrations() fs.FS { return fstest.MapFS{} }

func TestBuild_MigrationsStrategyMustMatchCode(t *testing.T) {
	versioned := newFake(manifestSpec{key: "acme.a", slug: "a", strategy: "versioned"}, nil)
	none := newFake(manifestSpec{key: "acme.a", slug: "a"}, nil)

	_, err := Build(testOpts(), versioned) // versioned tetapi tanpa MigrationSource
	p := expect(t, problemsOf(t, err), "migrations_mismatch", "acme.a")
	if p.Field != "/migrations/strategy" {
		t.Errorf("Field = %q", p.Field)
	}
	_, err = Build(testOpts(), migratingModule{none}) // none tetapi punya MigrationSource
	expect(t, problemsOf(t, err), "migrations_mismatch", "acme.a")

	if _, err := Build(testOpts(), migratingModule{versioned}); err != nil {
		t.Fatalf("versioned + MigrationSource harus valid: %v", err)
	}
	if _, err := Build(testOpts(), none); err != nil {
		t.Fatalf("none tanpa MigrationSource harus valid: %v", err)
	}
}
