package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/metric/noop"
	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
)

// cliRuntime adalah perakitan minimal untuk perintah CLI: DB, tenant default, connector, dan lifecycle.Service.
type cliRuntime struct {
	cfg      config.Config
	db       *database.DB
	reg      *registry.Registry
	svc      *lifecycle.Service
	tid      tenancy.TenantID
	conn     database.TenantConnector
	cleanup  func()
	services runtime.Services
}

// openCLI membuka DB, memastikan migrasi platform sudah diterapkan, membaca tenant default, dan membangun
// lifecycle.Service tanpa snapshot (CLI hanya menulis status). cleanup wajib dipanggil.
func openCLI(ctx context.Context, cfg config.Config, log *slog.Logger) (*cliRuntime, error) {
	reg, err := BuildRegistry()
	if err != nil {
		return nil, &ExitError{Code: 2, Err: fmt.Errorf("katalog modul tidak valid: %w", err)}
	}
	db, err := database.Open(ctx, cfg, tnoop.NewTracerProvider(), log)
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}
	pending, err := migrate.Pending(ctx, db)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("periksa migrasi: %w", err)
	}
	if pending > 0 {
		cleanup()
		return nil, errors.New("migrasi platform belum diterapkan: jalankan erpora migrate up")
	}
	tid, err := defaultTenant(ctx, cfg, db)
	if err != nil {
		cleanup()
		return nil, err
	}
	prov := observability.Providers{Tracer: tnoop.NewTracerProvider(), Meter: noop.NewMeterProvider()}
	services := platformServices(cfg, log)
	return &cliRuntime{
		cfg:      cfg,
		db:       db,
		reg:      reg,
		svc:      buildLifecycle(reg, db, tid, nil, nil, prov, log, services),
		tid:      tid,
		conn:     database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}},
		cleanup:  cleanup,
		services: services,
	}, nil
}

// guardWrite menolak command tulis binary dev terhadap DB berpenanda production (FR-22.5).
func (r *cliRuntime) guardWrite(ctx context.Context) error { return GuardDevWrite(ctx, r.db, r.cfg) }

// NewGuardedLifecycle seperti NewLifecycle, ditambah guard yang dipanggil CLI sebelum mutasi modul (FR-22.5).
func NewGuardedLifecycle(ctx context.Context, cfg config.Config, log *slog.Logger) (*lifecycle.Service, tenancy.TenantID, func(context.Context) error, func(), error) {
	rt, err := openCLI(ctx, cfg, log)
	if err != nil {
		return nil, tenancy.TenantID{}, nil, nil, err
	}
	return rt.svc, rt.tid, rt.guardWrite, rt.cleanup, nil
}

// NewLifecycle dipakai CLI `erpora modules`: lihat openCLI. cleanup wajib dipanggil.
func NewLifecycle(ctx context.Context, cfg config.Config, log *slog.Logger) (*lifecycle.Service, tenancy.TenantID, func(), error) {
	rt, err := openCLI(ctx, cfg, log)
	if err != nil {
		return nil, tenancy.TenantID{}, nil, err
	}
	return rt.svc, rt.tid, rt.cleanup, nil
}

// EnsureRequiredModules meng-install dan meng-enable modul required (beserta dependensinya) dengan actor
// system:migrate. Dipanggil `migrate up` setelah migrasi platform (keputusan 17).
func EnsureRequiredModules(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	svc, tid, cleanup, err := NewLifecycle(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := svc.EnsureRequired(ctx, tid, "system:migrate")
	for _, r := range res {
		if r.Status == "failed" {
			return fmt.Errorf("modul required %s gagal pada operasi %s: %s", r.Module, r.Op, r.Error.Message)
		}
	}
	if err != nil {
		return fmt.Errorf("modul required: %w", err)
	}
	return nil
}
