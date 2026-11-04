//go:build integration

package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/modules/lifecycle"
	sdkidentity "erpora/sdk/identity"
)

// Tes HTTP ujung ke ujung modul core (C1.11): handler tipis -> middleware platform -> identity.Service sungguhan.

const (
	authBase     = "/api/core/auth"
	refreshCName = "erp_refresh"
)

// newCoreEnv merakit lingkungan dan memasang modul required (core) seperti yang dilakukan migrate pada deployment.
func newCoreEnv(t *testing.T) *svcEnv {
	t.Helper()
	e := newSvcEnv(t)
	res, err := e.a.Lifecycle.EnsureRequired(e.ctx, e.a.Tenant, "system:migrate")
	if err != nil {
		t.Fatalf("EnsureRequired: %v", err)
	}
	for _, r := range res {
		if r.Status == "failed" {
			t.Fatalf("EnsureRequired gagal: %+v", r)
		}
	}
	return e
}

// coreResp membungkus respons HTTP modul core.
type coreResp struct {
	*httptest.ResponseRecorder
}

// body mengurai amplop {"data": ...} atau {"error": ...} menjadi map generik.
func (r coreResp) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if r.Body.Len() == 0 {
		return nil
	}
	if err := json.Unmarshal(r.Body.Bytes(), &m); err != nil {
		t.Fatalf("body bukan JSON: %v: %q", err, r.Body.String())
	}
	return m
}

func (r coreResp) data(t *testing.T) map[string]any {
	t.Helper()
	d, _ := r.json(t)["data"].(map[string]any)
	if d == nil {
		t.Fatalf("respons tanpa data: %d %s", r.Code, r.Body.String())
	}
	return d
}

func (r coreResp) errCode(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

// refreshCookie adalah Set-Cookie erp_refresh dari respons (nil bila tidak ada).
func (r coreResp) refreshCookie() *http.Cookie {
	for _, c := range r.Result().Cookies() {
		if c.Name == refreshCName {
			return c
		}
	}
	return nil
}

// apiCall mengirim POST JSON ke route core. bearer boleh kosong; cookie boleh kosong.
func (e *svcEnv) apiCall(path, body, bearer, cookie string) coreResp {
	e.t.Helper()
	if body == "" {
		body = "{}"
	}
	req := httptest.NewRequest(http.MethodPost, authBase+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", svcOrigin)
	req.Header.Set("User-Agent", "uji-http")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: refreshCName, Value: cookie})
	}
	rec := httptest.NewRecorder()
	e.a.Router.ServeHTTP(rec, req)
	return coreResp{rec}
}

// assertAuthHeaders memeriksa header yang wajib pada setiap respons auth yang dihasilkan modul core.
func assertNoStore(t *testing.T, name string, r coreResp) {
	t.Helper()
	if cc := r.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("%s: Cache-Control = %q, want no-store", name, cc)
	}
}

