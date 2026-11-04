package registry

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"erpora/internal/platform/modules/modtest"
	"erpora/sdk/module"
)

func problemsOf(t *testing.T, err error) []Problem {
	t.Helper()
	var be *Error
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, diharapkan *Error", err)
	}
	return be.Problems
}

func expect(t *testing.T, ps []Problem, code, mod string) Problem {
	t.Helper()
	for _, p := range ps {
		if p.Code == code && p.Module == mod {
			return p
		}
	}
	t.Fatalf("problem {%s, %s} tidak ditemukan di %+v", code, mod, ps)
	return Problem{}
}

func spec(key, slug string, caps ...string) manifestSpec {
	return manifestSpec{key: key, slug: slug, caps: caps}
}

func registerWith(f func(r module.Registrar)) func(module.Registrar) error {
	return func(r module.Registrar) error { f(r); return nil }
}

func TestRules_DuplicateKeyAndSlug(t *testing.T) {
	_, err := Build(testOpts(), newFake(spec("acme.a", "a"), nil), newFake(spec("acme.a", "b"), nil))
	expect(t, problemsOf(t, err), "duplicate_key", "acme.a")

	_, err = Build(testOpts(), newFake(spec("acme.a", "same"), nil), newFake(spec("acme.b", "same"), nil))
	expect(t, problemsOf(t, err), "duplicate_slug", "acme.b")
}

func TestRules_InvalidManifestKeepsSchemaCode(t *testing.T) {
	_, err := Build(testOpts(), newFake(manifestSpec{key: "acme.a", slug: "a", version: "1.0"}, nil))
	p := expect(t, problemsOf(t, err), "schema_invalid", "acme.a")
	if p.Field != "/version" {
		t.Errorf("Field = %q", p.Field)
	}
}

func TestRules_ResourceKeys(t *testing.T) {
	// Kunci resource unik lintas modul: modul b mendeklarasikan resource milik slug a.
	b := newFake(spec("acme.b", "b"), nil)
	b.m = withResources(t, b.m, modtest.Resources("a.x.read"))
	_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), nil), b)
	ps := problemsOf(t, err)
	p := expect(t, ps, "invalid_resource", "acme.b")
	if !strings.Contains(p.Message, "a.x") {
		t.Errorf("Message harus menyebut kunci resource: %q", p.Message)
	}

	// Awalan slug: example-addon.summary di bawah slug example.
	e := newFake(spec("acme.example", "example"), nil)
	e.m = withResources(t, e.m, modtest.Resources("example-addon.summary.read"))
	_, err = Build(testOpts(), e)
	expect(t, problemsOf(t, err), "invalid_resource", "acme.example")

	// Capability tulis-tangan di luar resource ditolak.
	c := newFake(spec("acme.example", "example", "example.notes.read"), nil)
	c.m = withResources(t, c.m, nil)
	_, err = Build(testOpts(), c)
	expect(t, problemsOf(t, err), "capability_not_derived", "acme.example")
}

// withResources membangun ulang manifest m dengan bagian resources (JSON mentah ikut berubah agar lolos checkMatchesRaw).
func withResources(t *testing.T, m module.Manifest, rs []module.Resource) module.Manifest {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(m.Raw, &doc); err != nil {
		t.Fatal(err)
	}
	if rs == nil {
		delete(doc, "resources")
	} else {
		doc["resources"] = rs
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return module.MustParseManifest(b)
}

func TestRules_Permissions(t *testing.T) {
	_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(func(r module.Registrar) {
		r.Handle("GET", "/x", "", okHandler)
	})))
	expect(t, problemsOf(t, err), "permission_missing", "acme.a")

	_, err = Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(func(r module.Registrar) {
		r.Handle("GET", "/x", "a.y.write", okHandler)
	})))
	expect(t, problemsOf(t, err), "permission_undeclared", "acme.a")
}

func TestRules_RouteConflicts(t *testing.T) {
	cases := map[string]func(r module.Registrar){
		"duplicate": func(r module.Registrar) {
			r.Handle("GET", "/items", "a.x.read", okHandler)
			r.Handle("GET", "/items", "a.x.read", okHandler)
		},
		"ambiguous": func(r module.Registrar) {
			r.Handle("GET", "/{a}/x", "a.x.read", okHandler)
			r.Handle("GET", "/x/{b}", "a.x.read", okHandler)
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(fn)))
			p := expect(t, problemsOf(t, err), "route_conflict", "acme.a")
			if strings.Count(p.Message, "/api/a/") < 2 || !strings.Contains(p.Message, "acme.a") {
				t.Errorf("Message harus menyebut kedua pendaftaran: %q", p.Message)
			}
		})
	}
}

