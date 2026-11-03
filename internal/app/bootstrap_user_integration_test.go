//go:build integration

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

type bootUser struct {
	Data struct {
		User    map[string]any
		PermRev *int64
	}
}

// bootstrap wajib login; dengan token mengembalikan user (id, email, nama tampilan) dan permRev tanpa data sensitif.
func TestBootstrapAuth_RequiresTokenAndReturnsUserAndPermRev(t *testing.T) {
	e := newIDEnv(t)
	for name, hdr := range map[string]map[string]string{
		"tanpa token":   nil,
		"token buruk":   {"Authorization": "Bearer bukan.token.jwt"},
		"skema salah":   {"Authorization": "Basic Zm9vOmJhcg=="},
		"header tiruan": {"X-User-Id": uuid.NewString()},
	} {
		rec := get(e.a.Router, "/api/bootstrap", hdr)
		if rec.Code != http.StatusUnauthorized || errCodeOf(t, rec) != "unauthenticated" {
			t.Errorf("%s = %d %s", name, rec.Code, rec.Body)
		}
	}
	u := e.user(false)
	rec := get(e.a.Router, "/api/bootstrap", bearerHdr(e.token(u.ID, uuid.NewString(), "pwd")))
	if rec.Code != http.StatusOK {
		t.Fatalf("dengan token = %d %s", rec.Code, rec.Body)
	}
	var b bootUser
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Data.User["id"] != u.ID || b.Data.User["email"] != u.Email || b.Data.User["displayName"] != u.Name || len(b.Data.User) != 3 {
		t.Errorf("user = %v, want id/email/displayName milik %s", b.Data.User, u.ID)
	}
	if b.Data.PermRev == nil || *b.Data.PermRev != 0 {
		t.Errorf("permRev = %v, want 0 (stub sampai C2)", b.Data.PermRev)
	}
	// Tanpa data sensitif: tidak ada hash sandi, status akun, atau penanda keamanan di mana pun pada respons.
	for _, bad := range []string{"password", "hash", "argon2", "status\":\"active", "lockedAt", "mfa"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(bad)) {
			t.Errorf("respons memuat %q: %s", bad, rec.Body)
		}
	}
	if !strings.Contains(rec.Body.String(), `"permissions":[`) {
		t.Error("permissions tetap ada sampai C2 (perubahan hanya aditif)")
	}
}

// Cache dan IDOR: ETag per pengguna, private + Vary: Authorization, tidak ada respons/ETag/304 lintas pengguna.
func TestBootstrapAuth_PerUserETagCachingAndNoCrossUserData(t *testing.T) {
	e := newIDEnv(t)
	a, b := e.user(false), e.user(false)
	recA, recB := get(e.a.authedAs(t, a.ID), "/api/bootstrap", nil), get(e.a.authedAs(t, b.ID), "/api/bootstrap", nil)
	if recA.Code != 200 || recB.Code != 200 {
		t.Fatalf("A=%d B=%d", recA.Code, recB.Code)
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{"A": recA, "B": recB} {
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "private") {
			t.Errorf("%s: Cache-Control = %q", name, cc)
		}
		if v := rec.Header().Get("Vary"); !strings.Contains(v, "Authorization") {
			t.Errorf("%s: Vary = %q", name, v)
		}
	}
	etagA, etagB := recA.Header().Get("ETag"), recB.Header().Get("ETag")
	if etagA == "" || etagA == etagB {
		t.Fatalf("ETag A=%q B=%q harus berbeda", etagA, etagB)
	}
	// User A tidak pernah melihat data user B, dan sebaliknya.
	if strings.Contains(recA.Body.String(), b.Email) || strings.Contains(recA.Body.String(), b.ID) ||
		strings.Contains(recB.Body.String(), a.Email) || strings.Contains(recB.Body.String(), a.ID) {
		t.Errorf("data lintas pengguna bocor: A=%s B=%s", recA.Body, recB.Body)
	}
	// If-None-Match milik A tidak menghasilkan 304 untuk B (dan B mendapat datanya sendiri); untuk A sendiri 304.
	hdr := map[string]string{"If-None-Match": etagA}
	if rec := get(e.a.authedAs(t, b.ID), "/api/bootstrap", hdr); rec.Code != 200 || !strings.Contains(rec.Body.String(), b.Email) {
		t.Errorf("ETag milik A untuk B = %d %s", rec.Code, rec.Body)
	}
	if rec := get(e.a.authedAs(t, a.ID), "/api/bootstrap", hdr); rec.Code != http.StatusNotModified {
		t.Errorf("ETag milik A untuk A = %d, want 304", rec.Code)
	}
	// Identitas hanya dari token: parameter query/header tiruan tidak mengubah pemilik respons.
	forged := get(e.a.authedAs(t, a.ID), "/api/bootstrap?userId="+b.ID, map[string]string{"X-User-Id": b.ID})
	if forged.Code != 200 || !strings.Contains(forged.Body.String(), a.Email) || strings.Contains(forged.Body.String(), b.Email) {
		t.Errorf("identitas tiruan memengaruhi respons: %d %s", forged.Code, forged.Body)
	}
}

