//go:build integration

package settings_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/settings"
	"erpora/internal/platform/settings/store"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

type perTenant map[tenancy.TenantID]database.TenantDB

func (p perTenant) For(_ context.Context, id tenancy.TenantID) (database.TenantDB, error) {
	db, ok := p[id]
	if !ok {
		return database.TenantDB{}, database.ErrUnknownTenant
	}
	return db, nil
}

func openMigrated(t *testing.T) database.TenantDB {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: pgtest.Start(t), DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	return database.TenantDB{DB: db}
}

func ctxOf(t tenancy.TenantID) context.Context { return tenancy.WithTenant(context.Background(), t) }

// PublicURL melewati DB sungguhan: round trip, tersimpan ternormalisasi, nilai baru langsung terbaca setelah Set
// (cache tidak basi), dan tiap tenant memakai database terpisah.
func TestPublicURL_DatabaseRoundTripAndTenantIsolation(t *testing.T) {
	dbA, dbB := openMigrated(t), openMigrated(t)
	st := store.New(perTenant{tenantA: dbA, tenantB: dbB}, nil)
	svc := settings.NewService(st, settings.Options{Production: true})
	a, b := ctxOf(tenantA), ctxOf(tenantB)

	if _, err := svc.PublicURL(a); !errors.Is(err, settings.ErrNotConfigured) {
		t.Fatalf("belum diisi: err = %v", err)
	}
	if err := svc.SetPublicURL(a, "HTTPS://ERP.Example.com:443/app/", "cli:core:settings:set"); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.PublicURL(a); err != nil || got != "https://erp.example.com/app" {
		t.Fatalf("PublicURL = %q, %v", got, err)
	}
	var raw string
	if err := dbA.Raw(`SELECT value::text FROM settings WHERE scope = 'platform' AND module_key = '' AND key = 'platform.public_url'`).Scan(&raw).Error; err != nil || raw != `"https://erp.example.com/app"` {
		t.Fatalf("tersimpan %q, %v", raw, err)
	}
	if err := svc.SetPublicURL(a, "https://erp2.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.PublicURL(a); got != "https://erp2.example.com" {
		t.Errorf("cache basi setelah Set: %q", got)
	}
	if _, err := svc.PublicURL(b); !errors.Is(err, settings.ErrNotConfigured) {
		t.Errorf("tenant B melihat URL tenant A: err = %v", err)
	}
	if err := svc.SetPublicURL(a, "http://erp.example.com", "x"); !errors.Is(err, settings.ErrInvalidURL) {
		t.Errorf("http di production harus ditolak: %v", err)
	}
	if got, _ := svc.PublicURL(a); got != "https://erp2.example.com" {
		t.Errorf("nilai tidak valid mengubah nilai tersimpan: %q", got)
	}
	// nilai rusak yang ditulis langsung ke DB: gagal tertutup (bukan ErrNotConfigured)
	if err := dbB.Exec(`INSERT INTO settings (scope, module_key, key, value) VALUES ('platform', '', 'platform.public_url', '"javascript:alert(1)"')`).Error; err != nil {
		t.Fatal(err)
	}
	svc.InvalidateAll() // penulisan langsung ke DB tidak melewati Set; cache negatif B perlu dibuang
	if _, err := svc.PublicURL(b); err == nil || errors.Is(err, settings.ErrNotConfigured) {
		t.Errorf("nilai rusak: err = %v, want galat bukan ErrNotConfigured", err)
	}
	// development: default eksplisit hanya bila belum diisi
	dev := settings.NewService(st, settings.Options{Production: false})
	c := ctxOf(tenantA)
	if got, err := dev.PublicURLOrDevDefault(c, settings.DevURLFromHTTPAddr("127.0.0.1:8080")); err != nil || got != "https://erp2.example.com" {
		t.Errorf("dev dengan nilai terisi: %q, %v", got, err)
	}
}
