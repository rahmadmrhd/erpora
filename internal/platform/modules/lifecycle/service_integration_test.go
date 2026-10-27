//go:build integration

package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/sdk/module"
)

const fxMigDir = "../testdata/migrations/"

func openRealDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond},
		noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	return db
}

// realEnv merakit Service dengan Store dan Migrator nyata di atas satu pool.
type realEnv struct {
	svc  *Service
	db   *gorm.DB
	st   *store.Store
	mod  *fxModule
	snap *runtime.Snapshot
	reg  *registry.Registry
	dsn  string
}

func newRealService(t *testing.T, dsn string, db *gorm.DB, lockTimeout time.Duration, mods ...*fxModule) *realEnv {
	t.Helper()
	conn := database.SingleTenantConnector{Default: tenantA, DB: database.TenantDB{DB: db}}
	st := store.New(database.ControlDB{DB: db}, conn)
	reg := buildFxRegistry(t, mods...)
	snap := runtime.NewSnapshot()
	svc := New(Deps{
		Registry: reg, Store: st, Migrator: NewMigrator(conn), Tenants: conn,
		Snapshot: snap, Notifier: runtime.NewLocalNotifier(), LockTimeout: lockTimeout,
	})
	return &realEnv{svc: svc, db: db, st: st, snap: snap, reg: reg, dsn: dsn}
}

func newRealEnv(t *testing.T, mods ...*fxModule) *realEnv {
	t.Helper()
	dsn := pgtest.Start(t)
	db := openRealDB(t, dsn)
	if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	return newRealService(t, dsn, db, 0, mods...)
}

func (e *realEnv) exec(t *testing.T, op Op, key string) Result {
	t.Helper()
	res, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: op, Key: key, Actor: "cli:tester"})
	if err != nil || len(res) != 1 {
		t.Fatalf("%s %s = %+v, %v", op, key, res, err)
	}
	return res[0]
}

