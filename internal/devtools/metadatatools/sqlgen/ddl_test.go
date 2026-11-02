package sqlgen

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

func joinStatements(stmts []Statement) string {
	var sb strings.Builder
	for _, s := range stmts {
		sb.WriteString(s.SQL)
		sb.WriteString(";\n\n")
	}
	return sb.String()
}

func TestCreateSchema_FixtureGolden(t *testing.T) {
	s := fixtureSchema(t)
	stmts, err := CreateSchema(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "fixture_schema.golden.sql", joinStatements(stmts))
	for _, st := range stmts {
		if n := len(splitTopLevel(t, st.SQL)); n != 1 {
			t.Errorf("pernyataan %s terpecah menjadi %d:\n%s", st.Object, n, st.SQL)
		}
	}
}

func TestCreateSchema_QualifiedGolden(t *testing.T) {
	s := fixtureSchema(t).ForModule("fixturecore")
	stmts, err := CreateSchema(s, Options{Schema: "erpora_expected"})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "fixturecore_qualified.golden.sql", joinStatements(stmts))
}

func TestCreateSchema_OrderTablesBeforeConstraints(t *testing.T) {
	stmts, err := CreateSchema(fixtureSchema(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	seenFK := false
	for _, st := range stmts {
		switch st.Kind {
		case KindAddForeignKey:
			seenFK = true
		case KindCreateTable:
			if seenFK {
				t.Errorf("CREATE TABLE %s setelah FK: urutan harus semua tabel dulu", st.Object)
			}
		}
	}
	if !seenFK {
		t.Error("fixture punya FK satu modul")
	}
}

// Nilai berbahaya pada model (dikonstruksi langsung, melewati Derive) tidak boleh keluar dari literalnya.
func TestCreateTable_DangerousValuesGolden(t *testing.T) {
	nasty := []string{"a'b", "$$", "*/", "`", "x\ny", "'); DROP TABLE t; --", `\`, "$erpora$", `"quoted"`, "--", "/*", "a;b"}
	tb := &dbmodel.Table{
		Name: "t_things", Resource: "t.thing", Module: "t", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true},
			{Name: "kind", Type: dbmodel.TypeText, NotNull: true, Default: &dbmodel.Default{Type: metadata.TypeEnum, Value: "'); DROP TABLE t; --"}},
			{Name: `we"ird`, Type: dbmodel.TypeText},
		},
		PrimaryKey: []string{"id"},
		Checks: []*dbmodel.Check{
			{Name: "t_things_kind_check", Column: "kind", Kind: dbmodel.CheckEnum, Values: nasty},
		},
		Uniques: []*dbmodel.Unique{{Name: `t_things_"x"_key`, Columns: []string{`we"ird`}}},
	}
	sql, err := CreateTable(tb, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(splitTopLevel(t, sql)); n != 1 {
		t.Fatalf("CREATE TABLE terpecah menjadi %d pernyataan:\n%s", n, sql)
	}
	golden(t, "dangerous_table.golden.sql", sql+";\n")
}

func TestCreateTable_RejectsInvalidModel(t *testing.T) {
	base := func() *dbmodel.Table {
		return &dbmodel.Table{Name: "t_x", Kind: dbmodel.KindResource,
			Columns:    []*dbmodel.Column{{Name: "id", Type: dbmodel.TypeUUID, NotNull: true}},
			PrimaryKey: []string{"id"}}
	}
	cases := map[string]func(*dbmodel.Table){
		"nama tabel kosong": func(tb *dbmodel.Table) { tb.Name = "" },
		"nama kolom NUL":    func(tb *dbmodel.Table) { tb.Columns[0].Name = "a\x00b" },
		"range desimal berbahaya": func(tb *dbmodel.Table) {
			tb.Columns = append(tb.Columns, &dbmodel.Column{Name: "n", Type: dbmodel.Numeric(5, 2)})
			tb.Checks = append(tb.Checks, &dbmodel.Check{Name: "c", Column: "n", Kind: dbmodel.CheckRange, Min: "0); DROP TABLE x; --"})
		},
		"default angka berbahaya": func(tb *dbmodel.Table) {
			tb.Columns = append(tb.Columns, &dbmodel.Column{Name: "n", Type: dbmodel.Numeric(5, 2), Default: &dbmodel.Default{Type: metadata.TypeDecimal, Value: "1; DROP"}})
		},
		"kolom check tak ada": func(tb *dbmodel.Table) {
			tb.Checks = append(tb.Checks, &dbmodel.Check{Name: "c", Column: "nope", Kind: dbmodel.CheckEnum, Values: []string{"a"}})
		},
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			tb := base()
			mut(tb)
			if sql, err := CreateTable(tb, Options{}); err == nil {
				t.Errorf("harus galat, dapat: %s", sql)
			}
		})
	}
}

func TestAddColumnAndIndexStatements(t *testing.T) {
	tb := &dbmodel.Table{Name: "m_things", Kind: dbmodel.KindResource}
	col := &dbmodel.Column{Name: "note", Type: dbmodel.TypeText, NotNull: false}
	sql, err := AddColumn(tb, col, Options{})
	if err != nil || sql != `ALTER TABLE "m_things" ADD COLUMN "note" text` {
		t.Errorf("AddColumn = %q, %v", sql, err)
	}
	ix := &dbmodel.Index{Name: "m_things_note_idx", Columns: []string{"note"}}
	sql, err = CreateIndex(tb, ix, Options{Schema: "s"})
	if err != nil || sql != `CREATE INDEX "m_things_note_idx" ON "s"."m_things" ("note")` {
		t.Errorf("CreateIndex = %q, %v", sql, err)
	}
	fk := &dbmodel.ForeignKey{Name: "m_things_a_id_fkey", Columns: []string{"a_id"}, RefTable: "m_as", RefColumns: []string{"id"}, OnDelete: dbmodel.OnDeleteSetNull}
	sql, err = AddForeignKey(tb, fk, Options{})
	if err != nil || sql != `ALTER TABLE "m_things" ADD CONSTRAINT "m_things_a_id_fkey" FOREIGN KEY ("a_id") REFERENCES "m_as" ("id") ON DELETE SET NULL` {
		t.Errorf("AddForeignKey = %q, %v", sql, err)
	}
}

func TestDefaultLiteral_Now(t *testing.T) {
	got, err := DefaultLiteral(&dbmodel.Default{Type: metadata.TypeDateTime, Now: true})
	if err != nil || got != "now()" {
		t.Errorf("DefaultLiteral(now) = %q, %v", got, err)
	}
	if _, err := DefaultLiteral(&dbmodel.Default{Type: metadata.TypeString, Now: true}); err == nil {
		t.Error("now() hanya untuk datetime")
	}
}
