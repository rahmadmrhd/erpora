package migrate

import (
	"strings"
	"testing"
	"testing/fstest"

	"erpora/internal/devtools/metadatatools/diff"
)

func fnd(kind, table, object string) diff.Finding {
	return diff.Finding{Kind: kind, Table: table, Object: object}
}

func TestGenerate_EmptyReport(t *testing.T) {
	r := runGen(t, thingModel(t))
	if !r.Empty || r.SQL != "" || len(r.Todos) != 0 {
		t.Fatalf("hasil = %+v", r)
	}
}

func TestGenerate_FileFormat(t *testing.T) {
	r := runGen(t, thingModel(t), fnd(diff.KindColumnMissing, "m_things", "note"))
	if !strings.HasPrefix(r.SQL, "-- +goose Up\n") {
		t.Errorf("harus diawali Up:\n%s", r.SQL)
	}
	up, down := section(t, r.SQL)
	contains(t, up+down, "metadata:migrate")
	// Penanda goose hanya sebagai baris anotasi sungguhan; komentar tidak boleh memuatnya.
	for _, line := range strings.Split(r.SQL, "\n") {
		if strings.Contains(line, "+goose") && !strings.HasPrefix(line, "-- +goose Up") && !strings.HasPrefix(line, "-- +goose Down") &&
			!strings.HasPrefix(line, "-- +goose StatementBegin") && !strings.HasPrefix(line, "-- +goose StatementEnd") && !strings.HasPrefix(line, "-- +goose NO TRANSACTION") {
			t.Errorf("baris memuat penanda goose tak terduga: %q", line)
		}
	}
	if r.Irreversible || r.NoTransaction || len(r.Todos) != 0 {
		t.Errorf("hasil = %+v", r)
	}
}

func TestGenerate_NewTablesWithForeignKeys(t *testing.T) {
	model := modelOf(t,
		f("m/parent.resource.yaml", resourceDoc("m.parent", "fields:\n  name: {type: string, maxLength: 20, label: m.parent.name}\nindexes:\n  - fields: [name]\n    unique: true\n")),
		f("m/child.resource.yaml", resourceDoc("m.child", "fields:\n  n: {type: integer, index: true, label: m.child.n}\nrelations:\n  parent: {type: belongsTo, to: m.parent, required: true, onDelete: cascade}\n")),
	)
	r := runGen(t, model, fnd(diff.KindTableMissing, "m_children", ""), fnd(diff.KindTableMissing, "m_parents", ""))
	up, down := section(t, r.SQL)
	contains(t, up, `CREATE TABLE "m_children"`, `CREATE TABLE "m_parents"`,
		`ALTER TABLE "m_children" ADD CONSTRAINT "m_children_parent_id_fkey" FOREIGN KEY ("parent_id") REFERENCES "m_parents" ("id") ON DELETE CASCADE`,
		`CREATE INDEX "m_children_n_idx" ON "m_children" ("n")`)
	// Semua tabel dibuat sebelum foreign key pertama.
	if strings.Index(up, "ADD CONSTRAINT") < strings.LastIndex(up, "CREATE TABLE") {
		t.Errorf("FK harus setelah semua CREATE TABLE:\n%s", up)
	}
	// Down: FK dilepas dulu, lalu tabel dihapus; tidak ada TODO dan dapat dibalik.
	contains(t, down, `DROP CONSTRAINT "m_children_parent_id_fkey"`, `DROP TABLE "m_children"`, `DROP TABLE "m_parents"`)
	if strings.Index(down, "DROP CONSTRAINT") > strings.Index(down, "DROP TABLE") {
		t.Errorf("FK harus dilepas sebelum tabel dihapus:\n%s", down)
	}
	if r.Irreversible || len(r.Todos) != 0 || HasTodo(r.SQL) {
		t.Errorf("tabel baru aman: %+v", r)
	}
}