// assertRefreshCookie memeriksa atribut cookie refresh dan mengembalikan nilainya.
func assertRefreshCookie(t *testing.T, name string, r coreResp) string {
	t.Helper()
	c := r.refreshCookie()
	if c == nil || c.Value == "" {
		t.Fatalf("%s: cookie %s tidak ditulis (Set-Cookie=%v)", name, refreshCName, r.Header().Values("Set-Cookie"))
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != authBase+"/refresh" || c.Domain != "" {
		t.Errorf("%s: atribut cookie salah: %+v", name, c)
	}
	if c.MaxAge <= 0 {
		t.Errorf("%s: Max-Age = %d, want > 0", name, c.MaxAge)
	}
	return c.Value
}

// assertNoRefreshInBody memastikan refresh token tidak pernah ada di body respons.
func assertNoRefreshInBody(t *testing.T, name, refresh string, r coreResp) {
	t.Helper()
	b := r.Body.String()
	if strings.Contains(strings.ToLower(b), "refresh") || (refresh != "" && strings.Contains(b, refresh)) {
		t.Errorf("%s: refresh token bocor di body: %s", name, b)
	}
}

var requestIDRe = regexp.MustCompile(`"requestId":"[^"]*"`)

func jsonBody(kv ...string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// login -> refresh (rotasi) -> logout; cookie lama mati, cookie dihapus saat logout.
func TestCoreAPI_LoginRefreshLogout(t *testing.T) {
	e := newCoreEnv(t)
	e.createUser("alur@example.test", pwA)
	e.remember(pwA)

	lr := e.apiCall("/login", jsonBody("email", "Alur@Example.test", "password", pwA), "", "")
	if lr.Code != http.StatusOK {
		t.Fatalf("login = %d %s", lr.Code, lr.Body.String())
	}
	assertNoStore(t, "login", lr)
	ck1 := assertRefreshCookie(t, "login", lr)
	e.remember(ck1)
	assertNoRefreshInBody(t, "login", ck1, lr)
	d := lr.data(t)
	at1, _ := d["accessToken"].(string)
	e.remember(at1)
	if at1 == "" || d["mfaState"] != "none" || d["expiresIn"] == nil {
		t.Fatalf("data login = %v", d)
	}
	if e.status(at1) != http.StatusOK {
		t.Fatalf("access token login tidak berlaku: %d", e.status(at1))
	}

	// refresh: cookie baru, token lama mati (reuse = 401 dan sesi dicabut sesuai ADR-0013).
	rr := e.apiCall("/refresh", "", "", ck1)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh = %d %s", rr.Code, rr.Body.String())
	}
	assertNoStore(t, "refresh", rr)
	ck2 := assertRefreshCookie(t, "refresh", rr)
	e.remember(ck2)
	assertNoRefreshInBody(t, "refresh", ck2, rr)
	if ck2 == ck1 {
		t.Fatal("refresh harus merotasi cookie")
	}
	at2, _ := rr.data(t)["accessToken"].(string)
	e.remember(at2)
	if e.status(at2) != http.StatusOK {
		t.Fatalf("access token refresh tidak berlaku: %d", e.status(at2))
	}

	// refresh tanpa cookie -> 401 unauthenticated.
	if r := e.apiCall("/refresh", "", "", ""); r.Code != http.StatusUnauthorized || r.errCode(t) != "unauthenticated" {
		t.Fatalf("refresh tanpa cookie = %d %s", r.Code, r.Body.String())
	}

	// logout: 204, cookie dihapus, sesi mati.
	lo := e.apiCall("/logout", "", at2, ck2)
	if lo.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", lo.Code, lo.Body.String())
	}
	assertNoStore(t, "logout", lo)
	if c := lo.refreshCookie(); c == nil || c.Value != "" || c.MaxAge >= 0 || c.Path != authBase+"/refresh" {
		t.Fatalf("logout harus menghapus cookie: %+v", c)
	}
	if e.status(at2) != http.StatusUnauthorized {
		t.Fatalf("access token setelah logout harus 401, dapat %d", e.status(at2))
	}
	if r := e.apiCall("/refresh", "", "", ck2); r.Code != http.StatusUnauthorized {
		t.Fatalf("refresh setelah logout = %d", r.Code)
	}
	// logout tanpa token -> 401 (route Authenticated).
	if r := e.apiCall("/logout", "", "", ck2); r.Code != http.StatusUnauthorized {
		t.Fatalf("logout tanpa token = %d", r.Code)
	}
}

// Kegagalan login seragam (sandi salah = email tak dikenal), tanpa cookie, no-store.
func TestCoreAPI_LoginFailureUniform(t *testing.T) {
	e := newCoreEnv(t)
	e.createUser("ada@example.test", pwA)
	a := e.apiCall("/login", jsonBody("email", "ada@example.test", "password", "salah-salah-salah"), "", "")
	b := e.apiCall("/login", jsonBody("email", "tidakada@example.test", "password", "salah-salah-salah"), "", "")
	for name, r := range map[string]coreResp{"sandi salah": a, "email tak dikenal": b} {
		if r.Code != http.StatusUnauthorized || r.errCode(t) != "invalid_credentials" {
			t.Errorf("%s = %d %s", name, r.Code, r.Body.String())
		}
		if r.refreshCookie() != nil {
			t.Errorf("%s: tidak boleh menulis cookie", name)
		}
		assertNoStore(t, name, r)
	}
	strip := func(r coreResp) string { return requestIDRe.ReplaceAllString(r.Body.String(), "") }
	if strip(a) != strip(b) {
		t.Errorf("badan galat harus seragam:\n%s\n%s", a.Body.String(), b.Body.String())
	}
	// Badan tak sah -> 422 tanpa memanggil Service (field asing ditolak).
	if r := e.apiCall("/login", `{"email":"a@b.c","password":"x","userId":"u"}`, "", ""); r.Code != http.StatusUnprocessableEntity {
		t.Errorf("field asing = %d %s", r.Code, r.Body.String())
	}
}

