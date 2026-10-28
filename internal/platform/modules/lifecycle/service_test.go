package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/module"
)

func stepNames(r Result) []string {
	var out []string
	for _, s := range r.Steps {
		out = append(out, s.Name)
	}
	return out
}

func allSucceeded(r Result) bool {
	for _, s := range r.Steps {
		if s.Status != "succeeded" {
			return false
		}
	}
	return true
}

func TestExecute_EnableStepsAndHooksOrder(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	e := newEnv(t, rec, a)
	e.st.seed(tenantA, "acme.a", runtime.StatusInstalled, "1.0.0")

	res, err := e.exec(OpEnable, "acme.a")
	if err != nil || len(res) != 1 {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	r := res[0]
	if r.Status != "succeeded" || r.From != runtime.StatusInstalled || r.To != runtime.StatusEnabled {
		t.Errorf("result = %+v", r)
	}
	if got := rec.list(); !slices.Equal(got, []string{"acme.a:OnEnable", "acme.a:Health"}) {
		t.Errorf("hooks = %v", got)
	}
	if got := stepNames(r); !slices.Equal(got, []string{"dependencies", "hook:enable", "health"}) || !allSucceeded(r) {
		t.Errorf("steps = %+v", r.Steps)
	}
	if e.st.saveSteps != 2*3 {
		t.Errorf("SaveSteps = %d, want 6", e.st.saveSteps)
	}
	if n := len(e.st.commits); n != 1 || !e.st.commits[0].BumpRevision {
		t.Errorf("commits = %+v", e.st.commits)
	}
	if e.snap.Status(tenantA, "acme.a") != runtime.StatusEnabled {
		t.Error("snapshot belum enabled")
	}
	if !slices.Equal(e.revs, []int64{1}) || r.Revision != 1 {
		t.Errorf("notifier = %v, revision = %d", e.revs, r.Revision)
	}
}

func TestExecute_HealthFailRevertsToOrigin(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	a.on("Health", func(context.Context, module.Env) error { return errors.New("sakit") })
	e := newEnv(t, rec, a)
	e.st.seed(tenantA, "acme.a", runtime.StatusDisabled, "1.0.0")

	res, err := e.exec(OpEnable, "acme.a")
	if err != nil || len(res) != 1 {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	r := res[0]
	if r.Status != "failed" || r.Error == nil || r.Error.Step != "health" {
		t.Fatalf("result = %+v", r)
	}
	if got := rec.list(); !slices.Equal(got, []string{"acme.a:OnEnable", "acme.a:Health", "acme.a:OnDisable"}) {
		t.Errorf("hooks = %v", got)
	}
	if row := e.st.row("acme.a"); row.Status != "disabled" || row.InstalledVersion != "1.0.0" {
		t.Errorf("row = %+v", row)
	}
	if c := e.st.commits[len(e.st.commits)-1]; !c.BumpRevision {
		t.Error("revision harus naik pada setiap commit operasi, termasuk health gagal tanpa perubahan status (Q32 B-4)")
	}
	if !slices.Contains(e.st.eventNames("acme.a"), "operation_failed") {
		t.Errorf("events = %v", e.st.eventNames("acme.a"))
	}
	if e.snap.Status(tenantA, "acme.a") == runtime.StatusEnabled {
		t.Error("snapshot tidak boleh enabled")
	}
}

func TestExecute_EnableIdempotent(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")

	res, err := e.exec(OpEnable, "acme.a")
	if err != nil || len(res) != 1 || res[0].Status != "noop" {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	if len(e.st.startOps) != 0 || len(e.st.commits) != 0 || len(e.st.events) != 0 || len(rec.list()) != 0 {
		t.Errorf("no-op menulis: ops=%d commits=%d events=%d hooks=%v", len(e.st.startOps), len(e.st.commits), len(e.st.events), rec.list())
	}
}

func TestExecute_PrerequisiteRejectedHasNoOperation(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}), newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}}))
	e.st.seed(tenantA, "acme.a", runtime.StatusDisabled, "1.0.0")
	e.st.seed(tenantA, "acme.b", runtime.StatusInstalled, "1.0.0")

	res, err := e.exec(OpEnable, "acme.b")
	le := wantCode(t, err, CodeDependencyNotReady)
	if !slices.Equal(le.Related, []string{"acme.a"}) || len(res) != 0 {
		t.Errorf("err = %+v res = %+v", le, res)
	}
	if len(e.st.startOps) != 0 {
		t.Error("StartOperation tidak boleh dipanggil")
	}
}

