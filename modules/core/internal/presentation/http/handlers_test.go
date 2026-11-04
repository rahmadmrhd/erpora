package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erpora/sdk/httpx"
	"erpora/sdk/identity"
)

const (
	secretRefresh = "RT-super-secret-refresh-token-value"
	secretAccess  = "eyJ.access.token"
)

var fixedNow = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

// fakeService merekam masukan dan mengembalikan hasil/galat yang dikonfigurasi.
type fakeService struct {
	err error

	login      identity.LoginResult
	confirm    identity.ConfirmMFAResult
	enroll     identity.EnrollmentInfo
	recovery   identity.RecoveryCodes
	loginIn    identity.LoginInput
	refreshIn  identity.RefreshInput
	forgotIn   identity.ForgotInput
	resetIn    identity.ResetInput
	changeIn   identity.ChangeInput
	confirmIn  identity.ConfirmMFAInput
	verifyIn   identity.VerifyInput
	calls      []string
	principals []string // user id dari Principal di context pada tiap panggilan terautentikasi
}

func (f *fakeService) note(ctx context.Context, name string) {
	f.calls = append(f.calls, name)
	if p, ok := identity.FromContext(ctx); ok {
		f.principals = append(f.principals, p.UserID)
	}
}

func (f *fakeService) Login(ctx context.Context, in identity.LoginInput) (identity.LoginResult, error) {
	f.note(ctx, "Login")
	f.loginIn = in
	return f.login, f.err
}

func (f *fakeService) Refresh(ctx context.Context, in identity.RefreshInput) (identity.LoginResult, error) {
	f.note(ctx, "Refresh")
	f.refreshIn = in
	return f.login, f.err
}
func (f *fakeService) Logout(ctx context.Context) error { f.note(ctx, "Logout"); return f.err }
func (f *fakeService) ForgotPassword(ctx context.Context, in identity.ForgotInput) error {
	f.note(ctx, "ForgotPassword")
	f.forgotIn = in
	return f.err
}
func (f *fakeService) ResetPassword(ctx context.Context, in identity.ResetInput) error {
	f.note(ctx, "ResetPassword")
	f.resetIn = in
	return f.err
}
func (f *fakeService) ChangePassword(ctx context.Context, in identity.ChangeInput) error {
	f.note(ctx, "ChangePassword")
	f.changeIn = in
	return f.err
}
func (f *fakeService) BeginMFAEnrollment(ctx context.Context) (identity.EnrollmentInfo, error) {
	f.note(ctx, "BeginMFAEnrollment")
	return f.enroll, f.err
}
func (f *fakeService) ConfirmMFA(ctx context.Context, in identity.ConfirmMFAInput) (identity.ConfirmMFAResult, error) {
	f.note(ctx, "ConfirmMFA")
	f.confirmIn = in
	return f.confirm, f.err
}
func (f *fakeService) VerifyMFA(ctx context.Context, in identity.VerifyInput) (identity.LoginResult, error) {
	f.note(ctx, "VerifyMFA")
	f.verifyIn = in
	return f.login, f.err
}
func (f *fakeService) RegenerateRecoveryCodes(ctx context.Context) (identity.RecoveryCodes, error) {
	f.note(ctx, "RegenerateRecoveryCodes")
	return f.recovery, f.err
}

// call menjalankan handler di balik NoStore dengan Service tiruan dan IP klien di context, seperti rantai platform.
func call(t *testing.T, svc *fakeService, handler func(*Handlers) http.HandlerFunc, method, body string, mod func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	h := New().WithClock(func() time.Time { return fixedNow })
	req := httptest.NewRequest(method, "/api/core/auth/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "uji-ua")
	ctx := httpx.WithClientIP(req.Context(), "203.0.113.9")
	if svc != nil {
		ctx = identity.WithService(ctx, svc)
	}
	req = req.WithContext(ctx)
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	NoStore(handler(h)).ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) (string, []map[string]any) {
	t.Helper()
	var b struct {
		Error struct {
			Code    string
			Message string
			Details []map[string]any
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return b.Error.Code, b.Error.Details
}

func refreshCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName {
			return c
		}
	}
	return nil
}

func loginResult() identity.LoginResult {
	return identity.LoginResult{
		AccessToken: secretAccess, ExpiresIn: 600, MFAState: identity.MFANone,
		RefreshToken: secretRefresh, RefreshExpiresAt: fixedNow.Add(14 * 24 * time.Hour),
	}
}

func assertNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// assertCookie memeriksa atribut keamanan cookie refresh (ADR-0013 §1).
func assertCookie(t *testing.T, c *http.Cookie, wantValue string, wantMaxAge int) {
	t.Helper()
	if c == nil {
		t.Fatal("cookie refresh tidak ditulis")
	}
	if c.Value != wantValue || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/core/auth/refresh" {
		t.Errorf("atribut cookie salah: %+v", c)
	}
	if c.MaxAge != wantMaxAge {
		t.Errorf("Max-Age = %d, want %d", c.MaxAge, wantMaxAge)
	}
	if c.Domain != "" {
		t.Errorf("Domain harus kosong (host-only), dapat %q", c.Domain)
	}
}

