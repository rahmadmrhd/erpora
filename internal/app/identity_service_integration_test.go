//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/config"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/password"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/internal/platform/testsupport/secretkey"
	"erpora/sdk/authz"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
	sdkmail "erpora/sdk/mail"
)

const (
	svcOrigin = "http://127.0.0.1:8080"
	svcIP     = "203.0.113.10"
	pwA       = "kata-sandi-pengguna-a-2026"
	pwB       = "kata-sandi-pengguna-b-2026"
	pwAdmin   = "kata-sandi-admin-kuat-2026"
)

// syncBuffer adalah penampung log aman-serentak (pekerja latar reset menulis dari goroutine lain).
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureMail adalah pengirim surat tiruan: menyimpan pesan agar tautan reset dapat dibaca tes.
type captureMail struct {
	mu   sync.Mutex
	msgs []sdkmail.Message
}

func (c *captureMail) Send(_ context.Context, m sdkmail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

func (c *captureMail) all() []sdkmail.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sdkmail.Message(nil), c.msgs...)
}

func newBufLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// svcEnv adalah perakitan lengkap (assemble sungguhan) dengan database sungguhan untuk menguji
// identity.Service dan identity.Admin ujung ke ujung.
type svcEnv struct {
	t    *testing.T
	a    *assembly
	ctx  context.Context // tenant
	mail *captureMail
	logs *syncBuffer
}

func newSvcEnv(t *testing.T) *svcEnv {
	t.Helper()
	dsn := pgtest.Start(t)
	db := openTestDB(t, dsn)
	cfg := config.Config{
		DatabaseURL: dsn, Env: config.EnvDevelopment, HTTPAddr: "127.0.0.1:8080", AuthzStub: "allow",
		DBSlowQuery: 200 * time.Millisecond, SecretKey: secretkey.Bytes(),
		Argon2: password.Params{MemoryKiB: 8192, Time: 1, Parallelism: 1}, // murah: bukan default OWASP
	}
	if err := migrate.Up(context.Background(), db, cfg.Env); err != nil {
		t.Fatal(err)
	}
	reg, api, err := buildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logs := &syncBuffer{}
	mailer := &captureMail{}
	a, err := assemble(ctx, cfg, newBufLogger(logs), db, testProviders(), reg, api, withMailSender(mailer))
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity.Service == nil || a.Identity.Admin == nil {
		t.Fatal("identityStack.Service/Admin harus terisi")
	}
	e := &svcEnv{t: t, a: a, ctx: tenancy.WithTenant(ctx, a.Tenant), mail: mailer, logs: logs}
	t.Cleanup(e.assertNoSecretsInLogs)
	return e
}

// secrets adalah nilai rahasia yang tidak boleh muncul di log mana pun.
var secretsSeen sync.Map

func (e *svcEnv) remember(vals ...string) {
	for _, v := range vals {
		if v != "" {
			secretsSeen.Store(e.t.Name()+"|"+v, v)
		}
	}
}

func (e *svcEnv) assertNoSecretsInLogs() {
	log := e.logs.String()
	secretsSeen.Range(func(k, v any) bool {
		if strings.HasPrefix(k.(string), e.t.Name()+"|") && strings.Contains(log, v.(string)) {
			e.t.Errorf("rahasia muncul di log: %.12s...", v.(string))
		}
		return true
	})
}

func (e *svcEnv) svc() sdkidentity.Service { return e.a.Identity.Service }
func (e *svcEnv) adm() sdkidentity.Admin   { return e.a.Identity.Admin }

func (e *svcEnv) sys(reason string) context.Context { return authz.AsSystem(e.ctx, reason) }
func (e *svcEnv) ipCtx() context.Context            { return httpx.WithClientIP(e.ctx, svcIP) }

// admin membuat administrator lewat Admin.CreateAdmin (hak sistem).
func (e *svcEnv) createAdmin(email, pw string) sdkidentity.UserSummary {
	e.t.Helper()
	sum, err := e.adm().CreateAdmin(e.sys("cli:core:create-admin"), sdkidentity.CreateAdminInput{Email: email, Name: "Admin", Password: pw})
	if err != nil {
		e.t.Fatalf("CreateAdmin: %v", err)
	}
	return sum
}