func TestGenerate_SafeColumnAdditions(t *testing.T) {
	model := thingModel(t)
	r := runGen(t, model,
		fnd(diff.KindColumnMissing, "m_things", "note"),
		fnd(diff.KindColumnMissing, "m_things", "kind"),
		diff.Finding{Kind: diff.KindCheckMissing, Table: "m_things", Object: "m_things_kind_check", Columns: []string{"kind"}},
	)
	up, down := section(t, r.SQL)
	contains(t, up,
		`ALTER TABLE "m_things" ADD COLUMN "note" text`,
		`ALTER TABLE "m_things" ADD COLUMN "kind" text DEFAULT 'a' NOT NULL`,
		`ALTER TABLE "m_things" ADD CONSTRAINT "m_things_kind_check" CHECK ("kind" IN ('a', 'b', 'c'))`)
	contains(t, down, `DROP COLUMN "kind"`, `DROP COLUMN "note"`)
	if len(r.Todos) != 0 || HasTodo(r.SQL) || r.Irreversible {
		t.Errorf("hasil = %+v", r)
	}
}

func TestGenerate_NotNullWithoutDefaultIsSkeleton(t *testing.T) {
	model := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", "fields:\n  code: {type: string, maxLength: 10, label: m.thing.code}\n")))
	r := runGen(t, model, fnd(diff.KindColumnMissing, "m_things", "code"))
	up, down := section(t, r.SQL)
	if len(r.Todos) != 1 || !HasTodo(r.SQL) {
		t.Fatalf("harus kerangka TODO: %+v", r.Todos)
	}
	if got := activeLines(up); len(got) != 0 {
		t.Errorf("kerangka tidak boleh berisi pernyataan aktif: %v", got)
	}
	contains(t, up, TodoMarker, `-- ALTER TABLE "m_things" ADD COLUMN "code" text`, `-- ALTER TABLE "m_things" ALTER COLUMN "code" SET NOT NULL`)
	contains(t, down, TodoMarker)
	if r.NoTransaction {
		t.Error("tabel kecil tidak butuh NO TRANSACTION")
	}
}

func TestGenerate_LargeTableSkeletonHasBatchBackfillAndNoTransaction(t *testing.T) {
	model := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", "large: true\nfields:\n  code: {type: string, maxLength: 10, label: m.thing.code}\n")))
	r := runGen(t, model, fnd(diff.KindColumnMissing, "m_things", "code"))
	if !r.NoTransaction || !strings.HasPrefix(r.SQL, "-- +goose NO TRANSACTION\n-- +goose Up\n") {
		t.Fatalf("tabel large harus berpenanda NO TRANSACTION di awal:\n%s", r.SQL)
	}
	up, _ := section(t, r.SQL)
	contains(t, up, "-- DO $erpora$", "LIMIT 1000", "COMMIT;", "<ISI: ekspresi nilai>")
	if got := activeLines(up); len(got) != 0 {
		t.Errorf("templat backfill harus komentar: %v", got)
	}
}

func TestGenerate_Indexes(t *testing.T) {
	model := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", "fields:\n  a: {type: string, maxLength: 10, index: true, label: m.thing.a}\n")))
	r := runGen(t, model, fnd(diff.KindIndexMissing, "m_things", "m_things_a_idx"))
	up, down := section(t, r.SQL)
	contains(t, up, `CREATE INDEX "m_things_a_idx" ON "m_things" ("a")`)
	contains(t, down, `DROP INDEX "m_things_a_idx"`)
	// Indeks berlebih: hanya kerangka.
	r = runGen(t, model, fnd(diff.KindIndexExtra, "m_things", "zz_idx"))
	if len(r.Todos) != 1 || len(activeLines(mustUp(t, r.SQL))) != 0 {
		t.Errorf("index_extra harus kerangka: %+v", r)
	}
}

func mustUp(t *testing.T, sql string) string {
	up, _ := section(t, sql)
	return up
}

func TestGenerate_Nullability(t *testing.T) {
	model := thingModel(t)
	// Pelonggaran (model nullable, database NOT NULL): otomatis, tidak dapat dibalik.
	r := runGen(t, model, diff.Finding{Kind: diff.KindColumnNullability, Table: "m_things", Object: "note", Expected: "notNull=false", Actual: "notNull=true"})
	up, down := section(t, r.SQL)
	contains(t, up, `ALTER TABLE "m_things" ALTER COLUMN "note" DROP NOT NULL`)
	contains(t, down, "TIDAK DAPAT DIBALIK", "RAISE EXCEPTION")
	if !r.Irreversible || len(r.Todos) != 0 {
		t.Errorf("hasil = %+v", r)
	}
	// Pengetatan: kerangka.
	r = runGen(t, model, diff.Finding{Kind: diff.KindColumnNullability, Table: "m_things", Object: "title", Expected: "notNull=true", Actual: "notNull=false"})
	if len(r.Todos) != 1 || len(activeLines(mustUp(t, r.SQL))) != 0 {
		t.Errorf("NOT NULL pada kolom ada harus kerangka: %+v", r)
	}
	contains(t, mustUp(t, r.SQL), `-- ALTER TABLE "m_things" ALTER COLUMN "title" SET NOT NULL`)
}

