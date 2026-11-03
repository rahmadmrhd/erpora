//go:build integration

package migrate_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/migrations"
)

var identityControlTables = []string{
	"users", "tenant_memberships", "user_preferences", "signing_keys", "auth_sessions", "refresh_tokens",
	"user_mfa_factors", "user_recovery_codes", "password_resets", "login_attempts",
}

func tableExists(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	var reg *string
	if err := db.Raw("SELECT to_regclass(?)::text", name).Scan(&reg).Error; err != nil {
		t.Fatal(err)
	}
	return reg != nil
}

func gooseProvider(t *testing.T, db *gorm.DB, target string) *goose.Provider {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fs.Sub(migrations.FS, target)
	if err != nil {
		t.Fatal(err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub, goose.WithTableName("goose_"+target))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// 0005_identity (control) dan tenant 0003_roles_min: naik membuat semua tabel dan benih role administrator, turun
// menghapusnya hingga bersih, naik lagi memulihkan benih.
func TestIdentityTables_MigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	all := append(append([]string{}, identityControlTables...), "roles", "user_roles")
	for _, tbl := range all {
		if !tableExists(t, db, tbl) {
			t.Errorf("tabel %s tidak ada setelah Up", tbl)
		}
	}
	var admins int
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key = 'administrator' AND kind = 'system'`).Scan(&admins).Error; err != nil || admins != 1 {
		t.Fatalf("role sistem administrator: count=%d err=%v", admins, err)
	}

	ctl, ten := gooseProvider(t, db, "control"), gooseProvider(t, db, "tenant")
	if _, err := ctl.DownTo(ctx, 4); err != nil {
		t.Fatalf("turun control: %v", err)
	}
	if _, err := ten.DownTo(ctx, 2); err != nil {
		t.Fatalf("turun tenant: %v", err)
	}
	for _, tbl := range all {
		if tableExists(t, db, tbl) {
			t.Errorf("tabel %s masih ada setelah turun", tbl)
		}
	}
	if !tableExists(t, db, "desk_overrides") {
		t.Error("turun ke 4 tidak boleh menghapus desk_overrides")
	}
	if _, err := ctl.Up(ctx); err != nil {
		t.Fatalf("naik control: %v", err)
	}
	if _, err := ten.Up(ctx); err != nil {
		t.Fatalf("naik tenant: %v", err)
	}
	for _, tbl := range all {
		if !tableExists(t, db, tbl) {
			t.Errorf("tabel %s tidak ada setelah naik lagi", tbl)
		}
	}
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key = 'administrator'`).Scan(&admins).Error; err != nil || admins != 1 {
		t.Fatalf("benih administrator setelah naik lagi: count=%d err=%v", admins, err)
	}
	st, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		want := int64(3)
		if s.Target == migrate.Control {
			want = 5
		}
		if s.Latest != want || s.Current != want || s.Pending != 0 {
			t.Errorf("target %s: %+v, want Latest=Current=%d", s.Target, s, want)
		}
	}
}

