//go:build integration

package database_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/testsupport/pgtest"
)

func showTimeout(t *testing.T, c *sql.Conn) string {
	t.Helper()
	var v string
	if err := c.QueryRowContext(context.Background(), "SHOW statement_timeout").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func takeConns(t *testing.T, sqlDB *sql.DB, n int) []*sql.Conn {
	t.Helper()
	var conns []*sql.Conn
	for range n {
		c, err := sqlDB.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	return conns
}

func releaseAll(conns []*sql.Conn) {
	for _, c := range conns {
		_ = c.Close()
	}
}

// Review Focus 5: setiap koneksi pool membawa batas, SET LOCAL tidak bocor, statement lambat dibatalkan.
func TestStatementTimeout_AllPooledConnections(t *testing.T) {
	cfg := config.Config{DatabaseURL: pgtest.Start(t), DBSlowQuery: 200 * time.Millisecond}
	db, err := database.OpenWith(context.Background(), cfg, noop.NewTracerProvider(), slog.Default(),
		database.OpenOptions{StatementTimeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(4)

	conns := takeConns(t, sqlDB, 4)
	for i, c := range conns {
		if got := showTimeout(t, c); got != "300ms" {
			t.Errorf("koneksi %d: statement_timeout = %q, want 300ms", i, got)
		}
	}
	// SET LOCAL hanya berlaku di dalam transaksinya.
	for _, stmt := range []string{"BEGIN", "SET LOCAL statement_timeout = '5s'", "COMMIT"} {
		if _, err := conns[0].ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	_, err = conns[1].ExecContext(context.Background(), "SELECT pg_sleep(1)")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
		t.Errorf("pg_sleep(1): err = %v, want SQLSTATE 57014", err)
	}
	releaseAll(conns)

	for i, c := range takeConns(t, sqlDB, 4) {
		if got := showTimeout(t, c); got != "300ms" {
			t.Errorf("setelah rilis, koneksi %d: statement_timeout = %q, want 300ms", i, got)
		}
		_ = c.Close()
	}
}

func TestOpen_NoStatementTimeoutByDefault(t *testing.T) {
	cfg := config.Config{DatabaseURL: pgtest.Start(t), DBSlowQuery: 200 * time.Millisecond}
	db, err := database.Open(context.Background(), cfg, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	c, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := showTimeout(t, c); got != "0" {
		t.Errorf("statement_timeout = %q, want 0", got)
	}
}
