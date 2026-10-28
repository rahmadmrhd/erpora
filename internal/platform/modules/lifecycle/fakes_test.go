package lifecycle

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
)

var tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

type rowKey struct {
	t   tenancy.TenantID
	key string
}

// fakeStore adalah Store in-memory yang meniru semantik store.Store (termasuk penimpaan kolom
// installation oleh Commit), sehingga bug read-modify-write di engine terdeteksi di unit test.
type fakeStore struct {
	mu        sync.Mutex
	rows      map[rowKey]store.Installation
	ops       map[uuid.UUID]store.Operation
	opOrder   []uuid.UUID
	events    []store.Event
	revision  map[tenancy.TenantID]int64
	commits   []store.Commit
	startOps  []store.Operation
	saveSteps int
	lockErr   error
	migs      []store.ModuleMigration
	lockMu    sync.Mutex
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rows: map[rowKey]store.Installation{}, ops: map[uuid.UUID]store.Operation{},
		revision: map[tenancy.TenantID]int64{},
	}
}

func (f *fakeStore) seed(t tenancy.TenantID, key string, status runtime.Status, version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[rowKey{t, key}] = store.Installation{TenantID: t, ModuleKey: key, Status: string(status), InstalledVersion: version}
}

func (f *fakeStore) row(key string) store.Installation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows[rowKey{tenantA, key}]
}

func (f *fakeStore) WithTenantLock(ctx context.Context, _ tenancy.TenantID, _ time.Duration, fn func(context.Context) error) error {
	if f.lockErr != nil {
		return f.lockErr
	}
	f.lockMu.Lock()
	defer f.lockMu.Unlock()
	return fn(ctx)
}

func (f *fakeStore) Installations(_ context.Context, t tenancy.TenantID) ([]store.Installation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Installation
	for k, r := range f.rows {
		if k.t == t {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b store.Installation) int {
		switch {
		case a.ModuleKey < b.ModuleKey:
			return -1
		case a.ModuleKey > b.ModuleKey:
			return 1
		}
		return 0
	})
	return out, nil
}

func (f *fakeStore) Revision(_ context.Context, t tenancy.TenantID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.revision[t], nil
}

func (f *fakeStore) StartOperation(_ context.Context, op store.Operation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startOps = append(f.startOps, op)
	f.ops[op.ID] = op
	f.opOrder = append(f.opOrder, op.ID)
	if r, ok := f.rows[rowKey{op.TenantID, op.ModuleKey}]; ok {
		r.LastOperationID = op.ID
		f.rows[rowKey{op.TenantID, op.ModuleKey}] = r
	}
	return nil
}

func (f *fakeStore) SaveSteps(_ context.Context, t tenancy.TenantID, id uuid.UUID, steps []store.Step) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok || op.TenantID != t {
		return store.ErrNotFound
	}
	f.saveSteps++
	op.Steps = slices.Clone(steps)
	f.ops[id] = op
	return nil
}

func (f *fakeStore) Commit(_ context.Context, t tenancy.TenantID, c store.Commit) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits = append(f.commits, c)
	if in := c.Installation; in != nil {
		k := rowKey{t, in.ModuleKey}
		cur := f.rows[k]
		cur.TenantID, cur.ModuleKey = t, in.ModuleKey
		cur.InstalledVersion, cur.Status, cur.PreviousStatus = in.InstalledVersion, in.Status, in.PreviousStatus
		if in.LastOperationID != uuid.Nil {
			cur.LastOperationID = in.LastOperationID
		}
		f.rows[k] = cur
	}
	if fin := c.Finish; fin != nil {
		op, ok := f.ops[fin.OperationID]
		if !ok || op.TenantID != t {
			return 0, store.ErrNotFound
		}
		op.Status, op.Error = fin.Status, fin.Error
		now := time.Now()
		op.FinishedAt = &now
		f.ops[fin.OperationID] = op
	}
	for _, ev := range c.Events {
		ev.TenantID = t
		f.events = append(f.events, ev)
	}
	if c.BumpRevision {
		f.revision[t]++
	}
	return f.revision[t], nil
}

func (f *fakeStore) Operation(_ context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok || op.TenantID != t {
		return store.Operation{}, store.ErrNotFound
	}
	return op, nil
}