// MFA lewat HTTP: enroll -> confirm (cookie ditulis ulang, sesi otp) -> login ulang verify_required -> verify (Q27).
func TestCoreAPI_MFAFlowRewritesCookie(t *testing.T) {
	e := newCoreEnv(t)
	e.createAdmin("mfa@example.test", pwAdmin)
	e.remember(pwAdmin)

	lr := e.apiCall("/login", jsonBody("email", "mfa@example.test", "password", pwAdmin), "", "")
	if lr.Code != http.StatusOK || lr.data(t)["mfaState"] != "enroll_required" {
		t.Fatalf("login = %d %s", lr.Code, lr.Body.String())
	}
	preCookie := assertRefreshCookie(t, "login", lr)
	preToken, _ := lr.data(t)["accessToken"].(string)
	e.remember(preCookie, preToken)

	// recovery-codes menuntut MFA terverifikasi: token pra-MFA ditolak platform (403 mfa_required).
	if r := e.apiCall("/mfa/recovery-codes", "", preToken, ""); r.Code != http.StatusForbidden || r.errCode(t) != "mfa_required" {
		t.Fatalf("recovery-codes pra-MFA = %d %s", r.Code, r.Body.String())
	}

	// Sesi menunggu enroll boleh memanggil enroll dan confirm.
	en := e.apiCall("/mfa/enroll", "", preToken, "")
	if en.Code != http.StatusOK {
		t.Fatalf("enroll = %d %s", en.Code, en.Body.String())
	}
	assertNoStore(t, "enroll", en)
	ed := en.data(t)
	secretB32, _ := ed["secret"].(string)
	e.remember(secretB32)
	if secretB32 == "" || !strings.HasPrefix(ed["otpauthUri"].(string), "otpauth://") {
		t.Fatalf("enroll data = %v", ed)
	}
	secret := decodeSecret(t, secretB32)

	if r := e.apiCall("/mfa/confirm", jsonBody("code", "000000"), preToken, ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("confirm kode salah = %d %s", r.Code, r.Body.String())
	}
	cf := e.apiCall("/mfa/confirm", jsonBody("code", e.code(secret, -30*time.Second)), preToken, "")
	if cf.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", cf.Code, cf.Body.String())
	}
	assertNoStore(t, "confirm", cf)
	cookie2 := assertRefreshCookie(t, "confirm", cf)
	e.remember(cookie2)
	assertNoRefreshInBody(t, "confirm", cookie2, cf)
	if cookie2 == preCookie {
		t.Fatal("confirm harus menggantikan cookie refresh (Q27)")
	}
	cd := cf.data(t)
	codes, _ := cd["codes"].([]any)
	if len(codes) != 10 {
		t.Fatalf("recovery codes = %d", len(codes))
	}
	for _, c := range codes {
		e.remember(c.(string))
	}
	sess, _ := cd["session"].(map[string]any)
	otpToken, _ := sess["accessToken"].(string)
	e.remember(otpToken)
	if otpToken == "" || sess["mfaState"] != "none" {
		t.Fatalf("session confirm = %v", sess)
	}
	// Cookie pra-MFA mati tanpa mencabut sesi otp.
	if r := e.apiCall("/refresh", "", "", preCookie); r.Code != http.StatusUnauthorized {
		t.Fatalf("cookie pra-MFA harus mati: %d", r.Code)
	}
	if e.status(otpToken) != http.StatusOK {
		t.Fatalf("sesi otp harus tetap hidup: %d", e.status(otpToken))
	}
	// Token otp: recovery-codes menghasilkan 10 kode baru.
	rg := e.apiCall("/mfa/recovery-codes", "", otpToken, "")
	if rg.Code != http.StatusOK || len(rg.data(t)["codes"].([]any)) != 10 {
		t.Fatalf("recovery-codes otp = %d %s", rg.Code, rg.Body.String())
	}
	// Regenerasi mengganti seluruh kode: kode lama tidak berlaku lagi, kode baru yang dipakai di bawah.
	oldCode := codes[0].(string)
	codes = rg.data(t)["codes"].([]any)
	for _, c := range codes {
		e.remember(c.(string))
	}
	assertNoStore(t, "recovery-codes", rg)

	// Login ulang -> verify_required; verify mengganti cookie; token pra-MFA tidak dapat memanggil route lain.
	l2 := e.apiCall("/login", jsonBody("email", "mfa@example.test", "password", pwAdmin), "", "")
	if l2.Code != http.StatusOK || l2.data(t)["mfaState"] != "verify_required" {
		t.Fatalf("login ulang = %d %s", l2.Code, l2.Body.String())
	}
	cookie3 := assertRefreshCookie(t, "login2", l2)
	tok3, _ := l2.data(t)["accessToken"].(string)
	e.remember(cookie3, tok3)
	if e.status(tok3) != http.StatusForbidden {
		t.Fatalf("token verify_required untuk route modul harus 403: %d", e.status(tok3))
	}
	// Sesi verify_required tidak boleh memanggil enroll/confirm (daftar platform) ataupun change password.
	if r := e.apiCall("/mfa/enroll", "", tok3, ""); r.Code != http.StatusForbidden {
		t.Errorf("enroll dari sesi verify_required = %d, want 403", r.Code)
	}
	if r := e.apiCall("/password/change", jsonBody("currentPassword", pwAdmin, "newPassword", "kata-sandi-baru-sekali-2026"), tok3, ""); r.Code != http.StatusForbidden {
		t.Errorf("change dari sesi verify_required = %d, want 403", r.Code)
	}
	vr := e.apiCall("/mfa/verify", jsonBody("code", e.code(secret, 0)), tok3, "")
	if vr.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", vr.Code, vr.Body.String())
	}
	assertNoStore(t, "verify", vr)
	cookie4 := assertRefreshCookie(t, "verify", vr)
	e.remember(cookie4)
	assertNoRefreshInBody(t, "verify", cookie4, vr)
	if cookie4 == cookie3 {
		t.Fatal("verify harus menggantikan cookie refresh")
	}
	tok4, _ := vr.data(t)["accessToken"].(string)
	e.remember(tok4)
	if e.status(tok4) != http.StatusOK || vr.data(t)["mfaState"] != "none" {
		t.Fatalf("verify data = %v status=%d", vr.data(t), e.status(tok4))
	}
	if r := e.apiCall("/refresh", "", "", cookie3); r.Code != http.StatusUnauthorized {
		t.Fatalf("cookie pra-verify harus mati: %d", r.Code)
	}

	// Recovery code lewat /mfa/recovery pada sesi verify_required lain: sekali pakai.
	l3 := e.apiCall("/login", jsonBody("email", "mfa@example.test", "password", pwAdmin), "", "")
	tok5, _ := l3.data(t)["accessToken"].(string)
	e.remember(tok5)
	if r := e.apiCall("/mfa/recovery", jsonBody("code", oldCode), tok5, ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("kode lama setelah regenerasi = %d %s", r.Code, r.Body.String())
	}
	rc := e.apiCall("/mfa/recovery", jsonBody("code", codes[0].(string)), tok5, "")
	if rc.Code != http.StatusOK {
		t.Fatalf("recovery = %d %s", rc.Code, rc.Body.String())
	}
	assertNoStore(t, "recovery", rc)
	ck5 := assertRefreshCookie(t, "recovery", rc)
	e.remember(ck5)
	assertNoRefreshInBody(t, "recovery", ck5, rc)
}