func expect(t *testing.T, db *gorm.DB, name, code, sql string, args ...any) {
	t.Helper()
	err := db.Exec(sql, args...).Error
	if code == "" {
		if err != nil {
			t.Errorf("%s: harus lolos, err = %v", name, err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), code) {
		t.Errorf("%s: err = %v, want SQLSTATE %s", name, err, code)
	}
}

// CHECK, UNIQUE, dan FK pada skema identitas ditegakkan oleh database sendiri (bukan hanya aplikasi), sehingga SQL
// langsung tidak dapat melewatinya.
func TestIdentityTables_Constraints(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	const insUser = `INSERT INTO users (id, email, name, status) VALUES (?, ?, 'N', ?)`
	u1, u2, tn := uuid.New(), uuid.New(), uuid.New()

	// users: email ternormalisasi (huruf kecil, tanpa spasi tepi), unik, dan status terbatas.
	expect(t, db, "user sah", "", insUser, u1, "ada@example.com", "active")
	expect(t, db, "email berhuruf besar", "23514", insUser, u2, "Ada2@example.com", "active")
	expect(t, db, "email bersepasi tepi", "23514", insUser, u2, " ada2@example.com", "active")
	expect(t, db, "email kosong", "23514", insUser, u2, "", "active")
	expect(t, db, "email tanpa @", "23514", insUser, u2, "tanpa-at", "active")
	expect(t, db, "email 255 karakter", "23514", insUser, u2, strings.Repeat("a", 250)+"@b.co", "active")
	expect(t, db, "email duplikat persis", "23505", insUser, u2, "ada@example.com", "active")
	expect(t, db, "status ilegal", "23514", insUser, u2, "x@example.com", "deleted")
	for _, st := range []string{"active", "disabled", "locked"} {
		expect(t, db, "status "+st, "", insUser, uuid.New(), st+"@example.com", st)
	}
	expect(t, db, "nama kosong", "23514", `INSERT INTO users (id, email, name) VALUES (?, 'n1@example.com', '')`, uuid.New())

	// tenant_memberships
	expect(t, db, "membership sah", "", `INSERT INTO tenant_memberships (tenant_id, user_id, status) VALUES (?, ?, 'active')`, tn, u1)
	expect(t, db, "membership duplikat", "23505", `INSERT INTO tenant_memberships (tenant_id, user_id, status) VALUES (?, ?, 'active')`, tn, u1)
	expect(t, db, "membership status ilegal", "23514", `INSERT INTO tenant_memberships (tenant_id, user_id, status) VALUES (?, ?, 'zombie')`, uuid.New(), u1)
	expect(t, db, "membership user tak ada", "23503", `INSERT INTO tenant_memberships (tenant_id, user_id, status) VALUES (?, ?, 'active')`, tn, uuid.New())

	// signing_keys: status, kid, panjang kunci publik, dan cap waktu konsisten dengan status.
	const insKey = `INSERT INTO signing_keys (kid, status, public_key, private_key_sealed, activated_at, retiring_at, retired_at)
		VALUES (?, ?, ?, ?, ?::timestamptz, ?::timestamptz, ?::timestamptz)`
	pub, priv := make([]byte, 32), []byte{1, 2, 3}
	now := "2026-11-03T10:00:00Z"
	expect(t, db, "kunci pending", "", insKey, "kid-pending-1", "pending", pub, priv, nil, nil, nil)
	expect(t, db, "kunci active", "", insKey, "kid-active-01", "active", pub, priv, now, nil, nil)
	expect(t, db, "kunci retiring", "", insKey, "kid-retiring1", "retiring", pub, priv, now, now, nil)
	expect(t, db, "kunci retired", "", insKey, "kid-retired-1", "retired", pub, priv, now, now, now)
	expect(t, db, "kid duplikat", "23505", insKey, "kid-active-01", "pending", pub, priv, nil, nil, nil)
	expect(t, db, "status kunci ilegal", "23514", insKey, "kid-bogus-001", "enabled", pub, priv, nil, nil, nil)
	expect(t, db, "kid berpath", "23514", insKey, "../../etc/passwd", "pending", pub, priv, nil, nil, nil)
	expect(t, db, "kid berisi kutip", "23514", insKey, "kid';DROP TABLE x;--", "pending", pub, priv, nil, nil, nil)
	expect(t, db, "kid terlalu pendek", "23514", insKey, "k", "pending", pub, priv, nil, nil, nil)
	expect(t, db, "kunci publik 31 byte", "23514", insKey, "kid-shortpub-1", "pending", make([]byte, 31), priv, nil, nil, nil)
	expect(t, db, "kunci privat kosong", "23514", insKey, "kid-nopriv-001", "pending", pub, []byte{}, nil, nil, nil)
	expect(t, db, "active tanpa activated_at", "23514", insKey, "kid-noact-0001", "active", pub, priv, nil, nil, nil)
	expect(t, db, "retiring tanpa retiring_at", "23514", insKey, "kid-noret-0001", "retiring", pub, priv, now, nil, nil)
	expect(t, db, "retired tanpa retired_at", "23514", insKey, "kid-nored-0001", "retired", pub, priv, now, now, nil)
	expect(t, db, "pending dengan activated_at", "23514", insKey, "kid-pendact-01", "pending", pub, priv, now, nil, nil)

	// auth_sessions
	const insSess = `INSERT INTO auth_sessions (id, tenant_id, user_id, client_kind, absolute_expires_at) VALUES (?, ?, ?, ?, now() + interval '90 days')`
	s1 := uuid.New()
	expect(t, db, "sesi sah", "", insSess, s1, tn, u1, "web")
	expect(t, db, "client_kind ilegal", "23514", insSess, uuid.New(), tn, u1, "toaster")
	expect(t, db, "sesi user tak ada", "23503", insSess, uuid.New(), tn, uuid.New(), "web")
	expect(t, db, "amr bukan array", "23514", `UPDATE auth_sessions SET amr = '"pwd"'::jsonb WHERE id = ?`, s1)
	expect(t, db, "amr array kosong", "23514", `UPDATE auth_sessions SET amr = '[]'::jsonb WHERE id = ?`, s1)
	expect(t, db, "auth_method ilegal", "23514", `UPDATE auth_sessions SET auth_method = 'magic' WHERE id = ?`, s1)

	// refresh_tokens: hash 32 byte unik; parent_id dengan family_id dan session_id harus konsisten (FK komposit).
	const insRT = `INSERT INTO refresh_tokens (id, tenant_id, session_id, family_id, parent_id, token_hash, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, now() + interval '14 days')`
	h := func(b byte) []byte { x := make([]byte, 32); x[0] = b; return x }
	root, fam := uuid.New(), uuid.New()
	expect(t, db, "refresh akar", "", insRT, root, tn, s1, fam, nil, h(1))
	expect(t, db, "refresh anak konsisten", "", insRT, uuid.New(), tn, s1, fam, root, h(2))
	expect(t, db, "hash duplikat", "23505", insRT, uuid.New(), tn, s1, fam, root, h(1))
	expect(t, db, "hash 31 byte", "23514", insRT, uuid.New(), tn, s1, fam, nil, make([]byte, 31))
	expect(t, db, "family tidak cocok dengan induk", "23503", insRT, uuid.New(), tn, s1, uuid.New(), root, h(3))
	s2 := uuid.New()
	expect(t, db, "sesi kedua", "", insSess, s2, tn, u1, "web")
	expect(t, db, "sesi tidak cocok dengan induk", "23503", insRT, uuid.New(), tn, s2, fam, root, h(4))
	expect(t, db, "induk tak ada", "23503", insRT, uuid.New(), tn, s1, fam, uuid.New(), h(5))
	expect(t, db, "sesi tak ada", "23503", insRT, uuid.New(), tn, uuid.New(), uuid.New(), nil, h(6))

	// user_mfa_factors: satu faktor per (user, type), type hanya totp, last_used_step tak negatif.
	const insMFA = `INSERT INTO user_mfa_factors (id, user_id, type, secret_sealed, last_used_step) VALUES (?, ?, ?, ?, ?)`
	expect(t, db, "faktor sah", "", insMFA, uuid.New(), u1, "totp", priv, 0)
	expect(t, db, "faktor duplikat", "23505", insMFA, uuid.New(), u1, "totp", priv, 0)
	expect(t, db, "type ilegal", "23514", insMFA, uuid.New(), u1, "sms", priv, 0)
	expect(t, db, "secret kosong", "23514", insMFA, uuid.New(), u2, "totp", []byte{}, 0)
	expect(t, db, "step negatif", "23514", insMFA, uuid.New(), u2, "totp", priv, -1)

	// user_recovery_codes
	const insRC = `INSERT INTO user_recovery_codes (id, user_id, code_hash) VALUES (?, ?, ?)`
	expect(t, db, "recovery sah", "", insRC, uuid.New(), u1, h(9))
	expect(t, db, "recovery duplikat per user", "23505", insRC, uuid.New(), u1, h(9))
	expect(t, db, "recovery hash 31 byte", "23514", insRC, uuid.New(), u1, make([]byte, 31))

	// password_resets
	const insPR = `INSERT INTO password_resets (id, tenant_id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?, now() + interval '30 minutes')`
	expect(t, db, "reset sah", "", insPR, uuid.New(), tn, u1, h(20))
	expect(t, db, "reset hash duplikat", "23505", insPR, uuid.New(), tn, u1, h(20))
	expect(t, db, "reset hash 31 byte", "23514", insPR, uuid.New(), tn, u1, make([]byte, 31))

	// login_attempts
	const insLA = `INSERT INTO login_attempts (tenant_id, user_id, ip, kind, success) VALUES (?, ?, ?, ?, ?)`
	expect(t, db, "attempt sah", "", insLA, tn, u1, "203.0.113.7", "password", false)
	expect(t, db, "attempt kind ilegal", "23514", insLA, tn, u1, "203.0.113.7", "magic", false)
	expect(t, db, "attempt ip kosong", "23514", insLA, tn, u1, "", "password", false)
	expect(t, db, "attempt user tak ada", "23503", insLA, tn, uuid.New(), "203.0.113.7", "password", false)

	// user_preferences
	expect(t, db, "preferensi sah", "", `INSERT INTO user_preferences (user_id, locale, timezone) VALUES (?, 'id', 'Asia/Jakarta')`, u1)
	expect(t, db, "preferensi duplikat", "23505", `INSERT INTO user_preferences (user_id, locale, timezone) VALUES (?, 'en', 'UTC')`, u1)

	// roles / user_roles (tenant)
	const insRole = `INSERT INTO roles (key, label, kind) VALUES (?, 'L', ?)`
	expect(t, db, "role sah", "", insRole, "accountant", "custom")
	expect(t, db, "key duplikat", "23505", insRole, "administrator", "custom")
	expect(t, db, "key berhuruf besar", "23514", insRole, "Accountant2", "custom")
	expect(t, db, "kind ilegal", "23514", insRole, "role-x", "super")
	expect(t, db, "user_role role tak ada", "23503", `INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, u1, uuid.New())
	expect(t, db, "user_role sah", "", `INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE key = 'administrator'`, u1)
	expect(t, db, "user_role duplikat", "23505", `INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE key = 'administrator'`, u1)
}
