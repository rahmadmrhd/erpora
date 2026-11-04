//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

var authzTables = []string{"role_permissions", "role_field_access", "user_scope_assignments", "authz_state"}

// tenant 0005_authz: naik membuat tabel otorisasi, benih authz_state (1, 1) dan role sistem authenticated; turun
// menghapusnya hingga bersih (roles, user_roles, dan administrator tetap); naik lagi memulihkan semuanya.
func TestAuthzTables_MigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range authzTables {
		if !tableExists(t, db, tbl) {
			t.Errorf("tabel %s tidak ada setelah Up", tbl)
		}
	}
	var rev int64
	if err := db.Raw(`SELECT permission_revision FROM authz_state WHERE id = 1`).Scan(&rev).Error; err != nil || rev != 1 {
		t.Fatalf("benih authz_state: rev=%d err=%v", rev, err)
	}
	var sys int
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key IN ('administrator', 'authenticated') AND kind = 'system'`).Scan(&sys).Error; err != nil || sys != 2 {
		t.Fatalf("role sistem: count=%d err=%v", sys, err)
	}

	ten := gooseProvider(t, db, "tenant")
	if _, err := ten.DownTo(ctx, 4); err != nil {
		t.Fatalf("turun: %v", err)
	}
	for _, tbl := range authzTables {
		if tableExists(t, db, tbl) {
			t.Errorf("tabel %s masih ada setelah turun", tbl)
		}
	}
	for _, tbl := range []string{"roles", "user_roles", "settings"} {
		if !tableExists(t, db, tbl) {
			t.Errorf("turun ke 4 tidak boleh menghapus %s", tbl)
		}
	}
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key = 'authenticated'`).Scan(&sys).Error; err != nil || sys != 0 {
		t.Fatalf("role authenticated setelah turun: count=%d err=%v", sys, err)
	}
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key = 'administrator'`).Scan(&sys).Error; err != nil || sys != 1 {
		t.Fatalf("administrator setelah turun: count=%d err=%v", sys, err)
	}
	if _, err := ten.Up(ctx); err != nil {
		t.Fatalf("naik lagi: %v", err)
	}
	for _, tbl := range authzTables {
		if !tableExists(t, db, tbl) {
			t.Errorf("tabel %s tidak ada setelah naik lagi", tbl)
		}
	}
	if err := db.Raw(`SELECT permission_revision FROM authz_state WHERE id = 1`).Scan(&rev).Error; err != nil || rev != 1 {
		t.Fatalf("benih authz_state setelah naik lagi: rev=%d err=%v", rev, err)
	}
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key = 'authenticated' AND kind = 'system'`).Scan(&sys).Error; err != nil || sys != 1 {
		t.Fatalf("role authenticated setelah naik lagi: count=%d err=%v", sys, err)
	}
}