// Reset sandi lewat HTTP dengan pengirim tiruan (tautan di surat), lalu login dengan sandi baru.
func TestCoreAPI_ForgotResetPassword(t *testing.T) {
	e := newCoreEnv(t)
	u := e.createUser("reset@example.test", pwA)
	if err := e.adm().SetPublicURL(e.sys("cli:core:settings:set"), "https://erp.example.com/"); err != nil {
		t.Fatal(err)
	}
	old := e.mustLogin(u.Email, pwA)

	// Email tak dikenal dan dikenal memberi respons identik (204) dan tanpa bocor.
	un := e.apiCall("/password/forgot", jsonBody("email", "tidakada@example.test"), "", "")
	kn := e.apiCall("/password/forgot", jsonBody("email", u.Email), "", "")
	if un.Code != http.StatusNoContent || kn.Code != http.StatusNoContent || un.Body.String() != kn.Body.String() {
		t.Fatalf("forgot = %d/%d", un.Code, kn.Code)
	}
	assertNoStore(t, "forgot", kn)
	e.a.Identity.Reset.Wait()
	msgs := e.mail.all()
	if len(msgs) != 1 {
		t.Fatalf("surat = %d", len(msgs))
	}
	m := linkToken.FindStringSubmatch(msgs[0].Text)
	if m == nil {
		t.Fatalf("tautan reset tidak ada: %q", msgs[0].Text)
	}
	tok := m[1]
	e.remember(tok)

	const newPw = "kata-sandi-reset-http-2026"
	e.remember(newPw)
	// Kebijakan sandi: 422 password_policy_violation tanpa membocorkan daftar aturan.
	pv := e.apiCall("/password/reset", jsonBody("token", tok, "newPassword", "pendek"), "", "")
	if pv.Code != http.StatusUnprocessableEntity || pv.errCode(t) != "password_policy_violation" {
		t.Fatalf("reset pendek = %d %s", pv.Code, pv.Body.String())
	}
	assertNoStore(t, "reset-policy", pv)
	rs := e.apiCall("/password/reset", jsonBody("token", tok, "newPassword", newPw), "", "")
	if rs.Code != http.StatusNoContent {
		t.Fatalf("reset = %d %s", rs.Code, rs.Body.String())
	}
	assertNoStore(t, "reset", rs)
	if r := e.apiCall("/password/reset", jsonBody("token", tok, "newPassword", newPw), "", ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("token sekali pakai = %d %s", r.Code, r.Body.String())
	}
	// Reset mencabut sesi lama.
	if e.status(old.AccessToken) != http.StatusUnauthorized {
		t.Fatalf("sesi lama harus tercabut setelah reset: %d", e.status(old.AccessToken))
	}
	if r := e.apiCall("/login", jsonBody("email", u.Email, "password", pwA), "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("sandi lama harus ditolak: %d", r.Code)
	}
	if r := e.apiCall("/login", jsonBody("email", u.Email, "password", newPw), "", ""); r.Code != http.StatusOK {
		t.Fatalf("login sandi baru = %d %s", r.Code, r.Body.String())
	}
}