func TestGenerate_Defaults(t *testing.T) {
	model := thingModel(t)
	r := runGen(t, model, diff.Finding{Kind: diff.KindColumnDefault, Table: "m_things", Object: "kind", Expected: "'a'::text", Actual: ""})
	up, down := section(t, r.SQL)
	contains(t, up, `ALTER TABLE "m_things" ALTER COLUMN "kind" SET DEFAULT 'a'`)
	contains(t, down, `ALTER TABLE "m_things" ALTER COLUMN "kind" DROP DEFAULT`)
	if r.Irreversible {
		t.Error("default awal kosong dapat dibalik")
	}
	r = runGen(t, model, diff.Finding{Kind: diff.KindColumnDefault, Table: "m_things", Object: "kind", Expected: "'a'::text", Actual: "'b'::text"})
	up, down = section(t, r.SQL)
	contains(t, up, `SET DEFAULT 'a'`)
	contains(t, down, "TIDAK DAPAT DIBALIK")
	r = runGen(t, model, diff.Finding{Kind: diff.KindColumnDefault, Table: "m_things", Object: "note", Expected: "", Actual: "'x'::text"})
	up, down = section(t, r.SQL)
	contains(t, up, `ALTER TABLE "m_things" ALTER COLUMN "note" DROP DEFAULT`)
	contains(t, down, "TIDAK DAPAT DIBALIK")
}

func checkPair(oldName, oldDef, newName string, col string) []diff.Finding {
	return []diff.Finding{
		{Kind: diff.KindCheckExtra, Table: "m_things", Object: oldName, Actual: oldDef, Columns: []string{col}},
		{Kind: diff.KindCheckMissing, Table: "m_things", Object: newName, Columns: []string{col}},
	}
}

func TestGenerate_CheckLoosening(t *testing.T) {
	model := thingModel(t)
	auto := []struct {
		name, col, newName, oldDef string
		wantAdd                    string
	}{
		{"enum bertambah (extensible)", "kind", "m_things_kind_check", "CHECK ((kind = ANY (ARRAY['a'::text, 'b'::text])))", `CHECK ("kind" IN ('a', 'b', 'c'))`},
		{"panjang maksimum naik", "title", "m_things_title_check", "CHECK (((length(title) >= 1) AND (length(title) <= 120)))", `CHECK (length("title") >= 1 AND length("title") <= 200)`},
		{"panjang minimum turun", "title", "m_things_title_check", "CHECK (((length(title) >= 5) AND (length(title) <= 200)))", `CHECK (length("title") >= 1 AND length("title") <= 200)`},
		{"rentang melebar", "qty", "m_things_qty_check", "CHECK (((qty >= 1) AND (qty <= 50)))", `CHECK ("qty" >= 1 AND "qty" <= 100)`},
	}
	for _, c := range auto {
		t.Run(c.name, func(t *testing.T) {
			r := runGen(t, model, checkPair("old_check", c.oldDef, c.newName, c.col)...)
			up, down := section(t, r.SQL)
			contains(t, up, `ALTER TABLE "m_things" DROP CONSTRAINT "old_check"`, "ADD CONSTRAINT "+`"`+c.newName+`" `+c.wantAdd)
			contains(t, down, "TIDAK DAPAT DIBALIK")
			if len(r.Todos) != 0 || HasTodo(r.SQL) || !r.Irreversible {
				t.Errorf("hasil = %+v", r)
			}
		})
	}
	skeleton := []struct{ name, col, newName, oldDef string }{
		{"enum tidak extensible", "strict", "m_things_strict_check", "CHECK ((strict = ANY (ARRAY['x'::text])))"},
		{"nilai enum dibuang", "kind", "m_things_kind_check", "CHECK ((kind = ANY (ARRAY['a'::text, 'b'::text, 'c'::text, 'd'::text])))"},
		{"panjang maksimum turun", "title", "m_things_title_check", "CHECK (((length(title) >= 1) AND (length(title) <= 300)))"},
		{"rentang menyempit", "qty", "m_things_qty_check", "CHECK (((qty >= 1) AND (qty <= 500)))"},
		{"definisi tak dikenali", "qty", "m_things_qty_check", "CHECK ((qty <> 7))"},
	}
	for _, c := range skeleton {
		t.Run(c.name, func(t *testing.T) {
			r := runGen(t, model, checkPair("old_check", c.oldDef, c.newName, c.col)...)
			if len(r.Todos) == 0 || !HasTodo(r.SQL) {
				t.Fatalf("harus kerangka TODO: %+v", r)
			}
			if got := activeLines(mustUp(t, r.SQL)); len(got) != 0 {
				t.Errorf("kerangka tidak boleh berpernyataan aktif: %v", got)
			}
		})
	}
}

