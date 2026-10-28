//go:build integration

package integration

import (
	"database/sql"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"erpora/internal/platform/testsupport/pgtest"
)

const insertNotesSQL = "WITH ins AS (INSERT INTO example_notes (id, title) SELECT gen_random_uuid(), 'tambahan ' || g FROM generate_series(1, 5) g RETURNING 1) SELECT count(*) FROM ins"

func tableCount(t *testing.T, dsn string) int64 {
	t.Helper()
	return dbQuery(t, dsn, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'")
}

// Review Focus 4 (AC FR-22): db:fresh ditolak SEBELUM database disentuh.
func TestDBFresh_RefusedWithoutTouchingDB(t *testing.T) {
	// (a) ERP_ENV=production dengan DB tidak terjangkau: ditolak oleh pemeriksaan env, bukan error koneksi.
	dead := "postgres://erp:erp@127.0.0.1:1/erp?sslmode=disable&connect_timeout=1"
	out, code := cliRunBin(t, erpBin, dead, prodEnv, "", "db:fresh", "--yes")
	if code != 3 {
		t.Fatalf("exit %d, want 3\n%s", code, out)
	}
	if !strings.Contains(out, "db:fresh/db:seed hanya untuk ERP_ENV=development") {
		t.Errorf("pesan harus ErrNotDevelopment:\n%s", out)
	}
	if strings.Contains(out, "tidak terjangkau") {
		t.Errorf("DB tidak boleh dibuka sebelum pemeriksaan env:\n%s", out)
	}

	// (b) DB berpenanda production, ERP_ENV=development --yes: ditolak, data tidak berubah.
	dsn := pgtest.Start(t)
	if out, code := cliRunBin(t, erpBinRelease, dsn, prodEnv, "", "migrate", "up"); code != 0 {
		t.Fatalf("release migrate up: exit %d\n%s", code, out)
	}
	tables := tableCount(t, dsn)
	state := dbQuery(t, dsn, "SELECT count(*) FROM platform_state")
	for _, args := range [][]string{{"db:fresh", "--yes"}, {"db:seed"}} {
		out, code := cliRun(t, dsn, nil, "", args...)
		if code != 3 {
			t.Fatalf("%s: exit %d, want 3\n%s", args[0], code, out)
		}
		if !strings.Contains(out, "hanya untuk ERP_ENV=development") {
			t.Errorf("%s: pesan harus ErrNotDevelopment:\n%s", args[0], out)
		}
	}
	if n := tableCount(t, dsn); n != tables {
		t.Errorf("jumlah tabel berubah: %d -> %d", tables, n)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM platform_state"); n != state {
		t.Errorf("platform_state berubah: %d -> %d", state, n)
	}
}

func TestDBFresh_ConfirmAndRebuild(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	mustCLI(t, dsn, 0, "modules", "install", "acme.example-addon", "--with-deps", "--enable")
	if n := dbQuery(t, dsn, insertNotesSQL); n != 5 {
		t.Fatalf("insert tambahan = %d", n)
	}
	const notes = "SELECT count(*) FROM example_notes"
	if n := dbQuery(t, dsn, notes); n != 8 {
		t.Fatalf("notes awal = %d, want 8 (3 seed install + 5)", n)
	}

	// Nama database salah: dibatalkan (exit 1), data tetap.
	out, code := cliRun(t, dsn, nil, "salah\n", "db:fresh")
	if code != 1 {
		t.Fatalf("konfirmasi salah: exit %d, want 1\n%s", code, out)
	}
	if n := dbQuery(t, dsn, notes); n != 8 {
		t.Fatalf("data berubah setelah dibatalkan: %d", n)
	}

	// Nama database benar: dibangun ulang; kedua modul enabled, notes kembali ke seed install.
	out = mustCLIStdin(t, dsn, 0, dbNameOf(t, dsn)+"\n", "db:fresh")
	if n := dbQuery(t, dsn, notes); n != 3 {
		t.Fatalf("notes setelah fresh = %d, want 3\n%s", n, out)
	}
	list := mustCLI(t, dsn, 0, "modules", "list")
	for _, key := range []string{"acme.example", "acme.example-addon"} {
		if !regexp.MustCompile(regexp.QuoteMeta(key) + `\s+\S+\s+\S+\s+enabled`).MatchString(list) {
			t.Fatalf("%s harus enabled setelah fresh:\n%s", key, list)
		}
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM platform_state WHERE key = 'environment' AND value = 'development'"); n != 1 {
		t.Fatalf("penanda development tidak ada setelah fresh (n=%d)", n)
	}

	// --seed --yes: DevSeeder berjalan.
	out = mustCLI(t, dsn, 0, "db:fresh", "--seed", "--yes")
	if n := dbQuery(t, dsn, notes); n <= 3 {
		t.Fatalf("notes setelah fresh --seed = %d, want > 3\n%s", n, out)
	}
	if !strings.Contains(out, "seed acme.example selesai") {
		t.Errorf("keluaran seed:\n%s", out)
	}

	// db:seed satu modul (nama direktori) idempoten.
	before := dbQuery(t, dsn, notes)
	mustCLI(t, dsn, 0, "db:seed", "example")
	if n := dbQuery(t, dsn, notes); n != before {
		t.Fatalf("db:seed tidak idempoten: %d -> %d", before, n)
	}
}

func dbExec(t *testing.T, dsn, stmt string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

// G-8: db:fresh menghapus semua jenis tipe buatan pengguna: enum, domain, range (beserta multirange), fungsi, dan view.
func TestDBFresh_DropsEnumDomainRangeFunctionAndView(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	for _, stmt := range []string{
		"CREATE TYPE mood AS ENUM ('sad', 'ok')",
		"CREATE DOMAIN posint AS integer CHECK (VALUE > 0)",
		"CREATE TYPE floatrange AS RANGE (subtype = float8)",
		"CREATE FUNCTION twice(x integer) RETURNS integer LANGUAGE sql AS 'SELECT x * 2'",
		"CREATE TABLE zz_probe (id integer, m mood, r floatrange)",
		"CREATE VIEW zz_probe_v AS SELECT id FROM zz_probe",
		"CREATE SEQUENCE zz_seq",
	} {
		dbExec(t, dsn, stmt)
	}
	const userTypes = "SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype IN ('e', 'd', 'r', 'm')"
	if n := dbQuery(t, dsn, userTypes); n < 4 { // mood, posint, floatrange, multirange-nya
		t.Fatalf("persiapan: tipe pengguna = %d, want >= 4", n)
	}
	out, code := cliRun(t, dsn, nil, "", "db:fresh", "--yes")
	if code != 0 {
		t.Fatalf("db:fresh: exit %d\n%s", code, out)
	}
	if n := dbQuery(t, dsn, userTypes); n != 0 {
		t.Errorf("tipe enum/domain/range tersisa setelah db:fresh: %d", n)
	}
	for q, what := range map[string]string{
		"SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public' AND p.proname = 'twice'": "fungsi",
		"SELECT count(*) FROM pg_class WHERE relname IN ('zz_probe', 'zz_probe_v', 'zz_seq')":                                             "tabel/view/sequence",
	} {
		if n := dbQuery(t, dsn, q); n != 0 {
			t.Errorf("%s tersisa setelah db:fresh: %d", what, n)
		}
	}
}

func TestDBSeed_RequiresDevelopmentMarker(t *testing.T) {
	dsn := pgtest.Start(t) // database kosong: belum dimigrasi, belum berpenanda
	out, code := cliRun(t, dsn, nil, "", "db:seed")
	if code != 3 {
		t.Fatalf("exit %d, want 3\n%s", code, out)
	}
	if !strings.Contains(out, "migrate up") {
		t.Errorf("pesan harus menyebut migrate up:\n%s", out)
	}
	if n := tableCount(t, dsn); n != 0 {
		t.Errorf("db:seed tidak boleh membuat objek: %d tabel", n)
	}
}

func mustCLIStdin(t *testing.T, dsn string, want int, stdin string, args ...string) string {
	t.Helper()
	out, code := cliRun(t, dsn, nil, stdin, args...)
	if code != want {
		t.Fatalf("erpora %s: exit %d, want %d\n%s", strings.Join(args, " "), code, want, out)
	}
	return out
}

// dbNameOf mengembalikan nama database pada DSN (container memakai "erp"; server eksternal memakai nama sementara).
func dbNameOf(t *testing.T, dsn string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(u.Path, "/")
}
