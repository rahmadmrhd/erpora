//go:build integration

package migrate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

func TestLifecycleTables_Exist(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"tenant_state", "module_operations", "module_installations", "module_lifecycle_events", "module_migrations"} {
		var reg *string
		if err := db.Raw("SELECT to_regclass(?)::text", tbl).Scan(&reg).Error; err != nil {
			t.Fatal(err)
		}
		if reg == nil {
			t.Errorf("tabel %s tidak ada", tbl)
		}
	}
	st, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		var want int64 = 2 // tenant; control mencapai 3 sejak 0003_i18n_overrides
		if s.Target == migrate.Control {
			want = 3
		}
		if s.Latest != want || s.Pending != 0 || s.Current != want {
			t.Errorf("target %s: %+v, want Latest=%d Pending=0", s.Target, s, want)
		}
	}
}

func TestLifecycleEvents_AppendOnly(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := db.Exec(`INSERT INTO module_lifecycle_events (id, tenant_id, module_key, event, actor)
		VALUES (?, ?, 'acme.x', 'installed', 'test')`, id, uuid.New()).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	for name, sql := range map[string]string{
		"update":   "UPDATE module_lifecycle_events SET actor = 'x'",
		"delete":   "DELETE FROM module_lifecycle_events",
		"truncate": "TRUNCATE module_lifecycle_events",
	} {
		err := db.Exec(sql).Error
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: err = %v, want append-only", name, err)
		}
	}
}

func TestModuleInstallations_UniqueTenantKey(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	const ins = `INSERT INTO module_installations (id, tenant_id, module_key, status) VALUES (?, ?, 'acme.x', ?)`
	if err := db.Exec(ins, uuid.New(), tenant, "installed").Error; err != nil {
		t.Fatal(err)
	}
	err := db.Exec(ins, uuid.New(), tenant, "enabled").Error
	if err == nil || !strings.Contains(err.Error(), "23505") {
		t.Errorf("duplicate: err = %v, want 23505", err)
	}
	err = db.Exec(`INSERT INTO module_installations (id, tenant_id, module_key, status) VALUES (?, ?, 'acme.y', 'bogus')`, uuid.New(), tenant).Error
	if err == nil || !strings.Contains(err.Error(), "23514") {
		t.Errorf("bogus status: err = %v, want 23514", err)
	}
}

func TestMigrateUp_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	for range 2 {
		if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
			t.Fatal(err)
		}
	}
	st, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		var want int64 = 2 // tenant; control mencapai 3 sejak 0003_i18n_overrides
		if s.Target == migrate.Control {
			want = 3
		}
		if s.Current != want || s.Pending != 0 {
			t.Errorf("target %s: %+v, want Current=%d", s.Target, s, want)
		}
	}
}
