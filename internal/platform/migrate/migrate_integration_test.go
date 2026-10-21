//go:build integration

package migrate_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/rahmadmrhd/erp-go/internal/platform/config"
	"github.com/rahmadmrhd/erp-go/internal/platform/database"
	"github.com/rahmadmrhd/erp-go/internal/platform/migrate"
	"github.com/rahmadmrhd/erp-go/internal/platform/testsupport/pgtest"
)

func open(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond},
		noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	return db
}

func state(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()
	var v string
	if err := db.Raw("SELECT value FROM platform_state WHERE key = ?", key).Scan(&v).Error; err != nil {
		t.Fatal(err)
	}
	return v
}

func TestUp_EmptyDatabase(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	st, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 2 {
		t.Fatalf("targets = %+v", st)
	}
	for _, s := range st {
		if s.Pending != 0 || s.Current != s.Latest || s.Latest < 1 {
			t.Errorf("target %s not up to date: %+v", s.Target, s)
		}
	}
	if _, err := uuid.Parse(state(t, db, "default_tenant_id")); err != nil {
		t.Errorf("default_tenant_id: %v", err)
	}
	if got := state(t, db, "environment"); got != "production" {
		t.Errorf("environment = %q", got)
	}
}

func TestUp_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	first := state(t, db, "default_tenant_id")
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	if got := state(t, db, "default_tenant_id"); got != first {
		t.Errorf("tenant changed: %s -> %s", first, got)
	}
	id, err := migrate.DefaultTenantID(ctx, db)
	if err != nil || id.String() != first {
		t.Errorf("DefaultTenantID = %v, %v", id, err)
	}
}

func TestUp_Concurrent(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.Start(t)
	dbs := []*gorm.DB{open(t, dsn), open(t, dsn)}
	errs := make([]error, len(dbs))
	var wg sync.WaitGroup
	for i := range dbs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = migrate.Up(ctx, dbs[i], config.EnvProduction)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("Up #%d: %v", i, err)
		}
	}
	var n int64
	if err := dbs[0].Raw("SELECT count(*) FROM platform_state WHERE key = 'default_tenant_id'").Scan(&n).Error; err != nil || n != 1 {
		t.Errorf("default_tenant_id rows = %d (%v)", n, err)
	}
}

func TestEnsurePlatformState_EnvironmentNotOverwritten(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvDevelopment); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	if got := state(t, db, "environment"); got != "development" {
		t.Errorf("environment = %q, want development", got)
	}
}

func TestEnsurePlatformState_ConfiguredTenantMismatch(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	_, err := migrate.EnsurePlatformState(ctx, db, config.EnvProduction, &other)
	if err == nil || !strings.Contains(err.Error(), "ERP_DEFAULT_TENANT_ID") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsurePlatformState_UsesConfiguredTenantOnFreshDB(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	// Up memakai tenant yang dibuat; pastikan want yang sama diterima.
	id, err := migrate.DefaultTenantID(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := migrate.EnsurePlatformState(ctx, db, config.EnvProduction, &id)
	if err != nil || got != id {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPending_BeforeUp(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	n, err := migrate.Pending(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Errorf("pending = %d", n)
	}
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	if n, _ := migrate.Pending(ctx, db); n != 0 {
		t.Errorf("pending after Up = %d", n)
	}
}
