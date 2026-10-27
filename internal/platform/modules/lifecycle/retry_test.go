package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
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
	res, err := e.svc.RetryModule(context.Background(), tenantA, "acme.a", "cli:tester", nil, "")
	if err != nil || res.Status != "succeeded" || res.To != runtime.StatusInstalled {
		t.Fatalf("RetryModule = %+v, %v", res, err)
	}
	_, err = e.svc.RetryModule(context.Background(), tenantA, "acme.nope", "cli:tester", nil, "")
	wantCode(t, err, CodeModuleNotFound)
}

func TestRetryModule_NoOperationsIsInvalidTransition(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	_, err := e.svc.RetryModule(context.Background(), tenantA, "acme.a", "cli:tester", nil, "")
	wantCode(t, err, CodeInvalidTransition)
}

func TestRetryModule_CallsOnStartedWithOperationID(t *testing.T) {
	e, _, _ := failedInstall(t)
	var got uuid.UUID
	res, err := e.svc.RetryModule(context.Background(), tenantA, "acme.a", "cli:tester", func(id uuid.UUID) { got = id }, "")
	if err != nil || res.Status != "succeeded" {
		t.Fatalf("RetryModule = %+v, %v", res, err)
	}
	if got == uuid.Nil || got != res.OperationID {
		t.Errorf("onStarted = %s, operasi = %s", got, res.OperationID)
	}
}

// B-2 (Q32): retry yang akan membuat modul enabled harus memeriksa ulang dependensi wajib; langkah "dependencies" tidak
// boleh di-skip hanya karena tercatat succeeded pada operasi asal, dan penolakan terjadi sebelum operasi dimulai.
func TestRetry_EnableRechecksDependenciesBeforeStarting(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	b := newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}})
	e := newEnv(t, rec, a, b)
	for _, step := range []struct {
		op  Op
		key string
	}{{OpInstall, "acme.a"}, {OpEnable, "acme.a"}, {OpInstall, "acme.b"}} {
		if res, err := e.exec(step.op, step.key); err != nil || res[len(res)-1].Status != "succeeded" {
			t.Fatalf("%s %s = %+v, %v", step.op, step.key, res, err)
		}
	}
	fail := true
	b.on("OnEnable", func(context.Context, module.Env) error {
		if fail {
			return errors.New("gagal sementara")
		}
		return nil
	})
	res, err := e.exec(OpEnable, "acme.b")
	if err != nil || res[len(res)-1].Status != "failed" {
		t.Fatalf("enable b = %+v, %v", res, err)
	}
	// b gagal (previous installed): a boleh dinonaktifkan; sesudah itu retry b tidak boleh membuat b enabled.
	if res, err := e.exec(OpDisable, "acme.a"); err != nil || res[0].Status != "succeeded" {
		t.Fatalf("disable a = %+v, %v", res, err)
	}
	fail = false
	opsBefore := len(e.st.opOrder)
	_, err = e.svc.Retry(context.Background(), tenantA, res[len(res)-1].OperationID, "cli:tester")
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeDependencyNotReady {
		t.Fatalf("Retry err = %v, want dependency_not_ready", err)
	}
	if len(e.st.opOrder) != opsBefore {
		t.Error("penolakan harus terjadi sebelum operasi baru tercatat")
	}
	if row := e.st.row("acme.b"); row.Status == "enabled" {
		t.Errorf("b tidak boleh enabled saat a disabled: %+v", row)
	}
}

