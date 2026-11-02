//go:build integration

package diff

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func run(t *testing.T, dsn string, model *dbmodel.Schema, sources []Source) (*Report, error) {
	t.Helper()
	return Run(context.Background(), Options{DSN: dsn, Sources: sources, Model: model, NotePrefixes: []string{"fixturecore_", "fixturesales_"}})
}

func mustRun(t *testing.T, dsn string, model *dbmodel.Schema, sources []Source) *Report {
	t.Helper()
	r, err := run(t, dsn, model, sources)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r
}

// Selisih kosong untuk modul fixture: migrasi dijalankan, katalog dibaca, dibandingkan dengan model turunan.
func TestRun_FixtureDiffEmpty(t *testing.T) {
	model := fixtureModel(t)
	r := mustRun(t, pgtest.Start(t), model, fixtureSources(t, model, nil))
	if !r.Empty() || len(r.Notes) != 0 {
		t.Fatalf("selisih harus kosong:\n%s", r)
	}
}

// Pemicu dikenali: ada dan aktif -> kosong.
func TestRun_TriggerRecognized(t *testing.T) {
	model := fixtureModel(t)
	addLockTrigger(t, model)
	r := mustRun(t, pgtest.Start(t), model, fixtureSources(t, model, nil))
	if !r.Empty() {
		t.Fatalf("selisih harus kosong:\n%s", r)
	}
}

