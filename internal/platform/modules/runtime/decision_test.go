package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/internal/platform/modules/registry"
	"erpora/sdk/identity"
)

type mfaFake struct {
	st    MFAState
	err   error
	calls int
}

func (f *mfaFake) MFAState(context.Context, string) (MFAState, error) {
	f.calls++
	return f.st, f.err
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func human(amr ...string) context.Context {
	return identity.WithPrincipal(context.Background(), identity.Principal{
		UserID: "u1", Kind: identity.KindHuman, SessionID: "s1", AuthMethod: identity.AuthMethodPassword, AMR: amr,
	})
}

func newTyped(t *testing.T, perm Authorizer, mfa MFAStateReader) *TypedAuthorizer {
	t.Helper()
	a, err := NewTypedAuthorizer(perm, mfa, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDecision_ZeroValueFailsClosed(t *testing.T) {
	var d Decision
	if d == Allow {
		t.Fatal("nilai nol Decision tidak boleh Allow")
	}
}

func TestDecide_Matrix(t *testing.T) {
	pwd, both := []string{identity.AMRPassword}, []string{identity.AMRPassword, identity.AMROTP}
	service := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "svc", Kind: identity.KindService})
	scoped := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", Kind: identity.KindHuman, AMR: both, TokenScope: []string{"mfa"}})
	noUser := identity.WithPrincipal(context.Background(), identity.Principal{Kind: identity.KindHuman})
	cases := []struct {
		name  string
		ctx   context.Context
		state MFAState
		opts  RouteOptions
		perm  Authorizer
		want  Decision
	}{
		{"tanpa principal", context.Background(), MFAState{}, RouteOptions{}, allowAll, Unauthenticated},
		{"principal tanpa user id", noUser, MFAState{}, RouteOptions{}, allowAll, Unauthenticated},
		{"pengguna tanpa faktor, bukan admin", human(pwd...), MFAState{}, RouteOptions{}, allowAll, Allow},
		{"permission ditolak", human(pwd...), MFAState{}, RouteOptions{}, denyAll, Forbidden},
		// ADR-0013 §4 butir 1: faktor aktif belum terverifikasi (bukan hanya admin).
		{"non-admin faktor aktif, hanya sandi: route biasa", human(pwd...), MFAState{FactorActive: true}, RouteOptions{}, allowAll, MFARequired},
		{"faktor aktif belum verifikasi: mfa/verify", human(pwd...), MFAState{FactorActive: true}, RouteOptions{PendingVerify: true}, allowAll, Allow},
		{"faktor aktif belum verifikasi: mfa/enroll ditolak", human(pwd...), MFAState{FactorActive: true}, RouteOptions{PendingEnroll: true}, allowAll, MFARequired},
		{"faktor aktif, sudah otp", human(both...), MFAState{FactorActive: true}, RouteOptions{}, allowAll, Allow},
		// butir 2: administrator tanpa faktor.
		{"admin tanpa faktor: route biasa", human(pwd...), MFAState{EnrollmentRequired: true}, RouteOptions{}, allowAll, MFARequired},
		{"admin tanpa faktor: mfa/enroll", human(pwd...), MFAState{EnrollmentRequired: true}, RouteOptions{PendingEnroll: true}, allowAll, Allow},
		{"admin tanpa faktor: mfa/verify ditolak", human(pwd...), MFAState{EnrollmentRequired: true}, RouteOptions{PendingVerify: true}, allowAll, MFARequired},
		{"admin faktor aktif belum verifikasi: logout", human(pwd...), MFAState{EnrollmentRequired: true, FactorActive: true}, RouteOptions{PendingVerify: true, PendingEnroll: true}, allowAll, Allow},
		// butir 4: RequireMFA.
		{"RequireMFA tanpa otp, tanpa faktor", human(pwd...), MFAState{}, RouteOptions{RequireMFA: true}, allowAll, MFARequired},
		{"RequireMFA dengan otp", human(both...), MFAState{FactorActive: true}, RouteOptions{RequireMFA: true}, allowAll, Allow},
		{"MFA dicek sebelum permission", human(pwd...), MFAState{FactorActive: true}, RouteOptions{}, denyAll, MFARequired},
		{"token ber-scope terbatas", scoped, MFAState{}, RouteOptions{}, allowAll, Forbidden},
		{"akun layanan tanpa MFA", service, MFAState{FactorActive: true, EnrollmentRequired: true}, RouteOptions{}, allowAll, Allow},
		{"akun layanan RequireMFA", service, MFAState{}, RouteOptions{RequireMFA: true}, allowAll, MFARequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newTyped(t, c.perm, &mfaFake{st: c.state})
			if got := a.Decide(c.ctx, "x.y", c.opts); got != c.want {
				t.Fatalf("Decide = %s, want %s", got, c.want)
			}
		})
	}
}

func TestDecide_VerifiedSessionDoesNotQueryMFAState(t *testing.T) {
	f := &mfaFake{}
	a := newTyped(t, allowAll, f)
	if got := a.Decide(human(identity.AMRPassword, identity.AMROTP), "x.y", RouteOptions{}); got != Allow {
		t.Fatalf("Decide = %s", got)
	}
	if f.calls != 0 {
		t.Fatalf("sesi terverifikasi tidak boleh memicu pencarian MFA, calls = %d", f.calls)
	}
}