// Gagal tertutup: token sah tetapi pengguna tidak ada di tenant, nonaktif, atau bukan anggota tenant: 401, tanpa isi.
func TestBootstrapAuth_UnknownOrDisabledUserFailsClosed(t *testing.T) {
	e := newIDEnv(t)
	unknown := uuid.NewString()
	if rec := get(e.a.authedAs(t, unknown), "/api/bootstrap", nil); rec.Code != http.StatusUnauthorized || errCodeOf(t, rec) != "unauthenticated" {
		t.Errorf("pengguna tak ada = %d %s", rec.Code, rec.Body)
	}
	disabled, err := e.a.Identity.Store.CreateUser(tenancy.WithTenant(e.ctx, e.a.Tenant), e.a.Tenant,
		identitystore.NewUser{Email: uuid.NewString() + "@example.test", Name: "Nonaktif", Status: identitystore.UserDisabled})
	if err != nil {
		t.Fatal(err)
	}
	rec := get(e.a.authedAs(t, disabled.ID), "/api/bootstrap", nil)
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), disabled.Email) {
		t.Errorf("pengguna nonaktif = %d %s", rec.Code, rec.Body)
	}
}

// Q30 butir 4: status locked hanya menolak login baru dan tidak mencabut sesi; bootstrap tetap dilayani untuk sesi yang ada.
func TestBootstrapAuth_LockedUserKeepsExistingSession(t *testing.T) {
	e := newIDEnv(t)
	tctx := tenancy.WithTenant(e.ctx, e.a.Tenant)
	u, err := e.a.Identity.Store.CreateUser(tctx, e.a.Tenant,
		identitystore.NewUser{Email: uuid.NewString() + "@example.test", Name: "Terkunci", Status: identitystore.UserActive})
	if err != nil {
		t.Fatal(err)
	}
	tok := e.a.authedAs(t, u.ID)
	if rec := get(tok, "/api/bootstrap", nil); rec.Code != http.StatusOK {
		t.Fatalf("sebelum dikunci = %d %s", rec.Code, rec.Body)
	}
	if ok, err := e.a.Identity.Store.LockUser(tctx, e.a.Tenant, u.ID, time.Now()); err != nil || !ok {
		t.Fatalf("LockUser = %v %v", ok, err)
	}
	if rec := get(tok, "/api/bootstrap", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), u.Email) {
		t.Errorf("pengguna locked = %d %s, want 200 (sesi yang ada tetap valid)", rec.Code, rec.Body)
	}
}

