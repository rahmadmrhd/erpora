//go:build integration

package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"erpora/sdk/httpx"
)

// PRD-001 NFR-103: tidak ada rahasia (sandi, token akses, refresh token, secret TOTP, kode TOTP, recovery code, token
// reset) yang tercatat di log. Tes ini menjalankan SELURUH alur auth lewat HTTP (login gagal dan sukses, refresh lewat
// cookie, ubah sandi, logout, forgot dan reset, enroll dan confirm MFA, verify TOTP, recovery code) sambil menangkap
// SEMUA keluaran slog: logger stack (newBufLogger tingkat Debug, dipakai assemble untuk seluruh komponen platform,
// modul, dan pekerja latar) DAN slog.Default() (di production serve memanggil slog.SetDefault(log), sehingga
// sdk/httpx.RespondError, modul yang memakai slog.*Context paket, dan komponen yang jatuh ke slog.Default() saat
// logger-nya nil menulis ke log server yang sama). Keduanya diarahkan ke SATU penampung, lalu dipindai. Pengirim surat adalah penangkap (captureMail), bukan driver log: tautan reset
// memang tujuan driver log, sehingga yang diuji di sini adalah bahwa token reset tidak bocor lewat jalur LAIN.

// secretProbe adalah satu nilai rahasia yang dicari di log.
type secretProbe struct {
	kind  string
	value string
	// word: cocokkan hanya sebagai token utuh (bukan bagian dari angka/kata lain). Dipakai untuk kode TOTP 6 digit
	// agar durasi atau id di log tidak menghasilkan positif palsu.
	word bool
}

// secretProbes mengumpulkan nilai rahasia yang muncul selama alur. Nilai KOSONG menggagalkan tes: bila nama field
// respons berubah, probe tidak boleh hilang diam-diam (pemindaian tanpa probe selalu lolos).
type secretProbes struct {
	t    *testing.T
	list []secretProbe
}

func (p *secretProbes) add(kind string, values ...string) {
	p.t.Helper()
	for _, v := range values {
		if v == "" {
			p.t.Fatalf("probe rahasia %q kosong: nilai yang diharapkan tidak ada pada respons (nama field berubah?)", kind)
		}
		p.list = append(p.list, secretProbe{kind: kind, value: v})
	}
}

func (p *secretProbes) addCode(kind string, values ...string) {
	p.t.Helper()
	for _, v := range values {
		if v == "" {
			p.t.Fatalf("probe rahasia %q kosong", kind)
		}
		p.list = append(p.list, secretProbe{kind: kind, value: v, word: true})
	}
}

// addRecovery mendaftarkan recovery code dalam bentuk tampilan (XXXX-XXXX-XXXX-XXXX) dan ternormalisasi (16 karakter
// tanpa tanda hubung), huruf besar dan kecil.
func (p *secretProbes) addRecovery(codes ...string) {
	p.t.Helper()
	if len(codes) == 0 {
		p.t.Fatal("recovery code kosong: nilai yang diharapkan tidak ada pada respons")
	}
	for _, c := range codes {
		norm := strings.ReplaceAll(c, "-", "")
		p.add("recovery code (tampilan)", c, strings.ToLower(c))
		p.add("recovery code (ternormalisasi)", norm, strings.ToLower(norm))
	}
}

// jwtShape mengenali bentuk JWT (tiga segmen base64url berawalan "eyJ"), dicari di log TANPA tahu nilainya: token apa
// pun yang lolos ke log tertangkap walau tidak didaftarkan.
var jwtShape = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)

