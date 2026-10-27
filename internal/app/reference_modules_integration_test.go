//go:build integration

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/modules/example"
	"erpora/modules/exampleaddon"
	"erpora/sdk/module"
)

// refEnv adalah database migrasi platform + kemampuan merakit ulang assembly dengan katalog modul berbeda
// (simulasi restart binary baru) di atas database yang sama.
type refEnv struct {
	t   *testing.T
	dsn string
	db  *gorm.DB
}

func newRefEnv(t *testing.T) *refEnv {
	t.Helper()
	dsn := pgtest.Start(t)
	db := openTestDB(t, dsn)
	if err := migrate.Up(context.Background(), db, config.EnvDevelopment); err != nil {
		t.Fatal(err)
	}
	return &refEnv{t: t, dsn: dsn, db: db}
}

// assemble merakit assembly baru. mods nil = katalog terkompilasi asli. Konteksnya dihentikan saat test selesai.
func (e *refEnv) assemble(mods func() []module.Module) (context.Context, *assembly) {
	e.t.Helper()
	if mods != nil {
		old := compiledModulesFn
		compiledModulesFn = mods
		defer func() { compiledModulesFn = old }()
	}
	reg, api, err := buildRegistry()
	if err != nil {
		e.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.t.Cleanup(cancel)
	cfg := config.Config{DatabaseURL: e.dsn, Env: config.EnvDevelopment, AuthzStub: "allow", DBSlowQuery: 200 * time.Millisecond}
	a, err := assemble(ctx, cfg, testLog(), e.db, testProviders(), reg, api)
	if err != nil {
		e.t.Fatal(err)
	}
	return ctx, a
}

func (e *refEnv) count(query string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.Raw(query, args...).Scan(&n).Error; err != nil {
		e.t.Fatal(err)
	}
	return n
}

func run(t *testing.T, ctx context.Context, a *assembly, op lifecycle.Op, key string) lifecycle.Result {
	t.Helper()
	res, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: op, Key: key, Actor: "cli:ref"})
	if err != nil || len(res) != 1 {
		t.Fatalf("%s %s = %+v, %v", op, key, res, err)
	}
	return res[0]
}

func runErr(ctx context.Context, a *assembly, op lifecycle.Op, key string) error {
	_, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: op, Key: key, Actor: "cli:ref"})
	return err
}

func versionedCatalog(version string, migrations []string, rec *upgradeRecorder) func() []module.Module {
	return func() []module.Module {
		ex := example.New()
		return []module.Module{versioned(ex, version, migrations, rec), exampleaddon.New(ex.Notes())}
	}
}

const mig1 = "0001_create_example_notes.sql"

