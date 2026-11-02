package migrate

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/diff"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

type file struct{ name, src string }

func f(name, src string) file { return file{name, src} }

func resourceDoc(key, body string) string {
	return "schemaVersion: \"1\"\nkind: resource\nkey: " + key + "\n" + body
}

// modelOf menurunkan model modul "m" dari dokumen resource (sdk:audit otomatis).
func modelOf(t testing.TB, docs ...file) *dbmodel.Schema {
	t.Helper()
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	var ds []*load.Document
	for _, fl := range docs {
		d, lps := load.Parse(fl.name, []byte(fl.src))
		if len(lps) != 0 {
			t.Fatalf("%s: %v\n%s", fl.name, lps, fl.src)
		}
		ds = append(ds, d)
	}
	if aps := c.AddModule(resolve.Module{Slug: "m"}, ds...); len(aps) != 0 {
		t.Fatal(aps)
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatal(res.Problems)
	}
	s, dps := dbmodel.Derive(res.Resources)
	if len(dps) != 0 {
		t.Fatal(dps)
	}
	return s
}

func report(fs ...diff.Finding) *diff.Report { return &diff.Report{Findings: fs} }

func runGen(t testing.TB, model *dbmodel.Schema, fs ...diff.Finding) *Result {
	t.Helper()
	r, err := Generate(report(fs...), model, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return r
}

// section memotong isi berkas menjadi bagian Up dan Down.
func section(t testing.TB, sql string) (up, down string) {
	t.Helper()
	i := strings.Index(sql, "-- +goose Up\n")
	j := strings.Index(sql, "-- +goose Down\n")
	if i < 0 || j < i {
		t.Fatalf("berkas tanpa Up dan Down berurutan:\n%s", sql)
	}
	return sql[i+len("-- +goose Up\n") : j], sql[j+len("-- +goose Down\n"):]
}

func contains(t testing.TB, hay string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(hay, w) {
			t.Errorf("tidak memuat %q:\n%s", w, hay)
		}
	}
}

func notContains(t testing.TB, hay string, bads ...string) {
	t.Helper()
	for _, b := range bads {
		if strings.Contains(hay, b) {
			t.Errorf("tidak boleh memuat %q:\n%s", b, hay)
		}
	}
}

// activeLines mengembalikan baris bukan-komentar dan bukan-kosong.
func activeLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "--") {
			out = append(out, l)
		}
	}
	return out
}

const thingDoc = `
fields:
  title: {type: string, minLength: 1, maxLength: 200, label: m.thing.title}
  note:  {type: text, nullable: true, label: m.thing.note}
  kind:  {type: enum, values: [a, b, c], extensible: true, default: a, label: m.thing.kind}
  strict: {type: enum, values: [x, y], default: x, label: m.thing.strict}
  qty:   {type: integer, minimum: 1, maximum: 100, default: 1, label: m.thing.qty}
`

func thingModel(t testing.TB) *dbmodel.Schema {
	return modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", thingDoc)))
}

// lockedModel membuat model resource dokumen m.doc; Derive membuat pemicu penguncian m_docs_lock_trg.
func lockedModel(t testing.TB) *dbmodel.Schema {
	t.Helper()
	m := modelOf(t, f("m/doc.resource.yaml", resourceDoc("m.doc", "include:\n  - source: sdk:document\nfields:\n  n: {type: integer, label: m.doc.n}\n")))
	tb := m.Table("m_docs")
	if tb == nil || tb.Lock == nil || len(tb.Triggers) != 1 {
		t.Fatal("m_docs harus punya lockWhen dan satu pemicu penguncian bangkitan")
	}
	return m
}
