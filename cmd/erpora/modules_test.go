package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/resolver"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

var cliTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

// fakeBackend memenuhi modulesBackend dengan data kanonik dan merekam permintaan.
type fakeBackend struct {
	mods        []lifecycle.ModuleState
	rev         int64
	execReqs    []lifecycle.Request
	execResults []lifecycle.Result
	execErr     error
	plan        lifecycle.Plan
	planErr     error
	retryID     uuid.UUID
	retryRes    lifecycle.Result
	retryErr    error
	op          store.Operation
	opErr       error
	events      []store.Event
}

var (
	fixedTime = time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	exampleOp = uuid.MustParse("0190a1b2-0000-7000-8000-000000000001")
)

func canonicalModules() []lifecycle.ModuleState {
	lastOp := store.Operation{
		ID: exampleOp, ModuleKey: "acme.example", Operation: "enable", Status: "succeeded", Actor: "cli:tester",
		StartedAt: fixedTime, Steps: []store.Step{
			{Name: "dependencies", Status: "succeeded", StartedAt: fixedTime, DurationMS: 2},
			{Name: "hook:enable", Status: "succeeded", StartedAt: fixedTime, DurationMS: 15},
			{Name: "health", Status: "failed", StartedAt: fixedTime, DurationMS: 30, Error: "tabel hilang"},
		},
	}
	return []lifecycle.ModuleState{
		{
			Key: "acme.example", Slug: "example", Name: "Example", BinaryVersion: "1.0.0", InstalledVersion: "1.0.0",
			Status: runtime.StatusEnabled, Stored: "enabled", Compatible: true,
			Dependencies: []module.Dependency{{Module: "platform.core", Version: "^0.1"}},
			Dependents:   []string{"acme.example-addon"}, LastOperation: &lastOp,
		},
		{
			Key: "acme.example-addon", Slug: "example-addon", Name: "Example Addon", BinaryVersion: "1.0.0",
			Status: runtime.StatusAvailable, Stored: "available", Compatible: true,
			Dependencies: []module.Dependency{{Module: "platform.core", Version: "^0.1"}, {Module: "acme.example", Version: "^1.0"}},
		},
	}
}

func newFake() *fakeBackend { return &fakeBackend{mods: canonicalModules(), rev: 3} }

func (f *fakeBackend) Modules(context.Context, tenancy.TenantID) ([]lifecycle.ModuleState, int64, error) {
	return f.mods, f.rev, nil
}

func (f *fakeBackend) Module(_ context.Context, _ tenancy.TenantID, key string) (lifecycle.ModuleState, int64, error) {
	for _, m := range f.mods {
		if m.Key == key {
			return m, f.rev, nil
		}
	}
	return lifecycle.ModuleState{}, 0, &lifecycle.Error{Code: lifecycle.CodeModuleNotFound, Module: key, Message: "modul tidak dikenal"}
}

func (f *fakeBackend) History(context.Context, tenancy.TenantID, string, int, int) ([]store.Event, int64, error) {
	return f.events, int64(len(f.events)), nil
}

func (f *fakeBackend) Operation(context.Context, tenancy.TenantID, uuid.UUID) (store.Operation, error) {
	return f.op, f.opErr
}

func (f *fakeBackend) Execute(_ context.Context, req lifecycle.Request) ([]lifecycle.Result, error) {
	f.execReqs = append(f.execReqs, req)
	return f.execResults, f.execErr
}

func (f *fakeBackend) Retry(_ context.Context, _ tenancy.TenantID, id uuid.UUID, _ string) (lifecycle.Result, error) {
	f.retryID = id
	return f.retryRes, f.retryErr
}

func (f *fakeBackend) Plan(context.Context, lifecycle.Request) (lifecycle.Plan, error) {
	return f.plan, f.planErr
}

// runModules menjalankan `erpora modules ...` dengan backend fake. stdin dipakai untuk konfirmasi.
func runModules(t *testing.T, f *fakeBackend, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "erpora", SilenceUsage: true, SilenceErrors: true}
	root.AddGroup(&cobra.Group{ID: groupRuntime, Title: "Runtime"})
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(stdin))
	opened := 0
	root.AddCommand(newModulesCmd(&rootOpts{}, func(context.Context, *rootOpts, io.Writer) (modulesBackend, tenancy.TenantID, func(), error) {
		opened++
		return f, cliTenant, func() {}, nil
	}))
	root.SetArgs(append([]string{"modules"}, args...))
	err = root.Execute()
	_ = opened
	return out.String(), errOut.String(), err
}