// IDOR A vs B: logout/change/MFA hanya berlaku untuk Principal pemanggil; ID di body ditolak; token A tidak
// dapat memengaruhi sesi, sandi, atau MFA B.
func TestCoreAPI_IDOR_OwnPrincipalOnly(t *testing.T) {
	e := newCoreEnv(t)
	ua := e.createUser("a@example.test", pwA)
	ub := e.createUser("b@example.test", pwB)
	la := e.mustLogin(ua.Email, pwA)
	lb := e.mustLogin(ub.Email, pwB)
	e.remember(pwA, pwB)

	// Body dengan userId/sessionId: kolom asing ditolak 422 (change/confirm); logout dan enroll tidak membaca body
	// dan hanya bertindak atas Principal pemanggil. Tiap percobaan memakai sesi A baru.
	hostile := `{"userId":"` + ub.ID + `","sessionId":"x","currentPassword":"` + pwA + `","newPassword":"kata-sandi-lain-2026","code":"123456"}`
	for path, want := range map[string]int{
		"/password/change": http.StatusUnprocessableEntity,
		"/mfa/confirm":     http.StatusUnprocessableEntity,
		"/mfa/enroll":      http.StatusOK,
		"/logout":          http.StatusNoContent,
	} {
		l := e.mustLogin(ua.Email, pwA)
		if r := e.apiCall(path, hostile, l.AccessToken, ""); r.Code != want {
			t.Errorf("%s dengan userId di body = %d %s, want %d", path, r.Code, r.Body.String(), want)
		}
	}
	la = e.mustLogin(ua.Email, pwA)
	// Setelah tembakan di atas sesi B tidak tersentuh, sandi B tetap, dan B tidak punya faktor MFA.
	if e.status(lb.AccessToken) != http.StatusOK {
		t.Fatalf("sesi B tidak boleh terpengaruh: %d", e.status(lb.AccessToken))
	}
	if _, err := e.login(ub.Email, pwB); err != nil {
		t.Fatalf("sandi B tidak boleh berubah: %v", err)
	}

	// Ganti sandi A (sandi benar) memengaruhi A saja.
	const newA = "kata-sandi-a-baru-2026"
	e.remember(newA)
	cp := e.apiCall("/password/change", jsonBody("currentPassword", pwA, "newPassword", newA), la.AccessToken, "")
	if cp.Code != http.StatusNoContent {
		t.Fatalf("change = %d %s", cp.Code, cp.Body.String())
	}
	assertNoStore(t, "change", cp)
	if _, err := e.login(ub.Email, pwB); err != nil {
		t.Fatalf("login B: %v", err)
	}
	if e.status(lb.AccessToken) != http.StatusOK {
		t.Fatalf("sesi B tetap hidup setelah A ganti sandi: %d", e.status(lb.AccessToken))
	}
	// Sandi saat ini salah -> 422 (bukan 401: tidak memicu refresh klien), sandi tidak berubah.
	if r := e.apiCall("/password/change", jsonBody("currentPassword", "bukan-sandi-saya-2026", "newPassword", "kata-sandi-lain-2026"), lb.AccessToken, ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("change sandi salah = %d %s", r.Code, r.Body.String())
	}

	// Logout A tidak mencabut sesi B; logout memakai sesi dari token, bukan dari body.
	la2 := e.mustLogin(ua.Email, newA)
	if r := e.apiCall("/logout", "", la2.AccessToken, ""); r.Code != http.StatusNoContent {
		t.Fatalf("logout A = %d", r.Code)
	}
	if e.status(la2.AccessToken) != http.StatusUnauthorized || e.status(lb.AccessToken) != http.StatusOK {
		t.Fatalf("logout A harus hanya mencabut sesi A: A=%d B=%d", e.status(la2.AccessToken), e.status(lb.AccessToken))
	}

	// Cookie refresh milik A tidak dapat dipakai bersama bearer B untuk logout sesi lain: tanpa bearer 401.
	if r := e.apiCall("/logout", "", "", lb.RefreshToken); r.Code != http.StatusUnauthorized {
		t.Fatalf("logout tanpa bearer = %d", r.Code)
	}

	// MFA: enroll A hanya membuat faktor A; B tetap tanpa faktor (login B tetap mfaState none).
	lA := e.mustLogin(ua.Email, newA)
	en := e.apiCall("/mfa/enroll", "", lA.AccessToken, "")
	if en.Code != http.StatusOK {
		t.Fatalf("enroll A = %d %s", en.Code, en.Body.String())
	}
	e.remember(en.data(t)["secret"].(string))
	if lb2 := e.mustLogin(ub.Email, pwB); lb2.MFAState != sdkidentity.MFANone {
		t.Fatalf("MFA B tidak boleh berubah: %s", lb2.MFAState)
	}
	// Pendaftaran A tidak terlihat dari B: B tidak punya pendaftaran tertunda, jadi confirm B ditolak 409 invalid_state.
	if r := e.apiCall("/mfa/confirm", jsonBody("code", "000000"), lb.AccessToken, ""); r.Code != http.StatusConflict {
		t.Fatalf("confirm B tanpa pendaftaran = %d %s", r.Code, r.Body.String())
	}
}

