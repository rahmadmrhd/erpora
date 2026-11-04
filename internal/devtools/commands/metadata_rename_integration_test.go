//go:build integration

package commands

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"erpora/internal/devtools/metadatatools/diff"
	"erpora/internal/devtools/metadatatools/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

// Rename lintas rilis lewat CLI nyata (melewati metadata:check, bukan model buatan): rilis berikutnya menghapus
// field lama `phone` dan mempertahankan `migrateFrom: phone` pada `phoneE164`. metadata:check meloloskannya,
// metadata:migrate membangkitkan kerangka penghapusan BERPENJAGA, penjaga menggagalkan migrasi bila ada baris
// belum tersalin, dan setelah dilengkapi selisih kosong (Requirement 12.2, 12.4, 12.8; K18).
func TestMetadataMigrate_RemovingRenamedFieldKeepsGuard(t *testing.T) {
	admin := os.Getenv(diff.EnvAdminDSN)
	if admin == "" {
		t.Skip(diff.EnvAdminDSN + " tidak diset")
	}
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)

	exec := func(args ...string) (string, error) {
		t.Chdir(dir)
		root := newTestRoot(t)
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs(args)
		err := root.Execute()
		return out.String() + errOut.String(), err
	}

	if out, err := exec("metadata:migrate", "fixturecore", "initial_schema"); err != nil {
		t.Fatalf("migrasi awal: %v\n%s", err, out)
	}

	// Rilis berikutnya: field lama dihapus dari metadata; migrateFrom tetap.
	customer := filepath.Join(dir, "modules", "fixturecore", "metadata", "customer.resource.yaml")
	b, err := os.ReadFile(customer) //nolint:gosec // salinan fixture sementara
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(b), "  phone:\n")
	end := strings.Index(string(b), "  phoneE164:\n")
	if start < 0 || end < start {
		t.Fatalf("blok phone tidak ditemukan di fixture:\n%s", b)
	}
	edited := string(b[:start]) + string(b[end:])
	if err := os.WriteFile(customer, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	// metadata:check lolos (peringatan: modul tanpa tag rilis, migrateFrom tak dapat diverifikasi).
	out, err := exec("metadata:check")
	if err != nil || strings.Contains(out, "migrate_from_unknown") || !strings.Contains(out, "migrate_from_unbaselined") {
		t.Fatalf("metadata:check harus meloloskan migrateFrom ke field yang dihapus (dengan peringatan): %v\n%s", err, out)
	}

	if out, err := exec("metadata:migrate", "fixturecore", "drop_phone"); err != nil {
		t.Fatalf("migrasi penghapusan: %v\n%s", err, out)
	}
	migDir := filepath.Join(dir, "modules", "fixturecore", "migrations")
	raw, err := os.ReadFile(filepath.Join(migDir, "0002_drop_phone.sql")) //nolint:gosec // direktori uji
	if err != nil {
		t.Fatal(err)
	}
	skeleton := string(raw)
	for _, want := range []string{migrate.TodoMarker, `IF EXISTS (SELECT 1 FROM "fixturecore_customers" WHERE "phone" IS NOT NULL AND "phone_e164" IS NULL) THEN`,
		`-- ALTER TABLE "fixturecore_customers" DROP COLUMN "phone";`, "TIDAK DAPAT DIBALIK"} {
		if !strings.Contains(skeleton, want) {
			t.Fatalf("kerangka tidak memuat %q:\n%s", want, skeleton)
		}
	}
	// Selama TODO ada, metadata:check gagal.
	if out, err := exec("metadata:check"); exitCodeOf(err) != exitMetadataValidation || !strings.Contains(out, "migration_todo") {
		t.Fatalf("TODO harus menggagalkan metadata:check: %v\n%s", err, out)
	}

	// Tinjauan manusia: DROP COLUMN diaktifkan dan penanda dihapus.
	var kept []string
	for _, line := range strings.Split(skeleton, "\n") {
		switch {
		case strings.Contains(line, migrate.TodoMarker):
		case strings.HasPrefix(line, "-- ALTER TABLE") && strings.Contains(line, "DROP COLUMN"):
			kept = append(kept, strings.TrimPrefix(line, "-- "))
		default:
			kept = append(kept, line)
		}
	}
	if err := os.WriteFile(filepath.Join(migDir, "0002_drop_phone.sql"), []byte(strings.Join(kept, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	// Penjaga pada data nyata.
	dsn := pgtest.Start(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(migDir))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO fixturecore_customers (id, code, name, phone) VALUES (gen_random_uuid(), 'C1', 'Acme', '0811')`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 2); err == nil || !strings.Contains(err.Error(), "penghapusan kolom ditolak") {
		t.Fatalf("penjaga harus menggagalkan migrasi, err = %v", err)
	}
	if _, err := db.Exec(`UPDATE fixturecore_customers SET phone_e164 = phone WHERE phone_e164 IS NULL AND phone IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatalf("setelah disalin, penghapusan harus lolos: %v", err)
	}

	// Setelah dilengkapi: selisih kosong terhadap metadata.
	if out, err := exec("metadata:diff", "fixturecore"); err != nil || !strings.Contains(out, "selisih kosong") {
		t.Fatalf("selisih harus kosong: %v\n%s", err, out)
	}
	if out, err := exec("metadata:check"); err != nil {
		t.Fatalf("metadata:check setelah migrasi dilengkapi: %v\n%s", err, out)
	}
}
