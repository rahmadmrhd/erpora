//go:build integration

package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/config"
	"erpora/internal/platform/identity"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/session"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

// route platform yang butuh autentikasi dan, pada development, diizinkan stub permission.
const protectedPath = "/api/platform/modules"

func bearerHdr(raw string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + raw}
}

// idEnv menyiapkan pengguna nyata di database dan penerbit token untuk tenant assembly.
type idEnv struct {
	t   *testing.T
	a   *assembly
	ctx context.Context // membawa tenant
}

func newIDEnv(t *testing.T) *idEnv {
	t.Helper()
	ctx, a, _, _ := setupAssembly(t)
	return &idEnv{t: t, a: a, ctx: tenancy.WithTenant(ctx, a.Tenant)}
}

func (e *idEnv) user(admin bool) identitystore.User {
	e.t.Helper()
	st := e.a.Identity.Store
	u, err := st.CreateUser(e.ctx, e.a.Tenant, identitystore.NewUser{Email: uuid.NewString() + "@example.test", Name: "Uji", Status: identitystore.UserActive})
	if err != nil {
		e.t.Fatalf("CreateUser: %v", err)
	}
	if admin {
		if _, err := st.GrantRole(e.ctx, e.a.Tenant, u.ID, mfa.AdministratorRole, time.Now()); err != nil {
			e.t.Fatalf("GrantRole: %v", err)
		}
	}
	return u
}

// enrollFactor membuat faktor TOTP terkonfirmasi untuk userID (tanpa lewat alur HTTP, yang belum ada pada C1.10).
func (e *idEnv) enrollFactor(userID string) {
	e.t.Helper()
	st := e.a.Identity.Store
	sealed, err := e.a.Identity.Sealer.Seal([]byte("0123456789abcdef0123"), []byte("user_mfa_factors|secret|"+userID))
	if err != nil {
		e.t.Fatal(err)
	}
	if ok, err := st.UpsertPendingMFAFactor(e.ctx, e.a.Tenant, userID, sealed, time.Now()); err != nil || !ok {
		e.t.Fatalf("UpsertPendingMFAFactor = %v, %v", ok, err)
	}
	if ok, err := st.ConfirmMFAFactor(e.ctx, e.a.Tenant, userID, 1, time.Now()); err != nil || !ok {
		e.t.Fatalf("ConfirmMFAFactor = %v, %v", ok, err)
	}
}

func (e *idEnv) token(userID, sid string, amr ...string) string {
	e.t.Helper()
	raw, err := e.a.Identity.Issuer.Issue(token.Claims{Subject: userID, SessionID: sid, TenantID: e.a.Tenant.String(), AMR: amr})
	if err != nil {
		e.t.Fatal(err)
	}
	return raw
}

// startSession membuat sesi sungguhan (agar dapat dicabut lewat session.Service) dan menerbitkan token untuknya.
func (e *idEnv) startSession(userID string, amr ...string) (sid, raw string) {
	e.t.Helper()
	s, _, err := e.a.Identity.Sessions.Start(e.ctx, session.StartInput{UserID: userID, AMR: amr})
	if err != nil {
		e.t.Fatalf("Start: %v", err)
	}
	return s.ID, e.token(userID, s.ID, amr...)
}

