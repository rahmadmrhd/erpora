package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

// Kode kegagalan langkah yang tidak berasal dari *Error prasyarat.
const (
	codeHookFailed      = "hook_failed"
	codeHookPanic       = "hook_panic"
	codeHealthFailed    = "health_failed"
	codeMigrationFailed = "migration_failed"
	codeInternal        = "internal_error"
)

// stepFailure adalah kegagalan satu langkah operasi.
type stepFailure struct {
	step, code string
	err        error
	details    map[string]any
}

func (f *stepFailure) Error() string { return f.err.Error() }
func (f *stepFailure) Unwrap() error { return f.err }

// errSkip dikembalikan fungsi langkah yang tidak berlaku (mis. migrate untuk strategy "none").
var errSkip = errors.New("langkah dilewati")

// resume membawa konteks retry: langkah yang sudah succeeded pada operasi asal dilewati (kecuali migrate).
type resume struct {
	skip    map[string]bool
	retryOf uuid.UUID
}

type stepSpec struct {
	name    string
	defCode string
	fn      func(context.Context) error
}

// opRun adalah keadaan satu operasi lifecycle atas satu modul.
type opRun struct {
	s        *Service
	t        tenancy.TenantID
	key      string
	op       Op
	actor    string
	entry    registry.Entry
	cur      store.Installation // installation saat operasi dimulai (read-modify-write)
	from, to runtime.Status
	opID     uuid.UUID
	steps    []store.Step
	rs       *resume
}