// Semua route core berada di bawah /api/core/auth/ sehingga tercakup pembatas laju IP platform (isAuthEndpoint):
// banjir ke /refresh (yang tidak menyentuh pembatas Service) berujung 429 rate_limited + Retry-After, dan ember IP itu
// dipakai bersama route lain.
func TestCoreAPI_AuthRoutesRateLimited(t *testing.T) {
	e := newCoreEnv(t)
	var limited coreResp
	for i := 0; i < 200; i++ {
		r := e.apiCall("/refresh", "", "", "")
		if r.Code == http.StatusTooManyRequests {
			limited = r
			break
		}
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("refresh tanpa cookie = %d %s", r.Code, r.Body.String())
		}
	}
	if limited.ResponseRecorder == nil {
		t.Fatal("tidak pernah 429: route auth tidak terbatasi laju")
	}
	assertNoStore(t, "429 IPLimit", limited)
	if limited.errCode(t) != "rate_limited" || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("429 = %s retry-after=%q", limited.Body.String(), limited.Header().Get("Retry-After"))
	}
	for path, body := range map[string]string{
		"/login":           jsonBody("email", "x@example.test", "password", "salah-salah-salah"),
		"/password/forgot": jsonBody("email", "x@example.test"),
		"/password/reset":  jsonBody("token", "x", "newPassword", "kata-sandi-lain-2026"),
	} {
		if r := e.apiCall(path, body, "", ""); r.Code != http.StatusTooManyRequests {
			t.Errorf("%s setelah banjir = %d, want 429", path, r.Code)
		}
	}
}