func (e *realEnv) count(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := e.db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func fxMigModuleSpec(rec *fxRec) *fxModule {
	return newFx(rec, fxSpec{key: "acme.fx-mod", migrations: os.DirFS(fxMigDir + "ok")})
}

func TestService_InstallEnableDisableUninstallReinstall(t *testing.T) {
	ctx := context.Background()
	rec := &fxRec{}
	e := newRealEnv(t, fxMigModuleSpec(rec))

	r := e.exec(t, OpInstall, "acme.fx-mod")
	if r.Status != "succeeded" || r.To != runtime.StatusInstalled {
		t.Fatalf("install = %+v", r)
	}
	if v := e.count(t, "SELECT max(version_id) FROM goose_module_fx_mod"); v != 2 {
		t.Errorf("versi goose = %d, want 2", v)
	}
	if n := e.count(t, "SELECT count(*) FROM module_migrations WHERE module_key = 'acme.fx-mod' AND execution_status = 'succeeded'"); n != 2 {
		t.Errorf("module_migrations = %d, want 2", n)
	}
	rev1, _ := e.st.Revision(ctx, tenantA)

	if err := e.db.Exec("INSERT INTO fx_items (id, name) VALUES (?, 'x')", uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if r := e.exec(t, OpEnable, "acme.fx-mod"); r.Status != "succeeded" {
		t.Fatalf("enable = %+v", r)
	}
	if rev2, _ := e.st.Revision(ctx, tenantA); rev2 <= rev1 {
		t.Errorf("revision tidak naik: %d -> %d", rev1, rev2)
	}
	if e.snap.Status(tenantA, "acme.fx-mod") != runtime.StatusEnabled {
		t.Error("snapshot tidak enabled")
	}
	e.exec(t, OpDisable, "acme.fx-mod")
	if n := e.count(t, "SELECT count(*) FROM fx_items"); n != 1 {
		t.Errorf("data hilang setelah disable: %d", n)
	}
	e.exec(t, OpUninstall, "acme.fx-mod")
	if n := e.count(t, "SELECT count(*) FROM fx_items"); n != 1 {
		t.Errorf("data hilang setelah uninstall: %d", n)
	}
	ins, _ := e.st.Installations(ctx, tenantA)
	if len(ins) != 1 || ins[0].Status != "uninstalled" || ins[0].InstalledVersion != "1.0.0" {
		t.Errorf("installations = %+v", ins)
	}

	// Install ulang: migrasi tidak dijalankan lagi.
	if r := e.exec(t, OpInstall, "acme.fx-mod"); r.Status != "succeeded" {
		t.Fatalf("reinstall = %+v", r)
	}
	if n := e.count(t, "SELECT count(*) FROM module_migrations WHERE module_key = 'acme.fx-mod'"); n != 2 {
		t.Errorf("module_migrations setelah reinstall = %d, want 2", n)
	}

	// FR-21: semua baris membawa tenant default.
	for _, tbl := range []string{"module_installations", "module_operations", "module_lifecycle_events", "module_migrations"} {
		if n := e.count(t, "SELECT count(*) FROM "+tbl+" WHERE tenant_id <> ?", uuid.UUID(tenantA)); n != 0 {
			t.Errorf("%s: %d baris bukan tenant default", tbl, n)
		}
		if n := e.count(t, "SELECT count(*) FROM "+tbl); n == 0 {
			t.Errorf("%s kosong", tbl)
		}
	}
}

func TestService_ConcurrentEnableTwoStores(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.Start(t)
	db1 := openRealDB(t, dsn)
	if err := migrate.Up(ctx, db1, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	db2 := openRealDB(t, dsn)

	entered := make(chan struct{})
	release := make(chan struct{})
	rec := &fxRec{}
	m1 := fxMigModuleSpec(rec)
	m2 := fxMigModuleSpec(rec)
	m1.on("OnEnable", func(context.Context, module.Env) error {
		close(entered)
		<-release
		return nil
	})
	e1 := newRealService(t, dsn, db1, 300*time.Millisecond, m1)
	e2 := newRealService(t, dsn, db2, 300*time.Millisecond, m2)
	e1.exec(t, OpInstall, "acme.fx-mod")
	revBefore, _ := e1.st.Revision(ctx, tenantA)

	done := make(chan Result, 1)
	go func() {
		res, err := e1.svc.Execute(ctx, Request{Tenant: tenantA, Op: OpEnable, Key: "acme.fx-mod", Actor: "cli:a"})
		if err != nil || len(res) != 1 {
			t.Errorf("enable #1 = %+v, %v", res, err)
			done <- Result{}
			return
		}
		done <- res[0]
	}()
	<-entered

	_, err := e2.svc.Execute(ctx, Request{Tenant: tenantA, Op: OpEnable, Key: "acme.fx-mod", Actor: "cli:b"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeOperationInProgress {
		t.Fatalf("enable #2 err = %v, want operation_in_progress", err)
	}
	close(release)
	if r := <-done; r.Status != "succeeded" {
		t.Fatalf("enable #1 = %+v", r)
	}
	if n := e1.count(t, "SELECT count(*) FROM module_lifecycle_events WHERE module_key = 'acme.fx-mod' AND event = 'enabled'"); n != 1 {
		t.Errorf("event enabled = %d, want 1", n)
	}
	if revAfter, _ := e1.st.Revision(ctx, tenantA); revAfter != revBefore+1 {
		t.Errorf("revision %d -> %d, want +1", revBefore, revAfter)
	}
}

func TestService_AuditEventHasActor(t *testing.T) {
	ctx := context.Background()
	rec := &fxRec{}
	e := newRealEnv(t, newFx(rec, fxSpec{key: "acme.a"}))
	e.exec(t, OpInstall, "acme.a")
	e.exec(t, OpEnable, "acme.a")
	r := e.exec(t, OpDisable, "acme.a")

	evs, _, err := e.st.Events(ctx, tenantA, "acme.a", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range evs {
		if ev.Event == "disabled" {
			found = true
			if ev.Actor != "cli:tester" || ev.OperationID != r.OperationID || ev.OperationID == uuid.Nil {
				t.Errorf("event = %+v", ev)
			}
		}
	}
	if !found {
		t.Fatalf("event disabled tidak ada: %+v", evs)
	}
	// installed_version bertahan di DB nyata sepanjang install/enable/disable.
	ins, _ := e.st.Installations(ctx, tenantA)
	if len(ins) != 1 || ins[0].InstalledVersion != "1.0.0" || ins[0].Status != "disabled" {
		t.Errorf("installations = %+v", ins)
	}
}
