package dbmodel

import (
	"os"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

type file struct{ name, src string }

func f(name, src string) file { return file{name, src} }

func header(kind, key string) string {
	return "schemaVersion: \"1\"\nkind: " + kind + "\nkey: " + key + "\n"
}

func resourceDoc(key, body string) string {
	return header("resource", key) + "include:\n  - source: sdk:audit\n" + body
}

func resourceNoAudit(key, body string) string { return header("resource", key) + body }

func mustDoc(t *testing.T, name, src string) *load.Document {
	t.Helper()
	d, ps := load.Parse(name, []byte(src))
	if len(ps) != 0 {
		t.Fatalf("berkas uji %s tidak valid: %v\n%s", name, ps, src)
	}
	return d
}

type mod struct {
	slug string
	deps []string
	docs []file
}

// resolveMods membangun katalog (dengan metadata bawaan SDK), menyelesaikan semuanya, dan mengembalikan
// semua resource.
func resolveMods(t *testing.T, mods ...mod) []*resolve.Resolved {
	t.Helper()
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	for _, m := range mods {
		var docs []*load.Document
		for _, fl := range m.docs {
			docs = append(docs, mustDoc(t, fl.name, fl.src))
		}
		if aps := c.AddModule(resolve.Module{Slug: m.slug, Dependencies: m.deps}, docs...); len(aps) != 0 {
			t.Fatal(aps)
		}
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatalf("temuan resolusi: %v", res.Problems)
	}
	return res.Resources
}

// deriveOne menurunkan model satu modul "m" dari dokumen resource; gagal bila ada temuan.
func deriveOne(t *testing.T, docs ...file) *Schema {
	t.Helper()
	s, ps := Derive(resolveMods(t, mod{slug: "m", docs: docs}))
	if len(ps) != 0 {
		t.Fatalf("Derive: %v", ps)
	}
	return s
}

// deriveProblems menurunkan model dan mengembalikan temuan (tanpa gagal).
func deriveProblems(t *testing.T, mods ...mod) []load.Problem {
	t.Helper()
	_, ps := Derive(resolveMods(t, mods...))
	return ps
}

func hasCode(ps []load.Problem, code string) bool {
	for _, p := range ps {
		if p.Code == code {
			return true
		}
	}
	return false
}

func table(t *testing.T, s *Schema, name string) *Table {
	t.Helper()
	tb := s.Table(name)
	if tb == nil {
		var names []string
		for _, x := range s.Tables {
			names = append(names, x.Name)
		}
		t.Fatalf("tabel %q tidak ada; ada: %s", name, strings.Join(names, ","))
	}
	return tb
}

func col(t *testing.T, tb *Table, name string) *Column {
	t.Helper()
	c := tb.Column(name)
	if c == nil {
		var names []string
		for _, x := range tb.Columns {
			names = append(names, x.Name)
		}
		t.Fatalf("kolom %s.%s tidak ada; ada: %s", tb.Name, name, strings.Join(names, ","))
	}
	return c
}

// fixtureResources memuat modul fixture (testdata/repo) hanya-baca.
func fixtureResources(t *testing.T) []*resolve.Resolved {
	t.Helper()
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	root := os.DirFS("../testdata/repo/modules")
	for _, m := range []struct {
		mod  resolve.Module
		path string
	}{
		{resolve.Module{Slug: "fixturecore"}, "fixturecore/metadata"},
		{resolve.Module{Slug: "fixturesales", Dependencies: []string{"fixturecore"}}, "fixturesales/metadata"},
	} {
		docs, lps := load.LoadFS(root, m.path)
		if len(lps) != 0 {
			t.Fatal(lps)
		}
		if aps := c.AddModule(m.mod, docs...); len(aps) != 0 {
			t.Fatal(aps)
		}
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatal(res.Problems)
	}
	return res.Resources
}