// Setiap penyimpangan antara database dan metadata menjadi selisih (Requirement 10.1).
func TestRun_DetectsDifferences(t *testing.T) {
	cases := []struct {
		name    string
		trigger bool
		module  string
		sql     string
		want    []string
	}{
		{"kolom dihapus", false, "fixturecore", `ALTER TABLE fixturecore_customers DROP COLUMN blocked_reason;`, []string{KindCheckMissing, KindColumnMissing}},
		{"kolom berlebih", false, "fixturecore", `ALTER TABLE fixturecore_branches ADD COLUMN extra text;`, []string{KindColumnExtra}},
		{"tipe berubah", false, "fixturecore", `ALTER TABLE fixturecore_branches ALTER COLUMN name TYPE varchar(120);`, []string{KindCheckExtra, KindCheckMissing, KindColumnType}},
		{"nullability berubah", false, "fixturecore", `ALTER TABLE fixturecore_customers ALTER COLUMN contact_email SET NOT NULL;`, []string{KindColumnNullability}},
		{"default berubah", false, "fixturecore", `ALTER TABLE fixturecore_customers ALTER COLUMN status SET DEFAULT 'inactive';`, []string{KindColumnDefault}},
		{"default ditambah", false, "fixturecore", `ALTER TABLE fixturecore_branches ALTER COLUMN name SET DEFAULT 'x';`, []string{KindColumnDefault}},
		{"unique tak dideklarasikan", false, "fixturecore", `ALTER TABLE fixturecore_branches ADD CONSTRAINT zz_name_uq UNIQUE (name);`, []string{KindUniqueExtra}},
		{"indeks unik tak dideklarasikan", false, "fixturecore", `CREATE UNIQUE INDEX zz_name_uidx ON fixturecore_branches (name);`, []string{KindIndexExtra}},
		{"indeks tak dideklarasikan", false, "fixturecore", `CREATE INDEX zz_name_idx ON fixturecore_branches (name);`, []string{KindIndexExtra}},
		{"indeks dideklarasikan dihapus", false, "fixturecore", `DROP INDEX fixturecore_customers_status_name_idx;`, []string{KindIndexMissing}},
		{"unique dideklarasikan dihapus", false, "fixturecore", `ALTER TABLE fixturecore_branches DROP CONSTRAINT fixturecore_branches_code_key;`, []string{KindUniqueMissing}},
		{"CHECK dihapus", false, "fixturecore", `ALTER TABLE fixturecore_customers DROP CONSTRAINT fixturecore_customers_status_check;`, []string{KindCheckMissing}},
		{"CHECK berubah", false, "fixturecore", `ALTER TABLE fixturecore_branches DROP CONSTRAINT fixturecore_branches_name_check, ADD CONSTRAINT fixturecore_branches_name_check CHECK (length(name) <= 99);`, []string{KindCheckExtra, KindCheckMissing}},
		{"FK dihapus", false, "fixturesales", `ALTER TABLE fixturesales_invoice_lines DROP CONSTRAINT fixturesales_invoice_lines_invoice_id_fkey;`, []string{KindForeignKeyMissing}},
		{"FK berubah aksi", false, "fixturesales", `ALTER TABLE fixturesales_invoice_lines DROP CONSTRAINT fixturesales_invoice_lines_invoice_id_fkey, ADD CONSTRAINT fixturesales_invoice_lines_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES fixturesales_invoices (id);`, []string{KindForeignKeyExtra, KindForeignKeyMissing}},
		{"tabel dihapus", false, "fixturesales", `DROP TABLE fixturesales_invoice_tags;`, []string{KindTableMissing}},
		{"kolom turunan berubah", false, "fixturesales", `ALTER TABLE fixturesales_invoice_lines DROP COLUMN line_total, ADD COLUMN line_total numeric(18,2) GENERATED ALWAYS AS (round(quantity * unit_price, 3)) STORED NOT NULL;`, []string{KindColumnGenerated}},
		{"pemicu dihapus", true, "fixturesales", `DROP TRIGGER fixturesales_invoices_lock_trg ON fixturesales_invoices;`, []string{KindTriggerMissing}},
		{"pemicu dinonaktifkan", true, "fixturesales", `ALTER TABLE fixturesales_invoices DISABLE TRIGGER fixturesales_invoices_lock_trg;`, []string{KindTriggerDisabled}},
		{"pemicu replika saja", true, "fixturesales", `ALTER TABLE fixturesales_invoices ENABLE REPLICA TRIGGER fixturesales_invoices_lock_trg;`, []string{KindTriggerDisabled}},
		{"pemicu ENABLE biasa (dapat dilewati session_replication_role)", true, "fixturesales", `ALTER TABLE fixturesales_invoices ENABLE TRIGGER fixturesales_invoices_lock_trg;`, []string{KindTriggerDisabled}},
		{"pemicu anak dihapus", true, "fixturesales", `DROP TRIGGER fixturesales_invoice_lines_invoice_id_lock_trg ON fixturesales_invoice_lines;`, []string{KindTriggerMissing}},
		{"pemicu pivot dinonaktifkan", true, "fixturesales", `ALTER TABLE fixturesales_invoice_tags DISABLE TRIGGER fixturesales_invoice_tags_invoice_id_lock_trg;`, []string{KindTriggerDisabled}},
		{"pemicu dibuat ulang dengan WHEN (false) dan ENABLE ALWAYS", true, "fixturesales", `DROP TRIGGER fixturesales_invoices_lock_trg ON fixturesales_invoices;
CREATE TRIGGER fixturesales_invoices_lock_trg BEFORE DELETE OR UPDATE ON fixturesales_invoices FOR EACH ROW WHEN (false) EXECUTE FUNCTION fixturesales_invoices_lock_fn();
ALTER TABLE fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg;`, []string{KindTriggerChanged}},
		{"pemicu dibuat ulang dengan UPDATE OF kolom tertentu", true, "fixturesales", `DROP TRIGGER fixturesales_invoices_lock_trg ON fixturesales_invoices;
CREATE TRIGGER fixturesales_invoices_lock_trg BEFORE DELETE OR UPDATE OF approval_note ON fixturesales_invoices FOR EACH ROW EXECUTE FUNCTION fixturesales_invoices_lock_fn();
ALTER TABLE fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg;`, []string{KindTriggerChanged}},
		{"fungsi pemicu diberi SET search_path", true, "fixturesales", `ALTER FUNCTION fixturesales_invoices_lock_fn() SET search_path = pg_catalog;`, []string{KindTriggerChanged}},
		{"fungsi pemicu SECURITY DEFINER", true, "fixturesales", `ALTER FUNCTION fixturesales_invoices_lock_fn() SECURITY DEFINER;`, []string{KindTriggerChanged}},
		{"pemicu menunjuk fungsi di skema lain", true, "fixturesales", "CREATE SCHEMA zz_evil;\n-- +goose StatementBegin\nCREATE FUNCTION zz_evil.fixturesales_invoices_lock_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;\n-- +goose StatementEnd\nDROP TRIGGER fixturesales_invoices_lock_trg ON fixturesales_invoices;\nCREATE TRIGGER fixturesales_invoices_lock_trg BEFORE DELETE OR UPDATE ON fixturesales_invoices FOR EACH ROW EXECUTE FUNCTION zz_evil.fixturesales_invoices_lock_fn();\nALTER TABLE fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg;", []string{KindTriggerChanged}},
		{"pemicu tulis tangan sesudah pemicu penguncian dapat mengubah NEW", true, "fixturesales", "-- +goose StatementBegin\nCREATE FUNCTION zz_modify_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.status := 'draft'; RETURN NEW; END $$;\n-- +goose StatementEnd\nCREATE TRIGGER zz_modify_trg BEFORE UPDATE ON fixturesales_invoices FOR EACH ROW EXECUTE FUNCTION zz_modify_fn();", []string{KindTriggerShadow}},
		{"pemicu tulis tangan sebelum pemicu penguncian hanya catatan", true, "fixturesales", "-- +goose StatementBegin\nCREATE FUNCTION aa_audit_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;\n-- +goose StatementEnd\nCREATE TRIGGER aa_audit_trg BEFORE UPDATE ON fixturesales_invoices FOR EACH ROW EXECUTE FUNCTION aa_audit_fn();", nil},
		{"indeks DESC (opclass dan urutan ikut dibandingkan)", false, "fixturecore", `DROP INDEX fixturecore_customers_status_name_idx; CREATE INDEX fixturecore_customers_status_name_idx ON fixturecore_customers (status, name DESC);`, []string{KindIndexExtra, KindIndexMissing}},
		{"indeks parsial", false, "fixturecore", `DROP INDEX fixturecore_customers_status_name_idx; CREATE INDEX fixturecore_customers_status_name_idx ON fixturecore_customers (status, name) WHERE status = 'active';`, []string{KindIndexExtra, KindIndexMissing}},
		{"kolasi kolom berbeda", false, "fixturecore", `ALTER TABLE fixturecore_branches ALTER COLUMN name TYPE text COLLATE "C";`, []string{KindColumnCollation}},
		{"objek non-tabel (matview, view, INSTEAD OF) diabaikan tanpa panik", false, "fixturecore", "CREATE MATERIALIZED VIEW zz_mv AS SELECT id FROM fixturecore_branches;\nCREATE INDEX zz_mv_idx ON zz_mv (id);\nCREATE VIEW zz_v AS SELECT id FROM fixturecore_branches;\n-- +goose StatementBegin\nCREATE FUNCTION zz_v_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;\n-- +goose StatementEnd\nCREATE TRIGGER zz_v_trg INSTEAD OF INSERT ON zz_v FOR EACH ROW EXECUTE FUNCTION zz_v_fn();", nil},
		{"badan pemicu diganti", true, "fixturesales", "-- +goose StatementBegin\nCREATE OR REPLACE FUNCTION fixturesales_invoices_lock_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;\n-- +goose StatementEnd", []string{KindTriggerChanged}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model := fixtureModel(t)
			if c.trigger {
				addLockTrigger(t, model)
			}
			src := fixtureSources(t, model, map[string]map[string]string{
				c.module: {"0002_mutate.sql": "-- +goose Up\n" + c.sql + "\n-- +goose Down\nSELECT 1;\n"},
			})
			r := mustRun(t, pgtest.Start(t), model, src)
			got := findingKinds(r)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("kinds = %v, want %v\n%s", got, c.want, r)
			}
		})
	}
}

