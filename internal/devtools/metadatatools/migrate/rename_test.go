package migrate

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/diff"
)

const phoneBoth = `
fields:
  phone: {type: string, maxLength: 20, nullable: true, deprecated: {since: "1.1.0", replacedBy: phoneE164}, label: m.thing.phone}
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.thing.phoneE164}
`

const phoneNewOnly = `
fields:
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.thing.phoneE164}
`

func phoneModel(t testing.TB, large bool, body string) *dbmodel.Schema {
	head := ""
	if large {
		head = "large: true\n"
	}
	return modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", head+body)))
}

// Rename (Requirement 12.2): kolom baru dengan migrateFrom ditambahkan dan datanya disalin dari kolom lama;
// kolom lama tidak dihapus. Semua otomatis.
func TestGenerate_MigrateFromAddsColumnAndCopiesData(t *testing.T) {
	model := phoneModel(t, false, phoneBoth)
	r := runGen(t, model, fnd(diff.KindColumnMissing, "m_things", "phone_e164"))
	up, down := section(t, r.SQL)
	contains(t, up, `ALTER TABLE "m_things" ADD COLUMN "phone_e164" text`,
		`UPDATE "m_things" SET "phone_e164" = "phone" WHERE "phone_e164" IS NULL AND "phone" IS NOT NULL`)
	if strings.Index(up, "ADD COLUMN") > strings.Index(up, "UPDATE") {
		t.Errorf("kolom harus ditambahkan sebelum disalin:\n%s", up)
	}
	notContains(t, up, "DROP COLUMN")
	contains(t, down, `DROP COLUMN "phone_e164"`)
	if len(r.Todos) != 0 || HasTodo(r.SQL) || r.Irreversible || r.NoTransaction {
		t.Errorf("rename tanpa hapus aman: %+v", r)
	}
}

func TestGenerate_MigrateFromLargeTableCopiesInBatches(t *testing.T) {
	model := phoneModel(t, true, phoneBoth)
	r := runGen(t, model, fnd(diff.KindColumnMissing, "m_things", "phone_e164"))
	if !r.NoTransaction || !strings.HasPrefix(r.SQL, "-- +goose NO TRANSACTION\n-- +goose Up\n") {
		t.Fatalf("tabel large harus NO TRANSACTION:\n%s", r.SQL)
	}
	up, _ := section(t, r.SQL)
	contains(t, up, "-- +goose StatementBegin", "DO $erpora$", `SET "phone_e164" = "phone"`, "LIMIT 1000", "COMMIT;", "-- +goose StatementEnd")
	notContains(t, up, `UPDATE "m_things" SET "phone_e164" = "phone" WHERE "phone_e164" IS NULL AND "phone" IS NOT NULL;`)
	if len(r.Todos) != 0 {
		t.Errorf("todos = %+v", r.Todos)
	}
}

// Rename dan hapus dalam satu migrasi: data disalin dari kolom lama (yang masih ada di database) sebelum
// kerangka penghapusan.
func TestGenerate_CopyFromColumnThatIsBeingRemoved(t *testing.T) {
	model := phoneModel(t, false, phoneNewOnly)
	r := runGen(t, model,
		fnd(diff.KindColumnMissing, "m_things", "phone_e164"),
		diff.Finding{Kind: diff.KindColumnExtra, Table: "m_things", Object: "phone", Actual: "text"})
	up, _ := section(t, r.SQL)
	contains(t, up, `SET "phone_e164" = "phone"`)
	if strings.Index(up, `SET "phone_e164"`) > strings.Index(up, "DROP COLUMN") {
		t.Errorf("salinan harus sebelum kerangka penghapusan:\n%s", up)
	}
}