func TestGenerate_DecisionItemsAreSkeletonsOnly(t *testing.T) {
	model := thingModel(t)
	cases := []diff.Finding{
		{Kind: diff.KindColumnType, Table: "m_things", Object: "title", Expected: "text", Actual: "character varying(10)"},
		{Kind: diff.KindColumnGenerated, Table: "m_things", Object: "title", Expected: "(x)", Actual: "(y)"},
		{Kind: diff.KindColumnExtra, Table: "m_things", Object: "legacy", Actual: "text"},
		{Kind: diff.KindUniqueExtra, Table: "m_things", Object: "zz_uq", Actual: "UNIQUE (title)", Columns: []string{"title"}},
		{Kind: diff.KindForeignKeyExtra, Table: "m_things", Object: "zz_fk", Actual: "FOREIGN KEY (x)", Columns: []string{"x"}},
		{Kind: diff.KindPrimaryKey, Table: "m_things", Expected: "PRIMARY KEY (id)", Actual: "PRIMARY KEY (title)"},
		{Kind: diff.KindCheckExtra, Table: "m_things", Object: "zz_chk", Actual: "CHECK ((x > 0))", Columns: []string{"x"}},
		{Kind: diff.KindCheckMissing, Table: "m_things", Object: "m_things_title_check", Columns: []string{"title"}},
		{Kind: diff.KindTriggerChanged, Table: "m_things", Object: "zz_trg"},
	}
	for _, fd := range cases {
		t.Run(fd.Kind, func(t *testing.T) {
			r := runGen(t, model, fd)
			if len(r.Todos) != 1 || !HasTodo(r.SQL) {
				t.Fatalf("harus kerangka: %+v", r.Todos)
			}
			up, down := section(t, r.SQL)
			if got := activeLines(up); len(got) != 0 {
				t.Errorf("tidak boleh ada pernyataan aktif: %v", got)
			}
			contains(t, up, TodoMarker, "m_things")
			contains(t, down, TodoMarker)
		})
	}
}

func TestGenerate_ConstraintsOnNewColumnsAreSafe(t *testing.T) {
	model := modelOf(t,
		f("m/other.resource.yaml", resourceDoc("m.other", "fields:\n  x: {type: integer, label: m.other.x}\n")),
		f("m/thing.resource.yaml", resourceDoc("m.thing", "fields:\n  code: {type: string, maxLength: 10, nullable: true, unique: true, label: m.thing.code}\nrelations:\n  other: {type: belongsTo, to: m.other, onDelete: setNull}\n")),
	)
	r := runGen(t, model,
		fnd(diff.KindColumnMissing, "m_things", "code"),
		fnd(diff.KindColumnMissing, "m_things", "other_id"),
		diff.Finding{Kind: diff.KindUniqueMissing, Table: "m_things", Object: "m_things_code_key", Columns: []string{"code"}},
		diff.Finding{Kind: diff.KindCheckMissing, Table: "m_things", Object: "m_things_code_check", Columns: []string{"code"}},
		diff.Finding{Kind: diff.KindForeignKeyMissing, Table: "m_things", Object: "m_things_other_id_fkey", Columns: []string{"other_id"}},
	)
	if len(r.Todos) != 0 || HasTodo(r.SQL) {
		t.Fatalf("constraint pada kolom baru aman: %+v\n%s", r.Todos, r.SQL)
	}
	up, down := section(t, r.SQL)
	contains(t, up, `ADD COLUMN "code" text`, `ADD COLUMN "other_id" uuid`,
		`ADD CONSTRAINT "m_things_code_key" UNIQUE ("code")`, `ADD CONSTRAINT "m_things_code_check" CHECK (length("code") <= 10)`,
		`ADD CONSTRAINT "m_things_other_id_fkey" FOREIGN KEY ("other_id") REFERENCES "m_others" ("id") ON DELETE SET NULL`)
	// Urutan Up: kolom, lalu constraint dan FK; Down terbalik.
	if strings.Index(up, "ADD COLUMN") > strings.Index(up, "ADD CONSTRAINT") {
		t.Errorf("kolom harus lebih dulu:\n%s", up)
	}
	contains(t, down, `DROP COLUMN "code"`, `DROP COLUMN "other_id"`)
}

