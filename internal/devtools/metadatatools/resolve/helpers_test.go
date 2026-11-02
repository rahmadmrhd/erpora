package resolve

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/load"
)

// header menyusun kepala berkas metadata uji.
func header(kind, key string) string {
	return "schemaVersion: \"1\"\nkind: " + kind + "\nkey: " + key + "\n"
}

func resource(key, body string) string { return header("resource", key) + body }
func abstract(key, body string) string { return header("abstract", key) + body }
func fieldType(key, body string) string {
	return header("fieldType", key) + body
}

type file struct{ name, src string }

func f(name, src string) file { return file{name, src} }

func mustDoc(t *testing.T, name, src string) *load.Document {
	t.Helper()
	d, ps := load.Parse(name, []byte(src))
	if len(ps) != 0 {
		t.Fatalf("berkas uji %s tidak valid: %v", name, ps)
	}
	return d
}

// addModule menambah modul ke katalog dan gagal bila ada temuan.
func addModule(t *testing.T, c *Catalog, m Module, files ...file) {
	t.Helper()
	var docs []*load.Document
	for _, fl := range files {
		docs = append(docs, mustDoc(t, fl.name, fl.src))
	}
	if ps := c.AddModule(m, docs...); len(ps) != 0 {
		t.Fatalf("AddModule(%s): %v", m.Slug, ps)
	}
}

// sdkFiles adalah versi mini metadata bawaan SDK (identitas kunci sdk.audit/sdk.document yang dilindungi).
func sdkFiles() []file {
	return []file{
		f("sdk/audit.abstract.yaml", abstract("sdk.audit", `export: true
fields:
  id: {type: uuid, readOnly: true}
  version: {type: integer, readOnly: true}
  createdAt: {type: datetime, readOnly: true}
  createdBy: {type: uuid, readOnly: true}
`)),
		f("sdk/document.abstract.yaml", abstract("sdk.document", `export: true
include:
  - source: sdk:audit
fields:
  status: {type: enum, values: [draft, deleted, need_approval, accepted, rejected, cancelled, done], readOnly: true, default: draft}
  amendedFromId: {type: uuid, nullable: true, readOnly: true}
lockWhen: {field: status, op: ne, value: draft}
`)),
		f("sdk/email.type.yaml", fieldType("sdk.email", `export: true
base: {type: string, maxLength: 254, format: email}
classification: confidential
widget: email
operators: [eq, contains, isnull]
`)),
		f("sdk/money.type.yaml", fieldType("sdk.money", `export: true
base: {type: decimal, precision: 18, scale: 2}
classification: internal
widget: money
display: money
`)),
	}
}

func newCatalog(t *testing.T, mods ...func(*Catalog)) *Catalog {
	t.Helper()
	c := NewCatalog()
	addModule(t, c, Module{Slug: "sdk"}, sdkFiles()...)
	for _, m := range mods {
		m(c)
	}
	return c
}

func hasCode(ps []load.Problem, code string) bool {
	for _, p := range ps {
		if p.Code == code {
			return true
		}
	}
	return false
}

func findProblem(ps []load.Problem, code string) *load.Problem {
	for i := range ps {
		if ps[i].Code == code {
			return &ps[i]
		}
	}
	return nil
}

func fieldNames(r *Resolved) []string {
	out := make([]string, len(r.Fields))
	for i, fl := range r.Fields {
		out[i] = fl.Name
	}
	return out
}

func joined(r *Resolved) string { return strings.Join(fieldNames(r), ",") }

func mustResolve(t *testing.T, c *Catalog, key string) *Resolved {
	t.Helper()
	r, ps := c.Resolve(key)
	if len(ps) != 0 || r == nil {
		t.Fatalf("Resolve(%s): %v", key, ps)
	}
	return r
}