// core required: tidak dapat di-disable maupun di-uninstall (module_required), dan terpasang otomatis lewat EnsureRequired.
func TestCoreAPI_CoreCannotBeDisabledOrUninstalled(t *testing.T) {
	e := newCoreEnv(t)
	// Disable ditolak module_required. Uninstall dari status enabled ditolak lebih awal (invalid_transition, karena
	// harus disable dulu, yang sendiri terkunci); modul tetap enabled dan route tetap melayani.
	for _, op := range []lifecycle.Op{lifecycle.OpDisable, lifecycle.OpUninstall} {
		_, err := e.a.Lifecycle.Execute(e.ctx, lifecycle.Request{Tenant: e.a.Tenant, Op: op, Key: "erp.core", Actor: "cli:uji"})
		var le *lifecycle.Error
		if !errors.As(err, &le) {
			t.Fatalf("%s erp.core harus ditolak: %v", op, err)
		}
		if op == lifecycle.OpDisable && le.Code != lifecycle.CodeModuleRequired {
			t.Errorf("disable erp.core = %v, want module_required", le.Code)
		}
	}
	// Setelah ditolak, route auth tetap melayani.
	if r := e.apiCall("/login", jsonBody("email", "x@example.test", "password", "salah-salah-salah"), "", ""); r.Code != http.StatusUnauthorized {
		t.Errorf("login setelah percobaan disable = %d", r.Code)
	}
}

// ADR-0013 §1: respons yang dibuat PLATFORM pada route auth (401 Authenticate, 403 mfa_required dari Decide, 429)
// juga Cache-Control: no-store, bukan hanya respons handler modul.
func TestCoreAPI_PlatformErrorsAreNoStore(t *testing.T) {
	e := newCoreEnv(t)
	e.createAdmin("nostore@example.test", pwAdmin)
	l := e.mustLogin("nostore@example.test", pwAdmin) // enroll_required: token pra-MFA

	// 401 dari Authenticate (route Authenticated tanpa bearer).
	r := e.apiCall("/logout", "", "", "")
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("logout tanpa bearer = %d", r.Code)
	}
	assertNoStore(t, "401 Authenticate", r)
	// 403 mfa_required dari Decide (recovery-codes menuntut MFA terverifikasi).
	r = e.apiCall("/mfa/recovery-codes", "", l.AccessToken, "")
	if r.Code != http.StatusForbidden || r.errCode(t) != "mfa_required" {
		t.Fatalf("recovery-codes pra-MFA = %d %s", r.Code, r.Body.String())
	}
	assertNoStore(t, "403 mfa_required", r)
	// Sesi enroll_required memanggil route yang tidak ada di daftar sesi menunggu MFA: 403 dari platform.
	r = e.apiCall("/password/change", jsonBody("currentPassword", pwAdmin, "newPassword", "kata-sandi-baru-sekali-2026"), l.AccessToken, "")
	if r.Code != http.StatusForbidden {
		t.Fatalf("change dari sesi enroll_required = %d %s", r.Code, r.Body.String())
	}
	assertNoStore(t, "403 sesi menunggu MFA", r)
}