func TestExample_InstallViaPlatformMigrations(t *testing.T) {
	e := newRefEnv(t)
	ctx, a := e.assemble(nil)

	if r := run(t, ctx, a, lifecycle.OpInstall, "acme.example"); r.Status != "succeeded" {
		t.Fatalf("install = %+v", r)
	}
	if v := e.count("SELECT max(version_id) FROM goose_module_example WHERE is_applied"); v != 2 {
		t.Errorf("versi goose = %d, want 2", v)
	}
	if n := e.count("SELECT count(*) FROM module_migrations WHERE module_key = 'acme.example' AND execution_status = 'succeeded' AND length(checksum) = 64"); n != 2 {
		t.Errorf("module_migrations = %d, want 2 berchecksum", n)
	}
	if n := e.count("SELECT count(*) FROM example_notes"); n != 3 {
		t.Errorf("notes tersemai = %d, want 3", n)
	}
	if rec := call(a.Router, "GET", "/api/example/notes", ""); rec.Code != http.StatusForbidden {
		t.Errorf("notes sebelum enable = %d", rec.Code)
	}
	run(t, ctx, a, lifecycle.OpEnable, "acme.example")
	rec := call(a.Router, "GET", "/api/example/notes", "")
	if rec.Code != 200 {
		t.Fatalf("notes = %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Data []struct {
			Version   int64
			UpdatedAt string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Data) != 3 {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}
	for _, n := range body.Data {
		if n.Version != 1 || n.UpdatedAt == "" {
			t.Errorf("note = %+v", n)
		}
	}
}

func bootstrapRev(t *testing.T, a *assembly) int64 {
	t.Helper()
	b, _ := bootstrapOf(t, a.Router)
	return b.Data.Revision
}

// AC "Upgrade": 1.0.0 -> 1.1.0 lewat reconcile (upgrade_pending, 503) lalu operasi upgrade.
func TestExample_Upgrade_1_0_0_to_1_1_0(t *testing.T) {
	e := newRefEnv(t)
	ctx1, a1 := e.assemble(versionedCatalog("1.0.0", []string{mig1}, nil))
	run(t, ctx1, a1, lifecycle.OpInstall, "acme.example")
	run(t, ctx1, a1, lifecycle.OpEnable, "acme.example")
	oldNote := uuid.New()
	if err := e.db.Exec("INSERT INTO example_notes (id, title, created_at) VALUES (?, 'lama', ?)", oldNote, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}
	if v := e.count("SELECT max(version_id) FROM goose_module_example WHERE is_applied"); v != 1 {
		t.Fatalf("versi goose 1.0.0 = %d", v)
	}
	rev1 := bootstrapRev(t, a1)

	// "Restart" dengan binary 1.1.0 di atas database yang sama dan pool yang sama (statement cache hidup).
	rec := &upgradeRecorder{}
	ctx2, a2 := e.assemble(versionedCatalog("1.1.0", nil, rec))
	st, _, err := a2.Lifecycle.Module(ctx2, a2.Tenant, "acme.example")
	if err != nil || st.Status != "upgrade_pending" {
		t.Fatalf("status setelah reconcile = %+v, %v", st.Status, err)
	}
	resp := call(a2.Router, "GET", "/api/example/ping", "")
	if resp.Code != 503 || errCodeOf(t, resp) != "module_unavailable" {
		t.Fatalf("ping saat upgrade_pending = %d %s", resp.Code, resp.Body)
	}
	if got := resp.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q", got)
	}

	r := run(t, ctx2, a2, lifecycle.OpUpgrade, "acme.example")
	if r.Status != "succeeded" || r.To != "enabled" {
		t.Fatalf("upgrade = %+v", r)
	}
	if v := e.count("SELECT max(version_id) FROM goose_module_example WHERE is_applied"); v != 2 {
		t.Errorf("versi goose = %d, want 2", v)
	}
	var row struct {
		Version int64
		Same    bool
	}
	if err := e.db.Raw("SELECT version, updated_at = created_at AS same FROM example_notes WHERE id = ?", oldNote).Scan(&row).Error; err != nil || row.Version != 1 || !row.Same {
		t.Errorf("note lama = %+v (%v), want version=1 dan updated_at=created_at", row, err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != [2]string{"1.0.0", "1.1.0"} {
		t.Errorf("Upgrade calls = %v", rec.calls)
	}
	// Request in-process setelah ALTER TABLE tidak boleh gagal akibat statement cache (PrepareStmt).
	resp = call(a2.Router, "GET", "/api/example/notes", "")
	if resp.Code != 200 {
		t.Fatalf("notes setelah upgrade = %d %s", resp.Code, resp.Body)
	}
	if rev2 := bootstrapRev(t, a2); rev2 <= rev1 {
		t.Errorf("revision tidak naik: %d -> %d", rev1, rev2)
	}
}

// AC "Downgrade": DB terpasang 1.1.0, binary 1.0.0 -> failed + downgrade_not_supported, route 503.
func TestExample_DowngradeRejected(t *testing.T) {
	e := newRefEnv(t)
	ctx1, a1 := e.assemble(versionedCatalog("1.1.0", nil, nil))
	run(t, ctx1, a1, lifecycle.OpInstall, "acme.example")
	run(t, ctx1, a1, lifecycle.OpEnable, "acme.example")

	ctx2, a2 := e.assemble(versionedCatalog("1.0.0", []string{mig1}, nil))
	st, _, err := a2.Lifecycle.Module(ctx2, a2.Tenant, "acme.example")
	if err != nil || st.Status != "failed" || st.LastOperation == nil || st.LastOperation.Error == nil ||
		st.LastOperation.Error.Code != lifecycle.CodeDowngradeNotSupported {
		t.Fatalf("modul = %+v, %v", st, err)
	}
	if rec := call(a2.Router, "GET", "/api/example/ping", ""); rec.Code != 503 {
		t.Fatalf("ping = %d %s", rec.Code, rec.Body)
	}
}

// Review Focus 4 + AC "Dependency tidak tersedia" (dua arah).
func TestExampleModules_DependencyRules(t *testing.T) {
	e := newRefEnv(t)
	ctx, a := e.assemble(nil)
	run(t, ctx, a, lifecycle.OpInstall, "acme.example")
	run(t, ctx, a, lifecycle.OpInstall, "acme.example-addon")

	err := runErr(ctx, a, lifecycle.OpEnable, "acme.example-addon")
	var le *lifecycle.Error
	if !errors.As(err, &le) || le.Code != lifecycle.CodeDependencyNotReady || len(le.Related) != 1 || le.Related[0] != "acme.example" {
		t.Fatalf("enable addon saat example belum enabled: %v", err)
	}
	run(t, ctx, a, lifecycle.OpEnable, "acme.example")
	run(t, ctx, a, lifecycle.OpEnable, "acme.example-addon")

	err = runErr(ctx, a, lifecycle.OpDisable, "acme.example")
	if !errors.As(err, &le) || le.Code != lifecycle.CodeDependentsActive || len(le.Related) != 1 || le.Related[0] != "acme.example-addon" {
		t.Fatalf("disable example saat addon enabled: %v", err)
	}
	rec := call(a.Router, "GET", "/api/example-addon/summary", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"noteCount":3`) {
		t.Fatalf("summary = %d %s", rec.Code, rec.Body)
	}
}

// AC "Modul dinonaktifkan": 403, data tetap, event dengan actor.
func TestExample_DisableKeepsDataAndAudits(t *testing.T) {
	e := newRefEnv(t)
	ctx, a := e.assemble(nil)
	run(t, ctx, a, lifecycle.OpInstall, "acme.example")
	run(t, ctx, a, lifecycle.OpEnable, "acme.example")
	before := e.count("SELECT count(*) FROM example_notes")
	r := run(t, ctx, a, lifecycle.OpDisable, "acme.example")

	rec := call(a.Router, "GET", "/api/example/notes", "")
	if rec.Code != http.StatusForbidden || errCodeOf(t, rec) != "module_disabled" {
		t.Fatalf("notes = %d %s", rec.Code, rec.Body)
	}
	if after := e.count("SELECT count(*) FROM example_notes"); after != before {
		t.Errorf("data berubah: %d -> %d", before, after)
	}
	if n := e.count("SELECT count(*) FROM module_lifecycle_events WHERE module_key = 'acme.example' AND event = 'disabled' AND actor = 'cli:ref' AND operation_id = ?", r.OperationID); n != 1 {
		t.Errorf("event disabled berisi actor dan operation_id = %d, want 1", n)
	}
}

// AC "Uninstall": tabel dan data tetap; install ulang tidak menjalankan migrasi lagi.
func TestExample_UninstallReinstall(t *testing.T) {
	e := newRefEnv(t)
	ctx, a := e.assemble(nil)
	run(t, ctx, a, lifecycle.OpInstall, "acme.example")
	run(t, ctx, a, lifecycle.OpEnable, "acme.example")
	run(t, ctx, a, lifecycle.OpDisable, "acme.example")
	notes := e.count("SELECT count(*) FROM example_notes")
	migs := e.count("SELECT count(*) FROM module_migrations WHERE module_key = 'acme.example'")

	run(t, ctx, a, lifecycle.OpUninstall, "acme.example")
	if n := e.count("SELECT count(*) FROM example_notes"); n != notes || notes != 3 {
		t.Errorf("data setelah uninstall = %d (sebelum %d)", n, notes)
	}
	if r := run(t, ctx, a, lifecycle.OpInstall, "acme.example"); r.Status != "succeeded" {
		t.Fatalf("install ulang = %+v", r)
	}
	if n := e.count("SELECT count(*) FROM module_migrations WHERE module_key = 'acme.example'"); n != migs {
		t.Errorf("module_migrations bertambah: %d -> %d", migs, n)
	}
	if n := e.count("SELECT count(*) FROM example_notes"); n != 3 {
		t.Errorf("Install idempoten (menyemai hanya bila kosong): %d notes", n)
	}
}

func newGoose(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, example.New().Migrations(), goose.WithTableName("goose_module_example"))
}

func hasColumn(e *refEnv, col string) bool {
	e.t.Helper()
	return e.count("SELECT count(*) FROM information_schema.columns WHERE table_name = 'example_notes' AND column_name = ?", col) > 0
}

// Q26 butir 4 (dipindah dari tests/integration): backfill 0002 untuk baris lama; down lalu up idempoten.
func TestExampleMigration0002_BackfillDownUp(t *testing.T) {
	ctx := context.Background()
	e := newRefEnv(t)
	sqlDB, err := e.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	p, err := newGoose(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatalf("up 0001: %v", err)
	}
	old := uuid.New()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := e.db.Exec("INSERT INTO example_notes (id, title, created_at) VALUES (?, 'lama', ?)", old, created).Error; err != nil {
		t.Fatal(err)
	}
	if hasColumn(e, "version") || hasColumn(e, "updated_at") {
		t.Fatal("kolom 0002 tidak boleh ada sebelum migrasi 0002")
	}
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatalf("up 0002: %v", err)
	}
	check := func(when string) {
		t.Helper()
		var row struct {
			Version int64
			Same    bool
		}
		if err := e.db.Raw("SELECT version, updated_at = created_at AS same FROM example_notes WHERE id = ?", old).Scan(&row).Error; err != nil || row.Version != 1 || !row.Same {
			t.Fatalf("baris lama %s: %+v (%v)", when, row, err)
		}
	}
	check("setelah 0002")
	if _, err := p.DownTo(ctx, 1); err != nil {
		t.Fatalf("down 0002: %v", err)
	}
	if hasColumn(e, "version") || hasColumn(e, "updated_at") {
		t.Fatal("kolom 0002 harus hilang setelah down")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up ulang: %v", err)
	}
	check("setelah up ulang")
	if res, err := p.Up(ctx); err != nil || len(res) != 0 {
		t.Fatalf("up kedua: %d hasil, err=%v; want no-op", len(res), err)
	}
}
