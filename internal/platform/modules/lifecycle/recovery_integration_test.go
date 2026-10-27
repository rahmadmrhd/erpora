//go:build integration

package lifecycle

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"erpora/internal/platform/modules/resolver"
	rt "erpora/internal/platform/modules/runtime"
)

func fxSpecWith(rec *fxRec, dir string) *fxModule {
	return newFx(rec, fxSpec{key: "acme.fx-mod", migrations: os.DirFS(fxMigDir + dir)})
}

// Review Focus 3: migrasi ke-2 gagal -> 0001 tetap, 0002 rollback, modul failed; setelah diperbaiki,
// retry hanya menerapkan 0002.
func TestService_FailingMigrationThenRetry(t *testing.T) {
	ctx := context.Background()
	rec := &fxRec{}
	m := fxSpecWith(rec, "failing")
	e := newRealEnv(t, m)

	r := e.exec(t, OpInstall, "acme.fx-mod")
	if r.Status != "failed" || r.Error == nil || r.Error.Code != "migration_failed" || r.Error.Step != "migrate" {
		t.Fatalf("install = %+v (%+v)", r, r.Error)
	}
	ins, _ := e.st.Installations(ctx, tenantA)
	if len(ins) != 1 || ins[0].Status != "failed" || ins[0].PreviousStatus != "available" {
		t.Fatalf("installations = %+v", ins)
	}
	if v := e.count(t, "SELECT max(version_id) FROM goose_module_fx_mod WHERE is_applied"); v != 1 {
		t.Errorf("versi goose = %d, want 1", v)
	}
	if n := e.count(t, "SELECT count(*) FROM module_migrations WHERE migration_key = '0002_broken.sql' AND execution_status = 'failed'"); n != 1 {
		t.Errorf("module_migrations failed 0002 = %d, want 1", n)
	}
	if e.snap.Status(tenantA, "acme.fx-mod") != rt.StatusFailed {
		t.Error("snapshot harus failed")
	}

	m.mig.set(os.DirFS(fxMigDir + "failing_fixed"))
	res, err := e.svc.Retry(ctx, tenantA, r.OperationID, "cli:tester")
	if err != nil || res.Status != "succeeded" {
		t.Fatalf("Retry = %+v, %v", res, err)
	}
	if v := e.count(t, "SELECT max(version_id) FROM goose_module_fx_mod WHERE is_applied"); v != 2 {
		t.Errorf("versi goose = %d, want 2", v)
	}
	if n := e.count(t, "SELECT count(*) FROM fx_counter"); n != 1 {
		t.Errorf("0001 dijalankan ulang (fx_counter = %d)", n)
	}
	if n := e.count(t, "SELECT count(*) FROM module_migrations WHERE execution_status = 'succeeded'"); n != 2 {
		t.Errorf("module_migrations succeeded = %d, want 2", n)
	}
	ins, _ = e.st.Installations(ctx, tenantA)
	if ins[0].Status != "installed" || ins[0].InstalledVersion != "1.0.0" || ins[0].PreviousStatus != "" {
		t.Errorf("installations = %+v", ins)
	}
}

