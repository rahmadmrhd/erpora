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

const insertOverride = `INSERT INTO i18n_overrides (tenant_id, namespace, locale, key, message, updated_by) VALUES (?, ?, ?, ?, ?, 'test')`

func TestI18nTables_ExistAndMigrationIdempotent(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	for i := 0; i < 2; i++ { // naik dua kali: kedua kalinya tidak berbuat apa-apa
		if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
			t.Fatalf("Up #%d: %v", i+1, err)
		}
	}
	for _, tbl := range []string{"i18n_overrides", "i18n_revisions"} {
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
		if s.Target == migrate.Control && (s.Latest < 3 || s.Pending != 0 || s.Current != s.Latest) {
			t.Errorf("control: %+v, want Latest>=3 Pending=0", s)
		}
	}
}

func TestI18nOverrides_ConstraintsAndPrimaryKey(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	if err := db.Exec(insertOverride, tenant, "example", "id", "note.title", "Catatan").Error; err != nil {
		t.Fatalf("insert sah: %v", err)
	}
	// kunci utama komposit (tenant, namespace, locale, key)
	if err := db.Exec(insertOverride, tenant, "example", "id", "note.title", "Lain").Error; err == nil || !strings.Contains(err.Error(), "23505") {
		t.Errorf("duplikat: err = %v, want 23505", err)
	}
	// tenant lain boleh memakai kunci yang sama
	if err := db.Exec(insertOverride, uuid.New(), "example", "id", "note.title", "Catatan B").Error; err != nil {
		t.Errorf("tenant lain: %v", err)
	}
	bad := map[string][]any{
		"namespace huruf besar":     {tenant, "Example", "id", "a", "x"},
		"namespace terlalu panjang": {tenant, "a" + strings.Repeat("b", 50), "id", "a", "x"},
		"locale salah":              {tenant, "example", "ID", "a", "x"},
		"key berspasi":              {tenant, "example", "id", "a b", "x"},
		"key terlalu panjang":       {tenant, "example", "id", strings.Repeat("k", 201), "x"},
		"message kosong":            {tenant, "example", "id", "k1", ""},
		"message 2001 karakter":     {tenant, "example", "id", "k2", strings.Repeat("m", 2001)},
	}
	for name, args := range bad {
		if err := db.Exec(insertOverride, args...).Error; err == nil || !strings.Contains(err.Error(), "23514") {
			t.Errorf("%s: err = %v, want pelanggaran CHECK (23514)", name, err)
		}
	}
	if err := db.Exec(insertOverride, tenant, "example", "id", "k3", strings.Repeat("m", 2000)).Error; err != nil {
		t.Errorf("message 2000 karakter harus lolos: %v", err)
	}
	// tenant_id NOT NULL
	if err := db.Exec(insertOverride, nil, "example", "id", "k4", "x").Error; err == nil || !strings.Contains(err.Error(), "23502") {
		t.Errorf("tenant_id NULL: err = %v, want 23502", err)
	}
}

func TestI18nRevisions_DefaultAndConstraint(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	if err := db.Exec(`INSERT INTO i18n_revisions (tenant_id, namespace, locale) VALUES (?, 'ui', 'id')`, tenant).Error; err != nil {
		t.Fatal(err)
	}
	var rev int64
	if err := db.Raw(`SELECT revision FROM i18n_revisions WHERE tenant_id = ?`, tenant).Scan(&rev).Error; err != nil || rev != 0 {
		t.Errorf("revision awal = %d, %v, want 0", rev, err)
	}
	if err := db.Exec(`INSERT INTO i18n_revisions (tenant_id, namespace, locale, revision) VALUES (?, 'ui', 'en', -1)`, tenant).Error; err == nil {
		t.Error("revision negatif harus ditolak")
	}
	if err := db.Exec(`INSERT INTO i18n_revisions (tenant_id, namespace, locale) VALUES (?, 'ui', 'id')`, tenant).Error; err == nil {
		t.Error("kunci utama (tenant, namespace, locale) harus unik")
	}
}
