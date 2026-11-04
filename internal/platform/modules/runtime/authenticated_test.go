package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"erpora/internal/platform/modules/registry"
	"erpora/sdk/identity"
)

// PRD-001 Q29: route AuthenticatedOnly melewati pemeriksaan permission, tetapi Principal dan aturan MFA ADR-0013 §4
// tetap berlaku (hanya daftar route auth core yang boleh dipanggil sesi menunggu MFA).
func TestDecide_AuthenticatedOnlyBypassesPermissionButNotIdentityRules(t *testing.T) {
	pwd, both := []string{identity.AMRPassword}, []string{identity.AMRPassword, identity.AMROTP}
	scoped := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", Kind: identity.KindHuman, AMR: both, TokenScope: []string{"mfa"}})
	auth := RouteOptions{AuthenticatedOnly: true}
	cases := []struct {
		name  string
		ctx   context.Context
		state MFAState
		opts  RouteOptions
		want  Decision
	}{
		{"permission ditolak semua, tetap Allow", human(pwd...), MFAState{}, auth, Allow},
		{"tanpa principal: 401", context.Background(), MFAState{}, auth, Unauthenticated},
		{"token ber-scope terbatas", scoped, MFAState{}, auth, Forbidden},
		{"faktor aktif belum verifikasi: tetap mfa_required", human(pwd...), MFAState{FactorActive: true}, auth, MFARequired},
		{"faktor aktif belum verifikasi: daftar PendingVerify", human(pwd...), MFAState{FactorActive: true}, RouteOptions{AuthenticatedOnly: true, PendingVerify: true}, Allow},
		{"admin tanpa faktor: tetap mfa_required", human(pwd...), MFAState{EnrollmentRequired: true}, auth, MFARequired},
		{"admin tanpa faktor: daftar PendingEnroll", human(pwd...), MFAState{EnrollmentRequired: true}, RouteOptions{AuthenticatedOnly: true, PendingEnroll: true}, Allow},
		{"RequireMFA tanpa otp", human(pwd...), MFAState{}, RouteOptions{AuthenticatedOnly: true, RequireMFA: true}, MFARequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newTyped(t, denyAll, &mfaFake{st: c.state}) // stub deny: permission tidak boleh menentukan
			if got := a.Decide(c.ctx, "", c.opts); got != c.want {
				t.Errorf("Decide = %v, want %v", got, c.want)
			}
		})
	}
	// Route biasa dengan stub deny tetap Forbidden (opsi tidak bocor ke route lain).
	if got := newTyped(t, denyAll, &mfaFake{}).Decide(human(pwd...), "x.y", RouteOptions{}); got != Forbidden {
		t.Errorf("route biasa = %v, want Forbidden", got)
	}
}

// routeOptionsFor meneruskan Authenticated dari registry; pelonggaran MFA tetap hanya untuk route auth core.
func TestRouteOptionsFor_Authenticated(t *testing.T) {
	rt := registry.RouteInfo{Module: "acme.other", Method: "POST", Pattern: "/api/core/auth/logout", Authenticated: true}
	o := routeOptionsFor(rt)
	if !o.AuthenticatedOnly || o.PendingVerify || o.PendingEnroll {
		t.Errorf("modul non-core: %+v", o)
	}
	rt.Module = CoreModuleKey
	if o := routeOptionsFor(rt); !o.AuthenticatedOnly || !o.PendingVerify || !o.PendingEnroll {
		t.Errorf("logout core: %+v", o)
	}
}

type allowOnly struct{ allow bool }

func (a allowOnly) Allow(context.Context, string) bool { return a.allow }

// Authorizer lama (hanya Allow, tanpa Decide): route AuthenticatedOnly tetap mewajibkan Principal (401) dan melewati
// permission; route biasa tetap tunduk pada Allow.
func TestPermissionMW_AuthenticatedOnlyWithLegacyAuthorizer(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	run := func(authz Authorizer, opts RouteOptions, ctx context.Context) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		permissionMW(authz, "", opts, quietLog(), next).ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil).WithContext(ctx))
		return rec
	}
	if rec := run(allowOnly{false}, RouteOptions{AuthenticatedOnly: true}, human()); rec.Code != http.StatusNoContent {
		t.Errorf("terautentikasi, stub deny = %d, want 204", rec.Code)
	}
	if rec := run(allowOnly{true}, RouteOptions{AuthenticatedOnly: true}, context.Background()); rec.Code != http.StatusUnauthorized {
		t.Errorf("tanpa principal = %d, want 401", rec.Code)
	}
	if rec := run(allowOnly{false}, RouteOptions{}, human()); rec.Code != http.StatusForbidden {
		t.Errorf("route biasa, stub deny = %d, want 403", rec.Code)
	}
}