// Review Focus 2: proses mati setelah langkah migrate. Goexit mensimulasikan crash: hanya defer unlock
// yang berjalan, baris running disisakan.
func TestService_InterruptedOperationMarkedUnderLock(t *testing.T) {
	ctx := context.Background()
	rec := &fxRec{}
	m := fxSpecWith(rec, "ok")
	crash := func(d *Deps) {
		d.AfterStep = func(op Op, key, step string) {
			if op == OpInstall && step == "migrate" {
				runtime.Goexit()
			}
		}
	}
	dsn := newRealEnvDSN(t)
	e := newRealService(t, dsn.dsn, dsn.db, 0, []*fxModule{m}, crash)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = e.svc.Execute(ctx, Request{Tenant: tenantA, Op: OpInstall, Key: "acme.fx-mod", Actor: "cli:crash"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Execute tidak berhenti")
	}
	running, err := e.st.RunningOperations(ctx, tenantA)
	if err != nil || len(running) != 1 {
		t.Fatalf("operasi running = %+v, %v", running, err)
	}
	var names []string
	for _, s := range running[0].Steps {
		names = append(names, s.Name+":"+s.Status)
	}
	if len(names) != 3 || names[2] != "migrate:succeeded" {
		t.Fatalf("steps = %v", names)
	}
	migrationsBefore := e.count(t, "SELECT count(*) FROM module_migrations")

	// Startup berikutnya: reconcile menandai operasi interrupted di bawah lock.
	healthy := newRealService(t, dsn.dsn, dsn.db, 0, []*fxModule{fxSpecWith(rec, "ok")})
	rep, err := healthy.svc.Reconcile(ctx, tenantA)
	if err != nil || len(rep.Interrupted) != 1 || rep.Interrupted[0] != running[0].ID {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	op, err := healthy.st.Operation(ctx, tenantA, running[0].ID)
	if err != nil || op.Status != "failed" || op.Error == nil || op.Error.Code != CodeInterrupted {
		t.Fatalf("operasi = %+v, %v", op, err)
	}
	ins, _ := healthy.st.Installations(ctx, tenantA)
	if len(ins) != 1 || ins[0].Status != "failed" || ins[0].PreviousStatus != "available" {
		t.Fatalf("installations = %+v", ins)
	}

	res, err := healthy.svc.RetryModule(ctx, tenantA, "acme.fx-mod", "cli:tester", nil, "")
	if err != nil || res.Status != "succeeded" || res.To != rt.StatusInstalled {
		t.Fatalf("Retry = %+v, %v", res, err)
	}
	if n := healthy.count(t, "SELECT count(*) FROM module_migrations"); n != migrationsBefore {
		t.Errorf("module_migrations bertambah: %d -> %d", migrationsBefore, n)
	}
}

func TestService_DowngradeRejectedAndRestored(t *testing.T) {
	ctx := context.Background()
	rec := &fxRec{}
	h := newRealEnvDSN(t)
	v11 := func() *fxModule { return newFx(rec, fxSpec{key: "acme.fx-mod", version: "1.1.0"}) }
	v10 := func() *fxModule { return newFx(rec, fxSpec{key: "acme.fx-mod", version: "1.0.0"}) }
	e := newRealService(t, h.dsn, h.db, 0, []*fxModule{v11()})
	e.exec(t, OpInstall, "acme.fx-mod")
	e.exec(t, OpEnable, "acme.fx-mod")

	old := newRealService(t, h.dsn, h.db, 0, []*fxModule{v10()})
	rep, err := old.svc.Reconcile(ctx, tenantA)
	if err != nil || len(rep.Downgraded) != 1 {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	ins, _ := old.st.Installations(ctx, tenantA)
	if ins[0].Status != "failed" || ins[0].PreviousStatus != "enabled" || ins[0].InstalledVersion != "1.1.0" {
		t.Fatalf("installations = %+v", ins)
	}
	op, _ := old.st.Operation(ctx, tenantA, ins[0].LastOperationID)
	if op.Operation != "reconcile" || op.Error == nil || op.Error.Code != CodeDowngradeNotSupported {
		t.Fatalf("operasi = %+v", op)
	}
	_, err = old.svc.Retry(ctx, tenantA, op.ID, "cli:tester")
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeInvalidTransition {
		t.Fatalf("retry reconcile err = %v", err)
	}

	back := newRealService(t, h.dsn, h.db, 0, []*fxModule{v11()})
	rep, err = back.svc.Reconcile(ctx, tenantA)
	if err != nil || len(rep.Restored) != 1 {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	ins, _ = back.st.Installations(ctx, tenantA)
	if ins[0].Status != "enabled" || ins[0].PreviousStatus != "" {
		t.Errorf("installations = %+v", ins)
	}
	if back.snap.Status(tenantA, "acme.fx-mod") != rt.StatusEnabled {
		t.Error("snapshot harus enabled kembali")
	}
}

// Review Focus 5: modul required di-install dan di-enable; panggilan kedua no-op; tidak dapat di-disable.
func TestService_EnsureRequiredIdempotent(t *testing.T) {
	ctx := context.Background()
	rec := &fxRec{}
	e := newRealEnv(t, newFx(rec, fxSpec{key: "acme.req", required: true}))

	res, err := e.svc.EnsureRequired(ctx, tenantA, "system:migrate")
	if err != nil || len(res) != 2 || res[0].Status != "succeeded" || res[1].Status != "succeeded" {
		t.Fatalf("EnsureRequired = %+v, %v", res, err)
	}
	if e.snap.Status(tenantA, "acme.req") != rt.StatusEnabled {
		t.Error("harus enabled")
	}
	events := e.count(t, "SELECT count(*) FROM module_lifecycle_events")
	ops := e.count(t, "SELECT count(*) FROM module_operations")
	res, err = e.svc.EnsureRequired(ctx, tenantA, "system:migrate")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Status != "noop" {
			t.Errorf("%s %s = %s, want noop", r.Op, r.Module, r.Status)
		}
	}
	if n := e.count(t, "SELECT count(*) FROM module_lifecycle_events"); n != events {
		t.Errorf("event bertambah: %d -> %d", events, n)
	}
	if n := e.count(t, "SELECT count(*) FROM module_operations"); n != ops {
		t.Errorf("operasi bertambah: %d -> %d", ops, n)
	}
	_, err = e.svc.Execute(ctx, Request{Tenant: tenantA, Op: OpDisable, Key: "acme.req", Actor: "cli:tester"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeModuleRequired {
		t.Fatalf("disable required err = %v", err)
	}
}

func TestService_IncompatibleInstallRejected(t *testing.T) {
	rec := &fxRec{}
	e := newRealEnv(t, newFx(rec, fxSpec{key: "acme.bad", platformC: "^9.0"}))
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, Key: "acme.bad", Actor: "cli:tester"})
	var re *resolver.ResolutionError
	if !errors.As(err, &re) || len(re.Issues) == 0 || re.Issues[0].Code != resolver.CodePlatformIncompatible {
		t.Fatalf("err = %v", err)
	}
	if n := e.count(t, "SELECT count(*) FROM module_operations"); n != 0 {
		t.Errorf("operasi tercatat: %d", n)
	}
}
