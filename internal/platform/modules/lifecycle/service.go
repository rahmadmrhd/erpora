package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"slices"
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

// Store adalah persistensi lifecycle yang dibutuhkan engine; dipenuhi *store.Store, fake pada unit test.
type Store interface {
	WithTenantLock(ctx context.Context, t tenancy.TenantID, timeout time.Duration, fn func(context.Context) error) error
	Installations(ctx context.Context, t tenancy.TenantID) ([]store.Installation, error)
	Revision(ctx context.Context, t tenancy.TenantID) (int64, error)
	StartOperation(ctx context.Context, op store.Operation) error
	SaveSteps(ctx context.Context, t tenancy.TenantID, opID uuid.UUID, steps []store.Step) error
	Commit(ctx context.Context, t tenancy.TenantID, c store.Commit) (int64, error)
	Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error)
	Operations(ctx context.Context, t tenancy.TenantID, key string, limit int) ([]store.Operation, error)
	RunningOperations(ctx context.Context, t tenancy.TenantID) ([]store.Operation, error)
	Events(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
	RecordModuleMigration(ctx context.Context, m store.ModuleMigration) error
	// RecordedModuleMigrations mengembalikan migration_key yang sudah tercatat succeeded untuk modul (B-7).
	RecordedModuleMigrations(ctx context.Context, t tenancy.TenantID, key string) (map[string]bool, error)
}

// Migrator menerapkan migrasi modul pada tenant DB.
type Migrator interface {
	Apply(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS, after func(migrate.AppliedMigration) error) error
	Pending(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS) ([]string, error)
	// Applied mengembalikan file migrasi tertanam yang tercatat diterapkan di tabel goose modul (nama, versi, checksum).
	Applied(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS) ([]migrate.AppliedMigration, error)
}

type connMigrator struct{ conn database.TenantConnector }

// NewMigrator mengadaptasi migrate.ApplyModule/PendingModuleMigrations ke tenant DB milik connector.
func NewMigrator(conn database.TenantConnector) Migrator { return connMigrator{conn: conn} }

func (m connMigrator) Apply(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS, after func(migrate.AppliedMigration) error) error {
	db, err := m.conn.For(ctx, t)
	if err != nil {
		return err
	}
	return migrate.ApplyModule(ctx, db.DB, slug, fsys, after)
}

func (m connMigrator) Pending(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS) ([]string, error) {
	db, err := m.conn.For(ctx, t)
	if err != nil {
		return nil, err
	}
	return migrate.PendingModuleMigrations(ctx, db.DB, slug, fsys)
}

func (m connMigrator) Applied(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS) ([]migrate.AppliedMigration, error) {
	db, err := m.conn.For(ctx, t)
	if err != nil {
		return nil, err
	}
	return migrate.AppliedModuleMigrations(ctx, db.DB, slug, fsys)
}

// DefaultLockTimeout adalah batas tunggu advisory lock lifecycle tenant.
const DefaultLockTimeout = 5 * time.Second

// Deps adalah dependensi Service.
type Deps struct {
	Registry *registry.Registry
	Store    Store
	Migrator Migrator
	Tenants  database.TenantConnector
	Snapshot *runtime.Snapshot
	Notifier runtime.StateNotifier
	Tracer   trace.TracerProvider
	Meter    metric.MeterProvider
	Log      *slog.Logger
	// Services dipasang ke context hook modul (masker field, auditor ekspor); nol = tidak terpasang.
	Services runtime.Services
	// LockTimeout 0 berarti DefaultLockTimeout.
	LockTimeout time.Duration
	// AfterStep adalah seam test (mis. simulasi crash); nil di produksi.
	AfterStep func(op Op, key, step string)
	// BeforeStep adalah seam test (mis. memaksa kontensi lock); dipanggil sebelum langkah dicatat. nil di produksi.
	BeforeStep func(op Op, key, step string)
}

// Request adalah permintaan operasi lifecycle.
type Request struct {
	Tenant                tenancy.TenantID
	Op                    Op
	Key                   string
	All, WithDeps, Enable bool
	Actor                 string
	OnStarted             func(opID uuid.UUID)
	// IfMatch adalah nilai header If-Match mentah (RFC 9110); kosong = tanpa prasyarat. Dibandingkan dengan revision tenant
	// di DALAM advisory lock, sebelum simulate (tidak ada celah TOCTOU). Tidak cocok -> *Error version_mismatch.
	IfMatch string
}

