//go:build integration

package integration

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql untuk goose pada test
	"github.com/pressly/goose/v3"

	"erpora/internal/platform/testsupport/pgtest"
)

// Migrasi modul example dibaca langsung dari direktori modul (bukan lewat impor paket modul):
// tests/integration bukan internal/app, jadi D7 melarang mengimpor modules/** (juga dari file test).
const exampleModuleDir = "../../modules/example"

func exampleProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	// Nama tabel versi mengikuti konvensi PRD-000: goose_module_<slug_snake>.
	p, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(exampleModuleDir, "migrations")),
		goose.WithTableName("goose_module_example"))
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	return p
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

type noteCols struct {
	version   int64
	sameStamp bool // updated_at = created_at
}

func readNote(t *testing.T, db *sql.DB, id uuid.UUID) noteCols {
	t.Helper()
	var c noteCols
	err := db.QueryRowContext(context.Background(),
		"SELECT version, updated_at = created_at FROM example_notes WHERE id = $1", id).Scan(&c.version, &c.sameStamp)
	if err != nil {
		t.Fatalf("baca note: %v", err)
	}
	return c
}

func hasColumn(t *testing.T, db *sql.DB, col string) bool {
	t.Helper()
	var n int
	err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM information_schema.columns WHERE table_name = 'example_notes' AND column_name = $1", col).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// Migrasi 0002: baris lama (dibuat di 0001) mendapat version=1 dan updated_at=created_at; down 0002 lalu up
// lagi idempoten (Q26 butir 4).
func TestExampleMigration0002_BackfillDownUp(t *testing.T) {
	ctx := context.Background()
	db := openSQL(t, pgtest.Start(t))
	p := exampleProvider(t, db)

	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatalf("up 0001: %v", err)
	}
	old := uuid.New()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO example_notes (id, title, created_at) VALUES ($1, 'lama', $2)", old, created); err != nil {
		t.Fatal(err)
	}
	if hasColumn(t, db, "version") || hasColumn(t, db, "updated_at") {
		t.Fatal("kolom 0002 tidak boleh ada sebelum migrasi 0002")
	}

	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatalf("up 0002: %v", err)
	}
	if c := readNote(t, db, old); c.version != 1 || !c.sameStamp {
		t.Fatalf("baris lama setelah 0002: %+v, want version=1 dan updated_at=created_at", c)
	}
	// Baris baru (tanpa mengisi kolom) memakai default aman.
	fresh := uuid.New()
	if _, err := db.ExecContext(ctx, "INSERT INTO example_notes (id, title) VALUES ($1, 'baru')", fresh); err != nil {
		t.Fatal(err)
	}
	if c := readNote(t, db, fresh); c.version != 1 {
		t.Fatalf("baris baru: %+v", c)
	}

	// Down 0002 menghapus kolom tanpa menghilangkan data; up lagi mengisi ulang backfill.
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("down 0002: %v", err)
	}
	if hasColumn(t, db, "version") || hasColumn(t, db, "updated_at") {
		t.Fatal("kolom 0002 harus hilang setelah down")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up ulang: %v", err)
	}
	if c := readNote(t, db, old); c.version != 1 || !c.sameStamp {
		t.Fatalf("baris lama setelah up ulang: %+v", c)
	}
	// Up tanpa migrasi tertunda adalah no-op (idempoten).
	if res, err := p.Up(ctx); err != nil || len(res) != 0 {
		t.Fatalf("up kedua: %d hasil, err=%v; want no-op", len(res), err)
	}
}

// Test repo example (AddMany -> List: Version=1, UpdatedAt terisi) berada di paket infrastructure modul
// (internal/ tidak bisa diimpor dari sini); di sini database disiapkan lalu test modul dijalankan.
func TestExampleModule_RepoIntegration(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.Start(t)
	db := openSQL(t, dsn)
	if _, err := exampleProvider(t, db).Up(ctx); err != nil {
		t.Fatalf("up modul example: %v", err)
	}
	cmd := exec.Command("go", "-C", exampleModuleDir, "test", "-count=1", "-tags", "integration",
		"-run", "TestNotesRepo_AddManyThenListHasVersionAndUpdatedAt", "-v", "./internal/infrastructure/")
	cmd.Env = append(os.Environ(), "ERP_EXAMPLE_TEST_DSN="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test repo modul gagal: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- PASS: TestNotesRepo_AddManyThenListHasVersionAndUpdatedAt") {
		t.Fatalf("test repo modul tidak berjalan (dilewati?):\n%s", out)
	}
}
