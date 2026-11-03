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

	// Binary dev menolak melayani DB berpenanda production (FR-22.5); binary release tidak terpengaruh.
	if err := GuardDevWrite(ctx, db, cfg); err != nil {
		return err
	}

	pending, err := migrate.Pending(ctx, db)
	if err != nil {
		return fmt.Errorf("periksa migrasi: %w", err)
	}
	if pending > 0 {
		return errors.New("migrasi platform belum diterapkan: jalankan erpora migrate up")
	}

	// Production gagal start tanpa URL publik (keputusan pemilik produk 3 Nov 2026; sumber: settings platform).
	// Production tanpa ERP_MAIL_DRIVER eksplisit juga gagal start (surat reset sandi tidak akan terkirim; Config.RequireMailDriver).
	if err := cfg.RequireMailDriver(); err != nil {
		return &ExitError{Code: 2, Err: err}
	}
	tid, err := defaultTenant(ctx, cfg, db)
	if err != nil {
		return err
	}
	settingsSvc := newSettingsService(cfg, db, tid)
	if err := checkPublicURL(ctx, cfg, newSettingsURLSource(cfg, settingsSvc, tid)); err != nil {
		return &ExitError{Code: 2, Err: err}
	}
	a, err := assemble(ctx, cfg, log, db, prov, reg, api, withSettings(settingsSvc))
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
	shutdownErr := srv.Shutdown(sctx)
	// Pekerjaan latar reset sandi (pengiriman surat) harus selesai SEBELUM database ditutup (defer di atas), dibatasi sisa
	// shutdownTimeout (reset/doc.go: Wait dipanggil saat penutupan proses).
	if a.Identity != nil && a.Identity.Reset != nil {
		if !waitWithin(sctx, a.Identity.Reset.Wait) {
			log.Warn("pekerjaan latar reset sandi belum selesai saat batas shutdown habis")
		}
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown tidak tuntas: %w", shutdownErr)
	}
	return nil
}

// waitWithin menjalankan wait dan menunggunya paling lama sampai ctx selesai; true bila wait selesai tepat waktu.
func waitWithin(ctx context.Context, wait func()) bool {
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
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
