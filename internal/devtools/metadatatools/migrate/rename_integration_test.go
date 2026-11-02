//go:build integration

package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/diff"
	"erpora/internal/platform/testsupport/pgtest"
)

const (
	phoneV1 = `
fields:
  phone: {type: string, maxLength: 20, nullable: true, label: m.thing.phone}
`
	phoneV3 = `
fields:
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.thing.phoneE164}
`
)

// genNext membangkitkan migrasi berikutnya untuk model dari selisih terhadap semua migrasi yang ada.
func genNext(t *testing.T, model *dbmodel.Schema, files fstest.MapFS, batch int) *Result {
	t.Helper()
	rep, err := diff.Run(context.Background(), diff.Options{DSN: pgtest.Start(t), Sources: []diff.Source{{Slug: "m", Migrations: files}}, Model: model})
	if err != nil {
		t.Fatalf("diff.Run: %v", err)
	}
	res, err := Generate(rep, model, Options{Batch: batch})
	if err != nil {
		t.Fatal(err)
	}
	if res.Empty {
		t.Fatal("harus ada selisih")
	}
	return res
}

// completeRemoval meniru tinjauan manusia atas kerangka penghapusan: DROP COLUMN diaktifkan dan penanda TODO dihapus.
func completeRemoval(sqlText string) string {
	var out []string
	for _, line := range strings.Split(sqlText, "\n") {
		switch {
		case strings.Contains(line, TodoMarker):
			continue
		case strings.HasPrefix(line, "-- ALTER TABLE") && strings.Contains(line, "DROP COLUMN"):
			out = append(out, strings.TrimPrefix(line, "-- "))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

type renameFixture struct {
	files fstest.MapFS // 0001 (v1), 0002 (v2, otomatis), 0003 (v3, kerangka dilengkapi)
	v3    *dbmodel.Schema
	res2  *Result
	res3  *Result
}

func buildRenameFixture(t *testing.T, large bool, batch int) *renameFixture {
	t.Helper()
	head := ""
	if large {
		head = "large: true\n"
	}
	mk := func(body string) *dbmodel.Schema {
		return modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", head+body)))
	}
	v1, v2, v3 := mk(phoneV1), mk(phoneBoth), mk(phoneV3)
	files := baseFS(t, v1, nil)
	res2 := genNext(t, v2, files, batch)
	files["0002_rename.sql"] = &fstest.MapFile{Data: []byte(res2.SQL)}
	res3 := genNext(t, v3, files, batch)
	files["0003_drop_phone.sql"] = &fstest.MapFile{Data: []byte(completeRemoval(res3.SQL))}
	return &renameFixture{files: files, v3: v3, res2: res2, res3: res3}
}

func openSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedPhones(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO m_things (id, phone) SELECT gen_random_uuid(), '08' || g::text FROM generate_series(1, $1) g`, n); err != nil {
		t.Fatal(err)
	}
}

func snapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT phone_e164 FROM m_things ORDER BY phone_e164`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s.String)
	}
	return strings.Join(out, ",")
}

// Requirement 12.7: lompatan beberapa versi menjalankan semua migrasi berurutan dengan hasil yang sama
// (skema dan data) seperti upgrade bertahap; hasil akhir cocok dengan metadata v3.
func TestRename_VersionJumpEqualsStepwiseUpgrade(t *testing.T) {
	fx := buildRenameFixture(t, false, 0)
	if len(fx.res2.Todos) != 0 || fx.res2.Irreversible {
		t.Fatalf("migrasi rename harus otomatis dan dapat dibalik: %+v\n%s", fx.res2.Todos, fx.res2.SQL)
	}
	if len(fx.res3.Todos) != 1 || !fx.res3.Irreversible {
		t.Fatalf("penghapusan harus kerangka tak dapat dibalik: %+v", fx.res3)
	}
	ctx := context.Background()

	stepDSN := pgtest.Start(t)
	ps, gs := newProvider(t, stepDSN, fx.files)
	if _, err := ps.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	stepDB := openSQL(t, stepDSN)
	seedPhones(t, stepDB, 5)
	if _, err := ps.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, stepDB); got != "081,082,083,084,085" {
		t.Fatalf("setelah 0002 data harus tersalin: %q", got)
	}
	if _, err := ps.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}

	jumpDSN := pgtest.Start(t)
	pj, gj := newProvider(t, jumpDSN, fx.files)
	if _, err := pj.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	jumpDB := openSQL(t, jumpDSN)
	seedPhones(t, jumpDB, 5)
	results, err := pj.Up(ctx) // 0002 dan 0003 berurutan
	if err != nil || len(results) != 2 {
		t.Fatalf("lompatan: %d hasil, %v", len(results), err)
	}

	if a, b := snapshot(t, stepDB), snapshot(t, jumpDB); a != b || a != "081,082,083,084,085" {
		t.Errorf("data bertahap %q berbeda dengan lompatan %q", a, b)
	}
	step, err := diff.Introspect(ctx, gs, "public")
	if err != nil {
		t.Fatal(err)
	}
	jump, err := diff.Introspect(ctx, gj, "public")
	if err != nil {
		t.Fatal(err)
	}
	delete(step.Tables, "goose_db_version")
	delete(jump.Tables, "goose_db_version")
	if r := diff.Compare(jump, step, nil, diff.CompareOptions{}); !r.Empty() {
		t.Errorf("skema lompatan harus sama dengan bertahap:\n%s", r)
	}
	if jump.Tables["m_things"].Column("phone") != nil {
		t.Error("kolom lama harus sudah dihapus")
	}

	// Hasil akhir sama dengan metadata v3.
	rep, err := diff.Run(ctx, diff.Options{DSN: pgtest.Start(t), Sources: []diff.Source{{Slug: "m", Migrations: fx.files}}, Model: fx.v3})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Empty() {
		t.Errorf("selisih harus kosong setelah semua migrasi:\n%s", rep)
	}
}