// Aktor audit aksi modul dan override i18n adalah `user:<id>` dari Principal, per pengguna.
func TestActorAudit_ModuleActionAndI18nOverrideUsePrincipal(t *testing.T) {
	e := newIDEnv(t)
	a, b := e.user(false), e.user(false)
	if rec := call(e.a.authedAs(t, a.ID), "POST", "/api/platform/modules/acme.example/install", ""); rec.Code != 200 {
		t.Fatalf("install = %d %s", rec.Code, rec.Body)
	}
	if rec := call(e.a.authedAs(t, b.ID), "POST", "/api/platform/modules/acme.example/enable", ""); rec.Code != 200 {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	rec := get(e.a.authedAs(t, a.ID), "/api/platform/modules/acme.example/history", nil)
	for _, w := range []string{`"actor":"user:` + a.ID + `"`, `"actor":"user:` + b.ID + `"`} {
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), w) {
			t.Errorf("history tidak memuat %s: %d %s", w, rec.Code, rec.Body)
		}
	}
	if strings.Contains(rec.Body.String(), "api:dev") {
		t.Error("aktor api:dev sudah pensiun")
	}
	if put := callJSON(e.a.authedAs(t, b.ID), http.MethodPut, overrideURL, `{"message":"Oleh B"}`, nil); put.Code != 200 || !strings.Contains(put.Body.String(), `"updatedBy":"user:`+b.ID+`"`) {
		t.Errorf("PUT override = %d %s", put.Code, put.Body)
	}
	// Tanpa token aksi mutasi ditolak 401 dan tidak mengubah apa pun.
	if rec := call(e.a.Router, "POST", "/api/platform/modules/acme.example/disable", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("mutasi tanpa token = %d", rec.Code)
	}
}

// GET /api/bootstrap DIKECUALIKAN dari aturan mfa_required (keputusan pemilik produk 3 Nov 2026, PRD-001 Q29 butir 2,
// ADR-0013 section 4): UI membutuhkannya untuk layar enroll/verify MFA. Route modul berpermission biasa tetap 403
// mfa_required untuk sesi yang sama.
func TestBootstrapAuth_ExemptFromMFARequired(t *testing.T) {
	e := newIDEnv(t)
	sid := uuid.NewString()
	wantMFARequired := func(name, path string, hdr map[string]string) {
		t.Helper()
		rec := get(e.a.Router, path, hdr)
		if rec.Code != http.StatusForbidden || errCodeOf(t, rec) != "mfa_required" {
			t.Errorf("%s = %d %s, want 403 mfa_required", name, rec.Code, rec.Body)
		}
	}
	wantBootstrap := func(name string, hdr map[string]string) {
		t.Helper()
		if rec := get(e.a.Router, "/api/bootstrap", hdr); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "email") {
			t.Errorf("%s: bootstrap = %d %s, want 200 dengan data pengguna sendiri", name, rec.Code, rec.Body)
		}
	}

	totp := e.user(false)
	e.enrollFactor(totp.ID)
	e.a.Identity.MFAState.Invalidate(e.a.Tenant, totp.ID)
	pwd := bearerHdr(e.token(totp.ID, sid, "pwd"))
	wantBootstrap("TOTP aktif, amr pwd", pwd)
	wantMFARequired("TOTP aktif, amr pwd, route modul", protectedPath, pwd)
	if rec := get(e.a.Router, protectedPath, bearerHdr(e.token(totp.ID, sid, "pwd", "otp"))); rec.Code != http.StatusOK {
		t.Errorf("TOTP aktif, pwd+otp, route modul = %d %s, want 200", rec.Code, rec.Body)
	}

	admin := e.user(true)
	adminHdr := bearerHdr(e.token(admin.ID, sid, "pwd"))
	wantBootstrap("administrator tanpa faktor", adminHdr)
	wantMFARequired("administrator tanpa faktor, route modul", protectedPath, adminHdr)

	// Tetap wajib login: tanpa token 401.
	if rec := get(e.a.Router, "/api/bootstrap", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("tanpa token = %d, want 401", rec.Code)
	}
}
