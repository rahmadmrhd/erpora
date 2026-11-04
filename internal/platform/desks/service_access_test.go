package desks

import (
	"context"
	"testing"

	"erpora/internal/platform/modules/runtime"
	sdkidentity "erpora/sdk/identity"
	"erpora/sdk/module"
)

// authenticatedItem adalah item menu tanpa permission (PRD-000 Q43): hanya untuk pengguna yang login.
func authenticatedItem(id string) module.DeskMenu {
	return module.DeskMenu{ID: id, Label: "nav." + id, To: "/" + id, Access: module.DeskAccessAuthenticated}
}

func withUser(ctx context.Context, userID string) context.Context {
	return sdkidentity.WithPrincipal(ctx, sdkidentity.Principal{UserID: userID})
}

func TestList_AuthenticatedItemVisibleToLoggedInUserWithoutPermissions(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(
		item("alpha", "a", "read"),
		authenticatedItem("b"),
		group("g", authenticatedItem("c"), item("alpha", "d", "write")),
	)))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	svc := newService(reg, st, nil)

	// Principal login tanpa satu pun permission: hanya item authenticated (dan grup yang anaknya authenticated).
	v := mustList(t, svc, withUser(ctxFor(tenantA), "user-1"))
	d, ok := find(v, "alpha")
	if !ok {
		t.Fatal("Desk alpha harus ada karena memuat item authenticated")
	}
	eq(t, menuIDs(d), []string{"dashboard", "b", "g[c]"})

	// Dengan permission, item ber-permission ikut; item authenticated tetap ada.
	v = mustList(t, svc, withUser(ctxFor(tenantA, "alpha.items.read", "alpha.items.write"), "user-1"))
	d, _ = find(v, "alpha")
	eq(t, menuIDs(d), []string{"dashboard", "a", "b", "g[c,d]"})
}

func TestList_AuthenticatedItemHiddenWithoutPrincipal(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(authenticatedItem("b"), item("alpha", "a", "read"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	svc := newService(reg, st, nil)

	// Tanpa Principal di context (bukan pengguna login): item authenticated tidak dikirim (fail closed), dan Desk
	// tanpa menu yang diizinkan dibuang.
	v := mustList(t, svc, ctxFor(tenantA))
	if _, ok := find(v, "alpha"); ok {
		t.Fatalf("tanpa Principal Desk alpha tidak boleh ada: %v", ids(v.Desks))
	}
	// Principal tanpa UserID tidak dianggap login.
	v = mustList(t, svc, withUser(ctxFor(tenantA), ""))
	if _, ok := find(v, "alpha"); ok {
		t.Fatal("Principal tanpa UserID tidak boleh melihat item authenticated")
	}
}

func TestList_AccessItemWithPermissionOrUnknownValueIsNeverSent(t *testing.T) {
	// Pertahanan berlapis: manifest seperti ini sudah ditolak registry/schema, tetapi Service tetap fail closed.
	svc := &Service{authz: ctxAuthz{}}
	ctx := withUser(ctxFor(tenantA, "alpha.items.read"), "user-1")
	for name, m := range map[string]module.DeskMenu{
		"access dan permission":   {ID: "x", Access: module.DeskAccessAuthenticated, Permission: "alpha.items.read"},
		"access tidak dikenal":    {ID: "x", Access: "everyone"},
		"tanpa permission/access": {ID: "x"},
	} {
		if svc.visible(ctx, m) {
			t.Errorf("%s: item tidak boleh dikirim", name)
		}
	}
	if !svc.visible(ctx, authenticatedItem("x")) {
		t.Error("item authenticated harus dikirim untuk Principal login")
	}
	if !svc.visible(ctx, module.DeskMenu{ID: "x", Permission: "alpha.items.read"}) {
		t.Error("item ber-permission yang diizinkan harus dikirim")
	}
}

func TestList_AuthenticatedItemsDoNotLeakAcrossPrincipals(t *testing.T) {
	// Dua pengguna login memperoleh daftar yang sama untuk item authenticated, dan hanya yang berizin melihat item
	// ber-permission; tidak ada bocoran lintas pengguna.
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(authenticatedItem("b"), item("alpha", "a", "read"))))
	svc := newService(reg, statuses{"acme.alpha": runtime.StatusEnabled}, nil)
	dA, _ := find(mustList(t, svc, withUser(ctxFor(tenantA, "alpha.items.read"), "user-a")), "alpha")
	dB, _ := find(mustList(t, svc, withUser(ctxFor(tenantA), "user-b")), "alpha")
	eq(t, menuIDs(dA), []string{"dashboard", "b", "a"})
	eq(t, menuIDs(dB), []string{"dashboard", "b"})
}