func TestRules_RoutesReadOnly(t *testing.T) {
	reg, err := Build(testOpts(), notes(registerWith(func(r module.Registrar) {
		r.Handle("GET", "/items", "notes.items.read", okHandler)
	})))
	if err != nil {
		t.Fatal(err)
	}
	reg.Routes()[0].Pattern = "diubah"
	_ = append(reg.Routes()[:0], RouteInfo{Pattern: "lain"})
	if got := reg.Routes()[0].Pattern; got != "/api/notes/items" {
		t.Errorf("Pattern = %q", got)
	}
	reg.Commands()
	reg.Entries()[0] = Entry{}
	if reg.Entries()[0].Manifest.Key != PlatformKey {
		t.Error("Entries bukan salinan")
	}
}

func TestBuild_ReportsAllProblemsAtOnce(t *testing.T) {
	missingDep := manifestSpec{key: "acme.c", slug: "c", deps: []module.Dependency{{Module: PlatformKey, Version: "^0.1"}, {Module: "acme.none", Version: "^1"}}}
	_, err := Build(testOpts(),
		newFake(spec("acme.a", "same", "same.x.read"), registerWith(func(r module.Registrar) {
			r.Handle("GET", "/x", "", okHandler)
		})),
		newFake(spec("acme.b", "same"), nil),
		newFake(missingDep, nil))
	ps := problemsOf(t, err)
	expect(t, ps, "duplicate_slug", "acme.b")
	expect(t, ps, "permission_missing", "acme.a")
	expect(t, ps, "missing_dependency", "acme.c")
	if !strings.Contains(err.Error(), "\n") {
		t.Error("Error() harus satu baris per problem")
	}
}

func TestRules_ReservedSlug(t *testing.T) {
	// Daftar lengkap keputusan rencana 4 / ADR-006 keputusan 7, ditambah "ui" (namespace i18n) "sdk" (package @erpora/module-sdk), "desks" (namespace /desks/<slug-desk> Desk kustom, revisi M3a), serta "login", "logout", "forgot-password", dan "reset-password" (gerbang autentikasi pra-bootstrap, PRD-001 Q33); keputusan pemilik produk.
	slugs := []string{
		"platform", "api", "assets", "settings", "bootstrap", "healthz", "readyz", "sw", "make", "db",
		"module", "modules", "contracts", "route", "permission", "migrate", "serve", "inspect",
		"version", "list", "doctor", "completion", "help", "ui", "sdk", "desks",
		// Gerbang autentikasi pra-bootstrap milik shell di jalur tingkat atas (PRD-001 Q33 butir 2).
		"login", "logout", "forgot-password", "reset-password",
	}
	if len(slugs) != 30 {
		t.Fatalf("daftar uji harus 30 slug, ada %d", len(slugs))
	}
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			_, err := Build(testOpts(), newFake(spec("acme.x", slug), nil))
			expect(t, problemsOf(t, err), "reserved_slug", "acme.x")
		})
	}
}

func TestRules_InvalidRoute(t *testing.T) {
	// Tanpa trailing slash (api-guidelines §2): "/" dan "/x/" menjadi subtree ServeMux, jadi ditolak.
	paths := []string{"x", "", "/../x", "/a/../b", "//x", "example.com/x", "/", "/items/"}
	for _, p := range paths {
		t.Run("path="+p, func(t *testing.T) {
			_, err := Build(testOpts(), newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) {
				r.Public("GET", p, okHandler)
			})))
			expect(t, problemsOf(t, err), "invalid_route", "acme.a")
		})
	}
	t.Run("method", func(t *testing.T) {
		_, err := Build(testOpts(), newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) {
			r.Public("FETCH", "/x", okHandler)
		})))
		expect(t, problemsOf(t, err), "invalid_route", "acme.a")
	})
	t.Run("pola mux rusak tidak panic", func(t *testing.T) {
		_, err := Build(testOpts(), newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) {
			r.Public("GET", "/{broken", okHandler)
		})))
		expect(t, problemsOf(t, err), "invalid_route", "acme.a")
	})
}