// Penjaga (Requirement 12.8): migrasi penghapusan gagal bila masih ada baris belum disalin, tidak mengubah
// apa pun, dan lolos setelah disalin; Down tidak dapat dibalik.
func TestRename_RemovalGuardAndIrreversibleDown(t *testing.T) {
	fx := buildRenameFixture(t, false, 0)
	ctx := context.Background()
	dsn := pgtest.Start(t)
	p, g := newProvider(t, dsn, fx.files)
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	db := openSQL(t, dsn)
	// Penulis lama (aplikasi versi sebelumnya) menulis hanya kolom lama setelah penyalinan.
	if _, err := db.Exec(`INSERT INTO m_things (id, phone) VALUES (gen_random_uuid(), '0811')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO m_things (id) VALUES (gen_random_uuid())`); err != nil { // keduanya kosong: wajar
		t.Fatal(err)
	}
	_, err := p.UpTo(ctx, 3)
	if err == nil || !strings.Contains(err.Error(), "penghapusan kolom ditolak") {
		t.Fatalf("penjaga harus menggagalkan migrasi, err = %v", err)
	}
	cat, err := diff.Introspect(ctx, g, "public")
	if err != nil {
		t.Fatal(err)
	}
	if cat.Tables["m_things"].Column("phone") == nil {
		t.Fatal("migrasi gagal harus dibatalkan: kolom lama masih ada")
	}
	if _, err := db.Exec(`UPDATE m_things SET phone_e164 = phone WHERE phone_e164 IS NULL AND phone IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("setelah disalin, penghapusan harus lolos: %v", err)
	}
	cat, err = diff.Introspect(ctx, g, "public")
	if err != nil {
		t.Fatal(err)
	}
	if cat.Tables["m_things"].Column("phone") != nil {
		t.Error("kolom lama harus terhapus")
	}
	if _, err := p.Down(ctx); err == nil || !strings.Contains(err.Error(), "tidak dapat dibalik") {
		t.Fatalf("Down penghapusan harus ditolak dengan pesan jelas, err = %v", err)
	}
}

// Tabel large: true: salinan per batch di luar transaksi (NO TRANSACTION) menyalin semua baris.
func TestRename_LargeTableCopiesInBatches(t *testing.T) {
	fx := buildRenameFixture(t, true, 40)
	if !fx.res2.NoTransaction || !strings.HasPrefix(fx.res2.SQL, "-- +goose NO TRANSACTION\n") || !strings.Contains(fx.res2.SQL, "LIMIT 40") {
		t.Fatalf("migrasi large harus NO TRANSACTION dengan batch 40:\n%s", fx.res2.SQL)
	}
	ctx := context.Background()
	dsn := pgtest.Start(t)
	p, _ := newProvider(t, dsn, fx.files)
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	db := openSQL(t, dsn)
	seedPhones(t, db, 250)
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatalf("0002: %v\n%s", err, fx.res2.SQL)
	}
	var copied, total int
	if err := db.QueryRow(`SELECT count(*) FILTER (WHERE phone_e164 = phone), count(*) FROM m_things`).Scan(&copied, &total); err != nil || copied != 250 || total != 250 {
		t.Fatalf("tersalin %d dari %d (%v)", copied, total, err)
	}
	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("0003: %v", err)
	}
}

// Rename pada dokumen: pemicu penguncian diganti (pasangan migrateFrom baru) sebelum salinan sehingga baris
// terkunci ikut tersalin lewat pengecualian (b); setelahnya pemicu tetap menolak ubah kolom lain.
func TestRename_LockedDocumentRowsAreCopiedThroughTriggerException(t *testing.T) {
	doc := func(body string) *dbmodel.Schema {
		return modelOf(t, f("m/doc.resource.yaml", resourceDoc("m.doc", "include:\n  - source: sdk:document\n"+body)))
	}
	v1 := doc("fields:\n  phone: {type: string, maxLength: 20, nullable: true, label: m.doc.phone}\n")
	v2 := doc("fields:\n  phone: {type: string, maxLength: 20, nullable: true, deprecated: {since: \"1.1.0\", replacedBy: phoneE164}, label: m.doc.phone}\n  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.doc.phoneE164}\n")
	files := baseFS(t, v1, nil)
	res := genNext(t, v2, files, 0)
	if len(res.Todos) != 0 || !strings.Contains(res.SQL, "CREATE OR REPLACE FUNCTION") {
		t.Fatalf("pemicu harus diganti otomatis:\n%s", res.SQL)
	}
	files["0002_rename.sql"] = &fstest.MapFile{Data: []byte(res.SQL)}

	ctx := context.Background()
	dsn := pgtest.Start(t)
	p, _ := newProvider(t, dsn, files)
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	db := openSQL(t, dsn)
	for i, status := range []string{"draft", "accepted", "cancelled"} {
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
		if _, err := db.Exec(`INSERT INTO m_docs (id, phone) VALUES ($1::uuid, '0899' || right($1::text, 8))`, id); err != nil {
			t.Fatal(err)
		}
		if status == "draft" {
			continue
		}
		if _, err := db.Exec(`UPDATE m_docs SET status = 'accepted' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if status == "cancelled" {
			if _, err := db.Exec(`UPDATE m_docs SET status = 'cancelled' WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatalf("migrasi rename pada dokumen terkunci harus lolos: %v\n%s", err, res.SQL)
	}
	var copied, total int
	if err := db.QueryRow(`SELECT count(*) FILTER (WHERE phone_e164 = phone), count(*) FROM m_docs`).Scan(&copied, &total); err != nil || copied != 3 || total != 3 {
		t.Fatalf("tersalin %d dari %d (%v)", copied, total, err)
	}
	// Pemicu tetap aktif untuk perubahan lain.
	if _, err := db.Exec(`UPDATE m_docs SET phone = 'lain' WHERE status = 'accepted'`); err == nil || !strings.Contains(err.Error(), "dokumen terkunci") {
		t.Errorf("pemicu harus tetap menolak ubah kolom asal pada dokumen terkunci, err = %v", err)
	}
	// Dan selisih kosong terhadap metadata v2.
	rep, err := diff.Run(ctx, diff.Options{DSN: pgtest.Start(t), Sources: []diff.Source{{Slug: "m", Migrations: files}}, Model: v2})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Empty() {
		t.Errorf("selisih harus kosong:\n%s", rep)
	}
}