// bodyHasNoRefresh: nilai refresh token dan kunci refresh* tidak pernah ada di body.
func bodyHasNoRefresh(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	b := rec.Body.String()
	if strings.Contains(b, secretRefresh) || strings.Contains(strings.ToLower(b), "refresh") {
		t.Errorf("body memuat refresh token atau kunci refresh*: %s", b)
	}
}

func TestLogin_SuccessCookieAndBody(t *testing.T) {
	svc := &fakeService{login: loginResult()}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.Login }, "POST", `{"email":"ani@example.test","password":"kata-sandi-2026"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d %s", rec.Code, rec.Body)
	}
	assertNoStore(t, rec)
	assertCookie(t, refreshCookieOf(t, rec), secretRefresh, 14*24*3600)
	bodyHasNoRefresh(t, rec)
	var b struct {
		Data map[string]any
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if b.Data["accessToken"] != secretAccess || b.Data["expiresIn"] != float64(600) || b.Data["mfaState"] != "none" || len(b.Data) != 3 {
		t.Errorf("data = %v", b.Data)
	}
	// Metadata request diisi handler dari konteks platform, bukan dari klien.
	if svc.loginIn.ClientIP != "203.0.113.9" || svc.loginIn.UserAgent != "uji-ua" || svc.loginIn.Email != "ani@example.test" || svc.loginIn.Password != "kata-sandi-2026" {
		t.Errorf("input Login = %+v", svc.loginIn)
	}
}

func TestLogin_MFAStatesPassThrough(t *testing.T) {
	for _, st := range []identity.MFAState{identity.MFAEnrollRequired, identity.MFAVerifyRequired} {
		res := loginResult()
		res.MFAState = st
		rec := call(t, &fakeService{login: res}, func(h *Handlers) http.HandlerFunc { return h.Login }, "POST", `{"email":"a@b.test","password":"x"}`, nil)
		if !strings.Contains(rec.Body.String(), `"mfaState":"`+string(st)+`"`) {
			t.Errorf("mfaState %s: %s", st, rec.Body)
		}
	}
}

// Body tidak dapat menyuntik metadata request atau meminta refresh token: field tak dikenal ditolak dan Service tidak dipanggil.
func TestBodies_RejectUnknownFieldsAndNeverCallService(t *testing.T) {
	cases := []struct {
		name string
		h    func(*Handlers) http.HandlerFunc
		body string
	}{
		{"login clientIP", func(h *Handlers) http.HandlerFunc { return h.Login }, `{"email":"a@b.test","password":"x","clientIp":"1.1.1.1"}`},
		{"login refreshToken", func(h *Handlers) http.HandlerFunc { return h.Login }, `{"email":"a@b.test","password":"x","refreshToken":"x"}`},
		{"forgot userId", func(h *Handlers) http.HandlerFunc { return h.ForgotPassword }, `{"email":"a@b.test","userId":"u"}`},
		{"reset locale", func(h *Handlers) http.HandlerFunc { return h.ResetPassword }, `{"token":"t","newPassword":"p","clientIP":"1.1.1.1"}`},
		{"change userId", func(h *Handlers) http.HandlerFunc { return h.ChangePassword }, `{"currentPassword":"a","newPassword":"b","userId":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4aa1"}`},
		{"confirm sessionId", func(h *Handlers) http.HandlerFunc { return h.ConfirmMFA }, `{"code":"123456","sessionId":"s"}`},
		{"verify userId", func(h *Handlers) http.HandlerFunc { return h.VerifyMFA }, `{"code":"123456","userId":"u"}`},
		{"verify recovery flag", func(h *Handlers) http.HandlerFunc { return h.VerifyMFA }, `{"code":"123456","recovery":true}`},
		{"recovery recovery flag", func(h *Handlers) http.HandlerFunc { return h.RecoverMFA }, `{"code":"123456","recovery":false}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &fakeService{}
			rec := call(t, svc, c.h, "POST", c.body, nil)
			if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
				t.Fatalf("code %d %s", rec.Code, rec.Body)
			}
			if len(svc.calls) != 0 {
				t.Errorf("Service dipanggil: %v", svc.calls)
			}
			assertNoStore(t, rec)
		})
	}
}

func TestLogin_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid credentials", identity.ErrInvalidCredentials, 401, "invalid_credentials"},
		{"rate limited", &identity.RateLimitedError{RetryAfter: 3200 * time.Millisecond}, 429, "rate_limited"},
		{"account locked", identity.ErrAccountLocked, 403, "account_locked"},
		{"internal", errors.New("boom: detail rahasia internal"), 500, "internal_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := call(t, &fakeService{err: c.err}, func(h *Handlers) http.HandlerFunc { return h.Login }, "POST", `{"email":"a@b.test","password":"x"}`, nil)
			if rec.Code != c.status {
				t.Fatalf("code %d %s", rec.Code, rec.Body)
			}
			if code, _ := errCode(t, rec); code != c.code {
				t.Errorf("code = %q, want %q", code, c.code)
			}
			assertNoStore(t, rec)
			if refreshCookieOf(t, rec) != nil {
				t.Error("galat login tidak boleh menulis cookie")
			}
			if strings.Contains(rec.Body.String(), "boom") || strings.Contains(rec.Body.String(), "rahasia") {
				t.Errorf("detail internal bocor: %s", rec.Body)
			}
			if c.status == 429 && rec.Header().Get("Retry-After") != "4" {
				t.Errorf("Retry-After = %q, want 4 (dibulatkan ke atas)", rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestRefresh_CookieOriginAndRotation(t *testing.T) {
	svc := &fakeService{login: loginResult()}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.Refresh }, "POST", "", func(r *http.Request) {
		r.Header.Set("Origin", "https://erp.example.com")
		r.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: "token-lama"})
	})
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body)
	}
	if svc.refreshIn.RefreshToken != "token-lama" || svc.refreshIn.Origin != "https://erp.example.com" || svc.refreshIn.ClientIP != "203.0.113.9" {
		t.Errorf("input Refresh = %+v", svc.refreshIn)
	}
	assertCookie(t, refreshCookieOf(t, rec), secretRefresh, 14*24*3600) // ditulis ulang (rotasi)
	bodyHasNoRefresh(t, rec)
	assertNoStore(t, rec)
}

// Refresh token tidak pernah dibaca dari body, query, atau header selain cookie.
func TestRefresh_IgnoresBodyQueryAndHeader(t *testing.T) {
	svc := &fakeService{err: identity.ErrInvalidRefresh}
	call(t, svc, func(h *Handlers) http.HandlerFunc { return h.Refresh }, "POST", `{"refreshToken":"dari-body"}`, func(r *http.Request) {
		r.URL.RawQuery = "refreshToken=dari-query"
		r.Header.Set("X-Refresh-Token", "dari-header")
		r.Header.Set("Authorization", "Bearer dari-bearer")
	})
	if svc.refreshIn.RefreshToken != "" {
		t.Errorf("token terbaca dari sumber selain cookie: %q", svc.refreshIn.RefreshToken)
	}
}

func TestRefresh_InvalidTokenClearsCookie(t *testing.T) {
	rec := call(t, &fakeService{err: identity.ErrInvalidRefresh}, func(h *Handlers) http.HandlerFunc { return h.Refresh }, "POST", "", nil)
	if rec.Code != 401 {
		t.Fatalf("code %d", rec.Code)
	}
	if code, _ := errCode(t, rec); code != "unauthenticated" {
		t.Errorf("code = %q, want unauthenticated (generik)", code)
	}
	c := refreshCookieOf(t, rec)
	if c == nil || c.MaxAge >= 0 || c.Value != "" || c.Path != "/api/core/auth/refresh" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie harus dihapus dengan atribut yang sama: %+v", c)
	}
	assertNoStore(t, rec)
}

// Origin salah: 403 permission_denied dan cookie TIDAK dihapus (permintaan lintas situs tidak boleh melogout pengguna).
func TestRefresh_OriginNotAllowed(t *testing.T) {
	rec := call(t, &fakeService{err: identity.ErrOriginNotAllowed}, func(h *Handlers) http.HandlerFunc { return h.Refresh }, "POST", "", nil)
	if code, _ := errCode(t, rec); rec.Code != 403 || code != "permission_denied" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if refreshCookieOf(t, rec) != nil {
		t.Error("Origin salah tidak boleh menghapus cookie")
	}
}

func TestLogout_ClearsCookieAndUsesPrincipalOnly(t *testing.T) {
	svc := &fakeService{}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.Logout }, "POST", "", func(r *http.Request) {
		// body/query/header tidak berpengaruh: sesi hanya dari Principal di context.
		r.URL.RawQuery = "sessionId=sesi-lain&userId=pengguna-lain"
		r.Header.Set("X-Session-Id", "sesi-lain")
		r.SetPathValue("id", "sesi-lain")
	})
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("code %d %q", rec.Code, rec.Body)
	}
	if c := refreshCookieOf(t, rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("cookie harus dihapus: %+v", c)
	}
	assertNoStore(t, rec)
	if len(svc.calls) != 1 || svc.calls[0] != "Logout" {
		t.Errorf("calls = %v", svc.calls)
	}
}

func TestLogout_Unauthenticated(t *testing.T) {
	rec := call(t, &fakeService{err: identity.ErrUnauthenticated}, func(h *Handlers) http.HandlerFunc { return h.Logout }, "POST", "", nil)
	if code, _ := errCode(t, rec); rec.Code != 401 || code != "unauthenticated" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if refreshCookieOf(t, rec) != nil {
		t.Error("logout gagal tidak boleh menghapus cookie")
	}
}

func TestForgotPassword_Always204AndRateLimit(t *testing.T) {
	svc := &fakeService{}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.ForgotPassword }, "POST", `{"email":"siapa.saja@example.test"}`, nil)
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("code %d %q", rec.Code, rec.Body)
	}
	assertNoStore(t, rec)
	if svc.forgotIn.Email != "siapa.saja@example.test" || svc.forgotIn.ClientIP != "203.0.113.9" {
		t.Errorf("input = %+v", svc.forgotIn)
	}
	rec = call(t, &fakeService{err: &identity.RateLimitedError{RetryAfter: time.Second}}, func(h *Handlers) http.HandlerFunc { return h.ForgotPassword }, "POST", `{"email":"a@b.test"}`, nil)
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "1" {
		t.Errorf("rate limited: %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestResetPassword_ErrorsAreUniformAndPolicyDetailsAreCodesOnly(t *testing.T) {
	rec := call(t, &fakeService{err: identity.ErrInvalidResetToken}, func(h *Handlers) http.HandlerFunc { return h.ResetPassword }, "POST", `{"token":"t","newPassword":"kata-sandi-rahasia"}`, nil)
	code, details := errCode(t, rec)
	if rec.Code != 422 || code != "validation_failed" || len(details) != 1 || details[0]["code"] != "invalid_token" || details[0]["field"] != "token" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = call(t, &fakeService{err: &identity.PasswordPolicyError{Codes: []string{"too_short", "common"}}}, func(h *Handlers) http.HandlerFunc { return h.ResetPassword }, "POST", `{"token":"t","newPassword":"kata-sandi-rahasia"}`, nil)
	code, details = errCode(t, rec)
	if rec.Code != 422 || code != "password_policy_violation" || len(details) != 2 || details[0]["code"] != "too_short" || details[1]["code"] != "common" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "kata-sandi-rahasia") {
		t.Error("sandi bocor di respons galat")
	}
	rec = call(t, &fakeService{}, func(h *Handlers) http.HandlerFunc { return h.ResetPassword }, "POST", `{"token":"t","newPassword":"kata-sandi-rahasia-baru"}`, nil)
	if rec.Code != 204 {
		t.Fatalf("sukses = %d", rec.Code)
	}
}

func TestChangePassword_WrongCurrentAndSuccess(t *testing.T) {
	svc := &fakeService{err: identity.ErrWrongCurrentPassword}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.ChangePassword }, "POST", `{"currentPassword":"salah","newPassword":"baru-baru-baru-2026"}`, nil)
	code, details := errCode(t, rec)
	if rec.Code != 422 || code != "validation_failed" || len(details) != 1 || details[0]["code"] != "wrong_current_password" || details[0]["field"] != "currentPassword" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	svc = &fakeService{}
	rec = call(t, svc, func(h *Handlers) http.HandlerFunc { return h.ChangePassword }, "POST", `{"currentPassword":"lama-lama-2026","newPassword":"baru-baru-baru-2026"}`, nil)
	if rec.Code != 204 || svc.changeIn.CurrentPassword != "lama-lama-2026" || svc.changeIn.NewPassword != "baru-baru-baru-2026" || svc.changeIn.ClientIP != "203.0.113.9" {
		t.Fatalf("%d %+v", rec.Code, svc.changeIn)
	}
}

func TestMFAErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{identity.ErrMFARequired, 403, "mfa_required"},
		{identity.ErrMFAAlreadyEnrolled, 409, "conflict"},
		{identity.ErrMFANotEnrolled, 409, "invalid_state"},
		{identity.ErrInvalidMFACode, 422, "validation_failed"},
		{identity.ErrAccountLocked, 403, "account_locked"},
		{identity.ErrUnauthenticated, 401, "unauthenticated"},
		{identity.ErrForbidden, 403, "permission_denied"},
		{identity.ErrInvalidInput, 422, "validation_failed"},
	}
	for _, c := range cases {
		rec := call(t, &fakeService{err: c.err}, func(h *Handlers) http.HandlerFunc { return h.VerifyMFA }, "POST", `{"code":"123456"}`, nil)
		if got, _ := errCode(t, rec); rec.Code != c.status || got != c.code {
			t.Errorf("%v: %d %s, want %d %s", c.err, rec.Code, got, c.status, c.code)
		}
		assertNoStore(t, rec)
	}
}

func TestEnrollMFA(t *testing.T) {
	svc := &fakeService{enroll: identity.EnrollmentInfo{Secret: "JBSWY3DP", OTPAuthURI: "otpauth://totp/x?secret=JBSWY3DP"}}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.EnrollMFA }, "POST", "", nil)
	var b struct{ Data map[string]string }
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if rec.Code != 200 || b.Data["secret"] != "JBSWY3DP" || b.Data["otpauthUri"] == "" || len(b.Data) != 2 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	assertNoStore(t, rec)
}

// Q27: konfirmasi MFA menulis ulang cookie refresh dan memuat recovery code serta access token otp, tanpa refresh token.
func TestConfirmMFA_RewritesCookieAndReturnsSession(t *testing.T) {
	res := loginResult()
	res.MFAState = identity.MFANone
	svc := &fakeService{confirm: identity.ConfirmMFAResult{Codes: []string{"aaaa-bbbb", "cccc-dddd"}, Session: res}}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.ConfirmMFA }, "POST", `{"code":"123456"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	assertCookie(t, refreshCookieOf(t, rec), secretRefresh, 14*24*3600)
	var b struct {
		Data struct {
			Codes   []string
			Session map[string]any
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if len(b.Data.Codes) != 2 || b.Data.Session["accessToken"] != secretAccess || b.Data.Session["mfaState"] != "none" || len(b.Data.Session) != 3 {
		t.Errorf("data = %+v", b.Data)
	}
	if strings.Contains(rec.Body.String(), secretRefresh) {
		t.Error("refresh token bocor di body")
	}
	assertNoStore(t, rec)
	if svc.confirmIn.Code != "123456" || svc.confirmIn.ClientIP != "203.0.113.9" {
		t.Errorf("input = %+v", svc.confirmIn)
	}
}

// Bila penaikan sesi gagal setelah faktor aktif: recovery code tetap diserahkan, tanpa session dan tanpa cookie.
func TestConfirmMFA_NoSessionWhenUpgradeFailed(t *testing.T) {
	svc := &fakeService{confirm: identity.ConfirmMFAResult{Codes: []string{"aaaa-bbbb"}}}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.ConfirmMFA }, "POST", `{"code":"123456"}`, nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "session") || refreshCookieOf(t, rec) != nil {
		t.Fatalf("%d %s cookie=%v", rec.Code, rec.Body, refreshCookieOf(t, rec))
	}
	if !strings.Contains(rec.Body.String(), `"codes":["aaaa-bbbb"]`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestVerifyAndRecover_FlagAndCookie(t *testing.T) {
	svc := &fakeService{login: loginResult()}
	rec := call(t, svc, func(h *Handlers) http.HandlerFunc { return h.VerifyMFA }, "POST", `{"code":"123456"}`, nil)
	if rec.Code != 200 || svc.verifyIn.Recovery || svc.verifyIn.Code != "123456" {
		t.Fatalf("verify: %d %+v", rec.Code, svc.verifyIn)
	}
	assertCookie(t, refreshCookieOf(t, rec), secretRefresh, 14*24*3600)
	bodyHasNoRefresh(t, rec)

	svc = &fakeService{login: loginResult()}
	rec = call(t, svc, func(h *Handlers) http.HandlerFunc { return h.RecoverMFA }, "POST", `{"code":"aaaa-bbbb"}`, nil)
	if rec.Code != 200 || !svc.verifyIn.Recovery || svc.verifyIn.Code != "aaaa-bbbb" {
		t.Fatalf("recovery: %d %+v", rec.Code, svc.verifyIn)
	}
	assertCookie(t, refreshCookieOf(t, rec), secretRefresh, 14*24*3600)
}

func TestRegenerateRecoveryCodes(t *testing.T) {
	rec := call(t, &fakeService{recovery: identity.RecoveryCodes{Codes: []string{"a", "b"}}}, func(h *Handlers) http.HandlerFunc { return h.RegenerateRecoveryCodes }, "POST", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"codes":["a","b"]`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	assertNoStore(t, rec)
	rec = call(t, &fakeService{}, func(h *Handlers) http.HandlerFunc { return h.RegenerateRecoveryCodes }, "POST", "", nil)
	if !strings.Contains(rec.Body.String(), `"codes":[]`) {
		t.Errorf("codes nil harus []: %s", rec.Body)
	}
}

// Perakitan salah (Service tidak ada di context): 500 generik, tanpa menyebut sebab ke klien, tetap no-store.
func TestMissingService_Generic500(t *testing.T) {
	for name, h := range map[string]func(*Handlers) http.HandlerFunc{
		"login": func(h *Handlers) http.HandlerFunc { return h.Login }, "logout": func(h *Handlers) http.HandlerFunc { return h.Logout },
		"refresh": func(h *Handlers) http.HandlerFunc { return h.Refresh }, "enroll": func(h *Handlers) http.HandlerFunc { return h.EnrollMFA },
	} {
		rec := call(t, nil, h, "POST", `{"email":"a@b.test","password":"x"}`, nil)
		if code, _ := errCode(t, rec); rec.Code != 500 || code != "internal_error" || strings.Contains(rec.Body.String(), "identity.Service") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
		assertNoStore(t, rec)
	}
}

// Umur cookie mengikuti RefreshExpiresAt; token yang sudah/hampir kedaluwarsa tetap menulis Max-Age >= 1.
func TestCookieMaxAgeFloor(t *testing.T) {
	res := loginResult()
	res.RefreshExpiresAt = fixedNow.Add(-time.Hour)
	rec := call(t, &fakeService{login: res}, func(h *Handlers) http.HandlerFunc { return h.Login }, "POST", `{"email":"a@b.test","password":"x"}`, nil)
	if c := refreshCookieOf(t, rec); c == nil || c.MaxAge != 1 {
		t.Errorf("cookie = %+v", c)
	}
}

// Setiap respons (sukses dan galat) dari handler yang dibungkus NoStore membawa Cache-Control: no-store.
func TestNoStoreWrapperOnPanicFreePaths(t *testing.T) {
	rec := httptest.NewRecorder()
	NoStore(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	assertNoStore(t, rec)
}
