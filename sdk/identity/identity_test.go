package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"erpora/sdk/identity"
)

func TestFromContextEmpty(t *testing.T) {
	if p, ok := identity.FromContext(context.Background()); ok || p.UserID != "" {
		t.Fatalf("ctx kosong harus tanpa principal, got %+v ok=%v", p, ok)
	}
}

func TestWithPrincipalRoundTrip(t *testing.T) {
	in := identity.Principal{
		UserID: "u1", Kind: identity.KindHuman, SessionID: "s1", AuthMethod: identity.AuthMethodPassword,
		AMR: []string{identity.AMRPassword, identity.AMROTP}, TokenScope: []string{"read"},
	}
	ctx := identity.WithPrincipal(context.Background(), in)
	got, ok := identity.FromContext(ctx)
	if !ok {
		t.Fatal("principal tidak ditemukan")
	}
	if got.UserID != in.UserID || got.Kind != in.Kind || got.SessionID != in.SessionID ||
		got.AuthMethod != identity.AuthMethodPassword || !got.MFAVerified() ||
		len(got.TokenScope) != 1 || got.TokenScope[0] != "read" ||
		len(got.AMR) != 2 || got.AMR[0] != "pwd" || got.AMR[1] != "otp" {
		t.Fatalf("got %+v", got)
	}
}

func TestPrincipalIsValueCopy(t *testing.T) {
	scope := []string{"read", "write"}
	amr := []string{"pwd", "otp"}
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", TokenScope: scope, AMR: amr})
	scope[0] = "admin" // mutasi slice asal setelah WithPrincipal tidak boleh merambat
	amr[1] = "x"

	got, _ := identity.FromContext(ctx)
	if got.TokenScope[0] != "read" || got.AMR[1] != "otp" {
		t.Fatalf("mutasi slice asal merambat ke context: %v %v", got.TokenScope, got.AMR)
	}
	got.UserID = "x"
	got.TokenScope[1] = "admin" // mutasi hasil FromContext tidak boleh merambat
	got.AMR[0] = "x"
	again, _ := identity.FromContext(ctx)
	if again.UserID != "u1" || again.TokenScope[1] != "write" || again.AMR[0] != "pwd" {
		t.Fatalf("mutasi hasil FromContext merambat: %+v", again)
	}
}

func TestAuthMethodConstants(t *testing.T) {
	want := map[identity.AuthMethod]string{
		identity.AuthMethodPassword:          "password",
		identity.AuthMethodOIDC:              "oidc",
		identity.AuthMethodPAT:               "pat",
		identity.AuthMethodClientCredentials: "client_credentials",
	}
	for got, s := range want {
		if string(got) != s {
			t.Errorf("AuthMethod %q != %q", got, s)
		}
	}
}

func TestRequirePrincipal(t *testing.T) {
	if _, err := identity.RequirePrincipal(context.Background()); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", SessionID: "s1"})
	p, err := identity.RequirePrincipal(ctx)
	if err != nil || p.SessionID != "s1" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	// Principal tanpa UserID tidak dianggap terautentikasi.
	empty := identity.WithPrincipal(context.Background(), identity.Principal{})
	if _, err := identity.RequirePrincipal(empty); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("principal kosong err = %v, want ErrUnauthenticated", err)
	}
}