func stepOf(steps []store.Step, name string) int {
	for i, st := range steps {
		if st.Name == name {
			return i
		}
	}
	return -1
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// runOne menjalankan satu operasi atas satu modul; dipanggil di dalam advisory lock tenant. State dimuat
// ulang dari store tepat sebelum operasi (read-modify-write), karena Commit menimpa kolom installation.
func (s *Service) runOne(ctx context.Context, req Request, key string, op Op, rs *resume) (Result, error) {
	t := req.Tenant
	rows, err := s.loadRows(ctx, t)
	if err != nil {
		return Result{}, err
	}
	view := View{Reg: s.d.Registry, Rows: rows}
	entry, ok := s.d.Registry.Entry(key)
	if !ok {
		return Result{}, &Error{Code: CodeModuleNotFound, Module: key, Message: "modul tidak ada di katalog"}
	}
	cur, hasRow := rows[key]
	from := view.Stored(key)
	if rs != nil && from == runtime.StatusFailed {
		switch op {
		case OpInstall:
			from = runtime.Status(cur.PreviousStatus)
			if from == "" {
				from = runtime.StatusAvailable
			}
		case OpUpgrade:
			from = runtime.StatusUpgradePending
		}
	}
	to, noop, err := Transition(op, from, hasRow && cur.InstalledVersion == entry.Manifest.Version)
	if err != nil {
		var le *Error
		if errors.As(err, &le) {
			le.Module = key
		}
		return Result{}, err
	}
	if noop {
		rev, err := s.d.Store.Revision(ctx, t)
		if err != nil {
			return Result{}, err
		}
		return Result{Module: key, Op: op, From: from, To: to, Status: "noop", Revision: rev}, nil
	}
	if err := Check(op, key, view); err != nil {
		return Result{}, err
	}
	if op == OpUpgrade {
		to = upgradeTarget(cur)
	}

	started := time.Now()
	opID, err := uuid.NewV7()
	if err != nil {
		return Result{}, err
	}
	ctx, span := s.tel.tracer.Start(ctx, "module."+string(op), trace.WithAttributes(
		attribute.String("erp.module.key", key),
		attribute.String("erp.tenant.id", t.String()),
		attribute.String("erp.module.operation.id", opID.String()),
	))
	defer span.End()

	r := &opRun{s: s, t: t, key: key, op: op, actor: req.Actor, entry: entry, cur: cur, from: from, to: to, opID: opID, rs: rs}
	traceID := ""
	if sc := span.SpanContext(); sc.IsValid() {
		traceID = sc.TraceID().String()
	}
	if err := s.d.Store.StartOperation(ctx, store.Operation{
		ID: opID, TenantID: t, ModuleKey: key, Operation: string(op),
		FromVersion: cur.InstalledVersion, ToVersion: entry.Manifest.Version,
		Status: "running", Actor: req.Actor, TraceID: traceID, StartedAt: started.UTC(),
	}); err != nil {
		return Result{}, err
	}
	if req.OnStarted != nil {
		req.OnStarted(opID)
	}

	fail := r.execute(ctx)
	res, err := r.finish(ctx, fail)
	if err != nil {
		span.RecordError(err)
		return Result{}, err
	}
	s.tel.record(ctx, op, res.Status, time.Since(started))
	if res.Status == "failed" {
		span.SetAttributes(attribute.String("erp.module.error.code", res.Error.Code))
	}
	return res, nil
}

// execute menjalankan langkah-langkah FR-19.3. Mengembalikan kegagalan langkah pertama, atau nil.
func (r *opRun) execute(ctx context.Context) *stepFailure {
	switch r.op {
	case OpInstall:
		return r.runSteps(ctx, []stepSpec{
			{"resolve", codeInternal, r.stepResolve},
			{"compatibility", codeInternal, r.stepCompatibility},
			{"migrate", codeMigrationFailed, r.stepMigrate},
			{"hook:install", codeHookFailed, r.stepHookInstall},
		})
	case OpUpgrade:
		return r.runSteps(ctx, []stepSpec{
			{"resolve", codeInternal, r.stepResolve},
			{"compatibility", codeInternal, r.stepCompatibility},
			{"migrate", codeMigrationFailed, r.stepMigrate},
			{"hook:upgrade", codeHookFailed, r.stepHookUpgrade},
		})
	case OpEnable:
		fail := r.runSteps(ctx, []stepSpec{
			{"dependencies", codeInternal, r.stepPrereq},
			{"hook:enable", codeHookFailed, r.stepHookEnable},
			{"health", codeHealthFailed, r.stepHealth},
		})
		if fail != nil && fail.step == "health" {
			// Kesehatan gagal: kembalikan efek hook:enable; status modul tetap status asal (FR-19.3).
			// Kegagalan revert dicatat pada langkahnya sendiri; kegagalan health tetap yang dilaporkan.
			_ = r.step(ctx, stepSpec{"hook:disable", codeHookFailed, r.stepHookDisable})
		}
		return fail
	case OpDisable:
		return r.runSteps(ctx, []stepSpec{
			{"dependents", codeInternal, r.stepPrereq},
			{"hook:disable", codeHookFailed, r.stepHookDisable},
		})
	case OpUninstall:
		return r.runSteps(ctx, []stepSpec{{"dependents", codeInternal, r.stepPrereq}})
	}
	return &stepFailure{step: "execute", code: codeInternal, err: fmt.Errorf("operasi %q tidak dikenal", r.op)}
}

func (r *opRun) runSteps(ctx context.Context, specs []stepSpec) *stepFailure {
	for _, sp := range specs {
		if f := r.step(ctx, sp); f != nil {
			return f
		}
	}
	return nil
}

// step mencatat langkah ke module_operations.steps sebelum dan sesudah dijalankan, dalam child span.
func (r *opRun) step(ctx context.Context, sp stepSpec) *stepFailure {
	if r.rs != nil && r.rs.skip[sp.name] && sp.name != "migrate" {
		r.steps = append(r.steps, store.Step{Name: sp.name, Status: "skipped", StartedAt: time.Now().UTC()})
		return nil
	}
	if r.s.d.BeforeStep != nil {
		r.s.d.BeforeStep(r.op, r.key, sp.name)
	}
	started := time.Now()
	r.steps = append(r.steps, store.Step{Name: sp.name, Status: "running", StartedAt: started.UTC()})
	idx := len(r.steps) - 1
	if err := r.s.d.Store.SaveSteps(ctx, r.t, r.opID, r.steps); err != nil {
		r.s.log.WarnContext(ctx, "simpan langkah operasi gagal", "step", sp.name, "error", err)
	}

	sctx, span := r.s.tel.tracer.Start(ctx, "module."+string(r.op)+"."+sp.name)
	err := r.invoke(sctx, sp)
	if err != nil && !errors.Is(err, errSkip) {
		span.RecordError(err)
	}
	span.End()

	st := &r.steps[idx]
	st.DurationMS = time.Since(started).Milliseconds()
	var fail *stepFailure
	switch {
	case err == nil:
		st.Status = "succeeded"
	case errors.Is(err, errSkip):
		st.Status = "skipped"
	default:
		st.Status = "failed"
		st.Error = truncate(err.Error(), 500)
		fail = r.classify(sp, err)
	}
	if err := r.s.d.Store.SaveSteps(ctx, r.t, r.opID, r.steps); err != nil {
		r.s.log.WarnContext(ctx, "simpan langkah operasi gagal", "step", sp.name, "error", err)
	}
	if r.s.d.AfterStep != nil {
		r.s.d.AfterStep(r.op, r.key, sp.name)
	}
	return fail
}

func (r *opRun) classify(sp stepSpec, err error) *stepFailure {
	var sf *stepFailure
	if errors.As(err, &sf) {
		out := *sf
		out.step = sp.name
		return &out
	}
	var le *Error
	if errors.As(err, &le) {
		return &stepFailure{step: sp.name, code: le.Code, err: err}
	}
	return &stepFailure{step: sp.name, code: sp.defCode, err: err}
}

// invoke menjalankan fungsi langkah dengan recover: panic hook menjadi kegagalan langkah (stack hanya di log).
func (r *opRun) invoke(ctx context.Context, sp stepSpec) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.s.log.ErrorContext(ctx, "panic pada langkah lifecycle", "step", sp.name, "erp.module.key", r.key,
				"panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			code := codeInternal
			if strings.HasPrefix(sp.name, "hook:") || sp.name == "health" {
				code = codeHookPanic
			}
			err = &stepFailure{code: code, err: fmt.Errorf("panic: %v", p)}
		}
	}()
	return sp.fn(ctx)
}

