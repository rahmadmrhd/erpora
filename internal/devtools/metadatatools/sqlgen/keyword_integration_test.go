//go:build integration

package sqlgen

import (
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

// Field bernama kata kunci SQL, Go, atau TypeScript (type, order, user, group, limit, default, left, class, let)
// sah (keputusan pemilik produk, 2 Nov 2026): semua identifier SQL dikutip sehingga DDL, CHECK, kolom turunan,
// indeks, pemicu penguncian dokumen (termasuk lockWhen yang memperketat atas kolom itu) dan penjaga TRUNCATE
// valid di PostgreSQL.
const keywordDoc = `schemaVersion: "1"
kind: resource
key: kw.thing
include:
  - source: sdk:document
fields:
  type: {type: string, maxLength: 20, default: t, index: true}
  order: {type: integer, minimum: 0, default: 0, sortable: true, index: true}
  user: {type: string, maxLength: 40, nullable: true, unique: true}
  group: {type: enum, values: [a, b], default: a}
  limit: {type: decimal, precision: 18, scale: 2, default: "0.00"}
  default: {type: boolean, default: false}
  left: {type: string, maxLength: 10, nullable: true}
  class: {type: string, maxLength: 10, default: x}
  let: {type: date, nullable: true}
  tag:
    type: string
    nullable: true
    accessor: {kind: stored, expr: {concat: [{field: type}, {lit: "-"}, {field: class}]}}
lockWhen: {field: class, op: eq, value: sealed}
indexes:
  - {fields: [type, order]}
`

func TestKeywordFieldNamesProduceValidSQL(t *testing.T) {
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	d, lps := load.Parse("kw/thing.resource.yaml", []byte(keywordDoc))
	if len(lps) != 0 {
		t.Fatal(lps)
	}
	if aps := c.AddModule(resolve.Module{Slug: "kw"}, d); len(aps) != 0 {
		t.Fatal(aps)
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatal(res.Problems)
	}
	schema, dps := dbmodel.Derive(res.Resources)
	if len(dps) != 0 {
		t.Fatal(dps)
	}
	stmts, err := CreateSchema(schema, Options{})
	if err != nil {
		t.Fatal(err)
	}
	db := openDB(t)
	for _, s := range stmts {
		exec(t, db, s.SQL)
	}
	exec(t, db, `INSERT INTO kw_things (id, "order", "user", "left") VALUES ('00000000-0000-0000-0000-000000000001', 3, 'u', 'l')`)
	var tag string
	if err := db.QueryRow(`SELECT tag FROM kw_things`).Scan(&tag); err != nil || tag != "t-x" {
		t.Fatalf("kolom turunan dari field kata kunci: tag=%q err=%v", tag, err)
	}
	// lockWhen atas kolom "class" ditegakkan pemicu: baris tersegel tidak dapat diubah; draft bebas.
	exec(t, db, `UPDATE kw_things SET "limit" = 5, "default" = true, "let" = '2026-01-01', "group" = 'b'`)
	exec(t, db, `UPDATE kw_things SET class = 'sealed'`)
	if _, err := db.Exec(`UPDATE kw_things SET "limit" = 6`); sqlState(err) != "55000" {
		t.Errorf("baris tersegel harus ditolak, err = %v", err)
	}
	if _, err := db.Exec(`TRUNCATE kw_things`); sqlState(err) != "55000" {
		t.Errorf("TRUNCATE dengan baris tersegel harus ditolak, err = %v", err)
	}
}