// Turun menghapus juga pemberian role authenticated yang mungkin ada (tanpa FK yang tertinggal).
func TestAuthzTables_DownRemovesAuthenticatedHolders(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT gen_random_uuid(), id FROM roles WHERE key = 'authenticated'`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := gooseProvider(t, db, "tenant").DownTo(ctx, 4); err != nil {
		t.Fatalf("turun: %v", err)
	}
}

// CHECK dan FK pada skema otorisasi ditegakkan database sendiri: scope kosong atau rusak, aksi tidak sah, dan akses
// di luar none/read/write tidak dapat disimpan lewat SQL langsung (gagal tertutup).
func TestAuthzTables_Constraints(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	var roleID string
	if err := db.Raw(`INSERT INTO roles (key, label, kind) VALUES ('custom.uji', 'Uji', 'custom') RETURNING id::text`).Scan(&roleID).Error; err != nil {
		t.Fatal(err)
	}
	const perm = `INSERT INTO role_permissions (role_id, resource, actions, scope) VALUES (?::uuid, ?, ?::text[], ?::jsonb)`
	expect(t, db, "baris sah", "", perm, roleID, "example.note", "{read,update}", `["owner","assigned:inventory.warehouse"]`)
	expect(t, db, "baris kedua resource sama", "", perm, roleID, "example.note", "{read}", `["all"]`)
	expect(t, db, "custom scope sah", "", perm, roleID, "example-addon.summary", "{read}", `["custom:example.mine"]`)
	expect(t, db, "scope larik kosong", "23514", perm, roleID, "example.note", "{read}", `[]`)
	expect(t, db, "scope bukan larik (string)", "23514", perm, roleID, "example.note", "{read}", `"all"`)
	expect(t, db, "scope objek", "23514", perm, roleID, "example.note", "{read}", `{"all":true}`)
	expect(t, db, "scope null JSON", "23514", perm, roleID, "example.note", "{read}", `null`)
	expect(t, db, "scope elemen bukan string", "23514", perm, roleID, "example.note", "{read}", `["all",1]`)
	expect(t, db, "scope token tak dikenal", "23514", perm, roleID, "example.note", "{read}", `["everything"]`)
	expect(t, db, "scope assigned tanpa dimensi", "23514", perm, roleID, "example.note", "{read}", `["assigned:"]`)
	expect(t, db, "scope custom huruf besar", "23514", perm, roleID, "example.note", "{read}", `["custom:Mine"]`)
	expect(t, db, "scope NULL", "23502", perm, roleID, "example.note", "{read}", nil)
	expect(t, db, "aksi kosong", "23514", perm, roleID, "example.note", "{}", `["all"]`)
	expect(t, db, "aksi mengandung NULL", "23514", perm, roleID, "example.note", "{read,NULL}", `["all"]`)
	expect(t, db, "aksi huruf besar", "23514", perm, roleID, "example.note", "{Read}", `["all"]`)
	expect(t, db, "aksi berkoma", "23514", perm, roleID, "example.note", `{"read,update"}`, `["all"]`)
	expect(t, db, "aksi string kosong", "23514", perm, roleID, "example.note", `{""}`, `["all"]`)
	expect(t, db, "resource huruf besar", "23514", perm, roleID, "Example.Note", "{read}", `["all"]`)
	expect(t, db, "role tak ada", "23503", perm, "00000000-0000-0000-0000-000000000000", "example.note", "{read}", `["all"]`)

	const fa = `INSERT INTO role_field_access (role_id, resource, field_group, access) VALUES (?::uuid, ?, ?, ?)`
	expect(t, db, "field access sah", "", fa, roleID, "example.note", "profit", "none")
	expect(t, db, "field access duplikat", "23505", fa, roleID, "example.note", "profit", "read")
	expect(t, db, "access all", "23514", fa, roleID, "example.note", "general", "all")
	expect(t, db, "access huruf besar", "23514", fa, roleID, "example.note", "general", "None")
	expect(t, db, "field_group kosong", "23514", fa, roleID, "example.note", "", "read")

	const asg = `INSERT INTO user_scope_assignments (user_id, dimension, "values", include_descendants) VALUES (?::uuid, ?, ?::text[], ?)`
	user := "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	expect(t, db, "penugasan sah", "", asg, user, "inventory.warehouse", "{wh-01,wh-02}", false)
	expect(t, db, "penugasan duplikat", "23505", asg, user, "inventory.warehouse", "{wh-03}", false)
	expect(t, db, "penugasan nilai kosong", "23514", asg, user, "core.org_unit", "{}", true)
	expect(t, db, "penugasan elemen NULL", "23514", asg, user, "core.org_unit", "{a,NULL}", true)
	expect(t, db, "penugasan elemen string kosong", "23514", asg, user, "core.org_unit", `{""}`, true)
	expect(t, db, "dimensi huruf besar", "23514", asg, user, "Core.Org", "{a}", false)
	var inc bool
	if err := db.Raw(`SELECT include_descendants FROM user_scope_assignments WHERE dimension = 'inventory.warehouse'`).Scan(&inc).Error; err != nil || inc {
		t.Errorf("include_descendants default = %v, %v, want false", inc, err)
	}

	expect(t, db, "authz_state baris kedua", "23514", `INSERT INTO authz_state (id, permission_revision) VALUES (2, 1)`)
	expect(t, db, "authz_state duplikat id 1", "23505", `INSERT INTO authz_state (id, permission_revision) VALUES (1, 1)`)
	expect(t, db, "revisi nol", "23514", `UPDATE authz_state SET permission_revision = 0`)

	// ON DELETE CASCADE: menghapus role membersihkan matriks dan field access-nya.
	if err := db.Exec(`DELETE FROM roles WHERE id = ?::uuid`, roleID).Error; err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Raw(`SELECT (SELECT count(*) FROM role_permissions) + (SELECT count(*) FROM role_field_access)`).Scan(&n).Error; err != nil || n != 0 {
		t.Errorf("sisa baris setelah role dihapus = %d, %v", n, err)
	}
}
