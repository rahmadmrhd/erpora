//go:build integration

// Package pgtest menjalankan container PostgreSQL untuk test integrasi.
package pgtest

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image dipin ke major stabil terbaru (sama dengan docker-compose.dev.yml).
const Image = "postgres:18-alpine"

// Start menjalankan container PostgreSQL dan mengembalikan DSN; container dihentikan otomatis.
func Start(t *testing.T) (dsn string) {
	t.Helper()
	dsn, _ = StartWithStop(t)
	return dsn
}

// StartWithStop seperti Start, tetapi juga mengembalikan fungsi untuk menghentikan container lebih awal.
func StartWithStop(t *testing.T) (dsn string, stop func()) {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithDatabase("erp"),
		tcpostgres.WithUsername("erp"),
		tcpostgres.WithPassword("erp"),
		tcpostgres.BasicWaitStrategies(),
	)
	// Cleanup didaftarkan sebelum memeriksa err: Run bisa mengembalikan container yang sudah dibuat
	// bersama error (mis. wait strategy timeout) dan container itu harus tetap dihentikan.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = testcontainers.TerminateContainer(c, testcontainers.StopContext(ctx))
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	dsn, err = c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	stopFn := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = testcontainers.TerminateContainer(c, testcontainers.StopContext(ctx))
	}
	return dsn, stopFn
}