// bearerShape mengenali header Authorization yang tercatat mentah.
var bearerShape = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{16,}`)

// scanLog mengembalikan deskripsi setiap kebocoran di log (tanpa mengulang nilai rahasia utuh).
func scanLog(log string, probes []secretProbe) []string {
	var leaks []string
	for _, p := range probes {
		found := false
		if p.word {
			found = regexp.MustCompile(`(^|[^0-9A-Za-z])` + regexp.QuoteMeta(p.value) + `([^0-9A-Za-z]|$)`).MatchString(log)
		} else {
			found = strings.Contains(log, p.value)
		}
		if found {
			leaks = append(leaks, p.kind+": "+mask(p.value))
		}
	}
	if m := jwtShape.FindString(log); m != "" {
		leaks = append(leaks, "bentuk JWT di log: "+mask(m))
	}
	if m := bearerShape.FindString(log); m != "" {
		leaks = append(leaks, "header Authorization Bearer di log: "+mask(m))
	}
	return leaks
}

func mask(v string) string {
	if len(v) <= 6 {
		return strings.Repeat("*", len(v))
	}
	return v[:3] + "..." + "(" + strings.Repeat("*", 3) + ")"
}

// Kontrol: pemindai benar-benar menangkap kebocoran dan tidak berisik (bukti bahwa tes alur di bawah berarti).
func TestSecretScanner_DetectsLeaksAndStaysQuiet(t *testing.T) {
	p := secretProbes{t: t}
	p.add("sandi", "kata-sandi-rahasia-2026")
	p.addCode("kode TOTP", "483920")
	p.addRecovery("ABCD-EFGH-IJKL-MNOP")
	const jwt = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ1MSJ9.c2lnbmF0dXJl"

	leaky := []string{
		`level=INFO msg="login" password=kata-sandi-rahasia-2026`,
		`level=DEBUG msg=verify code=483920`,
		`level=DEBUG msg=recovery code=ABCD-EFGH-IJKL-MNOP`,
		`level=DEBUG msg=recovery code=abcdefghijklmnop`,
		`level=DEBUG msg=hdr Authorization="Bearer abcdefghijklmnopqrstuvwxyz012345"`,
		`level=DEBUG msg=tok value=` + jwt,
	}
	for _, line := range leaky {
		if len(scanLog(line, p.list)) == 0 {
			t.Errorf("kebocoran tidak terdeteksi: %q", line)
		}
	}
	quiet := `level=INFO msg="http" method=POST path=/api/core/auth/login status=200 dur=1483920µs request_id=0190a1b2 code=ok`
	if got := scanLog(quiet, p.list); len(got) != 0 {
		t.Errorf("positif palsu: %v", got)
	}
}

func TestCoreAPI_NoSecretsInLogs_FullAuthFlow(t *testing.T) {
	// slog.Default() ke penampung yang SAMA dengan logger stack, dipasang SEBELUM assemble seperti production
	// (cmd/erpora/serve.go): komponen yang menyimpan slog.Default() saat dikonstruksi ikut tertangkap.
	// Tes ini tidak boleh memakai t.Parallel (default slog adalah state proses); tidak ada tes lain di paket ini yang
	// memakai t.Parallel. Pekerja latar tes sebelumnya sudah berhenti (context dibatalkan di Cleanup) dan, kalaupun masih menulis,
	// hanya menambah baris tanpa rahasia milik tes ini. Pemulihan default lewat t.Cleanup di newSvcEnvWith.
	e := newCoreEnvWith(t, true)
	probes := secretProbes{t: t}

	// ---- Pengguna biasa: login gagal, login sukses, refresh lewat cookie, ubah sandi, logout ----
	const (
		emailUser = "scan-user@example.test"
		pwWrong   = "sandi-salah-percobaan-2026"
		pwUnknown = "sandi-akun-tak-dikenal-2026"
		pwChanged = "kata-sandi-diganti-scan-2026"
		pwReset   = "kata-sandi-reset-scan-2026"
	)
	e.createUser(emailUser, pwA)
	probes.add("sandi", pwA, pwWrong, pwUnknown, pwChanged, pwReset)

	if r := e.apiCall("/login", jsonBody("email", emailUser, "password", pwWrong), "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("login sandi salah = %d %s", r.Code, r.Body.String())
	}
	if r := e.apiCall("/login", jsonBody("email", "tidak-ada@example.test", "password", pwUnknown), "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("login akun tak dikenal = %d %s", r.Code, r.Body.String())
	}

	lr := e.apiCall("/login", jsonBody("email", emailUser, "password", pwA), "", "")
	if lr.Code != http.StatusOK {
		t.Fatalf("login = %d %s", lr.Code, lr.Body.String())
	}
	ck1 := assertRefreshCookie(t, "login", lr)
	at1, _ := lr.data(t)["accessToken"].(string)
	probes.add("refresh token", ck1)
	probes.add("access token", at1)

	rf := e.apiCall("/refresh", "", "", ck1)
	if rf.Code != http.StatusOK {
		t.Fatalf("refresh = %d %s", rf.Code, rf.Body.String())
	}
	ck2 := assertRefreshCookie(t, "refresh", rf)
	at2, _ := rf.data(t)["accessToken"].(string)
	probes.add("refresh token", ck2)
	probes.add("access token", at2)

	if r := e.apiCall("/password/change", jsonBody("currentPassword", pwWrong, "newPassword", pwChanged), at2, ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ubah sandi dengan sandi saat ini salah = %d %s", r.Code, r.Body.String())
	}
	if r := e.apiCall("/password/change", jsonBody("currentPassword", pwA, "newPassword", pwChanged), at2, ""); r.Code != http.StatusNoContent {
		t.Fatalf("ubah sandi = %d %s", r.Code, r.Body.String())
	}
	// Ubah sandi mencabut sesi lama; login ulang dengan sandi baru lalu logout.
	l2 := e.apiCall("/login", jsonBody("email", emailUser, "password", pwChanged), "", "")
	if l2.Code != http.StatusOK {
		t.Fatalf("login sandi baru = %d %s", l2.Code, l2.Body.String())
	}
	ck3 := assertRefreshCookie(t, "login2", l2)
	at3, _ := l2.data(t)["accessToken"].(string)
	probes.add("refresh token", ck3)
	probes.add("access token", at3)
	if r := e.apiCall("/logout", "", at3, ck3); r.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", r.Code, r.Body.String())
	}

	// ---- Forgot + reset (surat lewat penangkap; token tidak boleh ada di log) ----
	if err := e.adm().SetPublicURL(e.sys("cli:core:settings:set"), "https://erp.example.com/"); err != nil {
		t.Fatal(err)
	}
	if r := e.apiCall("/password/forgot", jsonBody("email", emailUser), "", ""); r.Code != http.StatusNoContent {
		t.Fatalf("forgot = %d %s", r.Code, r.Body.String())
	}
	e.a.Identity.Reset.Wait()
	msgs := e.mail.all()
	if len(msgs) != 1 {
		t.Fatalf("surat reset = %d, want 1", len(msgs))
	}
	m := linkToken.FindStringSubmatch(msgs[0].Text)
	if m == nil {
		t.Fatalf("tautan reset tidak ada di surat: %q", msgs[0].Text)
	}
	resetToken := m[1]
	probes.add("token reset", resetToken)
	if r := e.apiCall("/password/reset", jsonBody("token", "token-salah-"+resetToken[:10], "newPassword", pwReset), "", ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reset token salah = %d %s", r.Code, r.Body.String())
	}
	if r := e.apiCall("/password/reset", jsonBody("token", resetToken, "newPassword", pwReset), "", ""); r.Code != http.StatusNoContent {
		t.Fatalf("reset = %d %s", r.Code, r.Body.String())
	}
	if r := e.apiCall("/login", jsonBody("email", emailUser, "password", pwReset), "", ""); r.Code != http.StatusOK {
		t.Fatalf("login sandi reset = %d %s", r.Code, r.Body.String())
	}

	// ---- Administrator: enroll + confirm, verify TOTP, recovery code ----
	e.createAdmin("scan-admin@example.test", pwAdmin)
	probes.add("sandi", pwAdmin)
	al := e.apiCall("/login", jsonBody("email", "scan-admin@example.test", "password", pwAdmin), "", "")
	if al.Code != http.StatusOK || al.data(t)["mfaState"] != "enroll_required" {
		t.Fatalf("login admin = %d %s", al.Code, al.Body.String())
	}
	preCookie := assertRefreshCookie(t, "admin-login", al)
	preToken, _ := al.data(t)["accessToken"].(string)
	probes.add("refresh token", preCookie)
	probes.add("access token", preToken)

	en := e.apiCall("/mfa/enroll", "", preToken, "")
	if en.Code != http.StatusOK {
		t.Fatalf("enroll = %d %s", en.Code, en.Body.String())
	}
	secretB32, _ := en.data(t)["secret"].(string)
	probes.add("secret TOTP (base32)", secretB32)
	secret := decodeSecret(t, secretB32)
	otpauth, _ := en.data(t)["otpauthUri"].(string)
	probes.add("otpauth URI", otpauth)

	// Kode TOTP salah, lalu benar. Seluruh kode pada jendela yang berdekatan didaftarkan sebagai rahasia.
	wrong := e.code(secret, 10*time.Minute)
	probes.addCode("kode TOTP", wrong)
	if r := e.apiCall("/mfa/confirm", jsonBody("code", wrong), preToken, ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("confirm kode salah = %d %s", r.Code, r.Body.String())
	}
	good := e.code(secret, -30*time.Second)
	probes.addCode("kode TOTP", good, e.code(secret, 0), e.code(secret, -60*time.Second), e.code(secret, 30*time.Second))
	cf := e.apiCall("/mfa/confirm", jsonBody("code", good), preToken, "")
	if cf.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", cf.Code, cf.Body.String())
	}
	ck4 := assertRefreshCookie(t, "confirm", cf)
	probes.add("refresh token", ck4)
	cd := cf.data(t)
	var codes []string
	for _, c := range cd["codes"].([]any) {
		codes = append(codes, c.(string))
	}
	if len(codes) != 10 {
		t.Fatalf("recovery code confirm = %d, want 10", len(codes))
	}
	probes.addRecovery(codes...)
	sess, _ := cd["session"].(map[string]any)
	otpToken, _ := sess["accessToken"].(string)
	probes.add("access token", otpToken)

	// Verify TOTP pada sesi verify_required (login ulang), lalu recovery code pada sesi lain.
	v1 := e.apiCall("/login", jsonBody("email", "scan-admin@example.test", "password", pwAdmin), "", "")
	if v1.Code != http.StatusOK || v1.data(t)["mfaState"] != "verify_required" {
		t.Fatalf("login verify = %d %s", v1.Code, v1.Body.String())
	}
	ck5 := assertRefreshCookie(t, "verify-login", v1)
	tok5, _ := v1.data(t)["accessToken"].(string)
	probes.add("refresh token", ck5)
	probes.add("access token", tok5)
	probes.addCode("kode TOTP", e.code(secret, 30*time.Second), e.code(secret, 60*time.Second))
	if r := e.apiCall("/mfa/verify", jsonBody("code", wrong), tok5, ""); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("verify kode salah = %d %s", r.Code, r.Body.String())
	}
	vr := e.apiCall("/mfa/verify", jsonBody("code", e.code(secret, 0)), tok5, "")
	if vr.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", vr.Code, vr.Body.String())
	}
	ck6 := assertRefreshCookie(t, "verify", vr)
	tok6, _ := vr.data(t)["accessToken"].(string)
	probes.add("refresh token", ck6)
	probes.add("access token", tok6)

	// Regenerasi recovery code (token MFA terverifikasi), lalu pakai satu kode dalam bentuk tampilan dan satu
	// lagi dalam bentuk tanpa tanda hubung huruf kecil (bentuk ternormalisasi yang diterima server).
	rg := e.apiCall("/mfa/recovery-codes", "", tok6, "")
	if rg.Code != http.StatusOK {
		t.Fatalf("recovery-codes = %d %s", rg.Code, rg.Body.String())
	}
	var codes2 []string
	for _, c := range rg.data(t)["codes"].([]any) {
		codes2 = append(codes2, c.(string))
	}
	if len(codes2) != 10 {
		t.Fatalf("recovery code regenerasi = %d, want 10", len(codes2))
	}
	probes.addRecovery(codes2...)
	for i, form := range []func(string) string{
		func(c string) string { return c },
		func(c string) string { return strings.ToLower(strings.ReplaceAll(c, "-", "")) },
	} {
		l := e.apiCall("/login", jsonBody("email", "scan-admin@example.test", "password", pwAdmin), "", "")
		tok, _ := l.data(t)["accessToken"].(string)
		probes.add("access token", tok)
		if r := e.apiCall("/mfa/recovery", jsonBody("code", "SALAH-SALAH-SALAH-SALAH"), tok, ""); r.Code != http.StatusUnprocessableEntity {
			t.Fatalf("recovery salah = %d %s", r.Code, r.Body.String())
		}
		rc := e.apiCall("/mfa/recovery", jsonBody("code", form(codes2[i])), tok, "")
		if rc.Code != http.StatusOK {
			t.Fatalf("recovery[%d] = %d %s", i, rc.Code, rc.Body.String())
		}
		ck := assertRefreshCookie(t, "recovery", rc)
		probes.add("refresh token", ck)
		rt, _ := rc.data(t)["accessToken"].(string)
		probes.add("access token", rt)
	}

	// ---- Pemindaian ----
	log := e.logs.String()
	if strings.TrimSpace(log) == "" {
		t.Fatal("log kosong: penangkap tidak terpasang pada stack (pemindaian tidak bermakna)")
	}
	// Penanda positif: kedua jalur tertangkap. (1) logger stack: peristiwa keamanan yang pasti ditulis identity.Service
	// pada login gagal. (2) slog.Default(): RespondError mencatat galat tak terduga lewat slog.ErrorContext paket
	// (alur normal tidak memicunya, jadi dipicu langsung dengan galat berpenanda).
	rec := httptest.NewRecorder()
	httpx.RespondError(rec, httptest.NewRequest(http.MethodPost, authBase+"/login", nil), errors.New("penanda-jalur-slog-default"))
	log = e.logs.String()
	for _, marker := range []string{
		`event=identity.login_failed`,   // jalur logger stack
		`msg="unhandled handler error"`, // jalur slog.Default() (sdk/httpx)
		`penanda-jalur-slog-default`,
	} {
		if !strings.Contains(log, marker) {
			t.Fatalf("penanda %s tidak ada di log: salah satu jalur slog tidak tertangkap (pemindaian bermakna?)", marker)
		}
	}
	if leaks := scanLog(log, probes.list); len(leaks) > 0 {
		t.Fatalf("NFR-103: %d rahasia bocor ke log:\n  %s", len(leaks), strings.Join(leaks, "\n  "))
	}
}