func (r *opRun) version() string { return r.entry.Manifest.Version }

func (r *opRun) hookEnv(ctx context.Context) (context.Context, module.Env) {
	return runtime.ModuleContext(ctx, r.t, r.key, r.s.d.Tenants, r.s.log, "module.lifecycle:"+string(r.op), r.s.d.Services)
}

func (r *opRun) stepResolve(context.Context) error {
	e, ok := r.s.d.Registry.Entry(r.key)
	if !ok || e.Module == nil {
		return &Error{Code: CodeModuleNotFound, Module: r.key, Message: "modul tidak ada di katalog"}
	}
	return nil
}

func (r *opRun) stepCompatibility(context.Context) error {
	if issues := incompatibleIssues(r.s.d.Registry.Graph(), r.key); len(issues) > 0 {
		return &Error{Code: CodeModuleIncompatible, Module: r.key, Message: issues[0].String(), Issues: issues}
	}
	return nil
}

// stepPrereq memeriksa ulang prasyarat (dependensi/dependen) terhadap status terbaru.
func (r *opRun) stepPrereq(ctx context.Context) error {
	rows, err := r.s.loadRows(ctx, r.t)
	if err != nil {
		return err
	}
	return Check(r.op, r.key, View{Reg: r.s.d.Registry, Rows: rows})
}

func (r *opRun) stepMigrate(ctx context.Context) error {
	if r.entry.Manifest.Migrations.Strategy == "none" {
		return errSkip
	}
	src, ok := r.entry.Module.(module.MigrationSource)
	if !ok {
		return errors.New("modul tidak mengimplementasikan MigrationSource")
	}
	err := r.s.d.Migrator.Apply(ctx, r.t, r.entry.Manifest.Slug, src.Migrations(), func(a migrate.AppliedMigration) error {
		return r.s.d.Store.RecordModuleMigration(ctx, store.ModuleMigration{
			TenantID: r.t, ModuleKey: r.key, ModuleVersion: r.version(), MigrationKey: a.Name,
			Checksum: a.Checksum, ExecutionStatus: "succeeded", OperationID: r.opID,
		})
	})
	var me *migrate.MigrationError
	if errors.As(err, &me) {
		rerr := r.s.d.Store.RecordModuleMigration(ctx, store.ModuleMigration{
			TenantID: r.t, ModuleKey: r.key, ModuleVersion: r.version(), MigrationKey: me.Name,
			Checksum: me.Checksum, ExecutionStatus: "failed", OperationID: r.opID,
		})
		if rerr != nil {
			r.s.log.WarnContext(ctx, "catat migrasi gagal tidak berhasil", "migration", me.Name, "error", rerr)
		}
		return &stepFailure{code: codeMigrationFailed, err: err, details: map[string]any{"migration": me.Name, "version": me.Version}}
	}
	return err
}

func (r *opRun) hook(ctx context.Context, call func(ctx context.Context, env module.Env) error, implemented bool) error {
	if !implemented {
		return errSkip
	}
	hctx, env := r.hookEnv(ctx)
	return call(hctx, env)
}

func (r *opRun) stepHookInstall(ctx context.Context) error {
	h, ok := r.entry.Module.(module.Installer)
	return r.hook(ctx, func(c context.Context, e module.Env) error { return h.Install(c, e) }, ok)
}

func (r *opRun) stepHookUpgrade(ctx context.Context) error {
	h, ok := r.entry.Module.(module.Upgrader)
	return r.hook(ctx, func(c context.Context, e module.Env) error {
		return h.Upgrade(c, e, r.cur.InstalledVersion, r.version())
	}, ok)
}