// Result adalah hasil satu operasi; Status "succeeded", "failed", atau "noop".
type Result struct {
	OperationID uuid.UUID
	Module      string
	Op          Op
	From, To    runtime.Status
	Status      string
	Steps       []store.Step
	Error       *store.OpError
	Revision    int64
}

// Service adalah engine lifecycle modul.
type Service struct {
	d   Deps
	log *slog.Logger
	tel telemetry
}

// New membuat Service.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{d: d, log: log, tel: newTelemetry(d.Tracer, d.Meter)}
}

func (s *Service) lockTimeout() time.Duration {
	if s.d.LockTimeout > 0 {
		return s.d.LockTimeout
	}
	return DefaultLockTimeout
}

func (s *Service) loadRows(ctx context.Context, t tenancy.TenantID) (map[string]store.Installation, error) {
	list, err := s.d.Store.Installations(ctx, t)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]store.Installation, len(list))
	for _, r := range list {
		rows[r.ModuleKey] = r
	}
	return rows, nil
}

// Refresh memuat status dari store, menghitung status efektif, dan menerapkannya ke snapshot (compare-and-set
// per revision). Revision dibaca lebih dulu daripada baris: isi snapshot tidak pernah lebih lama dari
// revision-nya, dan perubahan berikutnya diterapkan oleh Refresh berikutnya.
func (s *Service) Refresh(ctx context.Context, t tenancy.TenantID) (int64, error) {
	rev, err := s.d.Store.Revision(ctx, t)
	if err != nil {
		return 0, err
	}
	rows, err := s.d.Store.Installations(ctx, t)
	if err != nil {
		return 0, err
	}
	if s.d.Snapshot != nil {
		s.d.Snapshot.Apply(t, Effective(s.d.Registry, rows), rev)
	}
	return rev, nil
}

type item struct {
	key string
	op  Op
}

type plannedStep struct {
	item
	from, to runtime.Status
	noop     bool
}

// expand menyusun daftar (modul, operasi) berurutan: --all melewati platform.core; --with-deps menambah
// dependensi wajib transitif (urutan topologis); disable/uninstall berurutan terbalik; --enable menambah
// rangkaian enable setelah seluruh install (keputusan 16).
func (s *Service) expand(req Request) ([]item, error) {
	reg := s.d.Registry
	var keys []string
	switch {
	case req.All:
		for _, k := range reg.Graph().Order() {
			if k != registry.PlatformKey {
				keys = append(keys, k)
			}
		}
	default:
		e, ok := reg.Entry(req.Key)
		if !ok || req.Key == registry.PlatformKey || e.Manifest.Key == "" {
			return nil, &Error{Code: CodeModuleNotFound, Module: req.Key, Message: "modul tidak ada di katalog"}
		}
		if req.WithDeps && (req.Op == OpInstall || req.Op == OpEnable) {
			for _, d := range reg.Graph().RequiredDependencies(req.Key) {
				if d != registry.PlatformKey {
					keys = append(keys, d)
				}
			}
		}
		keys = append(keys, req.Key)
	}
	if req.Op == OpDisable || req.Op == OpUninstall {
		slices.Reverse(keys)
	}
	items := make([]item, 0, len(keys)*2)
	for _, k := range keys {
		items = append(items, item{k, req.Op})
	}
	if req.Enable && req.Op == OpInstall {
		for _, k := range keys {
			items = append(items, item{k, OpEnable})
		}
	}
	return items, nil
}

// simulate memvalidasi seluruh rencana terhadap salinan status (tanpa menulis). Pada --all, transisi yang
// tidak sah dilewati (modul tidak relevan), sedangkan prasyarat yang gagal menolak seluruh rencana. Hasil
// parsial dikembalikan bersama error pertama.
func (s *Service) simulate(items []item, rows map[string]store.Installation, all bool) ([]plannedStep, item, error) {
	sim := maps.Clone(rows)
	if sim == nil {
		sim = map[string]store.Installation{}
	}
	var out []plannedStep
	for _, it := range items {
		view := View{Reg: s.d.Registry, Rows: sim}
		entry, _ := s.d.Registry.Entry(it.key)
		cur, hasRow := sim[it.key]
		from := view.Stored(it.key)
		to, noop, err := Transition(it.op, from, hasRow && cur.InstalledVersion == entry.Manifest.Version)
		if err != nil {
			var le *Error
			if errors.As(err, &le) {
				le.Module = it.key
			}
			if all {
				continue
			}
			return out, it, err
		}
		if !noop {
			if err := Check(it.op, it.key, view); err != nil {
				return out, it, err
			}
			if it.op == OpUpgrade {
				to = upgradeTarget(cur)
			}
		}
		out = append(out, plannedStep{item: it, from: from, to: to, noop: noop})
		if noop {
			continue
		}
		next := cur
		next.TenantID, next.ModuleKey, next.Status, next.PreviousStatus = cur.TenantID, it.key, string(to), ""
		if it.op == OpInstall || it.op == OpUpgrade {
			next.InstalledVersion = entry.Manifest.Version
		}
		sim[it.key] = next
	}
	return out, item{}, nil
}