func TestIdentity_NoStubAuthN_EveryProtectedRouteNeedsToken(t *testing.T) {
	e := newIDEnv(t)
	rec := get(e.a.Router, protectedPath, nil)
	if rec.Code != http.StatusUnauthorized || errCodeOf(t, rec) != "unauthenticated" {
		t.Fatalf("tanpa token: %d %s", rec.Code, rec.Body)
	}
	// Header palsu bukan identitas: tidak ada jalur header-berbasis ke Principal (IDOR).
	rec = get(e.a.Router, protectedPath, map[string]string{"X-User-Id": uuid.NewString(), "X-Actor": "api:dev"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("header identitas palsu: %d", rec.Code)
	}
	// Rute publik tetap terbuka tanpa token.
	for _, p := range []string{"/healthz", "/api/bootstrap"} {
		if rec := get(e.a.Router, p, nil); rec.Code != http.StatusOK {
			t.Errorf("%s publik = %d", p, rec.Code)
		}
	}
}

func TestIdentity_ValidTokenRealUserPasses_SessionRevocationIsImmediate(t *testing.T) {
	e := newIDEnv(t)
	u := e.user(false)
	sid, raw := e.startSession(u.ID, "pwd")
	if rec := get(e.a.Router, protectedPath, bearerHdr(raw)); rec.Code != http.StatusOK {
		t.Fatalf("token sah: %d %s", rec.Code, rec.Body)
	}
	if err := e.a.Identity.Sessions.Revoke(e.ctx, sid, session.ReasonLogout); err != nil {
		t.Fatal(err)
	}
	rec := get(e.a.Router, protectedPath, bearerHdr(raw))
	if rec.Code != http.StatusUnauthorized || errCodeOf(t, rec) != "unauthenticated" {
		t.Fatalf("sesi tercabut harus segera 401 generik: %d %s", rec.Code, rec.Body)
	}
}

func TestIdentity_TenantMismatchAndExpiredTokens(t *testing.T) {
	e := newIDEnv(t)
	u := e.user(false)
	other, err := e.a.Identity.Issuer.Issue(token.Claims{Subject: u.ID, SessionID: uuid.NewString(), TenantID: uuid.NewString(), AMR: []string{"pwd"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(e.a.Router, protectedPath, bearerHdr(other)); rec.Code != http.StatusUnauthorized || errCodeOf(t, rec) != "unauthenticated" {
		t.Fatalf("tid berbeda: %d %s", rec.Code, rec.Body)
	}
	old, err := token.NewIssuer(token.IssuerConfig{
		Keyring: e.a.Identity.Keyring, Issuer: identity.TokenIssuer, Audience: identity.TokenAudience,
		Now: func() time.Time { return time.Now().Add(-time.Hour) },
	})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := old.Issue(token.Claims{Subject: u.ID, SessionID: uuid.NewString(), TenantID: e.a.Tenant.String(), AMR: []string{"pwd"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(e.a.Router, protectedPath, bearerHdr(expired)); rec.Code != http.StatusUnauthorized || errCodeOf(t, rec) != "token_expired" {
		t.Fatalf("token kedaluwarsa: %d %s", rec.Code, rec.Body)
	}
}

// ADR-0013 §4 dengan pencarian status MFA sungguhan dari database.
func TestIdentity_MFARulesAgainstDatabase(t *testing.T) {
	e := newIDEnv(t)
	sid := uuid.NewString()

	t.Run("pengguna biasa tanpa faktor lolos dengan sandi saja", func(t *testing.T) {
		u := e.user(false)
		if rec := get(e.a.Router, protectedPath, bearerHdr(e.token(u.ID, sid, "pwd"))); rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("administrator tanpa faktor: 403 mfa_required (hanya mfa/enroll, confirm, logout yang boleh)", func(t *testing.T) {
		u := e.user(true)
		rec := get(e.a.Router, protectedPath, bearerHdr(e.token(u.ID, sid, "pwd")))
		if rec.Code != http.StatusForbidden || errCodeOf(t, rec) != "mfa_required" {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
	t.Run("pengguna non-admin dengan TOTP aktif tidak lolos hanya dengan sandi", func(t *testing.T) {
		u := e.user(false)
		e.enrollFactor(u.ID)
		e.a.Identity.MFAState.Invalidate(e.a.Tenant, u.ID)
		rec := get(e.a.Router, protectedPath, bearerHdr(e.token(u.ID, sid, "pwd")))
		if rec.Code != http.StatusForbidden || errCodeOf(t, rec) != "mfa_required" {
			t.Fatalf("sandi saja: %d %s", rec.Code, rec.Body)
		}
		if rec := get(e.a.Router, protectedPath, bearerHdr(e.token(u.ID, sid, "pwd", "otp"))); rec.Code != http.StatusOK {
			t.Fatalf("pwd+otp: %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("administrator berfaktor aktif: sandi saja ditolak, pwd+otp lolos", func(t *testing.T) {
		u := e.user(true)
		e.enrollFactor(u.ID)
		e.a.Identity.MFAState.Invalidate(e.a.Tenant, u.ID)
		if rec := get(e.a.Router, protectedPath, bearerHdr(e.token(u.ID, sid, "pwd"))); rec.Code != http.StatusForbidden || errCodeOf(t, rec) != "mfa_required" {
			t.Fatalf("sandi saja: %d %s", rec.Code, rec.Body)
		}
		if rec := get(e.a.Router, protectedPath, bearerHdr(e.token(u.ID, sid, "pwd", "otp"))); rec.Code != http.StatusOK {
			t.Fatalf("pwd+otp: %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("pengguna tak dikenal di database tidak punya faktor dan bukan admin", func(t *testing.T) {
		if rec := get(e.a.Router, protectedPath, bearerHdr(e.token(uuid.NewString(), sid, "pwd"))); rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
}

func TestIdentity_JWKSRouteIsPublicAndLeaksNoPrivateKey(t *testing.T) {
	e := newIDEnv(t)
	rec := get(e.a.Router, jwksPath, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("%d Cache-Control=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	var jwks struct{ Keys []map[string]any }
	if err := json.Unmarshal(rec.Body.Bytes(), &jwks); err != nil || len(jwks.Keys) == 0 {
		t.Fatalf("%v %s", err, rec.Body)
	}
	for _, k := range jwks.Keys {
		if _, priv := k["d"]; priv {
			t.Fatal("JWKS memuat parameter privat d")
		}
	}
	// Kid aktif muncul di JWKS dan tidak ada sisa material privat apa pun pada badan respons.
	if !strings.Contains(rec.Body.String(), e.a.Identity.Keyring.Active().ID) {
		t.Error("kid aktif tidak ada di JWKS")
	}
	// JWKS adalah satu-satunya rute di luar /api yang publik; metode lain ditolak.
	req := httptest.NewRequest(http.MethodPost, jwksPath, nil)
	rec2 := httptest.NewRecorder()
	e.a.Router.ServeHTTP(rec2, req)
	if rec2.Code == http.StatusOK {
		t.Errorf("POST JWKS = %d", rec2.Code)
	}
}

// serve gagal tertutup tanpa ERP_SECRET_KEY (ADR-0013 §3): kunci privat JWT tidak dapat dibuka/disegel.
func TestIdentity_AssembleFailsClosedWithoutSecretKey(t *testing.T) {
	dsn := pgtest.Start(t)
	db := openTestDB(t, dsn)
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, AuthzStub: "allow", DBSlowQuery: 200 * time.Millisecond}
	if err := migrate.Up(context.Background(), db, cfg.Env); err != nil {
		t.Fatal(err)
	}
	reg, api, err := buildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := assemble(ctx, cfg, testLog(), db, testProviders(), reg, api); err == nil || !strings.Contains(err.Error(), "ERP_SECRET_KEY") {
		t.Fatalf("assemble tanpa ERP_SECRET_KEY harus gagal menyebut ERP_SECRET_KEY, err = %v", err)
	}
}
