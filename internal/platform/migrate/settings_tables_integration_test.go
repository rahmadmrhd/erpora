//go:build integration

package migrate_test

import (
	"context"
	"strings"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

// tenant 0004_settings: naik membuat tabel, turun menghapusnya (tabel roles tetap), naik lagi memulihkannya.
func TestSettingsTable_MigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "settings") {
		t.Fatal("tabel settings tidak ada setelah Up")
	}
	ten := gooseProvider(t, db, "tenant")
	if _, err := ten.DownTo(ctx, 3); err != nil {
		t.Fatalf("turun: %v", err)
	}
	if tableExists(t, db, "settings") {
		t.Fatal("tabel settings masih ada setelah turun")
	}
	if !tableExists(t, db, "roles") {
		t.Fatal("turun ke 3 tidak boleh menghapus roles")
	}
	if _, err := ten.Up(ctx); err != nil {
		t.Fatalf("naik lagi: %v", err)
	}
	if !tableExists(t, db, "settings") {
		t.Fatal("tabel settings tidak ada setelah naik lagi")
	}
}

// CHECK dan UNIQUE pada settings ditegakkan database sendiri.
func TestSettingsTable_Constraints(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	const ins = `INSERT INTO settings (scope, module_key, key, value, encrypted) VALUES (?, ?, ?, ?::jsonb, ?)`
	expect(t, db, "platform sah", "", ins, "platform", "", "platform.public_url", `"https://erp.example.com"`, false)
	expect(t, db, "duplikat", "23505", ins, "platform", "", "platform.public_url", `"x"`, false)
	expect(t, db, "module sah", "", ins, "module", "erp.core", "core.banner", `{"a":1}`, false)
	expect(t, db, "module sama key berbeda modul", "", ins, "module", "erp.other", "core.banner", `{"a":1}`, false)
	expect(t, db, "scope ilegal", "23514", ins, "tenant", "", "k.one", `1`, false)
	expect(t, db, "platform dengan module_key", "23514", ins, "platform", "erp.core", "k.two", `1`, false)
	expect(t, db, "module tanpa module_key", "23514", ins, "module", "", "k.three", `1`, false)
	expect(t, db, "module_key berhuruf besar", "23514", ins, "module", "Erp.Core", "k.four", `1`, false)
	expect(t, db, "key berhuruf besar", "23514", ins, "platform", "", "Platform.X", `1`, false)
	expect(t, db, "key kosong", "23514", ins, "platform", "", "", `1`, false)
	expect(t, db, "key berspasi", "23514", ins, "platform", "", "a b", `1`, false)
	expect(t, db, "key 129 karakter", "23514", ins, "platform", "", "a"+strings.Repeat("b", 128), `1`, false)
	expect(t, db, "terenkripsi bukan string", "23514", ins, "platform", "", "secret.obj", `{"a":1}`, true)
	expect(t, db, "terenkripsi string", "", ins, "platform", "", "secret.ok", `"c2VhbGVk"`, true)
	expect(t, db, "value NULL", "23502", `INSERT INTO settings (scope, module_key, key, value) VALUES ('platform', '', 'null.value', NULL)`)
}