func (r *opRun) stepHookEnable(ctx context.Context) error {
	h, ok := r.entry.Module.(module.EnableHook)
	return r.hook(ctx, func(c context.Context, e module.Env) error { return h.OnEnable(c, e) }, ok)
}

func (r *opRun) stepHookDisable(ctx context.Context) error {
	h, ok := r.entry.Module.(module.DisableHook)
	return r.hook(ctx, func(c context.Context, e module.Env) error { return h.OnDisable(c, e) }, ok)
}

func (r *opRun) stepHealth(ctx context.Context) error {
	h, ok := r.entry.Module.(module.HealthChecker)
	return r.hook(ctx, func(c context.Context, e module.Env) error { return h.Health(c, e) }, ok)
}

// dataChanging menandai langkah yang mengubah data; kegagalannya membuat modul failed (keputusan 10).
func dataChanging(step string) bool {
	return step == "migrate" || step == "hook:install" || step == "hook:upgrade"
}

var successEvent = map[Op]string{
	OpInstall: "installed", OpEnable: "enabled", OpDisable: "disabled", OpUpgrade: "upgraded", OpUninstall: "uninstalled",
}

// finish menyusun satu Commit atomik (installation + operasi + event + revision), lalu refresh snapshot
// dan publikasi notifikasi. Installation selalu turunan dari r.cur (read-modify-write).
func (r *opRun) finish(ctx context.Context, fail *stepFailure) (Result, error) {
	c := store.Commit{Finish: &store.Finish{OperationID: r.opID, Status: "succeeded"}}
	ev := func(name string, details map[string]any) store.Event {
		return store.Event{ModuleKey: r.key, Event: name, Actor: r.actor, OperationID: r.opID, Details: details}
	}
	base := map[string]any{"operation": string(r.op), "from": string(r.from), "version": r.version()}
	if r.rs != nil && r.rs.retryOf != uuid.Nil {
		c.Events = append(c.Events, ev("retried", map[string]any{"retryOf": r.rs.retryOf.String(), "operation": string(r.op)}))
	}
	res := Result{OperationID: r.opID, Module: r.key, Op: r.op, From: r.from, To: r.to, Status: "succeeded"}

	in := r.cur
	in.TenantID, in.ModuleKey, in.LastOperationID = r.t, r.key, r.opID
	if fail == nil {
		in.Status = string(r.to)
		in.PreviousStatus = ""
		if r.op == OpInstall || r.op == OpUpgrade {
			in.InstalledVersion = r.version()
		}
		base["to"] = string(r.to)
		c.Installation = &in
		c.BumpRevision = true
		c.Events = append(c.Events, ev(successEvent[r.op], base))
	} else {
		opErr := &store.OpError{Code: fail.code, Message: truncate(fail.err.Error(), 500), Step: fail.step, Details: fail.details}
		c.Finish.Status, c.Finish.Error = "failed", opErr
		res.Status, res.Error, res.To = "failed", opErr, r.from
		fd := map[string]any{"operation": string(r.op), "step": fail.step, "code": fail.code, "message": opErr.Message}
		c.Events = append(c.Events, ev("operation_failed", fd))
		if dataChanging(fail.step) {
			in.Status = string(runtime.StatusFailed)
			if r.op == OpInstall {
				in.PreviousStatus = string(r.from)
			} else {
				in.PreviousStatus = r.cur.PreviousStatus
			}
			c.Installation = &in
			c.BumpRevision = true
			res.To = runtime.StatusFailed
		}
	}
	res.Steps = append([]store.Step(nil), r.steps...)

	rev, err := r.s.d.Store.Commit(ctx, r.t, c)
	if err != nil {
		return Result{}, fmt.Errorf("commit operasi %s %s: %w", r.op, r.key, err)
	}
	res.Revision = rev
	if _, err := r.s.Refresh(ctx, r.t); err != nil {
		r.s.log.WarnContext(ctx, "refresh snapshot gagal setelah operasi", "erp.module.key", r.key, "error", err)
	}
	if c.BumpRevision && r.s.d.Notifier != nil {
		r.s.d.Notifier.Publish(ctx, r.t, rev)
	}
	r.s.log.InfoContext(ctx, "operasi lifecycle selesai", "operation", string(r.op), "erp.module.key", r.key,
		"status", res.Status, "from", string(r.from), "to", string(res.To), "actor", r.actor, "operation_id", r.opID.String())
	return res, nil
}
