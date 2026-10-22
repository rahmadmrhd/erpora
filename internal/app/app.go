// Package app adalah composition root: merakit platform (dan nanti modul) menjadi proses `erp serve`.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/rahmadmrhd/erp-go/internal/platform/buildinfo"
	"github.com/rahmadmrhd/erp-go/internal/platform/config"
	"github.com/rahmadmrhd/erp-go/internal/platform/database"
	"github.com/rahmadmrhd/erp-go/internal/platform/httpserver"
	"github.com/rahmadmrhd/erp-go/internal/platform/migrate"
	"github.com/rahmadmrhd/erp-go/internal/platform/observability"
	"github.com/rahmadmrhd/erp-go/internal/platform/tenancy"
	"github.com/rahmadmrhd/erp-go/web"
)

const (
	shutdownTimeout  = 25 * time.Second
	readyPingTimeout = 2 * time.Second
)

// ExitError membawa kode keluar proses.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Run menjalankan server sampai ctx dibatalkan. serve tidak pernah menulis platform_state.
func Run(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	log := observability.NewLogger(stdout, cfg.LogFormat, slog.LevelInfo)

	prov, shutdownOTel, err := observability.Setup(ctx, observability.Options{
		ServiceVersion: buildinfo.Version, Environment: string(cfg.Env),
	})
	if err != nil {
		return fmt.Errorf("setup observability: %w", err)
	}
	defer func() {
		fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownOTel(fctx); err != nil {
			log.Warn("flush OpenTelemetry gagal", "error", err)
		}
	}()

	db, err := database.Open(ctx, cfg, prov.Tracer, log)
	if err != nil {
		return err
	}
	defer func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}()

	pending, err := migrate.Pending(ctx, db)
	if err != nil {
		return fmt.Errorf("periksa migrasi: %w", err)
	}
	if pending > 0 {
		return errors.New("migrasi platform belum diterapkan: jalankan erp migrate up")
	}

	tenantID, err := migrate.DefaultTenantID(ctx, db)
	if err != nil {
		return err
	}
	if cfg.DefaultTenantID != nil && *cfg.DefaultTenantID != tenantID {
		return fmt.Errorf("ERP_DEFAULT_TENANT_ID (%s) berbeda dengan tenant default di database (%s)", *cfg.DefaultTenantID, tenantID)
	}
	tid := tenancy.TenantID(tenantID)

	router := httpserver.NewRouter(httpserver.Deps{
		Logger:     log,
		Tracer:     prov.Tracer,
		Resolver:   tenancy.SingleTenantResolver{ID: tid},
		Connector:  database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}},
		Readiness:  dbReadiness{db: db},
		Assets:     web.Dist(),
		AppVersion: buildinfo.Version,
	})
	registerTestRoutes(router)
	srv := httpserver.New(cfg.HTTPAddr, router)

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("tidak dapat listen di %s: %w", cfg.HTTPAddr, err)
	}
	log.Info("server berjalan", "addr", ln.Addr().String(), "env", string(cfg.Env), "version", buildinfo.Version)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server berhenti: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("menghentikan server", "timeout", shutdownTimeout.String())
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutdown tidak tuntas: %w", err)
	}
	return nil
}

// dbReadiness: ping DB (2 dtk) + semua migrasi sudah diterapkan. Read-only.
type dbReadiness struct{ db *database.DB }

func (r dbReadiness) Ready(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, readyPingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	qctx, cancel2 := context.WithTimeout(ctx, readyPingTimeout)
	defer cancel2()
	n, err := migrate.Pending(qctx, r.db)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%d migrasi belum diterapkan", n)
	}
	return nil
}
