package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
)

func successOutcome(userID string) guard.Outcome {
	return guard.Outcome{Kind: guard.Success, User: store.User{ID: userID, Email: "a@example.test", Status: store.UserActive}}
}

func TestNewService_RequiresDeps(t *testing.T) {
	d := newFakeEnv().deps()
	d.Issuer = nil
	if _, err := NewService(d); err == nil || !strings.Contains(err.Error(), "Issuer") {
		t.Fatalf("NewService tanpa Issuer: %v", err)
	}
	d = newFakeEnv().deps()
	d.Keys = nil
	if _, err := NewAdmin(d); err == nil || !strings.Contains(err.Error(), "Keys") {
		t.Fatalf("NewAdmin tanpa Keys: %v", err)
	}
}

func TestLogin_NoMFA_IssuesTokenAndRegistersSuccess(t *testing.T) {
	e := newFakeEnv()
	e.login.out = successOutcome(userA)
	ctx := httpx.WithClientIP(tenantCtx(), "203.0.113.9")
	res, err := e.service().Login(ctx, sdkidentity.LoginInput{Email: "A@Example.test", Password: "pw", ClientIP: "198.51.100.1", UserAgent: "ua"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.MFAState != sdkidentity.MFANone || res.AccessToken != "ACCESS-"+sessA || res.ExpiresIn != 600 {
		t.Fatalf("hasil = %+v", res)
	}
	if res.RefreshToken != "RAW-REFRESH-TOKEN-SECRET" || res.RefreshExpiresAt.IsZero() {
		t.Fatalf("refresh token harus lewat LoginResult: %+v", res)
	}
	if e.login.in.IP != "203.0.113.9" {
		t.Errorf("IP klien harus dari httpx.ClientIP(ctx), bukan input: %q", e.login.in.IP)
	}
	c := e.issuer.claims[0]
	if c.Subject != userA || c.SessionID != sessA || c.TenantID != svcTenant.String() || len(c.AMR) != 1 || c.AMR[0] != "pwd" || len(c.Scope) != 0 {
		t.Errorf("klaim = %+v", c)
	}
	if got := e.sessions.started[0]; got.AMR[0] != "pwd" || got.AuthMethod != "password" || got.UserAgent != "ua" {
		t.Errorf("Start = %+v", got)
	}
	if len(e.lock.successes) != 1 || len(e.store.lastLogins) != 1 {
		t.Errorf("RegisterSuccess/TouchLastLogin harus sekali: %v %v", e.lock.successes, e.store.lastLogins)
	}
	// Refresh token tidak boleh muncul di JSON, String, maupun log.
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "RAW-REFRESH") || strings.Contains(fmt.Sprintf("%v %+v", res, res), "RAW-REFRESH") {
		t.Fatalf("refresh token bocor: %s", b)
	}
}

func TestLogin_MFAStates(t *testing.T) {
	cases := []struct {
		name      string
		st        MFAStatus
		want      sdkidentity.MFAState
		wantReg   int
		wantFresh bool
	}{
		{"none", MFAStatus{}, sdkidentity.MFANone, 1, true},
		{"admin tanpa faktor", MFAStatus{EnrollmentRequired: true}, sdkidentity.MFAEnrollRequired, 0, true},
		{"faktor aktif", MFAStatus{FactorActive: true}, sdkidentity.MFAVerifyRequired, 0, true},
		{"admin dengan faktor", MFAStatus{FactorActive: true, EnrollmentRequired: true}, sdkidentity.MFAVerifyRequired, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newFakeEnv()
			e.login.out = successOutcome(userA)
			e.status.st = tc.st
			res, err := e.service().Login(tenantCtx(), sdkidentity.LoginInput{Email: "a@example.test", Password: "pw"})
			if err != nil {
				t.Fatal(err)
			}
			if res.MFAState != tc.want {
				t.Errorf("MFAState = %q, want %q", res.MFAState, tc.want)
			}
			if len(e.lock.successes) != tc.wantReg {
				t.Errorf("RegisterSuccess = %d, want %d (hanya setelah autentikasi penuh)", len(e.lock.successes), tc.wantReg)
			}
			// Token selalu amr [pwd] pada login; keadaan dibaca segar (Invalidate sebelum Status).
			if c := e.issuer.claims[0]; len(c.AMR) != 1 || c.AMR[0] != "pwd" {
				t.Errorf("amr = %v", c.AMR)
			}
			if len(e.status.order) != 2 || e.status.order[0] != "invalidate" || e.status.order[1] != "status" {
				t.Errorf("urutan baca keadaan MFA = %v", e.status.order)
			}
		})
	}
}