func TestModules_ListGolden(t *testing.T) {
	out, _, err := runModules(t, newFake(), "", "list")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "modules_list.golden", out)
	js, _, err := runModules(t, newFake(), "", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "modules_list_json.golden", js)
}

func TestModules_StatusGolden(t *testing.T) {
	out, _, err := runModules(t, newFake(), "", "status", "acme.example")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "modules_status.golden", out)
}

func TestModules_PlanInstallGolden(t *testing.T) {
	f := newFake()
	f.plan = lifecycle.Plan{Ops: []lifecycle.PlannedOp{
		{Module: "acme.example", Op: lifecycle.OpInstall, From: runtime.StatusEnabled, To: runtime.StatusEnabled, NoOp: true},
		{
			Module: "acme.example-addon", Op: lifecycle.OpInstall, From: runtime.StatusAvailable, To: runtime.StatusInstalled,
			Steps: []string{"resolve", "compatibility", "migrate", "hook:install"}, PendingMigrations: []string{"0001_create.sql"},
		},
	}}
	out, _, err := runModules(t, f, "", "plan", "install", "acme.example-addon", "--with-deps")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "modules_plan_install.golden", out)
}

func TestModules_PlanDisableBlockedGolden(t *testing.T) {
	f := newFake()
	f.plan = lifecycle.Plan{Ops: []lifecycle.PlannedOp{{
		Module: "acme.example", Op: lifecycle.OpDisable,
		Blocked: &lifecycle.Error{Code: lifecycle.CodeDependentsActive, Module: "acme.example", Message: "modul yang bergantung masih enabled: acme.example-addon", Related: []string{"acme.example-addon"}},
	}}}
	out, _, err := runModules(t, f, "", "plan", "disable", "acme.example")
	if exitCode(err) != 3 {
		t.Fatalf("exit = %d (%v), want 3", exitCode(err), err)
	}
	golden(t, "modules_plan_disable_blocked.golden", out)
}

func TestModules_PlanAllOnlyForInstallEnableUpgrade(t *testing.T) {
	_, _, err := runModules(t, newFake(), "", "plan", "disable", "--all")
	if exitCode(err) != 2 {
		t.Fatalf("exit = %d (%v), want 2", exitCode(err), err)
	}
	f := newFake()
	if _, _, err := runModules(t, f, "", "plan", "install", "--all"); err != nil {
		t.Fatal(err)
	}
}

