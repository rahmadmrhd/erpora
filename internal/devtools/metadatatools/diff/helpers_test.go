package diff

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/internal/devtools/metadatatools/sqlgen"
)

// fixtureModel menurunkan model dari modul fixture (testdata/repo) hanya-baca.
func fixtureModel(t testing.TB) *dbmodel.Schema {
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
	s, ps := dbmodel.Derive(res.Resources)
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	return s
}

// gooseUp menyusun isi berkas migrasi goose dari pernyataan bangkitan. Pernyataan fungsi dibungkus
// StatementBegin/End karena berisi titik koma.
func gooseUp(stmts []sqlgen.Statement, extra ...string) string {
	var sb strings.Builder
	sb.WriteString("-- +goose Up\n")
	for _, s := range stmts {
		if s.Kind == sqlgen.KindCreateFunction {
			sb.WriteString("-- +goose StatementBegin\n" + s.SQL + ";\n-- +goose StatementEnd\n")
			continue
		}
		sb.WriteString(s.SQL + ";\n")
	}
	for _, e := range extra {
		sb.WriteString(e + "\n")
	}
	sb.WriteString("-- +goose Down\nSELECT 1;\n")
	return sb.String()
}

// moduleFS membuat FS migrasi dari pasangan nama berkas dan isi.
func moduleFS(files map[string]string) fstest.MapFS {
	fs := fstest.MapFS{}
	for n, c := range files {
		fs[n] = &fstest.MapFile{Data: []byte(c)}
	}
	return fs
}

// fixtureSources membuat sumber migrasi per modul fixture dari DDL bangkitan model, ditambah migrasi
// tambahan (mutasi) untuk modul tertentu.
func fixtureSources(t testing.TB, model *dbmodel.Schema, extra map[string]map[string]string) []Source {
	t.Helper()
	var out []Source
	for _, slug := range []string{"fixturecore", "fixturesales"} {
		stmts, err := sqlgen.CreateSchema(model.ForModule(slug), sqlgen.Options{})
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{"0001_init.sql": gooseUp(stmts)}
		for n, c := range extra[slug] {
			files[n] = c
		}
		out = append(out, Source{Slug: slug, Migrations: moduleFS(files)})
	}
	return out
}

// addLockTrigger memastikan tabel invoice fixture sudah punya pemicu penguncian bangkitan (Derive, task 13).
func addLockTrigger(t testing.TB, model *dbmodel.Schema) *dbmodel.Table {
	t.Helper()
	tb := model.Table("fixturesales_invoices")
	if tb == nil || len(tb.Triggers) == 0 {
		t.Fatal("tabel invoice fixture harus punya pemicu penguncian bangkitan")
	}
	return tb
}

func findingKinds(r *Report) []string {
	var ks []string
	for _, f := range r.Findings {
		ks = append(ks, f.Kind)
	}
	return ks
}