// Penghapusan kolom (Requirement 12.4, 12.8): kerangka berpenanda TODO, penjaga aktif yang menggagalkan
// migrasi bila masih ada baris belum disalin, DROP COLUMN hanya komentar, Down tidak dapat dibalik, dan
// objek yang ikut hilang bersama kolom tidak menjadi kerangka tersendiri.
func TestGenerate_ColumnRemovalSkeletonWithGuard(t *testing.T) {
	model := phoneModel(t, false, phoneNewOnly)
	r := runGen(t, model,
		diff.Finding{Kind: diff.KindColumnExtra, Table: "m_things", Object: "phone", Actual: "text"},
		diff.Finding{Kind: diff.KindCheckExtra, Table: "m_things", Object: "m_things_phone_check", Actual: "CHECK ((length(phone) <= 20))", Columns: []string{"phone"}},
		diff.Finding{Kind: diff.KindIndexExtra, Table: "m_things", Object: "m_things_phone_idx", Columns: []string{"phone"}},
	)
	if len(r.Todos) != 1 || !HasTodo(r.SQL) || !r.Irreversible {
		t.Fatalf("hasil = %+v", r)
	}
	up, down := section(t, r.SQL)
	contains(t, up, TodoMarker, "-- +goose StatementBegin",
		`IF EXISTS (SELECT 1 FROM "m_things" WHERE "phone" IS NOT NULL AND "phone_e164" IS NULL) THEN`, "RAISE EXCEPTION",
		`-- ALTER TABLE "m_things" DROP COLUMN "phone";`, "BACKUP WAJIB")
	for _, l := range activeLines(up) {
		if strings.Contains(strings.ToUpper(l), "DROP ") {
			t.Errorf("tidak boleh ada DROP aktif: %q", l)
		}
	}
	contains(t, down, "TIDAK DAPAT DIBALIK", "RAISE EXCEPTION")
	notContains(t, up, "m_things_phone_check", "m_things_phone_idx")
}

func TestGenerate_ColumnRemovalWithoutReplacementHasNoGuard(t *testing.T) {
	model := thingModel(t)
	r := runGen(t, model, diff.Finding{Kind: diff.KindColumnExtra, Table: "m_things", Object: "legacy", Actual: "text"})
	up, down := section(t, r.SQL)
	if got := activeLines(up); len(got) != 0 {
		t.Errorf("tanpa pengganti tidak ada penjaga aktif: %v", got)
	}
	contains(t, up, `-- ALTER TABLE "m_things" DROP COLUMN "legacy";`, "BACKUP WAJIB", "tanpa kolom pengganti")
	contains(t, down, "TIDAK DAPAT DIBALIK")
	if !r.Irreversible {
		t.Error("penghapusan tidak dapat dibalik")
	}
}

// Pemicu bangkitan yang berubah (mis. pasangan migrateFrom baru) diganti otomatis sebelum salinan data,
// supaya baris terkunci dapat disalin lewat pengecualian (b).
func TestGenerate_TriggerChangedIsReplacedBeforeBackfill(t *testing.T) {
	model := modelOf(t, f("m/doc.resource.yaml", `schemaVersion: "1"
kind: resource
key: m.doc
include:
  - source: sdk:document
fields:
  phone: {type: string, maxLength: 20, nullable: true, deprecated: {since: "1.1.0", replacedBy: phoneE164}, label: m.doc.phone}
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.doc.phoneE164}
`))
	r := runGen(t, model,
		fnd(diff.KindColumnMissing, "m_docs", "phone_e164"),
		fnd(diff.KindTriggerChanged, "m_docs", "m_docs_lock_trg"))
	up, down := section(t, r.SQL)
	contains(t, up, `DROP TRIGGER "m_docs_lock_trg" ON "m_docs"`, `CREATE OR REPLACE FUNCTION "m_docs_lock_fn"()`,
		`CREATE TRIGGER "m_docs_lock_trg"`, `ENABLE ALWAYS TRIGGER "m_docs_lock_trg"`, `SET "phone_e164" = "phone"`)
	if strings.Index(up, "CREATE OR REPLACE FUNCTION") > strings.Index(up, `SET "phone_e164"`) {
		t.Errorf("pemicu harus diganti sebelum salinan:\n%s", up)
	}
	contains(t, down, "TIDAK DAPAT DIBALIK")
	if len(r.Todos) != 0 {
		t.Errorf("todos = %+v", r.Todos)
	}
}