// createUser membuat pengguna biasa (tanpa role) dengan sandi pw.
func (e *svcEnv) createUser(email, pw string) identitystore.User {
	e.t.Helper()
	phc, err := e.a.Identity.Hasher.Hash(pw)
	if err != nil {
		e.t.Fatal(err)
	}
	u, err := e.a.Identity.Store.CreateUser(e.ctx, e.a.Tenant, identitystore.NewUser{Email: email, Name: "Pengguna", PasswordHash: phc})
	if err != nil {
		e.t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func (e *svcEnv) login(email, pw string) (sdkidentity.LoginResult, error) {
	e.remember(pw)
	res, err := e.svc().Login(e.ipCtx(), sdkidentity.LoginInput{Email: email, Password: pw, UserAgent: "uji"})
	e.remember(res.AccessToken, res.RefreshToken)
	return res, err
}

func (e *svcEnv) mustLogin(email, pw string) sdkidentity.LoginResult {
	e.t.Helper()
	res, err := e.login(email, pw)
	if err != nil {
		e.t.Fatalf("Login(%s): %v", email, err)
	}
	return res
}

// as membangun context handler untuk access token raw seperti yang dilakukan middleware Authenticate (principal
// dari klaim token yang sudah terverifikasi, IP klien dari httpx).
func (e *svcEnv) as(raw string) context.Context {
	e.t.Helper()
	c, err := e.a.Identity.Verifier.Verify(raw)
	if err != nil {
		e.t.Fatalf("token tidak terverifikasi: %v", err)
	}
	p := sdkidentity.Principal{UserID: c.Subject, Kind: sdkidentity.KindHuman, SessionID: c.SessionID,
		AuthMethod: sdkidentity.AuthMethodPassword, AMR: c.AMR}
	return httpx.WithClientIP(sdkidentity.WithPrincipal(e.ctx, p), svcIP)
}

// status adalah kode HTTP route terproteksi dengan token raw (akses nyata lewat middleware dan otorisasi bertipe).
func (e *svcEnv) status(raw string) int {
	return get(e.a.Router, protectedPath, bearerHdr(raw)).Code
}

func (e *svcEnv) refresh(raw string) (sdkidentity.LoginResult, error) {
	e.remember(raw)
	res, err := e.svc().Refresh(e.ipCtx(), sdkidentity.RefreshInput{RefreshToken: raw, Origin: svcOrigin, UserAgent: "uji"})
	e.remember(res.AccessToken, res.RefreshToken)
	return res, err
}

var totp mfa.TOTP

func (e *svcEnv) code(secret []byte, offset time.Duration) string {
	return totp.Code(secret, time.Now().Add(offset))
}

func decodeSecret(t *testing.T, b32 string) []byte {
	t.Helper()
	s, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(b32)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Alur penuh administrator: create-admin -> login (enroll_required, 403 mfa_required) -> enroll -> confirm -> refresh
// (amr otp, akses) -> login ulang (verify_required) -> verify TOTP, replay ditolak -> recovery code sekali pakai.
func TestIdentityService_AdminMFAFullFlow(t *testing.T) {
	e := newSvcEnv(t)
	adm := e.createAdmin("admin@example.test", pwAdmin)

	login := e.mustLogin("Admin@Example.test", pwAdmin)
	if login.MFAState != sdkidentity.MFAEnrollRequired || login.RefreshToken == "" || login.ExpiresIn != 600 {
		t.Fatalf("login = %+v", login)
	}
	if code := e.status(login.AccessToken); code != http.StatusForbidden {
		t.Fatalf("administrator tanpa MFA harus 403 mfa_required, dapat %d", code)
	}

	ctx := e.as(login.AccessToken)
	info, err := e.svc().BeginMFAEnrollment(ctx)
	if err != nil {
		t.Fatalf("BeginMFAEnrollment: %v", err)
	}
	e.remember(info.Secret)
	secret := decodeSecret(t, info.Secret)

	// Kode salah ditolak seragam; kode benar (langkah sebelumnya, masih dalam jendela) mengaktifkan faktor.
	if _, err := e.svc().ConfirmMFA(ctx, sdkidentity.ConfirmMFAInput{Code: "000000"}); !errors.Is(err, sdkidentity.ErrInvalidMFACode) {
		t.Fatalf("kode salah: %v", err)
	}
	rc, err := e.svc().ConfirmMFA(ctx, sdkidentity.ConfirmMFAInput{Code: e.code(secret, -30*time.Second)})
	if err != nil || len(rc.Codes) != 10 {
		t.Fatalf("ConfirmMFA: %d kode, %v", len(rc.Codes), err)
	}
	e.remember(rc.Codes...)
	// Token lama (amr pwd) kini berhadapan dengan faktor aktif: tetap 403 sampai amr memuat otp.
	if code := e.status(login.AccessToken); code != http.StatusForbidden {
		t.Fatalf("token pwd setelah faktor aktif harus 403, dapat %d", code)
	}

	// Konfirmasi MFA menaikkan sesi dan MEROTASI refresh token (PRD-001 Q27): hasil memuat access token otp dan
	// refresh token baru; klien tidak perlu memanggil Refresh lagi.
	conf := rc.Session
	e.remember(conf.AccessToken, conf.RefreshToken)
	if conf.AccessToken == "" || conf.RefreshToken == "" || conf.RefreshToken == login.RefreshToken || conf.MFAState != sdkidentity.MFANone {
		t.Fatalf("hasil konfirmasi harus membawa sesi otp baru: %+v", conf)
	}
	if c, _ := e.a.Identity.Verifier.Verify(conf.AccessToken); strings.Join(c.AMR, ",") != "pwd,otp" || e.status(conf.AccessToken) != http.StatusOK {
		t.Fatalf("access token konfirmasi: amr=%v status=%d", c.AMR, e.status(conf.AccessToken))
	}
	// Refresh token pra-MFA mati TANPA mencabut sesi (bukan serangan reuse): sesi ber-otp tetap hidup.
	if _, err := e.refresh(login.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("refresh token pra-MFA harus ditolak: %v", err)
	}
	if code := e.status(conf.AccessToken); code != http.StatusOK {
		t.Fatalf("penolakan token pra-MFA tidak boleh mencabut sesi, status %d", code)
	}
	ref, err := e.refresh(conf.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh dengan token baru: %v", err)
	}
	if ref.MFAState != sdkidentity.MFANone || e.status(ref.AccessToken) != http.StatusOK {
		t.Fatalf("setelah konfirmasi: state=%s status=%d", ref.MFAState, e.status(ref.AccessToken))
	}
	if c, _ := e.a.Identity.Verifier.Verify(ref.AccessToken); strings.Join(c.AMR, ",") != "pwd,otp" {
		t.Fatalf("amr = %v", c.AMR)
	}

	// Login ulang: verify_required; token pwd 403; kode TOTP naik ke otp; replay kode yang sama ditolak.
	l2 := e.mustLogin("admin@example.test", pwAdmin)
	if l2.MFAState != sdkidentity.MFAVerifyRequired || e.status(l2.AccessToken) != http.StatusForbidden {
		t.Fatalf("login ulang = %+v", l2)
	}
	ctx2 := e.as(l2.AccessToken)
	totpNow := e.code(secret, 0)
	// Pemegang cookie pra-MFA yang salah kode tidak pernah mendapat token otp dan tidak mengubah sesi.
	if bad, err := e.svc().VerifyMFA(ctx2, sdkidentity.VerifyInput{Code: "000000"}); err == nil || bad.AccessToken != "" || bad.RefreshToken != "" {
		t.Fatalf("verifikasi gagal tidak boleh menghasilkan token: %+v %v", bad, err)
	}
	v, err := e.svc().VerifyMFA(ctx2, sdkidentity.VerifyInput{Code: totpNow})
	e.remember(v.AccessToken, v.RefreshToken)
	if err != nil || v.RefreshToken == "" || v.RefreshToken == l2.RefreshToken || e.status(v.AccessToken) != http.StatusOK {
		t.Fatalf("VerifyMFA: %+v %v", v, err)
	}
	if _, err := e.refresh(l2.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("refresh token pra-MFA harus mati setelah verify: %v", err)
	}
	if e.status(v.AccessToken) != http.StatusOK {
		t.Fatal("penolakan token pra-MFA tidak boleh mencabut sesi")
	}
	if r2, err := e.refresh(v.RefreshToken); err != nil || e.status(r2.AccessToken) != http.StatusOK {
		t.Fatalf("refresh dengan token hasil verify: %+v %v", r2, err)
	}
	if _, err := e.svc().VerifyMFA(ctx2, sdkidentity.VerifyInput{Code: totpNow}); !errors.Is(err, sdkidentity.ErrInvalidMFACode) {
		t.Fatalf("replay harus ditolak: %v", err)
	}

	// Recovery code: sekali pakai.
	l3 := e.mustLogin("admin@example.test", pwAdmin)
	ctx3 := e.as(l3.AccessToken)
	r, err := e.svc().VerifyMFA(ctx3, sdkidentity.VerifyInput{Code: rc.Codes[0], Recovery: true})
	if err != nil || e.status(r.AccessToken) != http.StatusOK || r.RefreshToken == "" || r.RefreshToken == l3.RefreshToken {
		t.Fatalf("recovery harus merotasi refresh token juga: %+v %v", r, err)
	}
	if _, err := e.refresh(l3.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("refresh token pra-MFA (jalur recovery) harus mati: %v", err)
	}
	l4 := e.mustLogin("admin@example.test", pwAdmin)
	if _, err := e.svc().VerifyMFA(e.as(l4.AccessToken), sdkidentity.VerifyInput{Code: rc.Codes[0], Recovery: true}); !errors.Is(err, sdkidentity.ErrInvalidMFACode) {
		t.Fatalf("recovery code dipakai ulang harus ditolak: %v", err)
	}

	// Regenerasi recovery code menuntut sesi ber-otp: sesi l4 baru berhak setelah verify TOTP (langkah berikutnya).
	if _, err := e.svc().RegenerateRecoveryCodes(e.as(l4.AccessToken)); !errors.Is(err, sdkidentity.ErrMFARequired) {
		t.Fatalf("regenerasi tanpa otp: %v", err)
	}
	v4, err := e.svc().VerifyMFA(e.as(l4.AccessToken), sdkidentity.VerifyInput{Code: e.code(secret, 30*time.Second)})
	if err != nil {
		t.Fatalf("verify l4: %v", err)
	}
	rc2, err := e.svc().RegenerateRecoveryCodes(e.as(v4.AccessToken))
	if err != nil || len(rc2.Codes) != 10 {
		t.Fatalf("regenerasi: %v", err)
	}
	e.remember(rc2.Codes...)

	// Reset MFA oleh admin lain/CLI: faktor hilang, seluruh sesi admin tercabut (token ber-otp lama 401).
	if err := e.adm().ResetMFA(e.sys("cli:core:reset-mfa"), adm.Email); err != nil {
		t.Fatalf("ResetMFA: %v", err)
	}
	if code := e.status(v4.AccessToken); code != http.StatusUnauthorized {
		t.Fatalf("sesi ber-otp setelah reset MFA harus 401, dapat %d", code)
	}
	if code := e.status(ref.AccessToken); code != http.StatusUnauthorized {
		t.Fatalf("sesi lain setelah reset MFA harus 401, dapat %d", code)
	}
	if l := e.mustLogin(adm.Email, pwAdmin); l.MFAState != sdkidentity.MFAEnrollRequired {
		t.Fatalf("setelah reset MFA: %+v", l)
	}
}

// Kegagalan login seragam (tak dikenal, sandi salah, terkunci, nonaktif), waktu respons tidak lebih cepat dari lantai,
// jeda bertingkat memberi RateLimitedError, dan unlock admin membuka akun.
func TestIdentityService_LoginFailuresUniformAndUnlock(t *testing.T) {
	e := newSvcEnv(t)
	u := e.createUser("u@example.test", pwA)
	locked := e.createUser("locked@example.test", pwA)
	disabled := e.createUser("off@example.test", pwA)
	if ok, err := e.a.Identity.Store.LockUser(e.ctx, e.a.Tenant, locked.ID, time.Now()); err != nil || !ok {
		t.Fatalf("LockUser: %v %v", ok, err)
	}
	if err := e.a.Identity.Store.SetUserStatus(e.ctx, e.a.Tenant, disabled.ID, identitystore.UserDisabled); err != nil {
		t.Fatal(err)
	}

	floor := 150 * time.Millisecond // lantai guard 200 ms dikurangi toleransi jam
	for name, c := range map[string][2]string{
		"tak dikenal":   {"tidak-ada@example.test", pwA},
		"sandi salah":   {u.Email, "salah-salah-salah-1"},
		"akun terkunci": {locked.Email, pwA},
		"akun nonaktif": {disabled.Email, pwA},
	} {
		start := time.Now()
		_, err := e.login(c[0], c[1])
		if !errors.Is(err, sdkidentity.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials seragam", name, err)
		}
		if d := time.Since(start); d < floor {
			t.Errorf("%s: jawaban %v lebih cepat dari lantai waktu seragam", name, d)
		}
	}

	// Pasangan (u, IP) dijeda setelah 5 kegagalan: sandi benar pun ditolak dengan 429 + Retry-After.
	for i := 0; i < 5; i++ {
		_, _ = e.login(u.Email, "salah-salah-salah-1")
	}
	_, err := e.login(u.Email, pwA)
	if d, ok := sdkidentity.AsRateLimited(err); !ok || d <= 0 {
		t.Fatalf("setelah 5 gagal harus RateLimitedError dengan Retry-After, err = %v", err)
	}
	// Admin.UnlockAccount menghapus jeda pasangan dan membuka akun locked.
	for _, email := range []string{u.Email, locked.Email} {
		if err := e.adm().UnlockAccount(e.sys("cli:core:unlock"), email); err != nil {
			t.Fatalf("UnlockAccount(%s): %v", email, err)
		}
	}
	if _, err := e.login(u.Email, pwA); err != nil {
		t.Fatalf("setelah unlock: %v", err)
	}
	if _, err := e.login(locked.Email, pwA); err != nil {
		t.Fatalf("akun locked setelah unlock: %v", err)
	}
	if err := e.adm().UnlockAccount(e.sys("cli:core:unlock"), u.Email); err != nil { // idempoten
		t.Fatalf("unlock idempoten: %v", err)
	}
}

// Refresh berotasi, reuse mencabut family, Origin diperiksa; logout dan ganti sandi; IDOR antar pengguna.
func TestIdentityService_RefreshLogoutChangePasswordAndIDOR(t *testing.T) {
	e := newSvcEnv(t)
	ua := e.createUser("a@example.test", pwA)
	ub := e.createUser("b@example.test", pwB)

	// --- Refresh ---
	la := e.mustLogin(ua.Email, pwA)
	if la.MFAState != sdkidentity.MFANone || e.status(la.AccessToken) != http.StatusOK {
		t.Fatalf("login A = %+v", la)
	}
	if _, err := e.svc().Refresh(e.ipCtx(), sdkidentity.RefreshInput{RefreshToken: la.RefreshToken, Origin: "https://jahat.example"}); !errors.Is(err, sdkidentity.ErrOriginNotAllowed) {
		t.Fatalf("origin asing: %v", err)
	}
	if _, err := e.svc().Refresh(e.ipCtx(), sdkidentity.RefreshInput{RefreshToken: la.RefreshToken}); !errors.Is(err, sdkidentity.ErrOriginNotAllowed) {
		t.Fatalf("tanpa origin: %v", err)
	}
	r1, err := e.refresh(la.RefreshToken) // token belum terbakar oleh percobaan Origin salah
	if err != nil || r1.RefreshToken == "" || r1.RefreshToken == la.RefreshToken || e.status(r1.AccessToken) != http.StatusOK {
		t.Fatalf("rotasi: %+v %v", r1, err)
	}
	// Reuse token lama: ditolak dan family (beserta sesinya) dicabut.
	if _, err := e.refresh(la.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("reuse: %v", err)
	}
	if _, err := e.refresh(r1.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("token turunan setelah reuse harus mati: %v", err)
	}
	if code := e.status(r1.AccessToken); code != http.StatusUnauthorized {
		t.Fatalf("sesi family tercabut harus 401, dapat %d", code)
	}
	if _, err := e.refresh("bukan-token"); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("token cacat: %v", err)
	}

	// --- Logout: hanya sesi Principal ---
	a1 := e.mustLogin(ua.Email, pwA)
	a2 := e.mustLogin(ua.Email, pwA)
	b1 := e.mustLogin(ub.Email, pwB)
	if err := e.svc().Logout(e.as(a1.AccessToken)); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if e.status(a1.AccessToken) != http.StatusUnauthorized || e.status(a2.AccessToken) != http.StatusOK || e.status(b1.AccessToken) != http.StatusOK {
		t.Fatalf("logout hanya mencabut sesi sendiri: a1=%d a2=%d b1=%d", e.status(a1.AccessToken), e.status(a2.AccessToken), e.status(b1.AccessToken))
	}
	if err := e.svc().Logout(e.ctx); !errors.Is(err, sdkidentity.ErrUnauthenticated) {
		t.Fatalf("logout tanpa principal: %v", err)
	}

	// --- Ganti sandi: sesi lain dicabut, sesi ini hidup; sandi lama tak berlaku ---
	const newPw = "kata-sandi-a-yang-baru-2026"
	if err := e.svc().ChangePassword(e.as(a2.AccessToken), sdkidentity.ChangeInput{CurrentPassword: "salah-salah-salah-1", NewPassword: newPw}); !errors.Is(err, sdkidentity.ErrWrongCurrentPassword) {
		t.Fatalf("sandi lama salah: %v", err)
	}
	if err := e.svc().ChangePassword(e.as(a2.AccessToken), sdkidentity.ChangeInput{CurrentPassword: pwA, NewPassword: "pendek"}); err == nil {
		t.Fatal("kebijakan sandi harus menolak")
	} else {
		var pe *sdkidentity.PasswordPolicyError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v", err)
		}
	}
	a3 := e.mustLogin(ua.Email, pwA) // sesi lain milik A
	e.remember(newPw)
	if err := e.svc().ChangePassword(e.as(a2.AccessToken), sdkidentity.ChangeInput{CurrentPassword: pwA, NewPassword: newPw}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if e.status(a2.AccessToken) != http.StatusOK || e.status(a3.AccessToken) != http.StatusUnauthorized {
		t.Fatalf("sesi ini harus hidup dan sesi lain mati: a2=%d a3=%d", e.status(a2.AccessToken), e.status(a3.AccessToken))
	}
	if _, err := e.login(ua.Email, pwA); !errors.Is(err, sdkidentity.ErrInvalidCredentials) {
		t.Fatalf("sandi lama masih berlaku: %v", err)
	}
	if _, err := e.login(ua.Email, newPw); err != nil {
		t.Fatalf("sandi baru: %v", err)
	}

	// --- IDOR: tidak ada jalur A menyentuh B ---
	// Sandi B tidak berubah oleh perubahan sandi A; sesi B tetap hidup.
	if _, err := e.login(ub.Email, pwB); err != nil || e.status(b1.AccessToken) != http.StatusOK {
		t.Fatalf("B terdampak aksi A: %v", err)
	}
	// Principal A dengan sid sesi B (tidak mungkin dari token sah; pertahanan berlapis): konfirmasi MFA milik A tidak
	// boleh menaikkan sesi B.
	cb, _ := e.a.Identity.Verifier.Verify(b1.AccessToken)
	forged := httpx.WithClientIP(sdkidentity.WithPrincipal(e.ctx, sdkidentity.Principal{
		UserID: ua.ID, SessionID: cb.SessionID, Kind: sdkidentity.KindHuman, AMR: []string{"pwd"}}), svcIP)
	info, err := e.svc().BeginMFAEnrollment(forged)
	if err != nil {
		t.Fatalf("A mendaftar MFA untuk dirinya: %v", err)
	}
	e.remember(info.Secret)
	fres, err := e.svc().ConfirmMFA(forged, sdkidentity.ConfirmMFAInput{Code: e.code(decodeSecret(t, info.Secret), -30*time.Second)})
	if err != nil {
		t.Fatalf("A mengonfirmasi MFA miliknya: %v", err)
	}
	if fres.Session.AccessToken != "" || fres.Session.RefreshToken != "" {
		t.Fatalf("sesi B tidak boleh dinaikkan/dirotasi oleh A: %+v", fres.Session)
	}
	sb, err := e.a.Identity.Store.GetSession(e.ctx, e.a.Tenant, cb.SessionID)
	if err != nil || len(sb.AMR) != 1 {
		t.Fatalf("sesi B tidak boleh dinaikkan oleh A: %+v %v", sb, err)
	}
	// B tidak punya faktor MFA (enrol milik A tidak bocor ke B).
	st, err := e.a.Identity.MFAState.Status(e.ctx, ub.ID)
	if err != nil || st.FactorActive {
		t.Fatalf("keadaan MFA B: %+v %v", st, err)
	}
	if f, err := e.a.Identity.Store.GetMFAFactor(e.ctx, e.a.Tenant, ub.ID); err == nil {
		t.Fatalf("B tidak boleh punya faktor: %+v", f)
	}

	// Admin.LogoutAllDevices mencabut semua sesi sasaran.
	if err := e.adm().LogoutAllDevices(e.sys("cli:core:test"), ub.ID); err != nil {
		t.Fatal(err)
	}
	if e.status(b1.AccessToken) != http.StatusUnauthorized {
		t.Fatal("sesi B harus tercabut")
	}
	if err := e.adm().LogoutAllDevices(e.sys("cli:core:test"), uuid.NewString()); !errors.Is(err, sdkidentity.ErrNotFound) {
		t.Fatalf("sasaran tak dikenal: %v", err)
	}
}

var linkToken = regexp.MustCompile(`#token=([A-Za-z0-9_-]{43})`)

// Lupa dan reset sandi ujung ke ujung dengan pengirim tiruan; URL publik dari settings (Admin.SetPublicURL).
func TestIdentityService_ForgotResetEndToEnd(t *testing.T) {
	e := newSvcEnv(t)
	u := e.createUser("lupa@example.test", pwA)
	if err := e.adm().SetPublicURL(e.sys("cli:core:settings:set"), "https://erp.example.com/"); err != nil {
		t.Fatalf("SetPublicURL: %v", err)
	}
	if err := e.adm().SetPublicURL(e.sys("cli:core:settings:set"), "ftp://x"); !errors.Is(err, sdkidentity.ErrInvalidInput) {
		t.Fatalf("URL tak sah: %v", err)
	}
	old := e.mustLogin(u.Email, pwA)

	// Email tak dikenal: tanpa galat dan tanpa surat; email dikenal: satu surat dengan tautan sekali pakai.
	if err := e.svc().ForgotPassword(e.ipCtx(), sdkidentity.ForgotInput{Email: "tidak-ada@example.test", Locale: "id"}); err != nil {
		t.Fatalf("forgot tak dikenal: %v", err)
	}
	if err := e.svc().ForgotPassword(e.ipCtx(), sdkidentity.ForgotInput{Email: "LUPA@example.test", Locale: "id"}); err != nil {
		t.Fatalf("forgot: %v", err)
	}
	e.a.Identity.Reset.Wait()
	msgs := e.mail.all()
	if len(msgs) != 1 || len(msgs[0].To) != 1 || msgs[0].To[0] != u.Email {
		t.Fatalf("surat = %+v", msgs)
	}
	m := linkToken.FindStringSubmatch(msgs[0].Text)
	if m == nil || !strings.Contains(msgs[0].Text, "https://erp.example.com/reset-password#token=") {
		t.Fatalf("tautan reset tidak ditemukan atau memakai URL publik lain: %q", msgs[0].Text)
	}
	tok := m[1]
	e.remember(tok)

	const newPw = "kata-sandi-hasil-reset-2026"
	e.remember(newPw)
	if err := e.svc().ResetPassword(e.ipCtx(), sdkidentity.ResetInput{Token: tok, NewPassword: "pendek"}); err == nil {
		t.Fatal("kebijakan sandi harus menolak")
	} else {
		var pe *sdkidentity.PasswordPolicyError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v", err)
		}
	}
	if err := e.svc().ResetPassword(e.ipCtx(), sdkidentity.ResetInput{Token: tok, NewPassword: newPw}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if err := e.svc().ResetPassword(e.ipCtx(), sdkidentity.ResetInput{Token: tok, NewPassword: newPw}); !errors.Is(err, sdkidentity.ErrInvalidResetToken) {
		t.Fatalf("tautan sekali pakai: %v", err)
	}
	if err := e.svc().ResetPassword(e.ipCtx(), sdkidentity.ResetInput{Token: "cacat", NewPassword: newPw}); !errors.Is(err, sdkidentity.ErrInvalidResetToken) {
		t.Fatalf("token cacat: %v", err)
	}
	if e.status(old.AccessToken) != http.StatusUnauthorized {
		t.Fatal("reset harus mencabut semua sesi")
	}
	if _, err := e.login(u.Email, pwA); !errors.Is(err, sdkidentity.ErrInvalidCredentials) {
		t.Fatalf("sandi lama berlaku setelah reset: %v", err)
	}
	if _, err := e.login(u.Email, newPw); err != nil {
		t.Fatalf("sandi baru: %v", err)
	}
	// Pemeriksa Origin mengikuti URL publik dari settings: origin baru diterima, localhost dev tidak lagi.
	l := e.mustLogin(u.Email, newPw)
	if _, err := e.svc().Refresh(e.ipCtx(), sdkidentity.RefreshInput{RefreshToken: l.RefreshToken, Origin: svcOrigin}); !errors.Is(err, sdkidentity.ErrOriginNotAllowed) {
		t.Fatalf("origin dev tidak boleh diterima setelah URL publik diisi: %v", err)
	}
	if _, err := e.svc().Refresh(e.ipCtx(), sdkidentity.RefreshInput{RefreshToken: l.RefreshToken, Origin: "https://erp.example.com"}); err != nil {
		t.Fatalf("origin dari settings: %v", err)
	}
}

// Admin menegakkan otorisasi sendiri; rotasi kunci menghasilkan kunci pending; email ganda dan sandi lemah ditolak.
func TestIdentityService_AdminAuthorizationAndKeys(t *testing.T) {
	e := newSvcEnv(t)
	user := e.createUser("biasa@example.test", pwA)
	e.createAdmin("admin@example.test", pwAdmin)

	// Tanpa Principal maupun hak sistem; Principal biasa (walau ber-hak sistem); admin tanpa otp.
	in := sdkidentity.CreateAdminInput{Email: "lain@example.test", Name: "Lain", Password: pwAdmin}
	if _, err := e.adm().CreateAdmin(e.ctx, in); !errors.Is(err, sdkidentity.ErrForbidden) {
		t.Fatalf("tanpa hak: %v", err)
	}
	ul := e.mustLogin(user.Email, pwA)
	if _, err := e.adm().CreateAdmin(authz.AsSystem(e.as(ul.AccessToken), "cli:core:x"), in); !errors.Is(err, sdkidentity.ErrForbidden) {
		t.Fatalf("pengguna biasa: %v", err)
	}
	if err := e.adm().ResetMFA(e.as(ul.AccessToken), "admin@example.test"); !errors.Is(err, sdkidentity.ErrForbidden) {
		t.Fatalf("pengguna biasa ResetMFA: %v", err)
	}
	al := e.mustLogin("admin@example.test", pwAdmin) // amr pwd saja (enroll_required)
	if _, err := e.adm().RotateSigningKeys(e.as(al.AccessToken)); !errors.Is(err, sdkidentity.ErrForbidden) {
		t.Fatalf("admin tanpa otp: %v", err)
	}

	// Email ganda (huruf berbeda) dan sandi lemah.
	if _, err := e.adm().CreateAdmin(e.sys("cli:core:create-admin"), sdkidentity.CreateAdminInput{Email: "ADMIN@example.test", Name: "X", Password: pwAdmin}); !errors.Is(err, sdkidentity.ErrEmailTaken) {
		t.Fatalf("email ganda: %v", err)
	}
	var pe *sdkidentity.PasswordPolicyError
	if _, err := e.adm().CreateAdmin(e.sys("cli:core:create-admin"), sdkidentity.CreateAdminInput{Email: "lemah@example.test", Name: "L", Password: "pendek"}); !errors.As(err, &pe) {
		t.Fatalf("sandi lemah: %v", err)
	}
	if _, err := e.a.Identity.Store.FindUserByEmail(e.ctx, e.a.Tenant, "lemah@example.test"); !errors.Is(err, identitystore.ErrNotFound) {
		t.Fatalf("akun tidak boleh terbentuk: %v", err)
	}
	// Sandi admin tersimpan sebagai hash argon2id, bukan plaintext, dan administrator mendapat role.
	au, err := e.a.Identity.Store.FindUserByEmail(e.ctx, e.a.Tenant, "admin@example.test")
	if err != nil || !strings.HasPrefix(au.PasswordHash, "$argon2id$") {
		t.Fatalf("hash = %q, %v", au.PasswordHash, err)
	}
	if ok, err := e.a.Identity.Store.HasRole(e.ctx, e.a.Tenant, au.ID, mfa.AdministratorRole); err != nil || !ok {
		t.Fatalf("role administrator: %v %v", ok, err)
	}

	// Rotasi kunci: kunci baru pending (terpublikasi, belum menandatangani), kunci aktif tidak berubah.
	activeBefore := e.a.Identity.Keyring.Active().ID
	info, err := e.adm().RotateSigningKeys(e.sys("cli:core:rotate-keys"))
	if err != nil {
		t.Fatalf("RotateSigningKeys: %v", err)
	}
	if info.Status != sdkidentity.KeyStatusPending || info.KeyID == "" || info.KeyID == activeBefore || !info.ActivatesAt.After(info.CreatedAt) || info.Algorithm != "EdDSA" {
		t.Fatalf("info = %+v", info)
	}
	if e.a.Identity.Keyring.Active().ID != activeBefore {
		t.Fatal("kunci pending tidak boleh langsung menandatangani")
	}
	found := false
	for _, k := range e.a.Identity.Keyring.Keys() {
		found = found || (k.ID == info.KeyID && k.Status == "pending")
	}
	if !found {
		t.Fatal("kunci pending harus ada di keyring")
	}
	// Token baru tetap ditandatangani kunci aktif lama dan masih valid.
	if e.status(e.mustLogin(user.Email, pwA).AccessToken) != http.StatusOK {
		t.Fatal("login setelah rotasi")
	}
}

// Sesi pengguna dengan MFA non-admin: konfirmasi menaikkan sesi ber-pengguna sendiri; verifikasi pada sesi yang sudah
// dicabut gagal; context tanpa tenant ditolak.
func TestIdentityService_SessionGuards(t *testing.T) {
	e := newSvcEnv(t)
	u := e.createUser("mfa@example.test", pwA)
	l := e.mustLogin(u.Email, pwA)
	ctx := e.as(l.AccessToken)
	info, err := e.svc().BeginMFAEnrollment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e.remember(info.Secret)
	// Mendaftar lagi saat pending menimpa secret; setelah dikonfirmasi, pendaftaran ulang ditolak.
	info2, err := e.svc().BeginMFAEnrollment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e.remember(info2.Secret)
	secret := decodeSecret(t, info2.Secret)
	if _, err := e.svc().ConfirmMFA(ctx, sdkidentity.ConfirmMFAInput{Code: e.code(secret, -30*time.Second)}); err != nil {
		t.Fatalf("ConfirmMFA: %v", err)
	}
	if _, err := e.svc().BeginMFAEnrollment(ctx); !errors.Is(err, sdkidentity.ErrMFAAlreadyEnrolled) {
		t.Fatalf("daftar ulang: %v", err)
	}
	// Sesi dicabut: verifikasi berikutnya tidak menghasilkan token.
	if err := e.svc().Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc().VerifyMFA(ctx, sdkidentity.VerifyInput{Code: e.code(secret, 0)}); err == nil {
		t.Fatal("verifikasi pada sesi tercabut harus gagal")
	}
	if _, err := e.svc().Login(context.Background(), sdkidentity.LoginInput{Email: u.Email, Password: pwA}); err == nil {
		t.Fatal("tanpa tenant harus galat")
	}
}

// enrollAdmin membuat administrator lalu menyelesaikan pendaftaran MFA; mengembalikan secret TOTP dan recovery code.
func (e *svcEnv) enrollAdmin(email string) (secret []byte, codes []string) {
	e.t.Helper()
	e.createAdmin(email, pwAdmin)
	l := e.mustLogin(email, pwAdmin)
	ctx := e.as(l.AccessToken)
	info, err := e.svc().BeginMFAEnrollment(ctx)
	if err != nil {
		e.t.Fatalf("BeginMFAEnrollment: %v", err)
	}
	e.remember(info.Secret)
	secret = decodeSecret(e.t, info.Secret)
	rc, err := e.svc().ConfirmMFA(ctx, sdkidentity.ConfirmMFAInput{Code: e.code(secret, -30*time.Second)})
	if err != nil {
		e.t.Fatalf("ConfirmMFA: %v", err)
	}
	e.remember(rc.Codes...)
	e.remember(rc.Session.AccessToken, rc.Session.RefreshToken)
	return secret, rc.Codes
}

// Pencurian cookie refresh pra-MFA (PRD-001 Q27): pemegang cookie curian tidak pernah mendapat token otp, dan
// cookie itu mati begitu pemilik sah menyelesaikan MFA (tanpa mencabut sesi pemilik).
func TestIdentityService_StolenPreMFARefreshCookieNeverYieldsOTP(t *testing.T) {
	e := newSvcEnv(t)
	secret, _ := e.enrollAdmin("curian@example.test")

	l := e.mustLogin("curian@example.test", pwAdmin) // verify_required
	if l.MFAState != sdkidentity.MFAVerifyRequired {
		t.Fatalf("login = %+v", l)
	}
	stolen := l.RefreshToken

	// Penyerang menukar cookie curian: dapat token pwd saja (verify_required), tidak bisa lolos tanpa faktor.
	atk, err := e.refresh(stolen)
	if err != nil || atk.MFAState != sdkidentity.MFAVerifyRequired || e.status(atk.AccessToken) != http.StatusForbidden {
		t.Fatalf("token curian harus tetap pra-MFA: %+v %v", atk, err)
	}
	if c, _ := e.a.Identity.Verifier.Verify(atk.AccessToken); slices.Contains(c.AMR, sdkidentity.AMROTP) {
		t.Fatalf("token curian tidak boleh ber-otp: %v", c.AMR)
	}
	// Kode salah dari penyerang tidak menghasilkan token apa pun.
	if bad, err := e.svc().VerifyMFA(e.as(atk.AccessToken), sdkidentity.VerifyInput{Code: "000000"}); err == nil || bad.AccessToken != "" || bad.RefreshToken != "" {
		t.Fatalf("verifikasi gagal tidak boleh menghasilkan token: %+v %v", bad, err)
	}

	// Pemilik sah memverifikasi dengan token pra-MFA yang kini ada di tangan (hasil rotasi penyerang): sesi naik, cookie
	// baru terbit, dan cookie curian (token lama) tidak lagi berlaku.
	v, err := e.svc().VerifyMFA(e.as(atk.AccessToken), sdkidentity.VerifyInput{Code: e.code(secret, 0)})
	if err != nil || v.RefreshToken == "" {
		t.Fatalf("verifikasi sah: %+v %v", v, err)
	}
	e.remember(v.AccessToken, v.RefreshToken)
	if _, err := e.refresh(atk.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("cookie pra-MFA harus mati setelah verifikasi: %v", err)
	}
	if e.status(v.AccessToken) != http.StatusOK {
		t.Fatal("sesi ber-otp harus tetap hidup")
	}
	if r, err := e.refresh(v.RefreshToken); err != nil || e.status(r.AccessToken) != http.StatusOK {
		t.Fatalf("cookie baru harus bekerja: %+v %v", r, err)
	}
}

// Dua verifikasi serentak pada sesi yang sama (recovery code berbeda): sesi naik tepat sekali dan hanya satu hasil
// yang membawa RefreshToken; pihak yang kalah tetap mendapat access token otp tanpa menimpa cookie pemenang.
func TestIdentityService_ConcurrentVerifyUpgradesExactlyOnce(t *testing.T) {
	e := newSvcEnv(t)
	_, codes := e.enrollAdmin("serentak@example.test")
	l := e.mustLogin("serentak@example.test", pwAdmin)
	ctx := e.as(l.AccessToken)

	type out struct {
		res sdkidentity.LoginResult
		err error
	}
	ch := make(chan out, 2)
	gate := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-gate
			r, err := e.svc().VerifyMFA(ctx, sdkidentity.VerifyInput{Code: codes[i], Recovery: true})
			ch <- out{r, err}
		}()
	}
	close(gate)
	withRefresh := 0
	var winner sdkidentity.LoginResult
	for i := 0; i < 2; i++ {
		o := <-ch
		if o.err != nil || o.res.AccessToken == "" {
			t.Fatalf("hasil %d: %+v %v", i, o.res, o.err)
		}
		e.remember(o.res.AccessToken, o.res.RefreshToken)
		if o.res.RefreshToken != "" {
			withRefresh++
			winner = o.res
		}
	}
	if withRefresh != 1 {
		t.Fatalf("tepat satu hasil harus membawa refresh token, dapat %d", withRefresh)
	}
	if _, err := e.refresh(l.RefreshToken); !errors.Is(err, sdkidentity.ErrInvalidRefresh) {
		t.Fatalf("cookie pra-MFA harus mati: %v", err)
	}
	if r, err := e.refresh(winner.RefreshToken); err != nil || e.status(r.AccessToken) != http.StatusOK {
		t.Fatalf("cookie pemenang: %+v %v", r, err)
	}
}

