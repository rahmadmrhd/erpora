//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/desks/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

func setup(t *testing.T) (*gorm.DB, *store.Store) {
	t.Helper()
	dsn := pgtest.Start(t)
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
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
	return db, store.New(database.ControlDB{DB: db})
}

func insert(t *testing.T, db *gorm.DB, tn tenancy.TenantID, desk, scope, subject, config string, rev int64) {
	t.Helper()
	if err := db.Exec(`INSERT INTO desk_overrides (tenant_id, desk_id, scope, subject, config, revision, updated_by)
		VALUES (?, ?, ?, ?, ?::jsonb, ?, 'seed')`, uuid.UUID(tn), desk, scope, subject, config, rev).Error; err != nil {
		t.Fatal(err)
	}
}

func TestListSite_EmptyByDefault(t *testing.T) {
	_, s := setup(t)
	got, err := s.ListSite(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("ListSite = %+v, want kosong", got)
	}
}

// Isolasi tenant: tenant A tidak pernah melihat baris tenant B (security-guidelines: semua kueri menyaring tenant_id).
func TestListSite_IsolatesTenants(t *testing.T) {
	db, s := setup(t)
	insert(t, db, tenantA, "example", "site", "", `{"hidden":["notes"]}`, 3)
	insert(t, db, tenantB, "example", "site", "", `{"hidden":["rahasia-b"]}`, 1)
	insert(t, db, tenantB, "tenant-b-only", "site", "", `{}`, 0)

	a, err := s.ListSite(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0].DeskID != "example" || a[0].Revision != 3 || a[0].UpdatedBy != "seed" {
		t.Fatalf("tenant A = %+v", a)
	}
	var cfg struct{ Hidden []string }
	if err := json.Unmarshal(a[0].Config, &cfg); err != nil || len(cfg.Hidden) != 1 || cfg.Hidden[0] != "notes" {
		t.Errorf("config tenant A = %s (%v)", a[0].Config, err)
	}
	b, err := s.ListSite(context.Background(), tenantB)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2 || b[0].DeskID != "example" || b[1].DeskID != "tenant-b-only" {
		t.Fatalf("tenant B = %+v (terurut desk_id)", b)
	}
	for _, o := range b {
		if string(o.Config) == `{"hidden": ["notes"]}` {
			t.Errorf("tenant B melihat config tenant A: %+v", o)
		}
	}
}

// Hanya scope "site" tanpa subjek yang dibaca; scope "user" butuh identitas (PRD-001) dan tidak dibaca di revisi ini.
func TestListSite_IgnoresUserScope(t *testing.T) {
	db, s := setup(t)
	insert(t, db, tenantA, "example", "user", "u-1", `{"hidden":["x"]}`, 1)
	insert(t, db, tenantA, "example", "user", "", `{"hidden":["y"]}`, 1)
	insert(t, db, tenantA, "core", "site", "", `{}`, 0)
	got, err := s.ListSite(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].DeskID != "core" {
		t.Fatalf("ListSite = %+v, want hanya core (site)", got)
	}
}

func TestListSite_CanceledContext(t *testing.T) {
	_, s := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.ListSite(ctx, tenantA); err == nil {
		t.Fatal("konteks dibatalkan harus menghasilkan galat")
	}
}