func TestGenerate_Triggers(t *testing.T) {
	model := lockedModel(t)
	r := runGen(t, model, fnd(diff.KindTriggerMissing, "m_docs", "m_docs_lock_trg"))
	up, down := section(t, r.SQL)
	contains(t, up, "-- +goose StatementBegin", `CREATE FUNCTION "m_docs_lock_fn"()`, "-- +goose StatementEnd", `CREATE TRIGGER "m_docs_lock_trg" BEFORE DELETE OR UPDATE ON "m_docs"`)
	contains(t, down, `DROP TRIGGER "m_docs_lock_trg" ON "m_docs"`, `DROP FUNCTION "m_docs_lock_fn"()`)
	r = runGen(t, model, fnd(diff.KindTriggerMissing, "m_docs", "m_docs_truncate_lock_trg"))
	up, down = section(t, r.SQL)
	contains(t, up, `CREATE FUNCTION "m_docs_truncate_lock_fn"()`, `CREATE TRIGGER "m_docs_truncate_lock_trg" BEFORE TRUNCATE ON "m_docs" FOR EACH STATEMENT`, `ENABLE ALWAYS TRIGGER "m_docs_truncate_lock_trg"`)
	contains(t, down, `DROP TRIGGER "m_docs_truncate_lock_trg" ON "m_docs"`, `DROP FUNCTION "m_docs_truncate_lock_fn"()`)
	r = runGen(t, model, fnd(diff.KindTriggerDisabled, "m_docs", "m_docs_lock_trg"))
	up, down = section(t, r.SQL)
	contains(t, up, `ALTER TABLE "m_docs" ENABLE ALWAYS TRIGGER "m_docs_lock_trg"`)
	contains(t, down, "TIDAK DAPAT DIBALIK")
}

func TestGenerate_IgnoresUnknownTargets(t *testing.T) {
	model := thingModel(t)
	if _, err := Generate(report(fnd(diff.KindColumnMissing, "m_nope", "x")), model, Options{}); err == nil {
		t.Error("tabel yang tidak ada di model harus galat")
	}
	if _, err := Generate(report(fnd(diff.KindColumnMissing, "m_things", "nope")), model, Options{}); err == nil {
		t.Error("kolom yang tidak ada di model harus galat")
	}
}

func TestGenerate_CommentsAreSanitized(t *testing.T) {
	model := thingModel(t)
	evil := "x\n-- +goose Down\nDROP TABLE m_things;--"
	r := runGen(t, model,
		diff.Finding{Kind: diff.KindColumnExtra, Table: "m_things", Object: evil, Actual: "text\nDROP TABLE x"},
		diff.Finding{Kind: diff.KindIndexExtra, Table: "m_things", Object: evil},
	)
	if n := strings.Count(r.SQL, "-- +goose Down"); n != 1 {
		t.Fatalf("masukan berbahaya tidak boleh menyuntikkan penanda: %d\n%s", n, r.SQL)
	}
	if got := activeLines(mustUp(t, r.SQL)); len(got) != 0 {
		t.Errorf("pernyataan aktif muncul dari masukan: %v", got)
	}
	if strings.Contains(r.SQL, "DROP TABLE m_things") && !strings.Contains(r.SQL, "-- ") {
		t.Error("tidak mungkin")
	}
}