// Kolom di unmanagedColumns, beserta constraint dan indeks yang menyentuhnya, dikecualikan (Requirement 2.9).
func TestRun_UnmanagedColumnsExcluded(t *testing.T) {
	mut := `ALTER TABLE fixturecore_branches ADD COLUMN secret_token bytea;
ALTER TABLE fixturecore_branches ADD CONSTRAINT zz_secret_uq UNIQUE (secret_token);
CREATE INDEX zz_secret_idx ON fixturecore_branches (secret_token);
ALTER TABLE fixturecore_branches ADD CONSTRAINT zz_secret_chk CHECK (octet_length(secret_token) > 0);`
	extra := map[string]map[string]string{"fixturecore": {"0002_secret.sql": "-- +goose Up\n" + mut + "\n-- +goose Down\nSELECT 1;\n"}}

	model := fixtureModel(t)
	model.Table("fixturecore_branches").Unmanaged = []string{"secret_token"}
	if r := mustRun(t, pgtest.Start(t), model, fixtureSources(t, model, extra)); !r.Empty() {
		t.Fatalf("kolom tak terkelola harus dikecualikan:\n%s", r)
	}
	// Tanpa pernyataan unmanaged, semuanya selisih.
	model = fixtureModel(t)
	r := mustRun(t, pgtest.Start(t), model, fixtureSources(t, model, extra))
	if r.Empty() {
		t.Fatal("tanpa unmanagedColumns harus ada selisih")
	}
}

