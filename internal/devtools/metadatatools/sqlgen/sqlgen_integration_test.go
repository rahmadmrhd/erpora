//go:build integration

package sqlgen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/sdk/metadata"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", pgtest.Start(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%v\nSQL: %s", err, q)
	}
}

func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

var nastyStrings = []string{"a'b", "$$", "*/", "`", "x\ny", "'); DROP TABLE t_things; --", `\`, `a\'b`, "$erpora$", `"quoted"`, "--", "/*", "a;b", "日本語", "", "E'x'"}

// Literal terbaca kembali persis oleh PostgreSQL, juga bila standard_conforming_strings dimatikan.
func TestLiteral_RoundTripsThroughPostgres(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	for _, scs := range []string{"on", "off"} {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, "SET standard_conforming_strings = "+scs); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, "SET escape_string_warning = off"); err != nil {
			t.Fatal(err)
		}
		for _, s := range nastyStrings {
			lit, err := Literal(s)
			if err != nil {
				t.Fatal(err)
			}
			var got string
			if err := conn.QueryRowContext(ctx, "SELECT "+lit).Scan(&got); err != nil {
				t.Fatalf("scs=%s Literal(%q)=%s: %v", scs, s, lit, err)
			}
			if got != s {
				t.Errorf("scs=%s Literal(%q) = %s dibaca %q", scs, s, lit, got)
			}
		}
		_ = conn.Close()
	}
}

// DDL dengan nilai berbahaya dijalankan: tepat satu tabel dibuat, nilai berbahaya adalah data.
func TestCreateTable_DangerousValuesAreData(t *testing.T) {
	db := openDB(t)
	nasty := []string{"a'b", "$$", "*/", "`", "x\ny", "'); DROP TABLE t_things; --", `\`, "$erpora$", `"quoted"`, "--", "/*", "a;b"}
	tb := &dbmodel.Table{
		Name: "t_things", Resource: "t.thing", Module: "t", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true},
			{Name: "kind", Type: dbmodel.TypeText, NotNull: true, Default: &dbmodel.Default{Type: metadata.TypeEnum, Value: "'); DROP TABLE t_things; --"}},
			{Name: `we"ird`, Type: dbmodel.TypeText},
		},
		PrimaryKey: []string{"id"},
		Checks:     []*dbmodel.Check{{Name: "t_things_kind_check", Column: "kind", Kind: dbmodel.CheckEnum, Values: append([]string{"'); DROP TABLE t_things; --"}, nasty...)}},
		Uniques:    []*dbmodel.Unique{{Name: `t_things_"x"_key`, Columns: []string{`we"ird`}}},
	}
	q, err := CreateTable(tb, Options{})
	if err != nil {
		t.Fatal(err)
	}
	exec(t, db, q)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname = 'public'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("tabel = %d, %v", n, err)
	}
	exec(t, db, `INSERT INTO t_things (id) VALUES (gen_random_uuid())`)
	var def string
	if err := db.QueryRow(`SELECT kind FROM t_things`).Scan(&def); err != nil || def != "'); DROP TABLE t_things; --" {
		t.Fatalf("default = %q, %v", def, err)
	}
	for _, v := range nasty {
		exec(t, db, `INSERT INTO t_things (id, kind) VALUES (gen_random_uuid(), $1)`, v)
	}
	if _, err := db.Exec(`INSERT INTO t_things (id, kind) VALUES (gen_random_uuid(), 'bukan-nilai-sah')`); sqlState(err) != "23514" {
		t.Errorf("nilai di luar daftar harus melanggar CHECK, err = %v", err)
	}
}

