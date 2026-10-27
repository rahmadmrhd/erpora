package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/sdk/module"
)

// failedInstall menyiapkan modul bermigrasi yang install-nya gagal di hook:install.
func failedInstall(t *testing.T) (*env, *fxModule, Result) {
	t.Helper()
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a", migrations: fxMigFS()})
	fail := true
	a.on("Install", func(context.Context, module.Env) error {
		if fail {
			return errors.New("gagal sementara")
		}
		return nil
	})
	e := newEnv(t, rec, a)
	res, err := e.exec(OpInstall, "acme.a")
	if err != nil || len(res) != 1 || res[0].Status != "failed" {
		t.Fatalf("install = %+v, %v", res, err)
	}
	a.on("Install", nil) // diperbaiki
	return e, a, res[0]
}

func TestRetry_ReRunsFailedStepSkipsSucceededButMigrates(t *testing.T) {
	e, a, first := failedInstall(t)
	if row := e.st.row("acme.a"); row.Status != "failed" || row.PreviousStatus != "available" {
		t.Fatalf("row = %+v", row)
	}
	migCalls := e.mig.calls

	res, err := e.svc.Retry(context.Background(), tenantA, first.OperationID, "cli:tester")
	if err != nil || res.Status != "succeeded" {
		t.Fatalf("Retry = %+v, %v", res, err)
	}
	if got := rec2(a); got != 2 {
		t.Errorf("Install dipanggil %d kali, want 2", got)
	}
	if e.mig.calls != migCalls+1 {
		t.Errorf("migrate harus selalu dijalankan: %d -> %d", migCalls, e.mig.calls)
	}
	want := map[string]string{"resolve": "skipped", "compatibility": "skipped", "migrate": "succeeded", "hook:install": "succeeded"}
	for _, s := range res.Steps {
		if want[s.Name] != s.Status {
			t.Errorf("step %s = %s, want %s", s.Name, s.Status, want[s.Name])
		}
	}
	if row := e.st.row("acme.a"); row.Status != "installed" || row.InstalledVersion != "1.0.0" || row.PreviousStatus != "" {
		t.Errorf("row = %+v", row)
	}
	names := e.st.eventNames("acme.a")
	if !slices.Contains(names, "retried") || !slices.Contains(names, "installed") {
		t.Errorf("events = %v", names)
	}
	op, _ := e.st.Operation(context.Background(), tenantA, res.OperationID)
	if op.Operation != "install" {
		t.Errorf("nama operasi retry = %q, want install", op.Operation)
	}
}

func rec2(a *fxModule) int {
	n := 0
	for _, c := range a.rec.list() {
		if c == "acme.a:Install" {
			n++
		}
	}
	return n
}

func TestRetry_NotLastOperationIsInvalidTransition(t *testing.T) {
	e, _, first := failedInstall(t)
	if _, err := e.svc.Retry(context.Background(), tenantA, first.OperationID, "cli:tester"); err != nil {
		t.Fatal(err)
	}
	// Operasi pertama masih failed, tetapi bukan operasi terakhir modul.
	_, err := e.svc.Retry(context.Background(), tenantA, first.OperationID, "cli:tester")
	wantCode(t, err, CodeInvalidTransition)
}

func TestRetry_SucceededOperationIsInvalidTransition(t *testing.T) {
	e, _, first := failedInstall(t)
	res, err := e.svc.Retry(context.Background(), tenantA, first.OperationID, "cli:tester")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Retry(context.Background(), tenantA, res.OperationID, "cli:tester")
	wantCode(t, err, CodeInvalidTransition)
}

func TestRetry_OtherTenantOperationNotFound(t *testing.T) {
	e, _, _ := failedInstall(t)
	other := tenantA
	other[0] ^= 0xff
	id := uuid.New()
	e.st.ops[id] = store.Operation{ID: id, TenantID: other, ModuleKey: "acme.a", Operation: "install", Status: "failed"}
	_, err := e.svc.Retry(context.Background(), tenantA, id, "cli:tester")
	wantCode(t, err, CodeOperationNotFound)
	_, err = e.svc.Retry(context.Background(), tenantA, uuid.New(), "cli:tester")
	wantCode(t, err, CodeOperationNotFound)
}

func TestRetryModule_UsesLastOperation(t *testing.T) {
	e, _, _ := failedInstall(t)
	res, err := e.svc.RetryModule(context.Background(), tenantA, "acme.a", "cli:tester")
	if err != nil || res.Status != "succeeded" || res.To != runtime.StatusInstalled {
		t.Fatalf("RetryModule = %+v, %v", res, err)
	}
	_, err = e.svc.RetryModule(context.Background(), tenantA, "acme.nope", "cli:tester")
	wantCode(t, err, CodeModuleNotFound)
}
