package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
)

// swapRegistry mengganti registry (simulasi binary baru) dengan store dan snapshot yang sama.
func (e *env) swapRegistry(t *testing.T, mods ...*fxModule) {
	t.Helper()
	e.reg = buildFxRegistry(t, mods...)
	e.svc = New(Deps{
		Registry: e.reg, Store: e.st, Migrator: e.mig, Tenants: e.svc.d.Tenants,
		Snapshot: e.snap, Notifier: e.notifier,
	})
}

func TestReconcile_UpgradePendingThenIdempotent(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.1.0"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")

	rep, err := e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !slices.Equal(rep.UpgradePending, []string{"acme.a"}) {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	row := e.st.row("acme.a")
	if row.Status != "upgrade_pending" || row.PreviousStatus != "enabled" || row.InstalledVersion != "1.0.0" {
		t.Errorf("row = %+v", row)
	}
	if !slices.Contains(e.st.eventNames("acme.a"), "reconciled") {
		t.Errorf("events = %v", e.st.eventNames("acme.a"))
	}
	if e.snap.Status(tenantA, "acme.a") != runtime.StatusUpgradePending {
		t.Error("snapshot harus upgrade_pending")
	}
	if len(e.revs) != 1 {
		t.Errorf("notifier = %v", e.revs)
	}
	before := len(e.st.commits)
	rep, err = e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || len(rep.UpgradePending) != 0 || len(e.st.commits) != before {
		t.Errorf("reconcile kedua harus idempoten: %+v commits %d -> %d (%v)", rep, before, len(e.st.commits), err)
	}
	if len(e.st.startOps) != 0 {
		t.Error("reconcile upgrade_pending tidak membuat operasi")
	}
}