func TestLoginResultJSON(t *testing.T) {
	raw, err := json.Marshal(identity.LoginResult{
		AccessToken: "acc", ExpiresIn: 600, MFAState: identity.MFAVerifyRequired,
		RefreshToken: "SECRET-REFRESH", RefreshExpiresAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "SECRET-REFRESH") || strings.Contains(strings.ToLower(s), "refresh") {
		t.Fatalf("data refresh bocor ke JSON: %s", s)
	}
	for _, w := range []string{`"accessToken":"acc"`, `"expiresIn":600`, `"mfaState":"verify_required"`} {
		if !strings.Contains(s, w) {
			t.Fatalf("JSON tidak memuat %s: %s", w, s)
		}
	}
}

// Nama lama diganti: refresh token tetap tidak boleh ter-serialisasi.
func TestLoginResultRefreshTokenNotSerialized(t *testing.T) {
	raw, _ := json.Marshal(identity.LoginResult{RefreshToken: "SECRET-REFRESH"})
	if strings.Contains(string(raw), "SECRET-REFRESH") {
		t.Fatalf("refresh token bocor: %s", raw)
	}
}

func TestMFAState(t *testing.T) {
	if identity.MFANone != "none" || identity.MFAEnrollRequired != "enroll_required" || identity.MFAVerifyRequired != "verify_required" {
		t.Fatal("nilai MFAState berubah")
	}
	if (identity.LoginResult{MFAState: identity.MFANone}).MFARequired() ||
		(identity.LoginResult{}).MFARequired() {
		t.Error("none/nol tidak boleh MFARequired")
	}
	if !(identity.LoginResult{MFAState: identity.MFAEnrollRequired}).MFARequired() ||
		!(identity.LoginResult{MFAState: identity.MFAVerifyRequired}).MFARequired() {
		t.Error("enroll/verify harus MFARequired")
	}
}

type fakeService struct{ identity.Service }

func TestServiceContextRoundTrip(t *testing.T) {
	if _, ok := identity.ServiceFrom(context.Background()); ok {
		t.Fatal("ctx kosong tidak boleh membawa service")
	}
	svc := &fakeService{}
	ctx := identity.WithService(context.Background(), svc)
	got, ok := identity.ServiceFrom(ctx)
	if !ok || got != identity.Service(svc) {
		t.Fatalf("round trip gagal: %v %v", got, ok)
	}
	if _, ok := identity.ServiceFrom(identity.WithService(context.Background(), nil)); ok {
		t.Fatal("service nil dianggap tidak ada")
	}
}

type fakeAdmin struct{ identity.Admin }

type mapAdmin map[string]string

func (mapAdmin) CreateAdmin(context.Context, identity.CreateAdminInput) (identity.UserSummary, error) {
	return identity.UserSummary{}, nil
}
func (mapAdmin) RotateSigningKeys(context.Context) (identity.KeyInfo, error) {
	return identity.KeyInfo{}, nil
}
func (mapAdmin) ResetMFA(context.Context, string) error         { return nil }
func (mapAdmin) UnlockAccount(context.Context, string) error    { return nil }
func (mapAdmin) LogoutAllDevices(context.Context, string) error { return nil }

func TestAdminContextRoundTrip(t *testing.T) {
	if _, ok := identity.AdminFrom(context.Background()); ok {
		t.Fatal("ctx kosong tidak boleh membawa admin")
	}
	a := &fakeAdmin{}
	got, ok := identity.AdminFrom(identity.WithAdmin(context.Background(), a))
	if !ok || got != identity.Admin(a) {
		t.Fatalf("round trip gagal: %v %v", got, ok)
	}
	if _, ok := identity.AdminFrom(identity.WithAdmin(context.Background(), nil)); ok {
		t.Fatal("admin nil dianggap ada")
	}
}

// Nil bertipe di balik antarmuka tidak boleh dianggap ada dan tidak boleh panik (M-7).
func TestFromRejectsTypedNil(t *testing.T) {
	var ps *fakeService
	var pa *fakeAdmin
	var ma mapAdmin
	if _, ok := identity.ServiceFrom(identity.WithService(context.Background(), ps)); ok {
		t.Error("(*fakeService)(nil) dianggap ada")
	}
	if _, ok := identity.AdminFrom(identity.WithAdmin(context.Background(), pa)); ok {
		t.Error("(*fakeAdmin)(nil) dianggap ada")
	}
	if _, ok := identity.AdminFrom(identity.WithAdmin(context.Background(), ma)); ok {
		t.Error("map nil dianggap ada")
	}
	if _, ok := identity.AdminFrom(identity.WithAdmin(context.Background(), mapAdmin{})); !ok {
		t.Error("map non-nil ditolak")
	}
}

// D9: paket SDK identity hanya boleh mengimpor stdlib (tanpa titik pada elemen pertama path impor).
func TestOnlyStdlibImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") || first == "erpora" {
				t.Errorf("%s mengimpor non-stdlib %s", name, path)
			}
		}
	}
}
