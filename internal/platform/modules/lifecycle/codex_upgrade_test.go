package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	"erpora/internal/platform/modules/runtime"
)

// Codex PR#6 C7: upgrade dengan versi binary lebih rendah daripada installed_version ditolak (FR-19.6), di dalam lock,
// tanpa operasi upgrade, tanpa hook, dan tanpa menimpa installed_version (reconcile mencatatnya sebagai failed).
func TestExecute_UpgradeRejectsDowngrade(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"}) // versi binary 1.0.0
	e := newEnv(t, rec, a)
	e.st.seed(tenantA, "acme.a", runtime.StatusUpgradePending, "2.0.0")
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.a", Actor: "cli:t"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeDowngradeNotSupported {
		t.Fatalf("err = %v, want downgrade_not_supported", err)
	}
	for _, op := range e.st.startOps {
		if op.Operation == "upgrade" {
			t.Errorf("tidak boleh ada operasi upgrade: %+v", op)
		}
	}
	if e.st.row("acme.a").InstalledVersion != "2.0.0" || len(rec.list()) != 0 {
		t.Errorf("tidak boleh ada operasi/hook: row=%+v calls=%v", e.st.row("acme.a"), rec.list())
	}
}

// Codex PR#6 C6 (keputusan owner 28 Okt 2026, Q32): `upgrade` menjalankan reconcile lebih dulu di bawah lock yang sama,
// sehingga modul enabled/disabled/installed di versi lama dapat langsung di-upgrade setelah binary diganti (tanpa serve);
// status kembali ke status sebelumnya dan hook Upgrade terpanggil dengan pasangan versi yang benar.
func TestExecute_UpgradeReconcilesFirstAndRestoresStatus(t *testing.T) {
	for _, st := range []runtime.Status{runtime.StatusEnabled, runtime.StatusDisabled, runtime.StatusInstalled} {
		t.Run(string(st), func(t *testing.T) {
			rec := &fxRec{}
			a := newFx(rec, fxSpec{key: "acme.a", version: "1.1.0"})
			e := newEnv(t, rec, a)
			e.st.seed(tenantA, "acme.a", st, "1.0.0")
			res, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.a", Actor: "cli:t"})
			if err != nil || len(res) != 1 || res[0].Status != "succeeded" {
				t.Fatalf("upgrade = %+v, %v", res, err)
			}
			row := e.st.row("acme.a")
			if res[0].To != st || row.Status != string(st) || row.InstalledVersion != "1.1.0" || row.PreviousStatus != "" {
				t.Errorf("hasil=%+v row=%+v, want status %s versi 1.1.0", res[0], row, st)
			}
			if !slices.Contains(rec.list(), "acme.a:Upgrade") {
				t.Errorf("hook Upgrade tidak terpanggil: %v", rec.list())
			}
			if !slices.Contains(e.st.eventNames("acme.a"), "reconciled") {
				t.Errorf("event reconciled tidak tercatat: %v", e.st.eventNames("acme.a"))
			}
			if e.snap.Status(tenantA, "acme.a") != st {
				t.Errorf("snapshot = %s, want %s", e.snap.Status(tenantA, "acme.a"), st)
			}
		})
	}
}

// B-2 tetap berlaku: upgrade yang mengembalikan modul ke enabled ditolak bila dependensi wajib tidak enabled.
func TestExecute_UpgradeReconcileKeepsDependencyInvariant(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	b := newFx(rec, fxSpec{key: "acme.b", version: "1.1.0", deps: []moduleDep{dep("acme.a")}})
	e := newEnv(t, rec, a, b)
	e.st.seed(tenantA, "acme.a", runtime.StatusDisabled, "1.0.0")
	e.st.seed(tenantA, "acme.b", runtime.StatusEnabled, "1.0.0")
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.b", Actor: "cli:t"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeDependencyNotReady {
		t.Fatalf("err = %v, want dependency_not_ready", err)
	}
	// Reconcile tetap tercatat dan snapshot mengikuti (upgrade_pending), seperti reconcile saat startup.
	if e.snap.Status(tenantA, "acme.b") != runtime.StatusUpgradePending {
		t.Errorf("snapshot = %s, want upgrade_pending", e.snap.Status(tenantA, "acme.b"))
	}
}

// C7 lewat reconcile: binary lebih lama daripada versi ter-install tetap menghasilkan downgrade_not_supported.
func TestExecute_UpgradeAfterReconcileStillRejectsDowngrade(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.0.0"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.1.0")
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.a", Actor: "cli:t"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeDowngradeNotSupported {
		t.Fatalf("err = %v, want downgrade_not_supported", err)
	}
	if len(rec.list()) != 0 || e.st.row("acme.a").InstalledVersion != "1.1.0" {
		t.Errorf("tidak boleh ada hook: %v row=%+v", rec.list(), e.st.row("acme.a"))
	}
}

// `upgrade --all` ikut mereconcile semua modul dan meng-upgrade yang versinya lebih lama.
func TestExecute_UpgradeAllReconcilesFirst(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a", version: "1.1.0"})
	b := newFx(rec, fxSpec{key: "acme.b", version: "1.0.0"})
	e := newEnv(t, rec, a, b)
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")
	e.st.seed(tenantA, "acme.b", runtime.StatusEnabled, "1.0.0")
	res, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, All: true, Actor: "cli:t"})
	if err != nil || len(res) == 0 || res[0].Module != "acme.a" || res[0].Status != "succeeded" {
		t.Fatalf("upgrade --all = %+v, %v", res, err)
	}
	if e.st.row("acme.a").InstalledVersion != "1.1.0" || e.st.row("acme.b").Status != "enabled" {
		t.Errorf("a=%+v b=%+v", e.st.row("acme.a"), e.st.row("acme.b"))
	}
}

// plan upgrade konsisten dengan Execute: modul enabled di versi lama direncanakan upgrade_pending -> enabled, read-only.
func TestPlan_UpgradeSimulatesReconcile(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.1.0"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")
	p, err := e.svc.Plan(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.a"})
	if err != nil || len(p.Ops) != 1 || p.Ops[0].Blocked != nil || p.Ops[0].To != runtime.StatusEnabled {
		t.Fatalf("Plan = %+v, %v", p, err)
	}
	if e.st.row("acme.a").Status != "enabled" || len(e.st.commits) != 0 {
		t.Errorf("Plan harus read-only: %+v", e.st.row("acme.a"))
	}
}