// B-3 (Q32): binary kembali ke versi ter-install saat status upgrade_pending (rollback binary): status dikembalikan ke
// previous_status dengan event reconciled (version_restored); upgrade_pending tidak boleh tertinggal tanpa alasan.
func TestReconcile_UpgradePendingRestoredWhenBinaryVersionMatchesInstalled(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.0.0"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusUpgradePending, "1.0.0")
	e.st.mu.Lock()
	r := e.st.rows[rowKey{tenantA, "acme.a"}]
	r.PreviousStatus = "enabled"
	e.st.rows[rowKey{tenantA, "acme.a"}] = r
	e.st.mu.Unlock()

	rep, err := e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !slices.Equal(rep.Restored, []string{"acme.a"}) {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	row := e.st.row("acme.a")
	if row.Status != "enabled" || row.PreviousStatus != "" || row.InstalledVersion != "1.0.0" {
		t.Errorf("row = %+v", row)
	}
	var found bool
	for _, ev := range e.st.events {
		if ev.ModuleKey == "acme.a" && ev.Event == "reconciled" && ev.Details["reason"] == "version_restored" {
			found = true
		}
	}
	if !found {
		t.Errorf("event reconciled/version_restored tidak ada: %v", e.st.eventNames("acme.a"))
	}
	if e.snap.Status(tenantA, "acme.a") != runtime.StatusEnabled {
		t.Error("snapshot harus enabled kembali")
	}
	// Idempoten.
	before := len(e.st.commits)
	if rep, err = e.svc.Reconcile(context.Background(), tenantA); err != nil || len(rep.Restored) != 0 || len(e.st.commits) != before {
		t.Errorf("reconcile kedua: %+v commits %d -> %d (%v)", rep, before, len(e.st.commits), err)
	}
}

func TestReconcile_DowngradeThenRestore(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.0.0"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.1.0")

	rep, err := e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !slices.Equal(rep.Downgraded, []string{"acme.a"}) {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	row := e.st.row("acme.a")
	if row.Status != "failed" || row.PreviousStatus != "enabled" || row.InstalledVersion != "1.1.0" {
		t.Fatalf("row = %+v", row)
	}
	if len(e.st.startOps) != 1 || e.st.startOps[0].Operation != "reconcile" {
		t.Fatalf("operasi = %+v", e.st.startOps)
	}
	op, _ := e.st.Operation(context.Background(), tenantA, row.LastOperationID)
	if op.Status != "failed" || op.Error == nil || op.Error.Code != CodeDowngradeNotSupported {
		t.Errorf("operasi = %+v", op)
	}
	if e.snap.Status(tenantA, "acme.a") != runtime.StatusFailed {
		t.Error("snapshot harus failed")
	}
	before := len(e.st.commits)
	if rep, _ = e.svc.Reconcile(context.Background(), tenantA); len(rep.Downgraded) != 0 || len(e.st.commits) != before {
		t.Errorf("reconcile kedua harus idempoten: %+v", rep)
	}

	// Retry operasi reconcile tidak diizinkan.
	_, err = e.svc.Retry(context.Background(), tenantA, op.ID, "cli:tester")
	wantCode(t, err, CodeInvalidTransition)

	// Binary kembali ke versi ter-install: status semula dipulihkan.
	e.swapRegistry(t, newFx(rec, fxSpec{key: "acme.a", version: "1.1.0"}))
	rep, err = e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !slices.Equal(rep.Restored, []string{"acme.a"}) {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	if row := e.st.row("acme.a"); row.Status != "enabled" || row.PreviousStatus != "" || row.InstalledVersion != "1.1.0" {
		t.Errorf("row = %+v", row)
	}
}

func TestReconcile_MissingModuleReportedWithoutWrites(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.seed(tenantA, "acme.gone", runtime.StatusEnabled, "1.0.0")
	rep, err := e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !slices.Equal(rep.Missing, []string{"acme.gone"}) {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	if len(e.st.commits) != 0 {
		t.Error("modul missing tidak boleh ditulis")
	}
	if e.snap.Status(tenantA, "acme.gone") != runtime.StatusMissing {
		t.Error("snapshot harus missing")
	}
}

func TestReconcile_InterruptedOperationMarkedFailed(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	id := uuid.New()
	e.st.ops[id] = store.Operation{
		ID: id, TenantID: tenantA, ModuleKey: "acme.a", Operation: "install", Status: "running", Actor: "cli:x",
		Steps: []store.Step{{Name: "resolve", Status: "succeeded"}, {Name: "migrate", Status: "running"}},
	}
	e.st.opOrder = append(e.st.opOrder, id)

	rep, err := e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !slices.Equal(rep.Interrupted, []uuid.UUID{id}) {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	op, _ := e.st.Operation(context.Background(), tenantA, id)
	if op.Status != "failed" || op.Error == nil || op.Error.Code != CodeInterrupted || op.Error.Step != "migrate" {
		t.Errorf("operasi = %+v (%+v)", op, op.Error)
	}
	if row := e.st.row("acme.a"); row.Status != "failed" || row.PreviousStatus != "available" {
		t.Errorf("row = %+v", row)
	}
	if !slices.Contains(e.st.eventNames("acme.a"), "interrupted") {
		t.Errorf("events = %v", e.st.eventNames("acme.a"))
	}
}

func TestReconcile_InterruptedEnableKeepsModuleStatus(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusDisabled, "1.0.0")
	id := uuid.New()
	e.st.ops[id] = store.Operation{ID: id, TenantID: tenantA, ModuleKey: "acme.a", Operation: "enable", Status: "running", Actor: "cli:x"}
	e.st.opOrder = append(e.st.opOrder, id)
	if _, err := e.svc.Reconcile(context.Background(), tenantA); err != nil {
		t.Fatal(err)
	}
	if row := e.st.row("acme.a"); row.Status != "disabled" || row.InstalledVersion != "1.0.0" {
		t.Errorf("hanya install/upgrade yang membuat modul failed: %+v", row)
	}
}

func TestReconcile_LockTimeoutOnlyRefreshes(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.1.0"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")
	e.st.revision[tenantA] = 4
	e.st.lockErr = store.ErrLockTimeout

	rep, err := e.svc.Reconcile(context.Background(), tenantA)
	if err != nil || !rep.LockSkipped {
		t.Fatalf("Reconcile = %+v, %v", rep, err)
	}
	if len(e.st.commits) != 0 {
		t.Error("tanpa lock tidak boleh menulis")
	}
	if e.snap.Status(tenantA, "acme.a") != runtime.StatusEnabled || e.snap.Revision(tenantA) != 4 {
		t.Error("snapshot harus tetap dimuat read-only")
	}
}

func TestReconcile_PropagatesStoreError(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	boom := errors.New("db mati")
	e.st.lockErr = boom
	if _, err := e.svc.Reconcile(context.Background(), tenantA); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