// B-2 (Q32): upgrade yang akan mengembalikan modul ke enabled ditolak sebelum dimulai bila dependensi wajib tidak enabled;
// ketika dependensi enabled, upgrade berjalan dan modul kembali enabled.
func TestExecute_UpgradeRestoringEnabledRequiresEnabledDependencies(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	b := newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}})
	e := newEnv(t, rec, a, b)
	e.st.seed(tenantA, "acme.a", runtime.StatusDisabled, "1.0.0")
	e.st.seed(tenantA, "acme.b", runtime.StatusUpgradePending, "0.9.0")
	e.st.mu.Lock()
	br := e.st.rows[rowKey{tenantA, "acme.b"}]
	br.PreviousStatus = "enabled"
	e.st.rows[rowKey{tenantA, "acme.b"}] = br
	e.st.mu.Unlock()

	ops := len(e.st.opOrder)
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.b", Actor: "cli:t"})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeDependencyNotReady {
		t.Fatalf("err = %v, want dependency_not_ready", err)
	}
	if len(e.st.opOrder) != ops || e.st.row("acme.b").Status != "upgrade_pending" {
		t.Errorf("penolakan harus sebelum operasi: ops %d -> %d, row=%+v", ops, len(e.st.opOrder), e.st.row("acme.b"))
	}
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")
	res, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpUpgrade, Key: "acme.b", Actor: "cli:t"})
	if err != nil || len(res) != 1 || res[0].Status != "succeeded" || res[0].To != runtime.StatusEnabled {
		t.Fatalf("upgrade saat a enabled = %+v, %v", res, err)
	}
}

// B-4 (Q32): revision naik pada setiap commit operasi, termasuk operasi gagal yang tidak mengubah status, sehingga
// ETag (= revision) berubah ketika lastOperation berubah dan klien tidak menerima 304 basi.
func TestRevisionBumpsOnFailedOperationWithoutStatusChange(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	e := newEnv(t, rec, a)
	if res, err := e.exec(OpInstall, "acme.a"); err != nil || res[0].Status != "succeeded" {
		t.Fatalf("install = %+v, %v", res, err)
	}
	a.on("Health", func(context.Context, module.Env) error { return errors.New("tidak sehat") })
	before, _ := e.st.Revision(context.Background(), tenantA)
	res, err := e.exec(OpEnable, "acme.a")
	if err != nil || res[0].Status != "failed" {
		t.Fatalf("enable = %+v, %v", res, err)
	}
	after, _ := e.st.Revision(context.Background(), tenantA)
	if after <= before {
		t.Errorf("revision tidak naik setelah operasi gagal: %d -> %d", before, after)
	}
	if res[0].Revision != after {
		t.Errorf("Result.Revision = %d, want %d", res[0].Revision, after)
	}
	if got := e.st.row("acme.a").Status; got != "installed" {
		t.Errorf("status tidak boleh berubah: %s", got)
	}
}

// B-10 (Q32): message kegagalan migration_failed dan internal_error diringkas (tanpa teks SQL/hook mentah) pada hasil,
// operasi tersimpan, dan event; code dan step tetap ada. Detail penuh masuk log (dengan trace_id).
func TestFailureMessageSummarizedAndDetailLogged(t *testing.T) {
	var logBuf bytes.Buffer
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a", migrations: fxMigFS()})
	e := newEnvOpts(t, rec, envOpts{log: slog.New(slog.NewTextHandler(&logBuf, nil))}, a)
	e.mig.failOn = "0002_two.sql"
	res, err := e.exec(OpInstall, "acme.a")
	if err != nil || res[0].Status != "failed" || res[0].Error == nil {
		t.Fatalf("install = %+v, %v", res, err)
	}
	const raw = "boom" // teks mentah dari migrator (di produksi: SQLSTATE/teks SQL)
	f := res[0].Error
	if f.Code != "migration_failed" || f.Step != "migrate" {
		t.Errorf("code/step = %s/%s", f.Code, f.Step)
	}
	if strings.Contains(f.Message, raw) || f.Message == "" {
		t.Errorf("message harus diringkas tanpa teks mentah: %q", f.Message)
	}
	op, _ := e.st.Operation(context.Background(), tenantA, res[0].OperationID)
	if op.Error == nil || strings.Contains(op.Error.Message, raw) {
		t.Errorf("operasi tersimpan memuat teks mentah: %+v", op.Error)
	}
	for _, ev := range e.st.events {
		if m, _ := ev.Details["message"].(string); strings.Contains(m, raw) {
			t.Errorf("event %s memuat teks mentah: %q", ev.Event, m)
		}
	}
	logs := logBuf.String()
	if !strings.Contains(logs, raw) || !strings.Contains(logs, "trace_id") || !strings.Contains(logs, res[0].OperationID.String()) {
		t.Errorf("log harus memuat detail penuh, trace_id, dan operation_id:\n%s", logs)
	}
}