func (f *fakeStore) Operations(_ context.Context, t tenancy.TenantID, key string, limit int) ([]store.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Operation
	for i := len(f.opOrder) - 1; i >= 0; i-- {
		op := f.ops[f.opOrder[i]]
		if op.TenantID == t && (key == "" || op.ModuleKey == key) {
			out = append(out, op)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeStore) RunningOperations(_ context.Context, t tenancy.TenantID) ([]store.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Operation
	for _, id := range f.opOrder {
		if op := f.ops[id]; op.TenantID == t && op.Status == "running" {
			out = append(out, op)
		}
	}
	return out, nil
}

func (f *fakeStore) Events(_ context.Context, t tenancy.TenantID, key string, _, _ int) ([]store.Event, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Event
	for i := len(f.events) - 1; i >= 0; i-- {
		if ev := f.events[i]; ev.TenantID == t && (key == "" || ev.ModuleKey == key) {
			out = append(out, ev)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) RecordModuleMigration(_ context.Context, m store.ModuleMigration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.migs = append(f.migs, m)
	return nil
}

func (f *fakeStore) eventNames(key string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, ev := range f.events {
		if ev.ModuleKey == key {
			out = append(out, ev.Event)
		}
	}
	return out
}

// fakeMigrator meniru goose: file yang sudah diterapkan per slug tidak dijalankan ulang.
type fakeMigrator struct {
	mu      sync.Mutex
	applied map[string]map[string]bool
	calls   int
	failOn  string // nama file yang gagal
}

func newFakeMigrator() *fakeMigrator { return &fakeMigrator{applied: map[string]map[string]bool{}} }

func (m *fakeMigrator) Apply(_ context.Context, _ tenancy.TenantID, slug string, fsys fs.FS, after func(migrate.AppliedMigration) error) error {
	m.mu.Lock()
	m.calls++
	if m.applied[slug] == nil {
		m.applied[slug] = map[string]bool{}
	}
	done := m.applied[slug]
	failOn := m.failOn
	m.mu.Unlock()
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return err
	}
	for i, e := range entries {
		if done[e.Name()] {
			continue
		}
		if e.Name() == failOn {
			return &migrate.MigrationError{Version: int64(i + 1), Name: e.Name(), Checksum: "sum", Err: errors.New("boom")}
		}
		m.mu.Lock()
		done[e.Name()] = true
		m.mu.Unlock()
		if err := after(migrate.AppliedMigration{Version: int64(i + 1), Name: e.Name(), Checksum: "sum"}); err != nil {
			return err
		}
	}
	return nil
}

func (m *fakeMigrator) Pending(_ context.Context, _ tenancy.TenantID, slug string, fsys fs.FS) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !m.applied[slug][e.Name()] {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func fxMigFS() fs.FS {
	return fstest.MapFS{
		"0001_one.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")},
		"0002_two.sql": {Data: []byte("-- +goose Up\nSELECT 2;\n")},
	}
}

// env menyatukan Service dan semua fake untuk unit test.
type env struct {
	svc      *Service
	st       *fakeStore
	mig      *fakeMigrator
	rec      *fxRec
	snap     *runtime.Snapshot
	notifier *runtime.LocalNotifier
	reg      *registry.Registry
	revs     []int64
}

type envOpts struct {
	tracer trace.TracerProvider
	meter  metric.MeterProvider
}

func newEnv(t *testing.T, rec *fxRec, mods ...*fxModule) *env {
	return newEnvOpts(t, rec, envOpts{}, mods...)
}

func newEnvOpts(t *testing.T, rec *fxRec, o envOpts, mods ...*fxModule) *env {
	t.Helper()
	e := &env{
		st: newFakeStore(), mig: newFakeMigrator(), rec: rec,
		snap: runtime.NewSnapshot(), notifier: runtime.NewLocalNotifier(),
		reg: buildFxRegistry(t, mods...),
	}
	e.notifier.Subscribe(func(_ context.Context, _ tenancy.TenantID, rev int64) { e.revs = append(e.revs, rev) })
	e.svc = New(Deps{
		Registry: e.reg, Store: e.st, Migrator: e.mig,
		Tenants:  database.SingleTenantConnector{Default: tenantA},
		Snapshot: e.snap, Notifier: e.notifier, Tracer: o.tracer, Meter: o.meter,
	})
	return e
}

func (e *env) exec(op Op, key string) ([]Result, error) {
	return e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: op, Key: key, Actor: "cli:tester"})
}
