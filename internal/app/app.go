// Package app adalah composition root: merakit platform (dan nanti modul) menjadi proses `erpora serve`.
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

	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/observability"
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

// NewLogger membuat logger proses (trace-aware) sesuai konfigurasi. Perintah serve memakainya,
// memasangnya sebagai slog.Default() (agar log handler modul seragam), lalu meneruskannya ke Run.
func NewLogger(cfg config.Config, w io.Writer) *slog.Logger {
	return observability.NewLogger(w, cfg.LogFormat, slog.LevelInfo)
}

// Run menjalankan server sampai ctx dibatalkan. serve tidak pernah menulis platform_state.
// Run tidak mengubah slog.Default(); pemanggil (perintah serve) yang memasangnya.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {

	// Katalog modul divalidasi sebelum DB dibuka; katalog rusak adalah bug build (exit 2).
	reg, api, err := buildRegistry()
	if err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("katalog modul tidak valid: %w", err)}
	}

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

	db, err := database.OpenWith(ctx, cfg, prov.Tracer, log, serveOpenOptions(cfg))
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
		return errors.New("migrasi platform belum diterapkan: jalankan erpora migrate up")
	}

	a, err := assemble(ctx, cfg, log, db, prov, reg, api)
	if err != nil {
		return err
	}
	srv := httpserver.New(cfg.HTTPAddr, a.Router)

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

// serveOpenOptions adalah opsi koneksi khusus pool `serve`: batas statement (Q26 butir 7).
// CLI (migrate, modules, command modul) memakai database.Open tanpa batas agar migrasi panjang tidak terputus.
func serveOpenOptions(cfg config.Config) database.OpenOptions {
	return database.OpenOptions{StatementTimeout: cfg.DBStatementTimeout}
}