func TestModules_ExitCodes(t *testing.T) {
	mk := func(code string) error { return &lifecycle.Error{Code: code, Module: "acme.example", Message: "x"} }
	cases := []struct {
		name string
		prep func(*fakeBackend)
		args []string
		want int
	}{
		{"unknown key", nil, []string{"status", "acme.nope"}, 2},
		{"retry bukan uuid", nil, []string{"retry", "not-a-uuid"}, 2},
		{"tenant lain", nil, []string{"list", "--tenant", uuid.NewString()}, 2},
		{"tenant default ok", nil, []string{"list", "--tenant", cliTenant.String()}, 0},
		{"tanpa argumen key", nil, []string{"enable"}, 2},
		{"flag tidak dikenal", nil, []string{"enable", "acme.example", "--bogus"}, 2},
		{"invalid_transition", func(f *fakeBackend) { f.execErr = mk(lifecycle.CodeInvalidTransition) }, []string{"enable", "acme.example"}, 3},
		{"dependents_active", func(f *fakeBackend) { f.execErr = mk(lifecycle.CodeDependentsActive) }, []string{"disable", "acme.example"}, 3},
		{"module_required", func(f *fakeBackend) { f.execErr = mk(lifecycle.CodeModuleRequired) }, []string{"disable", "acme.example"}, 3},
		{"operation_in_progress", func(f *fakeBackend) { f.execErr = mk(lifecycle.CodeOperationInProgress) }, []string{"enable", "acme.example"}, 3},
		{"module_incompatible", func(f *fakeBackend) {
			f.execErr = &lifecycle.Error{Code: lifecycle.CodeModuleIncompatible, Issues: []resolver.Issue{{Code: resolver.CodePlatformIncompatible, Module: "acme.example"}}}
		}, []string{"install", "acme.example"}, 3},
		{"module_not_found dari backend", func(f *fakeBackend) { f.execErr = mk(lifecycle.CodeModuleNotFound) }, []string{"enable", "acme.example"}, 2},
		{"operasi gagal", func(f *fakeBackend) {
			f.execResults = []lifecycle.Result{{
				Module: "acme.example", Op: lifecycle.OpEnable, From: runtime.StatusInstalled, To: runtime.StatusInstalled,
				Status: "failed", OperationID: exampleOp, Error: &store.OpError{Code: "health_failed", Message: "sakit", Step: "health"},
			}}
		}, []string{"enable", "acme.example"}, 4},
		{"no-op", func(f *fakeBackend) {
			f.execResults = []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpEnable, From: runtime.StatusEnabled, To: runtime.StatusEnabled, Status: "noop"}}
		}, []string{"enable", "acme.example"}, 0},
		{"sukses", func(f *fakeBackend) {
			f.execResults = []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpEnable, From: runtime.StatusInstalled, To: runtime.StatusEnabled, Status: "succeeded", OperationID: exampleOp}}
		}, []string{"enable", "acme.example"}, 0},
		{"retry operasi tidak ada", func(f *fakeBackend) { f.retryErr = &lifecycle.Error{Code: lifecycle.CodeOperationNotFound} }, []string{"retry", exampleOp.String()}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			if tc.prep != nil {
				tc.prep(f)
			}
			_, _, err := runModules(t, f, "", tc.args...)
			if got := exitCode(err); got != tc.want {
				t.Fatalf("exit = %d (%v), want %d", got, err, tc.want)
			}
		})
	}
}

func TestModules_NoopPrintsMessage(t *testing.T) {
	f := newFake()
	f.execResults = []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpEnable, From: runtime.StatusEnabled, To: runtime.StatusEnabled, Status: "noop"}}
	out, _, err := runModules(t, f, "", "enable", "acme.example")
	if err != nil || !strings.Contains(out, "tidak ada perubahan") {
		t.Fatalf("out = %q err = %v", out, err)
	}
}

func TestModules_ResultLineFormat(t *testing.T) {
	f := newFake()
	f.execResults = []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpEnable, From: runtime.StatusInstalled, To: runtime.StatusEnabled, Status: "succeeded", OperationID: exampleOp}}
	out, _, err := runModules(t, f, "", "enable", "acme.example")
	if err != nil {
		t.Fatal(err)
	}
	want := "acme.example  enable  installed → enabled  succeeded  op=" + exampleOp.String()
	if !strings.Contains(out, want) {
		t.Errorf("out = %q, want substring %q", out, want)
	}
	if len(f.execReqs) != 1 || f.execReqs[0].Actor != cliActor() || !strings.HasPrefix(f.execReqs[0].Actor, "cli:") ||
		f.execReqs[0].Tenant != cliTenant || f.execReqs[0].Op != lifecycle.OpEnable || f.execReqs[0].Key != "acme.example" {
		t.Errorf("request = %+v", f.execReqs)
	}
}

func TestModules_UninstallConfirmation(t *testing.T) {
	ok := []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpUninstall, From: runtime.StatusDisabled, To: runtime.StatusUninstalled, Status: "succeeded", OperationID: exampleOp}}

	f := newFake()
	f.execResults = ok
	_, _, err := runModules(t, f, "", "uninstall", "acme.example")
	if exitCode(err) != 2 || len(f.execReqs) != 0 {
		t.Fatalf("stdin kosong: exit = %d (%v), exec = %d", exitCode(err), err, len(f.execReqs))
	}
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("pesan harus menyebut --yes: %v", err)
	}

	f = newFake()
	f.execResults = ok
	if _, _, err := runModules(t, f, "y\n", "uninstall", "acme.example"); err != nil || len(f.execReqs) != 1 {
		t.Fatalf("stdin y: err = %v, exec = %d", err, len(f.execReqs))
	}

	f = newFake()
	f.execResults = ok
	out, _, err := runModules(t, f, "n\n", "uninstall", "acme.example")
	if err != nil || len(f.execReqs) != 0 || !strings.Contains(out, "dibatalkan") {
		t.Fatalf("stdin n: err = %v, exec = %d, out = %q", err, len(f.execReqs), out)
	}

	f = newFake()
	f.execResults = ok
	if _, _, err := runModules(t, f, "", "uninstall", "acme.example", "--yes"); err != nil || len(f.execReqs) != 1 {
		t.Fatalf("--yes: err = %v, exec = %d", err, len(f.execReqs))
	}
}

