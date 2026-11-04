package sqlgen

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

func alterTable() *dbmodel.Table {
	return &dbmodel.Table{
		Name: "m_things", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true},
			{Name: "status", Type: dbmodel.TypeText, NotNull: true, Default: &dbmodel.Default{Type: metadata.TypeEnum, Value: "draft"}},
			{Name: "note", Type: dbmodel.TypeText},
		},
		PrimaryKey: []string{"id"},
	}
}

func TestAlterStatements(t *testing.T) {
	tb := alterTable()
	o := Options{}
	check := func(name string, got string, err error, want string) {
		t.Helper()
		if err != nil || got != want {
			t.Errorf("%s = %q, %v\nwant %q", name, got, err, want)
		}
	}
	s, err := DropTable(tb, o)
	check("DropTable", s, err, `DROP TABLE "m_things"`)
	s, err = DropColumn(tb, "note", o)
	check("DropColumn", s, err, `ALTER TABLE "m_things" DROP COLUMN "note"`)
	s, err = DropIndex("m_things_note_idx", o)
	check("DropIndex", s, err, `DROP INDEX "m_things_note_idx"`)
	s, err = DropConstraint(tb, "m_things_x_key", o)
	check("DropConstraint", s, err, `ALTER TABLE "m_things" DROP CONSTRAINT "m_things_x_key"`)
	s, err = SetDefault(tb, tb.Columns[1], o)
	check("SetDefault", s, err, `ALTER TABLE "m_things" ALTER COLUMN "status" SET DEFAULT 'draft'`)
	s, err = DropDefault(tb, "status", o)
	check("DropDefault", s, err, `ALTER TABLE "m_things" ALTER COLUMN "status" DROP DEFAULT`)
	s, err = DropNotNull(tb, "status", o)
	check("DropNotNull", s, err, `ALTER TABLE "m_things" ALTER COLUMN "status" DROP NOT NULL`)
	s, err = SetNotNull(tb, "status", o)
	check("SetNotNull", s, err, `ALTER TABLE "m_things" ALTER COLUMN "status" SET NOT NULL`)
	s, err = AddUnique(tb, &dbmodel.Unique{Name: "m_things_note_key", Columns: []string{"note"}}, o)
	check("AddUnique", s, err, `ALTER TABLE "m_things" ADD CONSTRAINT "m_things_note_key" UNIQUE ("note")`)
	s, err = AddCheck(tb, &dbmodel.Check{Name: "m_things_status_check", Column: "status", Kind: dbmodel.CheckEnum, Values: []string{"a", "b'c"}}, o)
	check("AddCheck", s, err, `ALTER TABLE "m_things" ADD CONSTRAINT "m_things_status_check" CHECK ("status" IN ('a', 'b''c'))`)
	s, err = DropTrigger(tb, "m_things_lock_trg", o)
	check("DropTrigger", s, err, `DROP TRIGGER "m_things_lock_trg" ON "m_things"`)
	s, err = DropFunction("m_things_lock_fn", o)
	check("DropFunction", s, err, `DROP FUNCTION "m_things_lock_fn"()`)
	s, err = EnableTrigger(tb, "m_things_lock_trg", o)
	check("EnableTrigger", s, err, `ALTER TABLE "m_things" ENABLE ALWAYS TRIGGER "m_things_lock_trg"`)
	s, err = DropTable(tb, Options{Schema: "s"})
	check("DropTable schema", s, err, `DROP TABLE "s"."m_things"`)

	if _, err := DropColumn(tb, "nope", o); err == nil {
		t.Error("DropColumn kolom tak ada tidak boleh lolos")
	}
	if _, err := SetDefault(tb, tb.Columns[2], o); err == nil {
		t.Error("SetDefault tanpa default tidak boleh lolos")
	}
	if _, err := DropTable(&dbmodel.Table{Name: "a\x00b"}, o); err == nil {
		t.Error("nama tak sah harus ditolak")
	}
}

func TestBatchCopyBackfill(t *testing.T) {
	tb := &dbmodel.Table{
		Name: "m_things", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true},
			{Name: "phone", Type: dbmodel.TypeText},
			{Name: "phone_e164", Type: dbmodel.TypeText},
		},
		PrimaryKey: []string{"id"},
	}
	sql, err := BatchCopyBackfill(tb, "phone_e164", "phone", 500, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`UPDATE "m_things"`, `SET "phone_e164" = "phone"`, `"phone_e164" IS NULL AND "phone" IS NOT NULL`, "LIMIT 500", "COMMIT"} {
		if !strings.Contains(sql, want) {
			t.Errorf("tidak memuat %q:\n%s", want, sql)
		}
	}
	if n := len(splitTopLevel(t, sql)); n != 1 {
		t.Errorf("harus satu pernyataan, dapat %d:\n%s", n, sql)
	}
	golden(t, "batch_backfill.golden.sql", sql+";\n")
	for _, bad := range []int{0, -1, 1000001} {
		if _, err := BatchCopyBackfill(tb, "phone_e164", "phone", bad, Options{}); err == nil {
			t.Errorf("batch %d harus ditolak", bad)
		}
	}
	if _, err := BatchCopyBackfill(tb, "nope", "phone", 10, Options{}); err == nil {
		t.Error("kolom tujuan tak ada harus ditolak")
	}
	tb.PrimaryKey = []string{"a", "b"}
	if _, err := BatchCopyBackfill(tb, "phone_e164", "phone", 10, Options{}); err == nil {
		t.Error("kunci utama komposit harus ditolak")
	}
}

func TestBatchBackfillTemplateIsCommented(t *testing.T) {
	tb := alterTable()
	txt, err := BatchBackfillTemplate(tb, "note", 1000, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimRight(txt, "\n"), "\n") {
		if !strings.HasPrefix(line, "-- ") && line != "--" {
			t.Errorf("baris templat harus komentar: %q", line)
		}
	}
	if !strings.Contains(txt, "LIMIT 1000") || strings.Contains(txt, "+goose") {
		t.Errorf("templat = %s", txt)
	}
}