// Paritas penerjemah ekspresi dan evaluator Go: kolom turunan PostgreSQL menghasilkan nilai yang sama dengan
// metadata.EvaluateExpr untuk semua operasi dan mode pembulatan.
func TestStoredExpr_ParityWithGoEvaluator(t *testing.T) {
	db := openDB(t)
	modes := []string{"half_up", "half_down", "half_even", "up", "down", "floor", "ceiling"}
	ops := []string{"add", "sub", "mul", "div"}
	env := Env{"a": {Column: "a", Type: metadata.TypeDecimal}, "b": {Column: "b", Type: metadata.TypeDecimal}}
	types := metadata.Types{"a": metadata.TypeDecimal, "b": metadata.TypeDecimal}

	type exprCase struct {
		col string
		raw map[string]any
	}
	var cases []exprCase
	var cols []string
	for _, op := range ops {
		for _, mode := range modes {
			for _, scale := range []int{0, 2} {
				raw := map[string]any{op: []any{map[string]any{"field": "a"}, map[string]any{"field": "b"}}, "scale": scale, "round": mode}
				e, err := metadata.ParseExpr(raw)
				if err != nil {
					t.Fatal(err)
				}
				sqlText, err := StoredExpr(e, env)
				if err != nil {
					t.Fatal(err)
				}
				col := fmt.Sprintf("g_%s_%s_%d", op, mode, scale)
				cols = append(cols, `"`+col+`" numeric(60,24) GENERATED ALWAYS AS (`+sqlText+`) STORED`)
				cases = append(cases, exprCase{col, raw})
			}
		}
	}
	exec(t, db, `CREATE TABLE p (id serial PRIMARY KEY, a numeric(38,22), b numeric(38,22), `+strings.Join(cols, ", ")+`)`)

	rng := rand.New(rand.NewSource(20261101))
	dec := func() string {
		// nilai dengan sebaran pecahan termasuk titik tengah (…5) agar mode pembulatan terbedakan
		whole := rng.Intn(2000) - 1000
		frac := []int{0, 5000, 2500, 7500, 500, 1250, 3750, 1, 9999, rng.Intn(10000)}[rng.Intn(10)]
		sign := ""
		if whole < 0 {
			sign = "-"
			whole = -whole
		}
		return fmt.Sprintf("%s%d.%04d", sign, whole, frac)
	}
	dec22 := func() string {
		sign := ""
		if rng.Intn(2) == 0 {
			sign = "-"
		}
		return fmt.Sprintf("%s%d.%011d%011d", sign, rng.Intn(3), rng.Int63n(100_000_000_000), rng.Int63n(100_000_000_000))
	}
	type row struct{ a, b string }
	var rows []row
	for i := 0; i < 150; i++ {
		r := row{dec(), dec()}
		if i%25 == 0 {
			r.b = "0.0000"
		}
		if i == 1 {
			r = row{"2.5000", "1.0000"}
		}
		if i == 2 {
			r = row{"-2.5000", "1.0000"}
		}
		if i == 3 {
			r = row{"0.1250", "1.0000"}
		}
		if i >= 4 && i < 40 {
			r = row{dec22(), dec22()} // digit panjang: bagi sejati vs pembulatan ganda
		}
		rows = append(rows, r)
		exec(t, db, `INSERT INTO p (a, b) VALUES ($1, $2)`, r.a, r.b)
	}
	// NULL merambat.
	exec(t, db, `INSERT INTO p (a, b) VALUES (NULL, 1.5000), (1.5000, NULL)`)
	rows = append(rows, row{"", "1.5000"}, row{"1.5000", ""})
	// Kasus pembulatan ganda dan titik tengah pada pembagian (M3).
	for _, r := range []row{{"0.0149999999999999999999", "3"}, {"-0.0149999999999999999999", "3"}, {"0.0150000000000000000001", "1"}, {"5", "2"}, {"-5", "2"}, {"7", "2"}, {"-7", "2"}, {"0.125", "1"}, {"-0.125", "1"}, {"1", "3"}, {"2", "3"}, {"0.001", "7"}} {
		exec(t, db, `INSERT INTO p (a, b) VALUES ($1, $2)`, r.a, r.b)
		rows = append(rows, r)
	}

	mismatches := 0
	for _, c := range cases {
		q, err := db.Query(`SELECT "` + c.col + `"::text FROM p ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		for q.Next() {
			var got sql.NullString
			if err := q.Scan(&got); err != nil {
				t.Fatal(err)
			}
			rec := metadata.Record{"a": nilIfEmpty(rows[i].a), "b": nilIfEmpty(rows[i].b)}
			res := metadata.EvaluateExpr(c.raw, types, rec)
			switch {
			case len(res.Reasons) > 0 || res.Value == nil:
				if got.Valid {
					mismatches++
					t.Errorf("%s a=%q b=%q: Go tak tentu/null, PostgreSQL %s", c.col, rows[i].a, rows[i].b, got.String)
				}
			case !got.Valid:
				mismatches++
				t.Errorf("%s a=%q b=%q: Go %v, PostgreSQL NULL", c.col, rows[i].a, rows[i].b, res.Value)
			default:
				want, _ := metadata.ParseDecimal(res.Value.(string))
				have, ok := metadata.ParseDecimal(got.String)
				if !ok || want.Cmp(have) != 0 {
					mismatches++
					t.Errorf("%s a=%q b=%q: Go %v, PostgreSQL %s", c.col, rows[i].a, rows[i].b, res.Value, got.String)
				}
			}
			if mismatches > 20 {
				t.Fatal("terlalu banyak selisih")
			}
			i++
		}
		_ = q.Close()
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Perilaku pemicu penguncian dokumen (templat tetap).
func TestLockTrigger_Behaviour(t *testing.T) {
	db := openDB(t)
	tb := &dbmodel.Table{
		Name: "m_docs", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true, FieldType: metadata.TypeUUID},
			{Name: "status", Type: dbmodel.TypeText, NotNull: true, Field: "status", FieldType: metadata.TypeEnum, Default: &dbmodel.Default{Type: metadata.TypeEnum, Value: "draft"}},
			{Name: "version", Type: dbmodel.TypeBigint, NotNull: true, Field: "version", FieldType: metadata.TypeInteger, Default: &dbmodel.Default{Type: metadata.TypeInteger, Value: "1"}},
			{Name: "updated_at", Type: dbmodel.TypeTimestamp, Field: "updatedAt", FieldType: metadata.TypeDateTime},
			{Name: "updated_by", Type: dbmodel.TypeUUID, Field: "updatedBy", FieldType: metadata.TypeUUID},
			{Name: "note", Type: dbmodel.TypeText, Field: "note", FieldType: metadata.TypeText},
			{Name: "phone", Type: dbmodel.TypeText, Field: "phone", FieldType: metadata.TypeString},
			{Name: "phone_e164", Type: dbmodel.TypeText, Field: "phoneE164", FieldType: metadata.TypeString, MigrateFrom: "phone"},
			{Name: "version2", Type: dbmodel.Numeric(20, 2), Field: "version2", FieldType: metadata.TypeDecimal,
				Generated: mustExpr(t, map[string]any{"mul": []any{map[string]any{"field": "version"}, map[string]any{"dec": "2"}}, "scale": 2, "round": "half_up"})},
		},
		PrimaryKey: []string{"id"},
	}
	cond, err := metadata.ParseCondition(map[string]any{"field": "status", "op": "ne", "value": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	tr := &dbmodel.Trigger{Name: "m_docs_lock_trg", Function: "m_docs_lock_fn", Events: []string{"DELETE", "UPDATE"}, Lock: &dbmodel.LockSpec{
		Condition: cond, StatusColumn: "status", VersionColumn: "version", UpdatedAtColumn: "updated_at", UpdatedByColumn: "updated_by",
		Transitions:  [][2]string{{"draft", "submitted"}, {"submitted", "cancelled"}},
		MigratePairs: [][2]string{{"phone_e164", "phone"}},
	}}
	tb.Triggers = []*dbmodel.Trigger{tr}
	stmts, err := CreateSchema(&dbmodel.Schema{Tables: []*dbmodel.Table{tb}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		exec(t, db, s.SQL)
	}

	id := func(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }
	exec(t, db, `INSERT INTO m_docs (id, phone) VALUES ($1, 'x'), ($2, 'x'), ($3, NULL)`, id(1), id(2), id(3))

	mustOK := func(name, q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Errorf("%s: harus lolos: %v", name, err)
		}
	}
	mustLocked := func(name, q string, args ...any) {
		t.Helper()
		_, err := db.Exec(q, args...)
		if sqlState(err) != "55000" {
			t.Errorf("%s: harus ditolak SQLSTATE 55000, err = %v", name, err)
		}
	}

	mustOK("ubah draft bebas", `UPDATE m_docs SET note = 'a', phone = 'y' WHERE id = $1`, id(1))
	mustOK("draft -> submitted (status, version, updated_at)", `UPDATE m_docs SET status = 'submitted', version = version + 1, updated_at = now() WHERE id = $1`, id(1))
	mustLocked("ubah kolom lain pada baris terkunci", `UPDATE m_docs SET note = 'b' WHERE id = $1`, id(1))
	mustLocked("ubah kolom lain bersama transisi", `UPDATE m_docs SET status = 'cancelled', note = 'c' WHERE id = $1`, id(1))
	mustLocked("hanya version", `UPDATE m_docs SET version = version + 1 WHERE id = $1`, id(1))
	mustLocked("submitted -> draft", `UPDATE m_docs SET status = 'draft' WHERE id = $1`, id(1))
	mustOK("submitted -> cancelled", `UPDATE m_docs SET status = 'cancelled', version = version + 1 WHERE id = $1`, id(1))
	mustLocked("cancelled -> submitted", `UPDATE m_docs SET status = 'submitted' WHERE id = $1`, id(1))
	mustLocked("DELETE baris terkunci", `DELETE FROM m_docs WHERE id = $1`, id(1))

	mustOK("draft -> submitted (baris 2)", `UPDATE m_docs SET status = 'submitted' WHERE id = $1`, id(2))
	mustOK("isi kolom tujuan migrateFrom dengan salinan persis", `UPDATE m_docs SET phone_e164 = phone WHERE id = $1`, id(2))
	mustLocked("kolom tujuan sudah terisi tak boleh berubah", `UPDATE m_docs SET phone_e164 = 'lain' WHERE id = $1`, id(2))

	mustOK("draft -> submitted (baris 3)", `UPDATE m_docs SET status = 'submitted' WHERE id = $1`, id(3))
	mustLocked("kolom tujuan bukan salinan persis", `UPDATE m_docs SET phone_e164 = 'zzz' WHERE id = $1`, id(3))
	mustLocked("kolom asal ikut berubah", `UPDATE m_docs SET phone_e164 = 'q', phone = 'q' WHERE id = $1`, id(3))

	exec(t, db, `INSERT INTO m_docs (id) VALUES ($1)`, id(4))
	mustOK("DELETE draft", `DELETE FROM m_docs WHERE id = $1`, id(4))

	mustLocked("UPDATE dengan predikat status tetap ditolak", `UPDATE m_docs SET note = 'x' WHERE status <> 'draft' AND id = $1`, id(2))
}

func mustExpr(t *testing.T, raw any) *metadata.Expr {
	t.Helper()
	e, err := metadata.ParseExpr(raw)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// Pasangan migrateFrom dengan kolom asal yang sudah dihapus dari tabel: fungsi tidak rusak dan tidak ada yang
// disalin (kolom asal dianggap null); perubahan lain pada baris terkunci tetap ditolak 55000.
func TestLockTrigger_MigratePairWithDroppedSourceColumn(t *testing.T) {
	db := openDB(t)
	tb := &dbmodel.Table{
		Name: "m_docs", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true, FieldType: metadata.TypeUUID},
			{Name: "status", Type: dbmodel.TypeText, NotNull: true, Field: "status", FieldType: metadata.TypeEnum, Default: &dbmodel.Default{Type: metadata.TypeEnum, Value: "draft"}},
			{Name: "note", Type: dbmodel.TypeText, Field: "note", FieldType: metadata.TypeText},
			{Name: "phone_e164", Type: dbmodel.TypeText, Field: "phoneE164", FieldType: metadata.TypeString, MigrateFrom: "phone"},
		},
		PrimaryKey: []string{"id"},
	}
	cond, err := metadata.ParseCondition(map[string]any{"field": "status", "op": "ne", "value": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	tb.Triggers = []*dbmodel.Trigger{{Name: "m_docs_lock_trg", Function: "m_docs_lock_fn", Events: []string{"DELETE", "UPDATE"}, Lock: &dbmodel.LockSpec{
		Condition: cond, StatusColumn: "status", VersionColumn: "status", UpdatedAtColumn: "status", UpdatedByColumn: "status",
		MigratePairs: [][2]string{{"phone_e164", "phone"}},
	}}}
	stmts, err := CreateSchema(&dbmodel.Schema{Tables: []*dbmodel.Table{tb}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		exec(t, db, s.SQL)
	}
	id := "00000000-0000-0000-0000-000000000001"
	exec(t, db, `INSERT INTO m_docs (id, status) VALUES ($1, 'accepted')`, id)
	for name, q := range map[string]string{
		"isi dari kolom asal yang tidak ada": `UPDATE m_docs SET phone_e164 = 'x' WHERE id = $1`,
		"ubah kolom lain":                    `UPDATE m_docs SET note = 'x' WHERE id = $1`,
	} {
		if _, err := db.Exec(q, id); sqlState(err) != "55000" {
			t.Errorf("%s: harus ditolak 55000, err = %v", name, err)
		}
	}
	// Tanpa perubahan nyata pada kolom tujuan (tetap null) bukan pengecualian.
	if _, err := db.Exec(`UPDATE m_docs SET phone_e164 = NULL WHERE id = $1`, id); sqlState(err) != "55000" {
		t.Errorf("tanpa perubahan harus ditolak 55000, err = %v", err)
	}
}