func TestFileName(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_create_x.sql": {Data: []byte("x")},
		"0007_add_y.sql":    {Data: []byte("x")},
		"README.md":         {Data: []byte("x")},
	}
	got, err := FileName(fsys, "sync_metadata")
	if err != nil || got != "0008_sync_metadata.sql" {
		t.Errorf("FileName = %q, %v", got, err)
	}
	got, err = FileName(fstest.MapFS{}, "x")
	if err != nil || got != "0001_x.sql" {
		t.Errorf("FileName kosong = %q, %v", got, err)
	}
	for _, bad := range []string{"", "Bad", "a-b", "../x", "x y"} {
		if _, err := FileName(fsys, bad); err == nil {
			t.Errorf("nama %q harus ditolak", bad)
		}
	}
}

func TestFindTodos(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_ok.sql":   {Data: []byte("-- +goose Up\nSELECT 1;\n")},
		"0002_todo.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n-- " + TodoMarker + ": putuskan\n")},
		"notes.txt":     {Data: []byte(TodoMarker)},
	}
	todos, err := FindTodos(fsys)
	if err != nil || len(todos) != 1 || todos[0].File != "0002_todo.sql" || todos[0].Line != 3 {
		t.Errorf("todos = %+v, %v", todos, err)
	}
}

func TestGenerate_UniqueOnExistingColumnIsSkeleton(t *testing.T) {
	model := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", "fields:\n  code: {type: string, maxLength: 10, unique: true, label: m.thing.code}\n")))
	r := runGen(t, model, diff.Finding{Kind: diff.KindUniqueMissing, Table: "m_things", Object: "m_things_code_key", Columns: []string{"code"}})
	if len(r.Todos) != 1 || len(activeLines(mustUp(t, r.SQL))) != 0 {
		t.Fatalf("unique pada kolom ada harus kerangka: %+v", r)
	}
	contains(t, mustUp(t, r.SQL), `-- ALTER TABLE "m_things" ADD CONSTRAINT "m_things_code_key" UNIQUE ("code")`)
}