// upgradeTarget adalah status setelah upgrade sukses: previous_status (installed bila kosong).
func upgradeTarget(cur store.Installation) runtime.Status {
	if cur.PreviousStatus == "" {
		return runtime.StatusInstalled
	}
	return runtime.Status(cur.PreviousStatus)
}

// Execute menjalankan operasi lifecycle di bawah advisory lock tenant. Prasyarat/lock yang gagal
// dikembalikan sebagai *Error tanpa baris operasi; kegagalan operasi menghasilkan Result.Status "failed"
// dengan error nil dan menghentikan sisa rangkaian.
func (s *Service) Execute(ctx context.Context, req Request) ([]Result, error) {
	switch req.Op {
	case OpInstall, OpEnable, OpDisable, OpUpgrade, OpUninstall:
	default:
		return nil, &Error{Code: CodeInvalidTransition, Message: fmt.Sprintf("operasi %q tidak dikenal", req.Op)}
	}
	items, err := s.expand(req)
	if err != nil {
		s.logRejected(ctx, req, err)
		return nil, err
	}
	var results []Result
	var reconciled bool
	var reconciledRev int64
	err = s.d.Store.WithTenantLock(ctx, req.Tenant, s.lockTimeout(), func(ctx context.Context) error {
		if _, err := s.markInterrupted(ctx, req.Tenant, req.Actor); err != nil {
			return err
		}
		if err := s.checkIfMatch(ctx, req.Tenant, req.IfMatch); err != nil {
			return err
		}
		if req.Op == OpUpgrade {
			// Upgrade dapat dijalankan langsung setelah binary diganti (tanpa serve): reconcile modul terkait dulu di
			// dalam lock yang sama, sehingga versi lama menjadi upgrade_pending dengan previous_status terisi
			// (Q32, 28 Okt 2026; temuan Codex PR #6 C6).
			keys := make(map[string]bool, len(items))
			for _, it := range items {
				keys[it.key] = true
			}
			var rep ReconcileReport
			b, r, rerr := s.reconcileRows(ctx, req.Tenant, keys, &rep)
			if b {
				reconciled, reconciledRev = true, r
			}
			if rerr != nil {
				return rerr
			}
			if !req.All && slices.Contains(rep.Downgraded, req.Key) {
				return &Error{Code: CodeDowngradeNotSupported, Module: req.Key,
					Message: "versi binary lebih lama daripada versi ter-install; downgrade tidak didukung"}
			}
		}
		rows, err := s.loadRows(ctx, req.Tenant)
		if err != nil {
			return err
		}
		planned, _, perr := s.simulate(items, rows, req.All)
		if perr != nil {
			return perr
		}
		for _, p := range planned {
			r, err := s.runOne(ctx, req, p.key, p.op, nil)
			if err != nil {
				return err
			}
			results = append(results, r)
			if r.Status == "failed" {
				break
			}
		}
		return nil
	})
	err = mapLockErr(err, req.Key)
	if reconciled {
		// Reconcile menaikkan revision walau operasi lalu ditolak atau gagal: snapshot dan watcher antarproses harus ikut.
		if _, rerr := s.Refresh(ctx, req.Tenant); rerr != nil && err == nil {
			err = rerr
		}
		if s.d.Notifier != nil {
			s.d.Notifier.Publish(ctx, req.Tenant, reconciledRev)
		}
	}
	if err != nil {
		s.logRejected(ctx, req, err)
	}
	return results, err
}

func (s *Service) logRejected(ctx context.Context, req Request, err error) {
	var le *Error
	if errors.As(err, &le) {
		s.log.WarnContext(ctx, "operasi lifecycle ditolak", "operation", string(req.Op), "erp.module.key", req.Key, "code", le.Code, "message", le.Message, "actor", req.Actor)
	}
}

// mapLockErr mengubah store.ErrLockTimeout menjadi *Error operation_in_progress.
func mapLockErr(err error, key string) error {
	if errors.Is(err, store.ErrLockTimeout) {
		return &Error{Code: CodeOperationInProgress, Module: key, Message: "operasi lifecycle lain sedang berjalan untuk tenant ini"}
	}
	return err
}
