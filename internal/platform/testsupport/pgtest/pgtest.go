//go:build integration

package pgtest

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
)

// Image dipin ke major stabil terbaru (sama dengan docker-compose.dev.yml).
const Image = "postgres:18-alpine"

// Start menyediakan database PostgreSQL dan mengembalikan DSN; dibersihkan otomatis.
// Bila ERP_TEST_DATABASE_URL diset, dipakai server eksternal (database sementara per test); selain itu container.
func Start(t *testing.T) (dsn string) {
	t.Helper()
	dsn, _ = StartWithStop(t)
	return dsn
}

// StartWithStop seperti Start, tetapi juga mengembalikan fungsi untuk membuat database tidak tersedia lebih awal
// (container dihentikan, atau pada mode eksternal database dihapus dengan memutus semua koneksinya).
func StartWithStop(t *testing.T) (dsn string, stop func()) {
	t.Helper()
	if admin := strings.TrimSpace(os.Getenv(EnvExternal)); admin != "" {
		return startExternal(t, admin)
	}
	return startContainer(t)
}

func startContainer(t *testing.T) (dsn string, stop func()) {
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

// startExternal membuat database sementara bernama unik di server eksternal (DSN admin) dan menghapusnya
// (DROP DATABASE ... WITH (FORCE)) saat test selesai.
func startExternal(t *testing.T, adminDSN string) (dsn string, stop func()) {
	t.Helper()
	name := uniqueDatabaseName()
	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("%s: %v", EnvExternal, err)
	}
	// name hanya berisi [a-z0-9_] (lihat uniqueDatabaseName) sehingga aman sebagai identifier tanpa kutip.
	drop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = execAdmin(ctx, adminDSN, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	}
	// Cleanup didaftarkan sebelum CREATE: bila CREATE gagal di tengah jalan, sisa tetap dibersihkan.
	t.Cleanup(drop)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := execAdmin(ctx, adminDSN, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("buat database sementara: %v", err)
	}
	return dsn, drop
}

// execAdmin membuka koneksi singkat (lewat internal/platform/database, satu-satunya pembuka koneksi) dan menjalankan SQL.
func execAdmin(ctx context.Context, adminDSN, stmt string) error {
	db, err := database.Open(ctx, config.Config{DatabaseURL: adminDSN}, noop.NewTracerProvider(), slog.New(slog.DiscardHandler))
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	return db.WithContext(ctx).Exec(stmt).Error
}