// B-7: pencatatan module_migrations gagal setelah goose commit (tidak atomik). Retry harus menyelaraskan jejak dengan
// goose_module_<slug>: file yang sudah diterapkan tetapi belum punya baris tercatat sebagai succeeded.
func TestRetry_ReconcilesMigrationTrailWithGoose(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a", migrations: fxMigFS()})
	e := newEnv(t, rec, a)
	e.st.failRecordN = 100 // DB error: semua pencatatan jejak gagal selama install
	res, err := e.exec(OpInstall, "acme.a")
	if err != nil || len(res) != 1 || res[0].Status != "failed" {
		t.Fatalf("install = %+v, %v", res, err)
	}
	if got := e.st.migrationKeys(); len(got) != 0 {
		t.Fatalf("belum ada jejak yang boleh tercatat: %v", got)
	}
	e.st.failRecordN = 0 // pulih; goose sudah menerapkan file pertama, retry tidak menjalankannya lagi tetapi jejaknya harus dilengkapi.
	r, err := e.svc.Retry(context.Background(), tenantA, res[0].OperationID, "cli:tester")
	if err != nil || r.Status != "succeeded" {
		t.Fatalf("Retry = %+v, %v", r, err)
	}
	got := e.st.migrationKeys()
	slices.Sort(got)
	if !slices.Equal(got, []string{"0001_one.sql", "0002_two.sql"}) {
		t.Errorf("jejak module_migrations = %v, want kedua file tercatat tepat sekali", got)
	}
}

// B-1: retry enable yang gagal di health harus menjalankan ulang hook:enable, karena efeknya sudah dibatalkan
// kompensasi hook:disable (langkah hook:enable pada operasi asal tercatat succeeded).
func TestRetry_EnableAfterHealthFailureRerunsHookEnable(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	e := newEnv(t, rec, a)
	if res, err := e.exec(OpInstall, "acme.a"); err != nil || res[0].Status != "succeeded" {
		t.Fatalf("install = %+v, %v", res, err)
	}
	a.on("Health", func(context.Context, module.Env) error { return errors.New("belum sehat") })
	res, err := e.exec(OpEnable, "acme.a")
	if err != nil || res[len(res)-1].Status != "failed" {
		t.Fatalf("enable = %+v, %v", res, err)
	}
	count := func(name string) int {
		n := 0
		for _, c := range rec.list() {
			if c == "acme.a:"+name {
				n++
			}
		}
		return n
	}
	if count("OnEnable") != 1 || count("OnDisable") != 1 {
		t.Fatalf("setelah enable gagal: OnEnable=%d OnDisable=%d", count("OnEnable"), count("OnDisable"))
	}
	a.on("Health", nil) // diperbaiki
	r, err := e.svc.Retry(context.Background(), tenantA, res[len(res)-1].OperationID, "cli:tester")
	if err != nil || r.Status != "succeeded" {
		t.Fatalf("Retry = %+v, %v", r, err)
	}
	if got := count("OnEnable"); got != 2 {
		t.Errorf("OnEnable dipanggil %d kali, want 2 (hook:enable tidak boleh di-skip saat retry)", got)
	}
	for _, s := range r.Steps {
		if s.Name == "hook:enable" && s.Status != "succeeded" {
			t.Errorf("hook:enable = %s, want succeeded", s.Status)
		}
	}
}