func TestExecute_HookPanicBecomesStepFailure(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	a.on("Install", func(context.Context, module.Env) error { panic("meledak") })
	e := newEnv(t, rec, a)

	res, err := e.exec(OpInstall, "acme.a")
	if err != nil || len(res) != 1 {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	if r := res[0]; r.Status != "failed" || r.Error == nil || r.Error.Code != "hook_panic" || r.Error.Step != "hook:install" {
		t.Fatalf("result = %+v (%+v)", r, r.Error)
	}
	if row := e.st.row("acme.a"); row.Status != "failed" || row.PreviousStatus != "available" {
		t.Errorf("row = %+v", row)
	}
}

func TestExecute_HooksGetModuleContext(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	e := newEnv(t, rec, a)
	if _, err := e.exec(OpInstall, "acme.a"); err != nil {
		t.Fatal(err)
	}
	ctx, env := a.seen("Install")
	if r, ok := authz.SystemReason(ctx); !ok || r != "module.lifecycle:install" {
		t.Errorf("system reason = %q, %v", r, ok)
	}
	if id, ok := tenancy.FromContext(ctx); !ok || id != tenantA {
		t.Errorf("tenant = %v, %v", id, ok)
	}
	if env.Logger == nil || env.TenantID != tenantA.String() {
		t.Errorf("env = %+v", env)
	}
}

func TestExecute_InstallWithDepsEnableOrder(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}), newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}}))
	res, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, Key: "acme.b", WithDeps: true, Enable: true, Actor: "cli:tester"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range res {
		got = append(got, string(r.Op)+" "+r.Module)
		if r.Status != "succeeded" {
			t.Errorf("%s %s = %s", r.Op, r.Module, r.Status)
		}
	}
	want := []string{"install acme.a", "install acme.b", "enable acme.a", "enable acme.b"}
	if !slices.Equal(got, want) {
		t.Fatalf("urutan = %v, want %v", got, want)
	}
	if e.snap.Status(tenantA, "acme.b") != runtime.StatusEnabled {
		t.Error("b belum enabled")
	}
}

func TestExecute_AllSkipsPlatformCoreAndPlansFirst(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}), newFx(rec, fxSpec{key: "acme.bad", platformC: "^9.0"}))
	res, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, All: true, Actor: "cli:tester"})
	wantCode(t, err, CodeModuleIncompatible)
	if len(res) != 0 || len(e.st.startOps) != 0 {
		t.Errorf("rencana ditolak seluruhnya: res=%+v ops=%d", res, len(e.st.startOps))
	}

	// Tanpa modul bermasalah: platform.core tidak ikut dan semua ter-install.
	rec2 := &fxRec{}
	e2 := newEnv(t, rec2, newFx(rec2, fxSpec{key: "acme.a"}), newFx(rec2, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}}))
	res, err = e2.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, All: true, Actor: "cli:tester"})
	if err != nil || len(res) != 2 {
		t.Fatalf("Execute --all = %+v, %v", res, err)
	}
	for _, r := range res {
		if r.Module == "platform.core" {
			t.Error("platform.core tidak boleh ikut")
		}
	}
}

// Catatan review 1: Commit menimpa installed_version/previous_status, sehingga engine harus selalu
// memuat installation saat ini lalu memodifikasinya (read-modify-write di bawah lock).
func TestExecute_InstalledVersionSurvivesEnableDisableSequence(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a", version: "1.2.3"}))
	for _, op := range []Op{OpInstall, OpEnable, OpDisable, OpEnable, OpDisable, OpUninstall} {
		res, err := e.exec(op, "acme.a")
		if err != nil || res[0].Status != "succeeded" {
			t.Fatalf("%s = %+v, %v", op, res, err)
		}
		row := e.st.row("acme.a")
		if row.InstalledVersion != "1.2.3" {
			t.Fatalf("setelah %s installed_version = %q, want 1.2.3", op, row.InstalledVersion)
		}
		if row.PreviousStatus != "" {
			t.Errorf("setelah %s previous_status = %q", op, row.PreviousStatus)
		}
	}
	if row := e.st.row("acme.a"); row.Status != "uninstalled" {
		t.Errorf("status akhir = %q", row.Status)
	}
}

func TestExecute_Telemetry(t *testing.T) {
	rec := &fxRec{}
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	e := newEnvOpts(t, rec, envOpts{tracer: tp, meter: mp}, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusInstalled, "1.0.0")

	if _, err := e.exec(OpEnable, "acme.a"); err != nil {
		t.Fatal(err)
	}
	spans := map[string]sdktrace.ReadOnlySpan{}
	for _, s := range sr.Ended() {
		spans[s.Name()] = s
	}
	root, ok := spans["module.enable"]
	if !ok {
		t.Fatalf("span module.enable tidak ada: %v", spans)
	}
	for _, child := range []string{"module.enable.hook:enable", "module.enable.health"} {
		s, ok := spans[child]
		if !ok {
			t.Errorf("span %s tidak ada", child)
			continue
		}
		if s.Parent().SpanID() != root.SpanContext().SpanID() {
			t.Errorf("%s bukan anak module.enable", child)
		}
	}
	attrs := map[string]string{}
	for _, a := range root.Attributes() {
		attrs[string(a.Key)] = a.Value.String()
	}
	if attrs["erp.module.key"] != "acme.a" || attrs["erp.tenant.id"] != tenantA.String() || attrs["erp.module.operation.id"] == "" {
		t.Errorf("atribut = %v", attrs)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var count int64
	hasHist := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				if m.Name != "erp.module.operations" {
					continue
				}
				for _, dp := range d.DataPoints {
					op, _ := dp.Attributes.Value("operation")
					res, _ := dp.Attributes.Value("result")
					if op.AsString() == "enable" && res.AsString() == "succeeded" {
						count += dp.Value
					}
				}
			case metricdata.Histogram[float64]:
				if m.Name == "erp.module.operation.duration" && len(d.DataPoints) > 0 {
					hasHist = true
				}
			}
		}
	}
	if count != 1 || !hasHist {
		t.Errorf("counter = %d, histogram = %v", count, hasHist)
	}
}
