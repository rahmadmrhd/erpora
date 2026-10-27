//go:build integration

package store_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

func openDB(t *testing.T, dsn string) *gorm.DB {
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

func newStore(db *gorm.DB) *store.Store {
	return store.New(database.ControlDB{DB: db},
		database.SingleTenantConnector{Default: tenantA, DB: database.TenantDB{DB: db}})
}

// setup menjalankan migrasi dan mengembalikan DSN, DB, dan Store.
func setup(t *testing.T) (string, *gorm.DB, *store.Store) {
	t.Helper()
	dsn := pgtest.Start(t)
	db := openDB(t, dsn)
	if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	return dsn, db, newStore(db)
}

func startOp(t *testing.T, s *store.Store, tenant tenancy.TenantID, key, op string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := s.StartOperation(context.Background(), store.Operation{
		ID: id, TenantID: tenant, ModuleKey: key, Operation: op, Status: "running", Actor: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func count(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCommit_AtomicWithRevision(t *testing.T) {
	ctx := context.Background()
	_, db, s := setup(t)
	opID := startOp(t, s, tenantA, "acme.x", "enable")

	rev, err := s.Commit(ctx, tenantA, store.Commit{
		Installation: &store.Installation{ModuleKey: "acme.x", InstalledVersion: "1.0.0", Status: "enabled", LastOperationID: opID},
		Finish:       &store.Finish{OperationID: opID, Status: "succeeded"},
		Events:       []store.Event{{ModuleKey: "acme.x", Event: "enabled", Actor: "test", OperationID: opID}},
		BumpRevision: true,
	})
	if err != nil || rev != 1 {
		t.Fatalf("Commit = %d, %v", rev, err)
	}
	if got, _ := s.Revision(ctx, tenantA); got != 1 {
		t.Errorf("Revision = %d", got)
	}
	ins, err := s.Installations(ctx, tenantA)
	if err != nil || len(ins) != 1 || ins[0].Status != "enabled" || ins[0].InstalledVersion != "1.0.0" || ins[0].LastOperationID != opID {
		t.Fatalf("installations = %+v, %v", ins, err)
	}
	op, err := s.Operation(ctx, tenantA, opID)
	if err != nil || op.Status != "succeeded" || op.FinishedAt == nil {
		t.Fatalf("operation = %+v, %v", op, err)
	}
	evs, total, err := s.Events(ctx, tenantA, "acme.x", 1, 10)
	if err != nil || total != 1 || len(evs) != 1 || evs[0].Event != "enabled" || evs[0].OperationID != opID {
		t.Fatalf("events = %+v total=%d %v", evs, total, err)
	}

	// Commit gagal di tengah (Finish.Status melanggar CHECK) -> tidak ada perubahan sama sekali.
	op2 := startOp(t, s, tenantA, "acme.y", "install")
	_, err = s.Commit(ctx, tenantA, store.Commit{
		Installation: &store.Installation{ModuleKey: "acme.y", Status: "installed", LastOperationID: op2},
		Finish:       &store.Finish{OperationID: op2, Status: "bogus"},
		Events:       []store.Event{{ModuleKey: "acme.y", Event: "installed", Actor: "test", OperationID: op2}},
		BumpRevision: true,
	})
	if err == nil {
		t.Fatal("Commit harus gagal")
	}
	if n := count(t, db, "SELECT count(*) FROM module_installations WHERE module_key = 'acme.y'"); n != 0 {
		t.Errorf("installation acme.y tersimpan (%d) walau commit gagal", n)
	}
	if n := count(t, db, "SELECT count(*) FROM module_lifecycle_events WHERE module_key = 'acme.y'"); n != 0 {
		t.Errorf("event acme.y tersimpan (%d)", n)
	}
	if got, _ := s.Revision(ctx, tenantA); got != 1 {
		t.Errorf("Revision = %d setelah commit gagal, want 1", got)
	}
}

func TestCommit_NoBumpKeepsRevision(t *testing.T) {
	ctx := context.Background()
	_, _, s := setup(t)
	if _, err := s.Commit(ctx, tenantA, store.Commit{
		Installation: &store.Installation{ModuleKey: "acme.x", Status: "installed"},
		BumpRevision: true,
	}); err != nil {
		t.Fatal(err)
	}
	rev, err := s.Commit(ctx, tenantA, store.Commit{
		Installation: &store.Installation{ModuleKey: "acme.x", Status: "failed", PreviousStatus: "installed"},
	})
	if err != nil || rev != 1 {
		t.Fatalf("Commit tanpa bump = %d, %v; want 1", rev, err)
	}
	ins, _ := s.Installations(ctx, tenantA)
	if len(ins) != 1 || ins[0].Status != "failed" || ins[0].PreviousStatus != "installed" {
		t.Errorf("installations = %+v", ins)
	}
}

// IDOR: operasi/event/installation milik tenant lain tidak terlihat.
func TestOperation_TenantScoped(t *testing.T) {
	ctx := context.Background()
	_, _, s := setup(t)
	opID := startOp(t, s, tenantA, "acme.x", "install")
	if _, err := s.Commit(ctx, tenantA, store.Commit{
		Installation: &store.Installation{ModuleKey: "acme.x", Status: "installed", LastOperationID: opID},
		Finish:       &store.Finish{OperationID: opID, Status: "succeeded"},
		Events:       []store.Event{{ModuleKey: "acme.x", Event: "installed", Actor: "test", OperationID: opID}},
		BumpRevision: true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Operation(ctx, tenantB, opID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Operation(B) err = %v, want ErrNotFound", err)
	}
	if err := s.SaveSteps(ctx, tenantB, opID, []store.Step{{Name: "x", Status: "running"}}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SaveSteps(B) err = %v, want ErrNotFound", err)
	}
	if evs, total, err := s.Events(ctx, tenantB, "acme.x", 1, 10); err != nil || total != 0 || len(evs) != 0 {
		t.Errorf("Events(B) = %+v total=%d %v", evs, total, err)
	}
	if ins, err := s.Installations(ctx, tenantB); err != nil || len(ins) != 0 {
		t.Errorf("Installations(B) = %+v %v", ins, err)
	}
	if ops, err := s.Operations(ctx, tenantB, "acme.x", 10); err != nil || len(ops) != 0 {
		t.Errorf("Operations(B) = %+v %v", ops, err)
	}
	if rev, _ := s.Revision(ctx, tenantB); rev != 0 {
		t.Errorf("Revision(B) = %d", rev)
	}
	// Finish untuk operasi tenant lain tidak boleh menutupnya.
	if _, err := s.Commit(ctx, tenantB, store.Commit{Finish: &store.Finish{OperationID: opID, Status: "failed"}}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Commit(B finish) err = %v, want ErrNotFound", err)
	}
}

func TestSaveSteps_PersistsBeforeAndAfter(t *testing.T) {
	ctx := context.Background()
	_, _, s := setup(t)
	opID := startOp(t, s, tenantA, "acme.x", "install")
	started := time.Now().UTC().Truncate(time.Millisecond)

	if err := s.SaveSteps(ctx, tenantA, opID, []store.Step{{Name: "resolve", Status: "running", StartedAt: started}}); err != nil {
		t.Fatal(err)
	}
	op, _ := s.Operation(ctx, tenantA, opID)
	if len(op.Steps) != 1 || op.Steps[0].Status != "running" {
		t.Fatalf("steps (before) = %+v", op.Steps)
	}
	final := []store.Step{{Name: "resolve", Status: "succeeded", StartedAt: started, DurationMS: 12}}
	if err := s.SaveSteps(ctx, tenantA, opID, final); err != nil {
		t.Fatal(err)
	}
	op, _ = s.Operation(ctx, tenantA, opID)
	if len(op.Steps) != 1 || op.Steps[0].Name != "resolve" || op.Steps[0].Status != "succeeded" ||
		op.Steps[0].DurationMS != 12 || !op.Steps[0].StartedAt.Equal(started) {
		t.Fatalf("steps (after) = %+v", op.Steps)
	}
	running, err := s.RunningOperations(ctx, tenantA)
	if err != nil || len(running) != 1 || running[0].ID != opID {
		t.Errorf("RunningOperations = %+v, %v", running, err)
	}
}

func TestWithTenantLock_SecondStoreTimesOut(t *testing.T) {
	ctx := context.Background()
	dsn, _, s1 := setup(t)
	s2 := newStore(openDB(t, dsn))

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s1.WithTenantLock(ctx, tenantA, time.Second, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	start := time.Now()
	err := s2.WithTenantLock(ctx, tenantA, 200*time.Millisecond, func(context.Context) error {
		t.Error("fn tidak boleh berjalan tanpa lock")
		return nil
	})
	if !errors.Is(err, store.ErrLockTimeout) {
		t.Fatalf("err = %v, want ErrLockTimeout", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("timeout memakan %v, want < 1s", d)
	}

	// Tenant lain tidak terblokir.
	ran := false
	if err := s2.WithTenantLock(ctx, tenantB, time.Second, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("lock tenant B: err=%v ran=%v", err, ran)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ran = false
	if err := s2.WithTenantLock(ctx, tenantA, time.Second, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("lock setelah dilepas: err=%v ran=%v", err, ran)
	}
}

func TestWithTenantLock_ReleasedOnPanicAndCancel(t *testing.T) {
	ctx := context.Background()
	dsn, _, s1 := setup(t)
	s2 := newStore(openDB(t, dsn))

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic harus diteruskan")
			}
		}()
		_ = s1.WithTenantLock(ctx, tenantA, time.Second, func(context.Context) error { panic("boom") })
	}()
	ran := false
	if err := s2.WithTenantLock(ctx, tenantA, 500*time.Millisecond, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("lock setelah panic: err=%v ran=%v", err, ran)
	}

	cctx, cancel := context.WithCancel(ctx)
	err := s1.WithTenantLock(cctx, tenantA, time.Second, func(c context.Context) error {
		cancel()
		<-c.Done()
		return c.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	ran = false
	if err := s2.WithTenantLock(ctx, tenantA, 500*time.Millisecond, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("lock setelah cancel: err=%v ran=%v", err, ran)
	}
}

func TestRecordModuleMigration_TenantDB(t *testing.T) {
	ctx := context.Background()
	_, db, s := setup(t)
	opID := uuid.New()
	err := s.RecordModuleMigration(ctx, store.ModuleMigration{
		TenantID: tenantA, ModuleKey: "acme.x", ModuleVersion: "1.0.0",
		MigrationKey: "0001_create.sql", Checksum: "abc123", ExecutionStatus: "succeeded", OperationID: opID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		TenantID uuid.UUID
		Checksum string
		Status   string
	}
	if err := db.Raw("SELECT tenant_id, checksum, execution_status AS status FROM module_migrations WHERE module_key = 'acme.x'").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.TenantID != uuid.UUID(tenantA) || row.Checksum != "abc123" || row.Status != "succeeded" {
		t.Errorf("row = %+v", row)
	}
	// Tenant tidak dikenal ditolak connector.
	if err := s.RecordModuleMigration(ctx, store.ModuleMigration{TenantID: tenantB, ModuleKey: "acme.x"}); !errors.Is(err, database.ErrUnknownTenant) {
		t.Errorf("err = %v, want ErrUnknownTenant", err)
	}
}