// Down yang berisi DROP sebagai pembalik operasi expand diberi komentar baku di baris pertama (keputusan pemilik
// produk, 2 Nov 2026); Down operasi contract tetap ditandai TIDAK DAPAT DIBALIK tanpa DROP aktif.
func TestGenerate_DownDevOnlyComment(t *testing.T) {
	firstDownLine := func(sql string) string {
		_, down := section(t, sql)
		return strings.SplitN(down, "\n", 2)[0]
	}
	const want = "-- HANYA DEVELOPMENT: Down menghapus data; produksi maju-saja (expand → migrate → contract)"
	if DownDevOnlyComment != want {
		t.Fatalf("komentar baku = %q", DownDevOnlyComment)
	}

	t.Run("kolom baru (DROP COLUMN pada Down)", func(t *testing.T) {
		r := runGen(t, thingModel(t), fnd(diff.KindColumnMissing, "m_things", "note"))
		_, down := section(t, r.SQL)
		contains(t, down, `DROP COLUMN "note"`)
		if got := firstDownLine(r.SQL); got != want {
			t.Errorf("baris pertama Down = %q", got)
		}
	})
	t.Run("tabel baru (DROP TABLE pada Down)", func(t *testing.T) {
		r := runGen(t, thingModel(t), fnd(diff.KindTableMissing, "m_things", ""))
		if got := firstDownLine(r.SQL); got != want {
			t.Errorf("baris pertama Down = %q", got)
		}
	})
	t.Run("indeks baru (DROP INDEX pada Down)", func(t *testing.T) {
		model := modelOf(t, f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  n: {type: integer, index: true, label: m.a.n}\n")))
		r := runGen(t, model, fnd(diff.KindIndexMissing, "m_as", "m_as_n_idx"))
		_, down := section(t, r.SQL)
		contains(t, down, "DROP INDEX")
		if got := firstDownLine(r.SQL); got != want {
			t.Errorf("baris pertama Down = %q", got)
		}
	})
	t.Run("pemicu baru (DROP TRIGGER/FUNCTION murni) tidak diberi penanda menghapus data", func(t *testing.T) {
		r := runGen(t, lockedModel(t), fnd(diff.KindTriggerMissing, "m_docs", "m_docs_lock_trg"))
		_, down := section(t, r.SQL)
		contains(t, down, `DROP TRIGGER`, `DROP FUNCTION`)
		if strings.Contains(down, "HANYA DEVELOPMENT") {
			t.Errorf("DROP TRIGGER/FUNCTION tidak menghilangkan data, tidak perlu penanda:\n%s", down)
		}
	})
	t.Run("constraint baru (DROP CONSTRAINT murni) tidak diberi penanda", func(t *testing.T) {
		if hasDestructiveDown([]stmt{{sql: `ALTER TABLE "t" DROP CONSTRAINT "c"`}}) {
			t.Error("DROP CONSTRAINT murni tidak boleh memicu penanda")
		}
	})
	t.Run("Down tanpa DROP tidak diberi komentar", func(t *testing.T) {
		r := runGen(t, thingModel(t), diff.Finding{Kind: diff.KindColumnDefault, Table: "m_things", Object: "kind", Expected: "'a'::text", Actual: ""})
		_, down := section(t, r.SQL)
		// DROP DEFAULT adalah pembalik yang tidak menghapus data.
		if strings.Contains(down, "HANYA DEVELOPMENT") {
			t.Errorf("Down tanpa penghapusan data tidak boleh diberi komentar:\n%s", down)
		}
	})
	t.Run("operasi contract tetap tidak dapat dibalik", func(t *testing.T) {
		r := runGen(t, thingModel(t), diff.Finding{Kind: diff.KindColumnExtra, Table: "m_things", Object: "legacy", Actual: "text"})
		_, down := section(t, r.SQL)
		contains(t, down, "TIDAK DAPAT DIBALIK", "RAISE EXCEPTION")
		if strings.Contains(r.SQL, "HANYA DEVELOPMENT") || !r.Irreversible {
			t.Errorf("Down contract tidak boleh diberi komentar dev-only dan tetap tidak dapat dibalik:\n%s", r.SQL)
		}
	})
	t.Run("nilai literal berisi kata DROP tidak memicu komentar", func(t *testing.T) {
		if hasDestructiveDown([]stmt{{sql: `ALTER TABLE "t" ALTER COLUMN "c" SET DEFAULT 'drop table x'`}}) {
			t.Error("literal terkutip harus diabaikan")
		}
		if !hasDestructiveDown([]stmt{{sql: `drop   table "t"`}}) {
			t.Error("DROP TABLE harus terdeteksi tanpa peduli huruf besar dan spasi")
		}
	})
}

// Penghapusan kolom pada modul tanpa tag rilis (belum ada konsumen): pesan tidak menuntut deprecated/removeAfter/
// MAJOR, penjaga data dan Down tidak-dapat-dibalik tetap; modul bertag tetap ketat (keputusan 2 Nov 2026).
func TestGenerate_ColumnRemovalUnreleasedVsReleased(t *testing.T) {
	finding := diff.Finding{Kind: diff.KindColumnExtra, Table: "m_things", Object: "legacy", Actual: "text"}
	gen := func(unreleased bool) *Result {
		t.Helper()
		r, err := Generate(report(finding), thingModel(t), Options{Unreleased: map[string]bool{"m": unreleased}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, unreleased := range []bool{true, false} {
		r := gen(unreleased)
		_, down := section(t, r.SQL)
		contains(t, down, "TIDAK DAPAT DIBALIK", "RAISE EXCEPTION")
		if !r.Irreversible || len(r.Todos) != 1 {
			t.Errorf("unreleased=%v: hasil = %+v", unreleased, r)
		}
	}
	rel, unrel := gen(false), gen(true)
	contains(t, rel.SQL, "hapus hanya pada rilis MAJOR setelah deprecated dan removeAfter", "BACKUP WAJIB")
	contains(t, unrel.SQL, "belum punya tag rilis", "boleh dihapus langsung")
	if strings.Contains(unrel.SQL, "hapus hanya pada rilis MAJOR") {
		t.Errorf("modul tanpa tag tidak boleh dituntut MAJOR/removeAfter:\n%s", unrel.SQL)
	}
}
