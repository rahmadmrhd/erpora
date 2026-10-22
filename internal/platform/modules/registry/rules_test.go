package registry

import (
	"context"
	"errors"
	"strings"
	"testing"

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

func TestRules_Capabilities(t *testing.T) {
	_, err := Build(testOpts(),
		newFake(spec("acme.a", "a", "a.x.read"), nil),
		newFake(spec("acme.b", "b", "a.x.read"), nil))
	p := expect(t, problemsOf(t, err), "duplicate_capability", "acme.b")
	if !strings.Contains(p.Message, "acme.a") || !strings.Contains(p.Message, "acme.b") {
		t.Errorf("Message harus menyebut kedua modul: %q", p.Message)
	}

	_, err = Build(testOpts(), newFake(spec("acme.example", "example", "example-addon.summary.read"), nil))
	expect(t, problemsOf(t, err), "capability_prefix", "acme.example")

	_, err = Build(testOpts(), newFake(spec("acme.example", "example", "other.x.read"), nil))
	expect(t, problemsOf(t, err), "capability_prefix", "acme.example")
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
	for _, slug := range []string{"settings", "db", "assets"} {
		_, err := Build(testOpts(), newFake(spec("acme.x", slug), nil))
		expect(t, problemsOf(t, err), "reserved_slug", "acme.x")
	}
}

func TestRules_InvalidRoute(t *testing.T) {
	paths := []string{"x", "", "/../x", "/a/../b", "//x", "example.com/x"}
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
		"nama besar":  {Name: "Recount", Run: run},
		"nama kosong": {Name: "", Run: run},
		"run nil":     {Name: "recount"},
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