func TestDecide_MFAStateErrorFailsClosed(t *testing.T) {
	a := newTyped(t, allowAll, &mfaFake{err: errors.New("db mati")})
	if got := a.Decide(human(identity.AMRPassword), "x.y", RouteOptions{}); got != MFARequired {
		t.Fatalf("galat pembacaan keadaan harus gagal tertutup, got %s", got)
	}
}

func TestNewTypedAuthorizer_RequiresDependencies(t *testing.T) {
	if _, err := NewTypedAuthorizer(nil, &mfaFake{}, nil); err == nil {
		t.Error("tanpa pemeriksa permission harus galat")
	}
	if _, err := NewTypedAuthorizer(allowAll, nil, nil); err == nil {
		t.Error("tanpa MFAStateReader harus galat")
	}
}

func TestTypedAuthorizer_AllowAdaptor(t *testing.T) {
	a := newTyped(t, allowAll, &mfaFake{})
	if !a.Allow(human(identity.AMRPassword), "x.y") {
		t.Error("Allow lama harus true untuk Principal sah")
	}
	if a.Allow(context.Background(), "x.y") {
		t.Error("Allow lama harus false tanpa Principal")
	}
}

type deciderFn func(context.Context, string, RouteOptions) Decision

func (f deciderFn) Allow(context.Context, string) bool { return false }
func (f deciderFn) Decide(ctx context.Context, p string, o RouteOptions) Decision {
	return f(ctx, p, o)
}

func TestPermissionMW_MapsDecisionToResponse(t *testing.T) {
	cases := []struct {
		d       Decision
		status  int
		code    string
		wwwAuth bool
	}{
		{Allow, 200, "", false},
		{Unauthenticated, 401, "unauthenticated", true},
		{TokenExpired, 401, "token_expired", true},
		{Forbidden, 403, "permission_denied", false},
		{MFARequired, 403, "mfa_required", false},
		{Decision(99), 403, "permission_denied", false},
	}
	for _, c := range cases {
		t.Run(c.d.String(), func(t *testing.T) {
			called := false
			h := permissionMW(deciderFn(func(context.Context, string, RouteOptions) Decision { return c.d }), "x.y", RouteOptions{}, quietLog(),
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
			if called != (c.d == Allow) {
				t.Fatalf("handler dipanggil = %v", called)
			}
			if c.d == Allow {
				return
			}
			if rec.Code != c.status || errCode(t, rec) != c.code {
				t.Fatalf("respons = %d %q (%s)", rec.Code, errCode(t, rec), rec.Body)
			}
			if (rec.Header().Get("WWW-Authenticate") != "") != c.wwwAuth {
				t.Errorf("WWW-Authenticate = %q", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestPermissionMW_AllowOnlyAuthorizerStaysBackwardCompatible(t *testing.T) {
	h := permissionMW(denyAll, "x.y", RouteOptions{}, quietLog(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != 403 || errCode(t, rec) != "permission_denied" {
		t.Fatalf("respons = %d %s", rec.Code, rec.Body)
	}
}

func TestRouteOptionsFor_PendingListsOnlyForCoreModule(t *testing.T) {
	logout := registry.RouteInfo{Module: CoreModuleKey, Method: "POST", Pattern: "/api/core/auth/logout"}
	if o := routeOptionsFor(logout); !o.PendingVerify || !o.PendingEnroll {
		t.Errorf("logout core = %+v", o)
	}
	verify := registry.RouteInfo{Module: CoreModuleKey, Method: "POST", Pattern: "/api/core/auth/mfa/verify"}
	if o := routeOptionsFor(verify); !o.PendingVerify || o.PendingEnroll {
		t.Errorf("mfa/verify = %+v", o)
	}
	enroll := registry.RouteInfo{Module: CoreModuleKey, Method: "POST", Pattern: "/api/core/auth/mfa/enroll"}
	if o := routeOptionsFor(enroll); o.PendingVerify || !o.PendingEnroll {
		t.Errorf("mfa/enroll = %+v", o)
	}
	// Modul lain dengan path/slug serupa TIDAK mendapat pelonggaran.
	fake := registry.RouteInfo{Module: "evil.core", Method: "POST", Pattern: "/api/core/auth/logout"}
	if o := routeOptionsFor(fake); o.PendingVerify || o.PendingEnroll {
		t.Errorf("modul non-core tidak boleh mendapat Pending*: %+v", o)
	}
	// Method lain pada path yang sama tidak mendapat pelonggaran.
	get := registry.RouteInfo{Module: CoreModuleKey, Method: "GET", Pattern: "/api/core/auth/logout"}
	if o := routeOptionsFor(get); o.PendingVerify || o.PendingEnroll {
		t.Errorf("GET logout tidak boleh pending: %+v", o)
	}
	// RequireMFA dari route diteruskan.
	if o := routeOptionsFor(registry.RouteInfo{Module: "acme.x", RequireMFA: true}); !o.RequireMFA {
		t.Error("RequireMFA tidak diteruskan")
	}
}

func TestIsAuthEndpoint(t *testing.T) {
	cases := map[string]bool{
		CoreModuleKey + "|/api/core/auth/login":      true,
		CoreModuleKey + "|/api/core/auth/mfa/verify": true,
		CoreModuleKey + "|/api/core/users":           false,
		CoreModuleKey + "|/api/core/auth/":           false,
		"evil.core|/api/core/auth/login":             false,
		"acme.notes|/api/notes/items":                false,
	}
	for k, want := range cases {
		mod, pat, _ := strings.Cut(k, "|")
		if got := isAuthEndpoint(registry.RouteInfo{Module: mod, Pattern: pat}); got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
}
