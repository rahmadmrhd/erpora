package lifecycle

import (
	"context"
	"errors"
	"testing"

	"erpora/internal/platform/modules/runtime"
)

// Codex PR#6 C7: upgrade dengan versi binary lebih rendah daripada installed_version ditolak (FR-19.6), di dalam lock,
// tanpa operasi dan tanpa menimpa installed_version.
func TestExecute_UpgradeRejectsDowngrade(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"}) // versi binary 1.0.0
	e := newEnv(t, rec, a)
	e.st.seed(tenantA, "acme.a", runtime.StatusUpgradePending, "2.0.0")
	ops := len(e.st.opOrder)
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.a", Actor: "cli:t"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeDowngradeNotSupported {
		t.Fatalf("err = %v, want downgrade_not_supported", err)
	}
	if len(e.st.opOrder) != ops || e.st.row("acme.a").InstalledVersion != "2.0.0" || len(rec.list()) != 0 {
		t.Errorf("tidak boleh ada operasi/hook: row=%+v calls=%v", e.st.row("acme.a"), rec.list())
	}
}