func TestLogin_FailuresAreUniform(t *testing.T) {
	for _, r := range []guard.Reason{guard.ReasonUnknownEmail, guard.ReasonWrongPassword, guard.ReasonAccountLocked, guard.ReasonAccountDisabled, guard.ReasonInvalidHash} {
		e := newFakeEnv()
		e.login.out = guard.Outcome{Kind: guard.InvalidCredentials, Reason: r}
		_, err := e.service().Login(tenantCtx(), sdkidentity.LoginInput{Email: "x@example.test", Password: "pw"})
		if !errors.Is(err, sdkidentity.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", r, err)
		}
		if len(e.sessions.started) != 0 || len(e.issuer.claims) != 0 {
			t.Errorf("%s: tidak boleh ada sesi atau token", r)
		}
	}
}

func TestLogin_RateLimitedAndInfraErrors(t *testing.T) {
	e := newFakeEnv()
	e.login.out = guard.Outcome{Kind: guard.RateLimited, RetryAfter: 45 * time.Second, Reason: guard.ReasonPairBlocked}
	_, err := e.service().Login(tenantCtx(), sdkidentity.LoginInput{Email: "x@example.test"})
	if d, ok := sdkidentity.AsRateLimited(err); !ok || d != 45*time.Second || !errors.Is(err, sdkidentity.ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	e = newFakeEnv()
	e.login.err = errBoom
	_, err = e.service().Login(tenantCtx(), sdkidentity.LoginInput{})
	if err == nil || errors.Is(err, sdkidentity.ErrInvalidCredentials) {
		t.Fatalf("galat infrastruktur tidak boleh menyamar jadi kredensial salah: %v", err)
	}
	if _, err := e.service().Login(context.Background(), sdkidentity.LoginInput{}); err == nil {
		t.Fatal("tanpa tenant harus galat")
	}
}

func TestLogin_RehashAndInactiveMembership(t *testing.T) {
	e := newFakeEnv()
	out := successOutcome(userA)
	out.NeedsRehash = true
	e.login.out = out
	if _, err := e.service().Login(tenantCtx(), sdkidentity.LoginInput{Email: "a@example.test", Password: "sandi-lama-123"}); err != nil {
		t.Fatal(err)
	}
	if e.store.hashSet[userA] != "h:sandi-lama-123" {
		t.Errorf("hash ulang = %q", e.store.hashSet[userA])
	}

	e = newFakeEnv()
	e.login.out = successOutcome(userA)
	e.sessions.startErr = session.ErrUserNotActive
	_, err := e.service().Login(tenantCtx(), sdkidentity.LoginInput{Email: "a@example.test"})
	if !errors.Is(err, sdkidentity.ErrInvalidCredentials) {
		t.Fatalf("keanggotaan tidak aktif harus seragam: %v", err)
	}
}

func TestLogin_IssueFailureRevokesSession(t *testing.T) {
	e := newFakeEnv()
	e.login.out = successOutcome(userA)
	e.issuer.err = errBoom
	_, err := e.service().Login(tenantCtx(), sdkidentity.LoginInput{Email: "a@example.test"})
	if err == nil || errors.Is(err, sdkidentity.ErrInvalidCredentials) {
		t.Fatalf("kegagalan penerbitan harus galat infrastruktur: %v", err)
	}
	if len(e.sessions.revoked) != 1 || !strings.HasPrefix(e.sessions.revoked[0], sessA) {
		t.Fatalf("sesi yatim harus dicabut: %v", e.sessions.revoked)
	}
}

func TestRefresh_KeepsSessionAMRAndForwardsOrigin(t *testing.T) {
	e := newFakeEnv()
	e.sessions.rotateSess = session.Session{ID: sessA, UserID: userA, AMR: []string{"pwd"}}
	e.status.st = MFAStatus{FactorActive: true}
	res, err := e.service().Refresh(tenantCtx(), sdkidentity.RefreshInput{RefreshToken: "raw", Origin: "https://erp.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if e.sessions.rotated[0] != "raw|https://erp.example.com" {
		t.Errorf("Rotate = %v", e.sessions.rotated)
	}
	if c := e.issuer.claims[0]; len(c.AMR) != 1 || c.AMR[0] != "pwd" {
		t.Errorf("refresh tidak boleh menaikkan amr: %v", c.AMR)
	}
	if res.MFAState != sdkidentity.MFAVerifyRequired || res.RefreshToken != "NEW-REFRESH" {
		t.Errorf("hasil = %+v", res)
	}

	e = newFakeEnv()
	e.sessions.rotateSess = session.Session{ID: sessA, UserID: userA, AMR: []string{"pwd", "otp"}}
	res, err = e.service().Refresh(tenantCtx(), sdkidentity.RefreshInput{RefreshToken: "raw", Origin: "o"})
	if err != nil || res.MFAState != sdkidentity.MFANone {
		t.Fatalf("sesi ber-otp: %+v %v", res, err)
	}
	if c := e.issuer.claims[0]; len(c.AMR) != 2 || c.AMR[1] != "otp" {
		t.Errorf("amr sesi harus dipertahankan: %v", c.AMR)
	}
	if len(e.status.order) != 0 {
		t.Error("sesi ber-otp tidak perlu membaca keadaan MFA")
	}
}

func TestRefresh_ErrorMapping(t *testing.T) {
	cases := map[error]error{
		session.ErrInvalidRefresh:   sdkidentity.ErrInvalidRefresh,
		session.ErrOriginNotAllowed: sdkidentity.ErrOriginNotAllowed,
	}
	for in, want := range cases {
		e := newFakeEnv()
		e.sessions.rotateErr = in
		if _, err := e.service().Refresh(tenantCtx(), sdkidentity.RefreshInput{}); !errors.Is(err, want) {
			t.Errorf("%v -> %v, want %v", in, err, want)
		}
	}
}

func TestLogout_UsesPrincipalSessionOnly(t *testing.T) {
	e := newFakeEnv()
	svc := e.service()
	if err := svc.Logout(principalCtx(userA, sessA, "pwd")); err != nil {
		t.Fatal(err)
	}
	if len(e.sessions.revoked) != 1 || e.sessions.revoked[0] != sessA+"|logout" {
		t.Fatalf("revoked = %v", e.sessions.revoked)
	}
	if err := svc.Logout(tenantCtx()); !errors.Is(err, sdkidentity.ErrUnauthenticated) {
		t.Fatalf("tanpa principal: %v", err)
	}
	e.sessions.revokeErr = session.ErrSessionNotFound // sesi sudah hilang: idempoten
	if err := svc.Logout(principalCtx(userA, sessA)); err != nil {
		t.Fatalf("logout idempoten: %v", err)
	}
}

func TestForgotAndReset_Delegate(t *testing.T) {
	e := newFakeEnv()
	svc := e.service()
	if err := svc.ForgotPassword(tenantCtx(), sdkidentity.ForgotInput{Email: "a@example.test", Locale: "en"}); err != nil {
		t.Fatal(err)
	}
	if e.reset.requested[0] != "a@example.test|en" {
		t.Errorf("Request = %v", e.reset.requested)
	}
	if err := svc.ResetPassword(tenantCtx(), sdkidentity.ResetInput{Token: "tok", NewPassword: "x"}); err != nil || e.reset.completed[0] != "tok" {
		t.Fatalf("Complete: %v %v", err, e.reset.completed)
	}
	e.reset.err = &guard.RateLimitedError{RetryAfter: 9 * time.Second, Cause: errBoom}
	err := svc.ForgotPassword(tenantCtx(), sdkidentity.ForgotInput{Email: "a@example.test"})
	if d, ok := sdkidentity.AsRateLimited(err); !ok || d != 9*time.Second {
		t.Fatalf("err = %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	const strong = "kata-sandi-baru-yang-panjang-2026"
	setup := func() (*fakeEnv, sdkidentity.Service) {
		e := newFakeEnv()
		e.store.addUser(userA, "a@example.test", "h:sandi-lama-panjang")
		e.store.changeRev = []string{sessB}
		return e, e.service()
	}
	t.Run("sukses mencabut sesi lain kecuali sesi ini", func(t *testing.T) {
		e, svc := setup()
		ctx := httpx.WithClientIP(principalCtx(userA, sessA, "pwd"), "203.0.113.9")
		if err := svc.ChangePassword(ctx, sdkidentity.ChangeInput{CurrentPassword: "sandi-lama-panjang", NewPassword: strong}); err != nil {
			t.Fatal(err)
		}
		if e.store.changed[0] != userA+"|"+sessA+"|h:"+strong {
			t.Errorf("ChangePassword = %v", e.store.changed)
		}
		if len(e.sessions.marked) != 1 || e.sessions.marked[0] != sessB {
			t.Errorf("MarkRevoked = %v", e.sessions.marked)
		}
		if len(e.lock.successes) != 1 {
			t.Errorf("sukses lockout = %v", e.lock.successes)
		}
	})
	t.Run("sandi lama salah dicatat sebagai kegagalan", func(t *testing.T) {
		e, svc := setup()
		err := svc.ChangePassword(principalCtx(userA, sessA), sdkidentity.ChangeInput{CurrentPassword: "salah-salah-salah", NewPassword: strong})
		if !errors.Is(err, sdkidentity.ErrWrongCurrentPassword) || len(e.store.changed) != 0 || len(e.lock.failures) != 1 {
			t.Fatalf("err=%v changed=%v failures=%v", err, e.store.changed, e.lock.failures)
		}
	})
	t.Run("kebijakan sandi", func(t *testing.T) {
		e, svc := setup()
		err := svc.ChangePassword(principalCtx(userA, sessA), sdkidentity.ChangeInput{CurrentPassword: "sandi-lama-panjang", NewPassword: "pendek"})
		var pe *sdkidentity.PasswordPolicyError
		if !errors.As(err, &pe) || len(pe.Codes) == 0 || len(e.store.changed) != 0 {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(err.Error(), "pendek") {
			t.Error("galat tidak boleh memuat sandi")
		}
	})
	t.Run("pasangan diblokir", func(t *testing.T) {
		e, svc := setup()
		e.lock.checkState = guard.State{PairBlocked: true, RetryAfter: 30 * time.Second}
		err := svc.ChangePassword(principalCtx(userA, sessA), sdkidentity.ChangeInput{CurrentPassword: "sandi-lama-panjang", NewPassword: strong})
		if d, ok := sdkidentity.AsRateLimited(err); !ok || d != 30*time.Second {
			t.Fatalf("err = %v", err)
		}
		if len(e.gate.hashed) != 0 || len(e.store.changed) != 0 {
			t.Error("tidak boleh memverifikasi atau menyimpan saat diblokir")
		}
	})
	t.Run("akun terkunci", func(t *testing.T) {
		e, svc := setup()
		e.lock.checkState = guard.State{AccountLocked: true}
		err := svc.ChangePassword(principalCtx(userA, sessA), sdkidentity.ChangeInput{CurrentPassword: "sandi-lama-panjang", NewPassword: strong})
		if !errors.Is(err, sdkidentity.ErrAccountLocked) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("tanpa principal", func(t *testing.T) {
		_, svc := setup()
		if err := svc.ChangePassword(tenantCtx(), sdkidentity.ChangeInput{}); !errors.Is(err, sdkidentity.ErrUnauthenticated) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("IDOR: pengguna selalu dari Principal", func(t *testing.T) {
		e, svc := setup()
		e.store.addUser(userB, "b@example.test", "h:punya-b-panjang-sekali")
		// Principal A tidak punya cara menyebut B; sandi lama B tidak berlaku untuk A.
		err := svc.ChangePassword(principalCtx(userA, sessA), sdkidentity.ChangeInput{CurrentPassword: "punya-b-panjang-sekali", NewPassword: strong})
		if !errors.Is(err, sdkidentity.ErrWrongCurrentPassword) {
			t.Fatalf("err = %v", err)
		}
		for _, c := range e.store.changed {
			if strings.HasPrefix(c, userB) {
				t.Fatal("sandi B tidak boleh berubah")
			}
		}
	})
}

func TestMFA_UserAlwaysFromPrincipal(t *testing.T) {
	e := newFakeEnv()
	svc := e.service()
	ctx := principalCtx(userA, sessA, "pwd")
	if _, err := svc.BeginMFAEnrollment(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmMFA(ctx, sdkidentity.ConfirmMFAInput{Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyMFA(ctx, sdkidentity.VerifyInput{Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range e.mfa.calls {
		if !strings.HasSuffix(c, "|"+userA) {
			t.Errorf("panggilan MFA untuk pengguna selain Principal: %s", c)
		}
	}
	if slicesIndex(e.status.invalidated, userA) < 0 {
		t.Error("cache keadaan MFA harus dibuang saat enroll/confirm")
	}
	// Tanpa principal: ditolak sebelum menyentuh MFA.
	e.mfa.calls = nil
	for _, f := range []func() error{
		func() error { _, err := svc.BeginMFAEnrollment(tenantCtx()); return err },
		func() error { _, err := svc.ConfirmMFA(tenantCtx(), sdkidentity.ConfirmMFAInput{}); return err },
		func() error { _, err := svc.VerifyMFA(tenantCtx(), sdkidentity.VerifyInput{}); return err },
		func() error { _, err := svc.RegenerateRecoveryCodes(tenantCtx()); return err },
	} {
		if err := f(); !errors.Is(err, sdkidentity.ErrUnauthenticated) {
			t.Errorf("err = %v", err)
		}
	}
	if len(e.mfa.calls) != 0 {
		t.Errorf("MFA tersentuh tanpa principal: %v", e.mfa.calls)
	}
}

func slicesIndex(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestConfirmMFA_ElevatesSessionAndRegistersSuccess(t *testing.T) {
	e := newFakeEnv()
	codes, err := e.service().ConfirmMFA(principalCtx(userA, sessA, "pwd"), sdkidentity.ConfirmMFAInput{Code: "123456"})
	if err != nil || len(codes.Codes) != 1 {
		t.Fatalf("%v %v", codes, err)
	}
	if len(e.sessions.upgraded) != 1 || e.sessions.upgraded[0] != userA+"|"+sessA+"|otp" {
		t.Errorf("Upgrade = %v", e.sessions.upgraded)
	}
	// Rotasi refresh token (PRD-001 Q27): hasil membawa refresh token baru yang wajib ditulis ulang ke cookie.
	if codes.Session.RefreshToken != "UPGRADED-REFRESH" || codes.Session.AccessToken == "" {
		t.Errorf("hasil sesi = %+v", codes.Session)
	}
	if c := e.issuer.claims[len(e.issuer.claims)-1]; len(c.AMR) != 2 || c.AMR[1] != "otp" {
		t.Errorf("klaim = %+v", c)
	}
	if len(e.lock.successes) != 1 {
		t.Errorf("ConfirmMFA harus mencatat sukses lockout (mfa.ConfirmEnrollment tidak): %v", e.lock.successes)
	}
}

// Kegagalan menaikkan sesi setelah faktor aktif tidak menghilangkan recovery code (tampil sekali); Session kosong.
func TestConfirmMFA_UpgradeFailureStillReturnsCodes(t *testing.T) {
	for _, uerr := range []error{errBoom, session.ErrSessionNotFound, session.ErrAlreadyUpgraded} {
		e := newFakeEnv()
		e.sessions.upgradeErr = uerr
		res, err := e.service().ConfirmMFA(principalCtx(userA, sessA, "pwd"), sdkidentity.ConfirmMFAInput{Code: "123456"})
		if err != nil || len(res.Codes) != 1 {
			t.Fatalf("%v: %+v %v", uerr, res, err)
		}
		if res.Session.RefreshToken != "" {
			t.Errorf("%v: refresh token tidak boleh ada tanpa rotasi: %+v", uerr, res.Session)
		}
	}
}

// Access token diterbitkan lebih dulu: kegagalan menandatangani tidak boleh merotasi refresh token sesi.
func TestVerifyMFA_IssueFailureDoesNotRotate(t *testing.T) {
	e := newFakeEnv()
	e.issuer.err = errBoom
	if _, err := e.service().VerifyMFA(principalCtx(userA, sessA, "pwd"), sdkidentity.VerifyInput{Code: "123456"}); err == nil {
		t.Fatal("harus galat")
	}
	if len(e.sessions.upgraded) != 0 {
		t.Errorf("sesi tidak boleh dirotasi: %v", e.sessions.upgraded)
	}
}

func TestConfirmMFA_WrongCodeDoesNotElevate(t *testing.T) {
	e := newFakeEnv()
	e.mfa.confirmErr = mfa.ErrInvalidCode
	_, err := e.service().ConfirmMFA(principalCtx(userA, sessA, "pwd"), sdkidentity.ConfirmMFAInput{Code: "000000"})
	if !errors.Is(err, sdkidentity.ErrInvalidMFACode) {
		t.Fatalf("err = %v", err)
	}
	if len(e.sessions.upgraded) != 0 {
		t.Errorf("sesi tidak boleh dinaikkan: %v", e.sessions.upgraded)
	}
}

func TestVerifyMFA_ElevatesAndIssuesOTPToken(t *testing.T) {
	e := newFakeEnv()
	for _, recovery := range []bool{false, true} {
		res, err := e.service().VerifyMFA(principalCtx(userA, sessA, "pwd"), sdkidentity.VerifyInput{Code: "123456", Recovery: recovery})
		if err != nil {
			t.Fatal(err)
		}
		c := e.issuer.claims[len(e.issuer.claims)-1]
		if len(c.AMR) != 2 || c.AMR[0] != "pwd" || c.AMR[1] != "otp" || c.SessionID != sessA || c.Subject != userA {
			t.Errorf("klaim = %+v", c)
		}
		if res.MFAState != sdkidentity.MFANone || res.RefreshToken != "UPGRADED-REFRESH" || res.AccessToken == "" {
			t.Errorf("hasil = %+v (refresh token hasil rotasi harus ada)", res)
		}
	}
	if e.mfa.calls[0] != "verify|"+userA || e.mfa.calls[1] != "recovery|"+userA {
		t.Errorf("calls = %v", e.mfa.calls)
	}
	// Sesi sudah otp (kalah balapan): access token otp tanpa RefreshToken, cookie pemenang tidak ditimpa.
	e.sessions.upgradeErr = session.ErrAlreadyUpgraded
	res, err := e.service().VerifyMFA(principalCtx(userA, sessA, "pwd"), sdkidentity.VerifyInput{Code: "123456"})
	if err != nil || res.RefreshToken != "" || res.AccessToken == "" {
		t.Errorf("sudah dinaikkan: %+v %v", res, err)
	}
}

func TestVerifyMFA_ErrorMapping(t *testing.T) {
	e := newFakeEnv()
	svc := e.service()
	ctx := principalCtx(userA, sessA, "pwd")
	e.mfa.verifyErr = mfa.ErrInvalidCode
	if _, err := svc.VerifyMFA(ctx, sdkidentity.VerifyInput{Code: "1"}); !errors.Is(err, sdkidentity.ErrInvalidMFACode) {
		t.Errorf("err = %v", err)
	}
	e.mfa.verifyErr = &mfa.LockedError{RetryAfter: 30 * time.Second}
	if d, ok := sdkidentity.AsRateLimited(func() error { _, err := svc.VerifyMFA(ctx, sdkidentity.VerifyInput{}); return err }()); !ok || d != 30*time.Second {
		t.Error("LockedError dengan RetryAfter harus jadi RateLimitedError")
	}
	e.mfa.verifyErr = &mfa.LockedError{AccountLocked: true}
	if _, err := svc.VerifyMFA(ctx, sdkidentity.VerifyInput{}); !errors.Is(err, sdkidentity.ErrAccountLocked) {
		t.Errorf("err = %v", err)
	}
	if len(e.sessions.upgraded) != 0 {
		t.Errorf("sesi tidak boleh dinaikkan saat verifikasi gagal: %v", e.sessions.upgraded)
	}
	// Sesi milik pengguna lain (klaim sid tidak cocok dengan pengguna): tidak dinaikkan.
	e.mfa.verifyErr = nil
	// (session.Service.Upgrade menolak sesi bukan milik pengguna dengan ErrSessionNotFound.)
	e.sessions.upgradeErr = session.ErrSessionNotFound
	if _, err := svc.VerifyMFA(principalCtx(userA, sessB, "pwd"), sdkidentity.VerifyInput{}); !errors.Is(err, sdkidentity.ErrUnauthenticated) {
		t.Errorf("sesi bukan milik principal: %v", err)
	}
	if len(e.sessions.upgraded) != 1 || e.sessions.upgraded[0] != userA+"|"+sessB+"|otp" {
		t.Errorf("Upgrade harus dipanggil dengan pengguna Principal: %v", e.sessions.upgraded)
	}
}

func TestRegenerateRecoveryCodes_RequiresMFAVerifiedSession(t *testing.T) {
	e := newFakeEnv()
	svc := e.service()
	if _, err := svc.RegenerateRecoveryCodes(principalCtx(userA, sessA, "pwd")); !errors.Is(err, sdkidentity.ErrMFARequired) {
		t.Fatalf("err = %v", err)
	}
	if len(e.mfa.calls) != 0 {
		t.Fatal("tidak boleh menyentuh MFA tanpa otp")
	}
	if c, err := svc.RegenerateRecoveryCodes(principalCtx(userA, sessA, "pwd", "otp")); err != nil || len(c.Codes) != 1 {
		t.Fatalf("%v %v", c, err)
	}
}

func TestSecretsNeverLogged(t *testing.T) {
	var buf bytes.Buffer
	e := newFakeEnv()
	d := e.deps()
	d.Log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	svc, err := NewService(d)
	if err != nil {
		t.Fatal(err)
	}
	e.login.out = successOutcome(userA)
	_, _ = svc.Login(tenantCtx(), sdkidentity.LoginInput{Email: "a@example.test", Password: "SANDI-RAHASIA-123"})
	e.login.out = guard.Outcome{Kind: guard.InvalidCredentials, Reason: guard.ReasonWrongPassword}
	_, _ = svc.Login(tenantCtx(), sdkidentity.LoginInput{Email: "a@example.test", Password: "SANDI-RAHASIA-123"})
	for _, s := range []string{"SANDI-RAHASIA", "RAW-REFRESH-TOKEN-SECRET", "ACCESS-", "a@example.test"} {
		if strings.Contains(buf.String(), s) {
			t.Errorf("log memuat %q: %s", s, buf.String())
		}
	}
}