// Migrasi dijalankan oleh role non-superuser: fasilitas superuser gagal (Requirement 10.8).
func TestRun_MigrationsRunAsNonSuperuser(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{"COPY PROGRAM", "COPY (SELECT 1) TO PROGRAM 'true';"},
		{"baca berkas server", "SELECT pg_read_file('/etc/passwd');"},
		{"buat role", "CREATE ROLE erpora_should_not_exist LOGIN;"},
		{"session_replication_role", "SET session_replication_role = replica;"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model := fixtureModel(t)
			src := fixtureSources(t, model, map[string]map[string]string{
				"fixturecore": {"0002_evil.sql": "-- +goose Up\n" + c.sql + "\n-- +goose Down\nSELECT 1;\n"},
			})
			_, err := run(t, pgtest.Start(t), model, src)
			var me *migrate.MigrationError
			if !errors.As(err, &me) {
				t.Fatalf("err = %v, want *migrate.MigrationError", err)
			}
			if msg := strings.ToLower(err.Error()); !strings.Contains(msg, "permission denied") && !strings.Contains(msg, "must be superuser") {
				t.Errorf("galat harus soal hak akses: %v", err)
			}
		})
	}
	t.Run("role sesi bukan superuser", func(t *testing.T) {
		model := fixtureModel(t)
		check := "-- +goose Up\n-- +goose StatementBegin\nDO $$ BEGIN IF (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication FROM pg_roles WHERE rolname = current_user) THEN RAISE EXCEPTION 'migrasi berjalan dengan hak istimewa'; END IF; END $$;\n-- +goose StatementEnd\n-- +goose Down\nSELECT 1;\n"
		src := fixtureSources(t, model, map[string]map[string]string{"fixturecore": {"0002_who.sql": check}})
		mustRun(t, pgtest.Start(t), model, src)
	})
}

// Role dan objeknya dibersihkan setelah Run (berhasil maupun gagal).
func TestRun_RoleCleanedUp(t *testing.T) {
	dsn := pgtest.Start(t)
	count := func() int {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM pg_roles WHERE rolname LIKE 'erpora_mig_%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	model := fixtureModel(t)
	r := mustRun(t, dsn, model, fixtureSources(t, model, nil))
	if r.Role == "" || !strings.HasPrefix(r.Role, "erpora_mig_") {
		t.Errorf("Role = %q", r.Role)
	}
	if after := count(); after != before {
		t.Errorf("role tersisa setelah Run sukses: %d -> %d", before, after)
	}
	// Gagal di tengah (migrasi jahat): tetap dibersihkan.
	src := fixtureSources(t, model, map[string]map[string]string{
		"fixturecore": {"0002_evil.sql": "-- +goose Up\nCOPY (SELECT 1) TO PROGRAM 'true';\n-- +goose Down\nSELECT 1;\n"},
	})
	dsn2 := pgtest.Start(t)
	if _, err := run(t, dsn2, model, src); err == nil {
		t.Fatal("harus gagal")
	}
	if after := count(); after != before {
		t.Errorf("role tersisa setelah Run gagal: %d -> %d", before, after)
	}
}

// Galat pada DDL diharapkan (model tidak valid bagi PostgreSQL) dilaporkan sebagai ExpectedSchemaError.
func TestRun_ExpectedSchemaError(t *testing.T) {
	model := fixtureModel(t)
	// default boolean pada kolom timestamptz: ditolak PostgreSQL.
	model.Table("fixturecore_branches").Column("created_at").Default = &dbmodel.Default{Type: "boolean", Value: "true"}
	src := []Source{{Slug: "fixturecore", Migrations: moduleFS(map[string]string{"0001_x.sql": "-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n"})}}
	_, err := Run(context.Background(), Options{DSN: pgtest.Start(t), Sources: src, Model: model.ForModule("fixturecore")})
	var ee *ExpectedSchemaError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v, want *ExpectedSchemaError", err)
	}
	if ee.Object != "fixturecore_branches" {
		t.Errorf("Object = %q", ee.Object)
	}
}

