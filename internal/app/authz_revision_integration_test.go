//go:build integration

package app

import (
	"testing"

	authzstore "erpora/internal/platform/authorization/store"
	"erpora/internal/platform/database"
)

// PRD-001 C2 Review Focus 3: pemberian role administrator oleh core:create-admin (identity.Admin.CreateAdmin lewat
// identity/store.GrantRole) menaikkan authz_state.permission_revision pada transaksi tenant yang sama, sehingga hak
// efektif pengguna baru tidak pernah basi di cache mesin.
func TestCreateAdminGrantBumpsRevision(t *testing.T) {
	e := newCoreEnv(t)
	db := openTestDB(t, e.cfg.DatabaseURL)
	az := authzstore.New(database.SingleTenantConnector{Default: e.a.Tenant, DB: database.TenantDB{DB: db}})
	before, err := az.Revision(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	e.remember(pwAdmin)
	sum := e.createAdmin("revisi.admin@example.test", pwAdmin)
	after, err := az.Revision(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("revisi %d -> %d setelah CreateAdmin, want naik tepat satu (role administrator diberikan sekali)", before, after)
	}
	g, err := az.LoadUserGrants(e.ctx, sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	hasAdmin := false
	for _, r := range g.Roles {
		if r.Key == authzstore.RoleAdministrator && !r.Implicit {
			hasAdmin = true
		}
	}
	if !hasAdmin || g.Revision != after {
		t.Errorf("hak pengguna baru = revisi %d (want %d), administrator=%v", g.Revision, after, hasAdmin)
	}
}