func TestModules_InstallAllEnableRequest(t *testing.T) {
	f := newFake()
	if _, _, err := runModules(t, f, "", "install", "--all", "--enable"); err != nil {
		t.Fatal(err)
	}
	if len(f.execReqs) != 1 {
		t.Fatalf("requests = %+v", f.execReqs)
	}
	r := f.execReqs[0]
	if !r.All || !r.Enable || r.Op != lifecycle.OpInstall || !strings.HasPrefix(r.Actor, "cli:") {
		t.Errorf("request = %+v", r)
	}
	f = newFake()
	if _, _, err := runModules(t, f, "", "install", "acme.example", "--with-deps"); err != nil {
		t.Fatal(err)
	}
	if r := f.execReqs[0]; r.All || !r.WithDeps || r.Key != "acme.example" {
		t.Errorf("request = %+v", r)
	}
	if _, _, err := runModules(t, newFake(), "", "install", "acme.example", "--all"); exitCode(err) != 2 {
		t.Errorf("key + --all harus exit 2, got %d", exitCode(err))
	}
}

// Perilaku --all didokumentasikan di help (transisi tidak sah dilewati; prasyarat gagal menolak seluruh rencana).
func TestModules_AllHelpDocumentsBehavior(t *testing.T) {
	for _, sub := range []string{"install", "enable", "upgrade"} {
		out, _, err := runModules(t, newFake(), "", sub, "--help")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"dilewati", "menolak seluruh rencana"} {
			if !strings.Contains(out, want) {
				t.Errorf("help %s tanpa %q:\n%s", sub, want, out)
			}
		}
	}
}

func TestModules_RetryUsesOperationID(t *testing.T) {
	f := newFake()
	f.retryRes = lifecycle.Result{Module: "acme.example", Op: lifecycle.OpInstall, From: runtime.StatusAvailable, To: runtime.StatusInstalled, Status: "succeeded", OperationID: exampleOp}
	if _, _, err := runModules(t, f, "", "retry", exampleOp.String()); err != nil || f.retryID != exampleOp {
		t.Fatalf("err = %v, id = %s", err, f.retryID)
	}
}

func TestModules_HistoryJSONAndPaging(t *testing.T) {
	f := newFake()
	f.events = []store.Event{{ID: exampleOp, ModuleKey: "acme.example", Event: "enabled", Actor: "cli:tester", OperationID: exampleOp, OccurredAt: fixedTime}}
	out, _, err := runModules(t, f, "", "history", "acme.example")
	if err != nil || !strings.Contains(out, "enabled") || !strings.Contains(out, "cli:tester") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	js, _, err := runModules(t, f, "", "history", "acme.example", "--json", "--page", "1", "--page-size", "5")
	if err != nil || !strings.Contains(js, `"event": "enabled"`) {
		t.Fatalf("json = %q, err = %v", js, err)
	}
	if _, _, err := runModules(t, f, "", "history", "acme.example", "--page", "0"); exitCode(err) != 2 {
		t.Errorf("page 0 harus exit 2, got %d", exitCode(err))
	}
	_ = errors.New
}

func TestModules_InRuntimeGroup(t *testing.T) {
	out, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "modules") {
		t.Errorf("modules tidak ada di list:\n%s", out)
	}
}

func TestCliActor_HasPrefix(t *testing.T) {
	if a := cliActor(); !strings.HasPrefix(a, "cli:") || len(a) <= len("cli:") {
		t.Errorf("actor = %q", a)
	}
}