// Modul referensi nyata: example (metadata di modules/example/metadata dan migrasi tulis tangan 0001-0003)
// harus berselisih kosong (task 14.1; K22: createdAt/updatedAt DEFAULT now() dan version DEFAULT 1 implisit
// dari sdk:audit, createdBy/updatedBy nullable).
func TestRun_ExampleModuleHandwrittenMigrations(t *testing.T) {
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	docs, lps := load.LoadFS(os.DirFS("../../../../modules"), "example/metadata")
	if len(lps) != 0 || len(docs) == 0 {
		t.Fatalf("dokumen=%d temuan=%v", len(docs), lps)
	}
	if aps := c.AddModule(resolve.Module{Slug: "example"}, docs...); len(aps) != 0 {
		t.Fatal(aps)
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatal(res.Problems)
	}
	model, dps := dbmodel.Derive(res.Resources)
	if len(dps) != 0 {
		t.Fatal(dps)
	}
	src := []Source{{Slug: "example", Migrations: os.DirFS("../../../../modules/example/migrations")}}
	r := mustRun(t, pgtest.Start(t), model, src)
	if !r.Empty() {
		t.Fatalf("example harus berselisih kosong:\n%s", r)
	}
}

// WithTempDatabase membuat database sementara dan menghapusnya, juga saat fn gagal.
func TestWithTempDatabase_CreatesAndDrops(t *testing.T) {
	admin := os.Getenv(pgtest.EnvExternal)
	if admin == "" {
		t.Skip(pgtest.EnvExternal + " tidak diset")
	}
	exists := func(name string) bool {
		db, err := sql.Open("pgx", admin)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	var seen string
	boom := errors.New("gagal sengaja")
	err := WithTempDatabase(context.Background(), admin, func(dsn string) error {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := db.QueryRow(`SELECT current_database()`).Scan(&seen); err != nil {
			return err
		}
		if !strings.HasPrefix(seen, "erpora_diff_") || !exists(seen) {
			t.Errorf("database sementara = %q", seen)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if exists(seen) {
		t.Errorf("database %s harus sudah dihapus", seen)
	}
}

// Pembatalan (Ctrl-C atau batas waktu) di tengah migrasi: Run kembali dengan galat, sesi role diputus, dan role
// serta database sementara tetap dibersihkan (M6).
func TestRun_CancelledContextStillCleansUp(t *testing.T) {
	admin := os.Getenv(pgtest.EnvExternal)
	if admin == "" {
		t.Skip(pgtest.EnvExternal + " tidak diset")
	}
	probe, err := sql.Open("pgx", admin)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	count := func(q string, arg string) int {
		var n int
		if err := probe.QueryRow(q, arg).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	model := fixtureModel(t)
	src := fixtureSources(t, model, map[string]map[string]string{
		"fixturecore": {"0002_slow.sql": "-- +goose Up\nSELECT pg_sleep(60);\n-- +goose Down\nSELECT 1;\n"},
	})
	role := "erpora_mig_cancel_" + randHex(4)
	var dbName string
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	err = WithTempDatabase(ctx, admin, func(dsn string) error {
		u, perr := url.Parse(dsn)
		if perr != nil {
			return perr
		}
		dbName = strings.TrimPrefix(u.Path, "/")
		_, rerr := Run(ctx, Options{DSN: dsn, Sources: src, Model: model, RoleName: role})
		return rerr
	})
	if err == nil {
		t.Fatal("Run harus gagal saat konteks habis")
	}
	if d := time.Since(start); d > 40*time.Second {
		t.Errorf("pembatalan terlalu lambat: %s", d)
	}
	if n := count(`SELECT count(*) FROM pg_roles WHERE rolname = $1`, role); n != 0 {
		t.Errorf("role %s tersisa setelah pembatalan", role)
	}
	if n := count(`SELECT count(*) FROM pg_database WHERE datname = $1`, dbName); n != 0 {
		t.Errorf("database %s tersisa setelah pembatalan", dbName)
	}
}
