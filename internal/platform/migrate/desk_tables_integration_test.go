//go:build integration

package migrate_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/migrations"
)

const insertDeskOverride = `INSERT INTO desk_overrides (tenant_id, desk_id, scope, subject, config) VALUES (?, ?, ?, ?, ?::jsonb)`

// 0004_desk_overrides (revisi M3a, design.md section 6): naik membuat tabel, turun menghapusnya, naik lagi
// memulihkannya.
func TestDeskOverrides_MigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	exists := func() bool {
		var reg *string
		if err := db.Raw("SELECT to_regclass('desk_overrides')::text").Scan(&reg).Error; err != nil {
			t.Fatal(err)
		}
		return reg != nil
	}
	if !exists() {
		t.Fatal("tabel desk_overrides tidak ada setelah Up")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fs.Sub(migrations.FS, "control")
	if err != nil {
		t.Fatal(err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub, goose.WithTableName("goose_control"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 3); err != nil {
		t.Fatalf("turun: %v", err)
	}
	if exists() {
		t.Fatal("tabel desk_overrides masih ada setelah turun")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("naik lagi: %v", err)
	}
	if !exists() {
		t.Fatal("tabel desk_overrides tidak ada setelah naik lagi")
	}
	st, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if s.Target == migrate.Control && (s.Latest < 4 || s.Pending != 0 || s.Current != s.Latest) {
			t.Errorf("control: %+v, want Latest>=4 Pending=0", s)
		}
	}
}

func TestDeskOverrides_ConstraintsDefaultsAndPrimaryKey(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	if err := db.Exec(insertDeskOverride, tenant, "example", "site", "", `{"hidden":["notes"]}`).Error; err != nil {
		t.Fatalf("insert sah: %v", err)
	}
	var row struct {
		Revision  int64
		UpdatedBy string
		Subject   string
	}
	if err := db.Raw(`SELECT revision, updated_by, subject FROM desk_overrides WHERE tenant_id = ?`, tenant).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Revision != 0 || row.UpdatedBy != "" || row.Subject != "" {
		t.Errorf("nilai bawaan = %+v, want revision 0, updated_by '', subject ''", row)
	}
	// kunci utama (tenant, desk, scope, subject)
	if err := db.Exec(insertDeskOverride, tenant, "example", "site", "", `{}`).Error; err == nil || !strings.Contains(err.Error(), "23505") {
		t.Errorf("duplikat: err = %v, want 23505", err)
	}
	// desk sama untuk subject lain, scope lain, dan tenant lain boleh
	for name, args := range map[string][]any{
		"subject lain": {tenant, "example", "user", "u-1", `{}`},
		"scope lain":   {tenant, "example", "user", "", `{}`},
		"tenant lain":  {uuid.New(), "example", "site", "", `{}`},
		"desk kustom":  {tenant, "custom:ops-1", "site", "", `{}`},
	} {
		if err := db.Exec(insertDeskOverride, args...).Error; err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string][]any{
		"desk_id huruf besar":     {tenant, "Example", "site", "", `{}`},
		"desk_id kosong":          {tenant, "", "site", "", `{}`},
		"desk_id berspasi":        {tenant, "a b", "site", "", `{}`},
		"desk_id bergaris miring": {tenant, "a/b", "site", "", `{}`},
		"desk_id 65 karakter":     {tenant, strings.Repeat("a", 65), "site", "", `{}`},
		"scope selain site/user":  {tenant, "example2", "global", "", `{}`},
		"scope kosong":            {tenant, "example2", "", "", `{}`},
		"subject 129 karakter":    {tenant, "example2", "user", strings.Repeat("s", 129), `{}`},
	}
	for name, args := range bad {
		if err := db.Exec(insertDeskOverride, args...).Error; err == nil || !strings.Contains(err.Error(), "23514") {
			t.Errorf("%s: err = %v, want pelanggaran CHECK (23514)", name, err)
		}
	}
	for name, args := range map[string][]any{
		"desk_id 64 karakter":  {tenant, strings.Repeat("a", 64), "site", "", `{}`},
		"subject 128 karakter": {tenant, "example3", "user", strings.Repeat("s", 128), `{}`},
	} {
		if err := db.Exec(insertDeskOverride, args...).Error; err != nil {
			t.Errorf("%s harus lolos: %v", name, err)
		}
	}
	if err := db.Exec(insertDeskOverride, nil, "example4", "site", "", `{}`).Error; err == nil || !strings.Contains(err.Error(), "23502") {
		t.Errorf("tenant_id NULL: err = %v, want 23502", err)
	}
	if err := db.Exec(`INSERT INTO desk_overrides (tenant_id, desk_id, scope, revision) VALUES (?, 'example5', 'site', -1)`, tenant).Error; err == nil || !strings.Contains(err.Error(), "23514") {
		t.Errorf("revision negatif: err = %v, want 23514", err)
	}
}