func TestRules_Commands(t *testing.T) {
	run := func(context.Context, module.Invocation) error { return nil }
	for name, c := range map[string]module.Command{
		"nama besar":    {Name: "Recount", Run: run},
		"nama kosong":   {Name: "", Run: run},
		"run nil":       {Name: "recount"},
		"dua grup":      {Name: "a:b:c", Run: run},
		"akhiran titik": {Name: "settings:", Run: run},
		"awalan titik":  {Name: ":set", Run: run},
		"segmen besar":  {Name: "settings:Set", Run: run},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Build(testOpts(), newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) { r.Command(c) })))
			expect(t, problemsOf(t, err), "invalid_command", "acme.a")
		})
	}
	_, err := Build(testOpts(), newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) {
		r.Command(module.Command{Name: "recount", Run: run})
		r.Command(module.Command{Name: "recount", Run: run})
	})))
	expect(t, problemsOf(t, err), "command_conflict", "acme.a")
}

// Satu segmen grup `<grup>:<aksi>` diizinkan (C1.12: core:settings:set, PRD-001 Q24); nama tercatat `<slug>:<grup>:<aksi>`.
func TestRules_CommandWithGroupSegment(t *testing.T) {
	run := func(context.Context, module.Invocation) error { return nil }
	reg, err := Build(testOpts(), newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) {
		r.Command(module.Command{Name: "settings:set", Run: run})
	})))
	if err != nil {
		t.Fatal(err)
	}
	cmds := reg.Commands()
	if len(cmds) != 1 || cmds[0].Name != "a:settings:set" {
		t.Fatalf("commands = %+v", cmds)
	}
}

// Pola rusak yang datang SETELAH route valid harus invalid_route, bukan route_conflict.
func TestRules_BrokenPatternAfterValidRouteIsInvalidNotConflict(t *testing.T) {
	_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(func(r module.Registrar) {
		r.Handle("GET", "/items", "a.x.read", okHandler)
		r.Handle("GET", "/{broken", "a.x.read", okHandler)
	})))
	ps := problemsOf(t, err)
	expect(t, ps, "invalid_route", "acme.a")
	if _, bad := findProblem(ps, "route_conflict"); bad {
		t.Fatalf("pola rusak tidak boleh dilaporkan sebagai route_conflict: %v", ps)
	}
}

// FR-110: permission route wajib berupa <resource>.<aksi> yang dideklarasikan di resources manifest.
func TestRouteMustReferenceDeclaredResourceAction(t *testing.T) {
	// Resource a.x hanya mendeklarasikan read; update tidak terdeklarasi.
	_, err := Build(testOpts(), newFake(spec("acme.a", "a", "a.x.read"), registerWith(func(r module.Registrar) {
		r.Handle("GET", "/x", "a.x.update", okHandler)
	})))
	p := expect(t, problemsOf(t, err), "permission_undeclared", "acme.a")
	if !strings.Contains(p.Message, "a.x.update") || !strings.Contains(p.Message, "resources") {
		t.Errorf("Message harus menyebut permission dan resources: %q", p.Message)
	}

	// Permission berbentuk capability warisan tanpa resource (a.y.read, resource a.y tidak ada) ditolak.
	legacy := newFake(spec("acme.a", "a"), registerWith(func(r module.Registrar) {
		r.Handle("GET", "/y", "a.y.read", okHandler)
	}))
	_, err = Build(testOpts(), legacy)
	expect(t, problemsOf(t, err), "permission_undeclared", "acme.a")

	// Aksi kustom resource sah sebagai permission route.
	custom := newFake(spec("acme.a", "a", "a.x.read", "a.x.adjust_price"), registerWith(func(r module.Registrar) {
		r.Handle("POST", "/x/adjust", "a.x.adjust_price", okHandler)
	}))
	if _, err := Build(testOpts(), custom); err != nil {
		t.Fatalf("aksi kustom terdeklarasi harus sah: %v", err)
	}
}

// FR-110: capabilities diturunkan otomatis dari resource x aksi, juga pada registry.
func TestRegistryCapabilitiesDerivedFromResources(t *testing.T) {
	m := newFake(spec("acme.a", "a", "a.x.read", "a.x.adjust_price", "a.y.read"), nil)
	m.m = withResources(t, m.m, modtest.Resources("a.x.read", "a.x.adjust_price", "a.y.read"))
	opts := testOpts()
	opts.PlatformResources = modtest.Resources("platform.modules.read")
	reg, err := Build(opts, m)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.x.adjust_price", "a.x.read", "a.y.read", "platform.modules.read"}
	if got := reg.Permissions(); !slices.Equal(got, want) {
		t.Errorf("Permissions = %v, want %v", got, want)
	}
	var keys []string
	for _, r := range reg.Resources() {
		keys = append(keys, r.Key+"@"+r.Module)
	}
	if want := []string{"a.x@acme.a", "a.y@acme.a", "platform.modules@platform.core"}; !slices.Equal(keys, want) {
		t.Errorf("Resources = %v, want %v", keys, want)
	}
}
