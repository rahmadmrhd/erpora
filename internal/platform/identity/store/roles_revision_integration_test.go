//go:build integration

package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authzstore "erpora/internal/platform/authorization/store"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

// Pemberian dan pencabutan role oleh store identitas menaikkan authz_state.permission_revision pada transaksi
// tenant yang sama (PRD-001 C2; Review Focus 3): hak pengguna tidak pernah berubah tanpa revisi baru. Operasi yang
// tidak mengubah apa pun (pemberian ulang, pencabutan role yang tak dimiliki, role tak ada) tidak menaikkan revisi.
func TestGrantAndRevokeRoleBumpPermissionRevision(t *testing.T) {
	db, s := setup(t)
	az := authzstore.New(bothTenants{db: database.TenantDB{DB: db}})
	actx := tenancy.WithTenant(ctx, tenantA)
	rev := func() int64 {
		t.Helper()
		r, err := az.Revision(actx)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	u := uuid.NewString()

	r0 := rev()
	if granted, err := s.GrantRole(ctx, tenantA, u, "administrator", now()); err != nil || !granted {
		t.Fatalf("GrantRole = %v, %v", granted, err)
	}
	if r := rev(); r != r0+1 {
		t.Fatalf("revisi setelah GrantRole = %d, want %d", r, r0+1)
	}
	if granted, err := s.GrantRole(ctx, tenantA, u, "administrator", now()); err != nil || granted {
		t.Fatalf("GrantRole ulang = %v, %v", granted, err)
	}
	if _, err := s.GrantRole(ctx, tenantA, u, "tidak-ada", now()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("role tak ada: err = %v", err)
	}
	if r := rev(); r != r0+1 {
		t.Fatalf("revisi setelah pemberian tanpa perubahan = %d, want %d (tidak naik)", r, r0+1)
	}
	if ok, err := s.RevokeRole(ctx, tenantA, uuid.NewString(), "administrator"); err != nil || ok {
		t.Fatalf("RevokeRole tak dimiliki = %v, %v", ok, err)
	}
	if r := rev(); r != r0+1 {
		t.Fatalf("revisi setelah pencabutan tanpa perubahan = %d, want %d", r, r0+1)
	}
	if ok, err := s.RevokeRole(ctx, tenantA, u, "administrator"); err != nil || !ok {
		t.Fatalf("RevokeRole = %v, %v", ok, err)
	}
	if r := rev(); r != r0+2 {
		t.Fatalf("revisi setelah RevokeRole = %d, want %d", r, r0+2)
	}
}

// Kegagalan menaikkan revisi membatalkan pemberian role (satu transaksi): tanpa authz_state, GrantRole gagal dan
// user_roles tetap kosong, bukan role tanpa revisi.
func TestGrantRoleIsAtomicWithRevisionBump(t *testing.T) {
	db, s := setup(t)
	if err := db.Exec(`DELETE FROM authz_state`).Error; err != nil {
		t.Fatal(err)
	}
	u := uuid.NewString()
	if _, err := s.GrantRole(ctx, tenantA, u, "administrator", now()); err == nil {
		t.Fatal("GrantRole harus galat bila revisi tidak dapat dinaikkan")
	}
	assertNoRoleRows(t, db, u)
}

func assertNoRoleRows(t *testing.T, db *gorm.DB, user string) {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM user_roles WHERE user_id = ?::uuid`, user).Scan(&n).Error; err != nil || n != 0 {
		t.Fatalf("user_roles tersisa = %d, %v, want 0 (pemberian harus batal bersama revisi)", n, err)
	}
}
