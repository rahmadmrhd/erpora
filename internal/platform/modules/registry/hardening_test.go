package registry

import (
	"net/http"
	"strings"
	"testing"

	"erpora/internal/platform/modules/modtest"
	"erpora/sdk/module"
)

// M4: setiap modul wajib bergantung (non-opsional) pada platform.core.
func TestRules_PlatformDependencyRequired(t *testing.T) {
	cases := map[string][]module.Dependency{
		"kosong":   {},
		"opsional": {{Module: PlatformKey, Version: "^0.1", Optional: true}},
	}
	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
			s := manifestSpec{key: "acme.a", slug: "a", deps: deps}
			if deps != nil && len(deps) == 0 {
				s.deps = []module.Dependency{}
			}
			_, err := Build(testOpts(), newFake(s, nil))
			p := expect(t, problemsOf(t, err), "missing_dependency", "acme.a")
			if !strings.Contains(p.Message, PlatformKey) {
				t.Errorf("Message harus menyebut platform.core: %q", p.Message)
			}
		})
	}
}

// M5: panic, modul nil, dan handler nil menjadi problem, bukan crash.
func TestBuild_RegisterPanicBecomesProblem(t *testing.T) {
	_, err := Build(testOpts(), newFake(spec("acme.a", "a"), func(module.Registrar) error { panic("boom") }))
	p := expect(t, problemsOf(t, err), "register_failed", "acme.a")
	if !strings.Contains(p.Message, "panic") {
		t.Errorf("Message = %q", p.Message)
	}
}

type panicManifestModule struct{}

func (panicManifestModule) Manifest() module.Manifest       { panic("manifest rusak") }
func (panicManifestModule) Register(module.Registrar) error { return nil }

func TestBuild_ManifestPanicBecomesProblem(t *testing.T) {
	_, err := Build(testOpts(), panicManifestModule{})
	if _, ok := findProblem(problemsOf(t, err), "register_failed"); !ok {
		t.Fatalf("register_failed diharapkan: %v", err)
	}
}

func TestBuild_NilModuleBecomesProblem(t *testing.T) {
	_, err := Build(testOpts(), nil)
	if _, ok := findProblem(problemsOf(t, err), "register_failed"); !ok {
		t.Fatalf("problem untuk modul nil diharapkan: %v", err)
	}
}

func TestBuild_NilHandlerIsInvalidRoute(t *testing.T) {
	for name, fn := range map[string]func(r module.Registrar){
		"Handle": func(r module.Registrar) { r.Handle("GET", "/x", "a.x.read", nil) },
		"Public": func(r module.Registrar) { r.Public("GET", "/x", nil) },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(fn)))
			expect(t, problemsOf(t, err), "invalid_route", "acme.a")
		})
	}
}

// M11: route platform.core melewati aturan permission yang sama (capability dideklarasikan lewat Options).
func TestBuild_PlatformRoutesRequireDeclaredCapability(t *testing.T) {
	opts := testOpts()
	opts.Platform = func(r module.Registrar) error {
		r.Handle("GET", "/modules", "platform.modules.read", okHandler)
		return nil
	}
	_, err := Build(opts)
	expect(t, problemsOf(t, err), "permission_undeclared", PlatformKey)

	opts.PlatformResources = modtest.Resources("platform.modules.read")
	reg, err := Build(opts)
	if err != nil {
		t.Fatal(err)
	}
	rs := reg.Routes()
	if len(rs) != 1 || rs[0].Pattern != "/api/platform/modules" || rs[0].Module != PlatformKey {
		t.Fatalf("routes = %+v", rs)
	}
	if got := reg.Permissions(); len(got) != 1 || got[0] != "platform.modules.read" {
		t.Errorf("Permissions = %v", got)
	}
}

// M12: klasifikasi konflik tidak boleh bergantung pada teks panic ServeMux.
func TestRouteConflict_IndependentOfPanicText(t *testing.T) {
	orig := muxRegister
	muxRegister = func(mux *http.ServeMux, rt RouteInfo) string {
		return strings.ReplaceAll(orig(mux, rt), "conflicts with", "kolidiert dengan")
	}
	defer func() { muxRegister = orig }()

	_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(func(r module.Registrar) {
		r.Handle("GET", "/items", "a.x.read", okHandler)
		r.Handle("GET", "/items", "a.x.read", okHandler)
	})))
	p := expect(t, problemsOf(t, err), "route_conflict", "acme.a")
	if strings.Count(p.Message, "/api/a/items") < 2 {
		t.Errorf("Message harus menyebut kedua pendaftaran: %q", p.Message)
	}
}

// Resource platform tunduk pada aturan yang sama dengan modul: awalan slug, keunikan lintas modul, grup default.
func TestBuild_PlatformResourceRules(t *testing.T) {
	opts := testOpts()
	opts.PlatformResources = modtest.Resources("example.x.read")
	_, err := Build(opts)
	p := expect(t, problemsOf(t, err), "invalid_resource", PlatformKey)
	if !strings.Contains(p.Message, "platform.") {
		t.Errorf("Message harus menyebut awalan platform.: %q", p.Message)
	}

	opts.PlatformResources = modtest.Resources("platform.modules.read")
	opts.PlatformResources[0].FieldGroups[0].Default = false
	_, err = Build(opts)
	p = expect(t, problemsOf(t, err), "invalid_resource", PlatformKey)
	if !strings.Contains(p.Message, "grup default") {
		t.Errorf("resource platform tanpa grup default harus ditolak: %q", p.Message)
	}

	opts.PlatformResources = modtest.Resources("platform.modules.read")
	if _, err := Build(opts); err != nil {
		t.Fatalf("resource platform valid ditolak: %v", err)
	}
}