// CreateAdmin dapat dilanjutkan (PRD-001 Q28) dengan toko sungguhan; keadaan lain tetap ErrEmailTaken.
func TestIdentityService_CreateAdminResumable(t *testing.T) {
	e := newSvcEnv(t)
	st := e.a.Identity.Store
	const oldPw = "sandi-lama-yang-panjang-1"
	in := sdkidentity.CreateAdminInput{Email: "lanjut@example.test", Name: "Lanjut", Password: pwAdmin}
	sys := e.sys("cli:core:create-admin")

	// Sisa kegagalan: akun nonaktif tanpa role, belum login, tanpa sesi.
	phc, err := e.a.Identity.Hasher.Hash(oldPw)
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(e.ctx, e.a.Tenant, identitystore.NewUser{Email: in.Email, Name: in.Name, PasswordHash: phc, Status: identitystore.UserDisabled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.login(in.Email, oldPw); err == nil {
		t.Fatal("akun nonaktif tidak boleh bisa login")
	}
	// Gagal login bukan sesi: akun masih sisa yang dapat dilanjutkan.
	sum, err := e.adm().CreateAdmin(sys, in)
	if err != nil || sum.ID != u.ID {
		t.Fatalf("lanjutan: %+v %v", sum, err)
	}
	if got, _ := st.GetUser(e.ctx, e.a.Tenant, u.ID); got.Status != identitystore.UserActive {
		t.Fatalf("status = %s", got.Status)
	}
	if ok, _ := st.HasRole(e.ctx, e.a.Tenant, u.ID, mfa.AdministratorRole); !ok {
		t.Fatal("role administrator harus diberikan")
	}
	if _, err := e.login(in.Email, oldPw); err == nil {
		t.Fatal("sandi lama harus tidak berlaku")
	}
	l := e.mustLogin(in.Email, pwAdmin) // sandi dari masukan baru; kini punya sesi
	if l.MFAState != sdkidentity.MFAEnrollRequired {
		t.Fatalf("login = %+v", l)
	}

	// Akun aktif, ber-role, pernah login, dan bersesi: tidak dapat dilanjutkan/diambil alih.
	if _, err := e.adm().CreateAdmin(sys, sdkidentity.CreateAdminInput{Email: in.Email, Name: "Penyerang", Password: "sandi-penyerang-yang-panjang-9"}); !errors.Is(err, sdkidentity.ErrEmailTaken) {
		t.Fatalf("akun aktif: %v", err)
	}
	if _, err := e.login(in.Email, pwAdmin); err != nil {
		t.Fatalf("sandi pemilik tidak boleh berubah: %v", err)
	}

	// Nonaktif tanpa role tetapi PERNAH punya sesi (walau kemudian dinonaktifkan): bukan sisa, tetap ErrEmailTaken.
	u2 := e.createUser("bekas@example.test", pwA)
	e.mustLogin(u2.Email, pwA)
	if err := st.SetUserStatus(e.ctx, e.a.Tenant, u2.ID, identitystore.UserDisabled); err != nil {
		t.Fatal(err)
	}
	if _, err := e.adm().CreateAdmin(sys, sdkidentity.CreateAdminInput{Email: u2.Email, Name: "X", Password: pwAdmin}); !errors.Is(err, sdkidentity.ErrEmailTaken) {
		t.Fatalf("akun bersesi: %v", err)
	}
	if ok, _ := st.HasRole(e.ctx, e.a.Tenant, u2.ID, mfa.AdministratorRole); ok {
		t.Fatal("akun bersesi tidak boleh diberi role administrator")
	}
}

// ChangePassword memakai reservasi atomik (kunci advisory di database): permintaan serentak dengan sandi lama salah pada
// pasangan (akun, IP) yang sedang di ambang jeda hanya meloloskan satu tebakan ke verifikasi; sisanya dijeda (429).
func TestIdentityService_ChangePasswordConcurrentGuessesReserveAtomically(t *testing.T) {
	e := newSvcEnv(t)
	u := e.createUser("cp@example.test", pwA)
	l := e.mustLogin(u.Email, pwA)
	ctx := e.as(l.AccessToken)
	for i := range 4 { // 4 gagal: gagal ke-5 masih dijatah, sesudahnya jeda 30 detik
		if err := e.a.Identity.Store.RecordLoginAttempt(e.ctx, e.a.Tenant, u.ID, svcIP, identitystore.AttemptPassword, false, time.Now().UTC().Add(time.Duration(i+1)*time.Millisecond)); err != nil { // sesudah baris sukses login
			t.Fatal(err)
		}
	}
	const burst = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	nWrong, nLimited := 0, 0
	start := make(chan struct{})
	for range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := e.svc().ChangePassword(ctx, sdkidentity.ChangeInput{CurrentPassword: "salah-salah-salah-1", NewPassword: "kata-sandi-baru-yang-panjang-2026"})
			mu.Lock()
			defer mu.Unlock()
			if errors.Is(err, sdkidentity.ErrWrongCurrentPassword) {
				nWrong++
			} else if _, ok := sdkidentity.AsRateLimited(err); ok {
				nLimited++
			} else {
				t.Errorf("err = %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	// Tebakan ke-5 yang lolos ke verifikasi sendiri dijawab 429 (gagal ke-5 memicu jeda), jadi yang terlihat klien adalah
	// `burst` jawaban 429 atau tepat satu ErrWrongCurrentPassword; buktinya adalah jumlah baris reservasi di database.
	if nWrong+nLimited != burst {
		t.Fatalf("salah=%d dibatasi=%d", nWrong, nLimited)
	}
	ps, err := e.a.Identity.Store.PairFailureState(e.ctx, e.a.Tenant, u.ID, svcIP)
	if err != nil || ps.ConsecutiveFailures != 5 {
		t.Fatalf("kegagalan pasangan = %+v err=%v, want 5 (4 awal + tepat 1 tebakan yang lolos)", ps, err)
	}
}
