// Package database satu-satunya tempat koneksi PostgreSQL (GORM + pgx) dibuka.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/rahmadmrhd/erp-go/internal/platform/config"
)

const pingTimeout = 5 * time.Second

// Open membuka koneksi GORM, memasang logger slog + tracing, dan memverifikasi dengan Ping (5 dtk).
// Error koneksi tidak pernah memuat password.
func Open(ctx context.Context, cfg config.Config, tp trace.TracerProvider, log *slog.Logger) (*gorm.DB, error) {
	if log == nil {
		log = slog.Default()
	}
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		// Ping otomatis GORM memakai context.Background() tanpa timeout; koneksi pertama dilakukan
		// oleh PingContext(ctx) di bawah agar menghormati ctx dan batas 5 detik.
		DisableAutomaticPing: true,
		Logger:               newSlogLogger(log, cfg.DBSlowQuery),
	})
	if err != nil {
		return nil, connError(cfg.DatabaseURL, err)
	}
	if err := db.Use(tracing.NewPlugin(
		tracing.WithoutQueryVariables(),
		tracing.WithTracerProvider(tp),
	)); err != nil {
		return nil, fmt.Errorf("pasang tracing database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pctx); err != nil {
		_ = sqlDB.Close()
		return nil, connError(cfg.DatabaseURL, err)
	}
	return db, nil
}

// connError membungkus error koneksi: menyebut host, tidak pernah membocorkan password DSN
// (bentuk URL, query, maupun keyword=value). Secret pendek (< 4 karakter) tidak dapat disensor
// dengan aman dari teks error, sehingga detail error disembunyikan seluruhnya.
func connError(dsn string, err error) error {
	host, secrets := config.DSNParts(dsn)
	msg := err.Error()
	if host == "?" {
		// DSN tidak dapat di-parse: kita tidak tahu bagian mana yang rahasia, jadi jangan tampilkan detail.
		msg = "DSN tidak dapat dibaca; detail disembunyikan"
	}
	for _, s := range secrets {
		if !strings.Contains(msg, s) {
			continue
		}
		if len(s) < 4 {
			msg = "detail disembunyikan"
			break
		}
		msg = strings.ReplaceAll(msg, s, "****")
	}
	return errors.New("database tidak terjangkau (host=" + host + "): " + msg)
}

// slogLogger mengarahkan log GORM ke slog dengan ambang query lambat.
type slogLogger struct {
	log  *slog.Logger
	slow time.Duration
}

func newSlogLogger(log *slog.Logger, slow time.Duration) gormlogger.Interface {
	return &slogLogger{log: log, slow: slow}
}

func (l *slogLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *slogLogger) Info(ctx context.Context, msg string, args ...any) {
	l.log.InfoContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *slogLogger) Warn(ctx context.Context, msg string, args ...any) {
	l.log.WarnContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *slogLogger) Error(ctx context.Context, msg string, args ...any) {
	l.log.ErrorContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *slogLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		l.log.ErrorContext(ctx, "database error", "error", err, "elapsed_ms", elapsed.Milliseconds(), "rows", rows, "sql", sql)
	case l.slow > 0 && elapsed > l.slow:
		sql, rows := fc()
		l.log.WarnContext(ctx, "slow query", "elapsed_ms", elapsed.Milliseconds(), "rows", rows, "sql", sql)
	}
}

// ParamsFilter (gorm.ParamsFilter) membuang nilai parameter sehingga log hanya memuat SQL
// berparameter ($1, $2, ...) dan tidak pernah data (PII/rahasia) milik pengguna.
func (l *slogLogger) ParamsFilter(_ context.Context, sql string, _ ...any) (string, []any) {
	return sql, nil
}
