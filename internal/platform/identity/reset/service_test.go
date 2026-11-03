package reset_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/store"
	sdkmail "erpora/sdk/mail"
)

func lockedUser(h *harness) store.User { return h.st.addUser(h.t, userEmail, store.UserLocked, "$old") }

func TestRequestKnownUserSendsOneMailWithLinkAndStoresOnlyHash(t *testing.T) {
	h := newHarness(t, hopts{})
	u := h.st.addUser(t, userEmail, store.UserActive, "$old")
	if err := h.request(ipCtx("198.51.100.7"), "  Budi@Example.org ", ""); err != nil {
		t.Fatal(err)
	}
	msgs := h.sender.messages()
	if len(msgs) != 1 {
		t.Fatalf("surat = %d, want 1", len(msgs))
	}
	m := msgs[0]
	if len(m.To) != 1 || m.To[0] != userEmail {
		t.Errorf("penerima = %v, want alamat tersimpan %q (bukan masukan mentah)", m.To, userEmail)
	}
	if err := sdkmail.Validate(m); err != nil {
		t.Errorf("pesan tidak lolos Validate: %v", err)
	}
	raw := tokenFrom(t, m)
	if !strings.Contains(m.Text, baseURL+"/reset-password#token="+raw) || !strings.Contains(m.HTML, baseURL+"/reset-password#token="+raw) {
		t.Errorf("tautan bukan dari basis terkonfigurasi:\nteks=%s\nhtml=%s", m.Text, m.HTML)
	}
	tok, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(tok) != 32 {
		t.Fatalf("token bukan 32 byte base64url: len=%d err=%v", len(tok), err)
	}
	// Hanya hash yang tersimpan: tidak ada kolom yang memuat token mentah.
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	if len(h.st.resets) != 1 {
		t.Fatalf("baris reset = %d, want 1", len(h.st.resets))
	}
	for _, r := range h.st.resets {
		want := sha256.Sum256(tok)
		if string(r.TokenHash) != string(want[:]) {
			t.Error("token_hash bukan SHA-256 dari token")
		}
		if string(r.TokenHash) == string(tok) || strings.Contains(string(r.TokenHash), raw) {
			t.Error("token mentah tersimpan")
		}
		if r.UserID != u.ID {
			t.Errorf("user = %s", r.UserID)
		}
		if got := r.ExpiresAt.Sub(r.CreatedAt); got != 30*time.Minute {
			t.Errorf("umur tautan = %v, want 30m", got)
		}
	}
}

func TestRequestUnknownAndDisabledSendNothingButSucceed(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, "off@example.org", store.UserDisabled, "$old")
	for _, e := range []string{"tidak-ada@example.org", "off@example.org", "", "bukan email", strings.Repeat("a", 5000) + "@example.org", "a\r\nb@example.org"} {
		if err := h.request(ipCtx("198.51.100.7"), e, ""); err != nil {
			t.Errorf("%q: err = %v, harus nil (selalu sukses secara eksternal)", e, err)
		}
	}
	if n := len(h.sender.messages()); n != 0 {
		t.Errorf("surat = %d, want 0", n)
	}
	if h.st.callCount("InsertPasswordReset") != 0 {
		t.Error("tautan dibuat untuk akun tak dikenal/nonaktif")
	}
}

func TestRequestLockedAccountGetsLink(t *testing.T) {
	h := newHarness(t, hopts{})
	lockedUser(h)
	if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
		t.Fatal(err)
	}
	if len(h.sender.messages()) != 1 {
		t.Fatal("akun locked harus menerima tautan (jalur unlock lewat reset)")
	}
}

func TestNewRequestInvalidatesPreviousLink(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	for range 2 {
		if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
			t.Fatal(err)
		}
	}
	msgs := h.sender.messages()
	if len(msgs) != 2 {
		t.Fatalf("surat = %d", len(msgs))
	}
	first, second := tokenFrom(t, msgs[0]), tokenFrom(t, msgs[1])
	if first == second {
		t.Fatal("token sama dua kali")
	}
	if err := h.svc.Complete(tctx(), first, goodPass); !errors.Is(err, reset.ErrInvalidToken) {
		t.Errorf("tautan lama: err = %v, want ErrInvalidToken", err)
	}
	if err := h.svc.Complete(tctx(), second, goodPass); err != nil {
		t.Errorf("tautan baru: %v", err)
	}
}

// Review Focus 5: respons dan waktu setara untuk email ada vs tak ada, walau pekerjaan sebenarnya (DB, render, SMTP)
// hanya terjadi pada akun yang ada. Jam nyata, latensi DB dan SMTP tiruan, urutan bergantian.
func TestRequestTimingKnownMatchesUnknown(t *testing.T) {
	h := newHarness(t, hopts{mod: func(c *reset.Config) {
		c.Sleep = nil // penundaan nyata
		c.MinRequestDuration = 25 * time.Millisecond
		c.MaxInFlight = 64
		burst := 1000
		c.Email, _ = guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: burst, Every: time.Millisecond})
		c.IP, _ = guard.NewIPLimiter(guard.LimiterConfig{Burst: burst, Every: time.Millisecond})
	}})
	h.st.delay = 12 * time.Millisecond
	h.sender.delay = 40 * time.Millisecond
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	once := func(email string) time.Duration {
		start := time.Now()
		if err := h.svc.Request(ipCtx("198.51.100.7"), email, ""); err != nil {
			t.Fatalf("Request(%q): %v", email, err)
		}
		return time.Since(start)
	}
	var known, unknown []time.Duration
	for i := range 30 {
		if i%2 == 0 {
			known = append(known, once(userEmail))
			unknown = append(unknown, once(fmt.Sprintf("tidak-ada-%d@example.org", i)))
		} else {
			unknown = append(unknown, once(fmt.Sprintf("tidak-ada-%d@example.org", i)))
			known = append(known, once(userEmail))
		}
	}
	h.svc.Wait()
	mk, mu := median(known), median(unknown)
	hi, lo := mk, mu
	if lo > hi {
		hi, lo = lo, hi
	}
	ratio := float64(hi) / float64(lo)
	t.Logf("median dikenal=%v tak dikenal=%v rasio=%.3f", mk, mu, ratio)
	if ratio > 1.2 {
		t.Errorf("rasio median %.3f > 1,2: respons dapat dibedakan lewat waktu", ratio)
	}
	if mk > 200*time.Millisecond {
		t.Errorf("Request menunggu pekerjaan akun (median %v): pengiriman harus lepas dari respons", mk)
	}
}

func median(d []time.Duration) time.Duration {
	c := append([]time.Duration(nil), d...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

func TestRequestRateLimitPerEmailAndIPUniform(t *testing.T) {
	newLimited := func(t *testing.T) *harness {
		clk := newClock()
		h := newHarness(t, hopts{mod: func(c *reset.Config) {
			c.Now = clk.Now
			c.Email, _ = guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 3, Every: 10 * time.Minute, Now: clk.Now})
			c.IP, _ = guard.NewIPLimiter(guard.LimiterConfig{Burst: 5, Every: time.Minute, Now: clk.Now})
		}})
		h.st.addUser(t, userEmail, store.UserActive, "$old")
		return h
	}
	t.Run("per email: ambang sama untuk akun ada dan tak ada", func(t *testing.T) {
		h := newLimited(t)
		count := func(email string) (okN int, last error) {
			for i := range 8 {
				err := h.svc.Request(ipCtx(fmt.Sprintf("198.51.100.%d", i+1)), email, "")
				if err != nil {
					last = err
					continue
				}
				okN++
			}
			h.svc.Wait()
			return
		}
		okKnown, lastKnown := count(userEmail)
		okUnknown, lastUnknown := count("tidak-ada@example.org")
		if okKnown != 3 || okUnknown != 3 {
			t.Fatalf("diterima dikenal=%d tak dikenal=%d, want 3 dan 3", okKnown, okUnknown)
		}
		for name, err := range map[string]error{"dikenal": lastKnown, "tak dikenal": lastUnknown} {
			retry, ok := guard.AsRateLimited(err)
			if !ok || retry <= 0 {
				t.Errorf("%s: err = %v, want *RateLimitedError dengan Retry-After > 0", name, err)
			}
		}
		if n := len(h.sender.messages()); n != 3 {
			t.Errorf("surat = %d, banjir email harus dicegah (maks 3)", n)
		}
	})
	t.Run("per IP: lintas email", func(t *testing.T) {
		h := newLimited(t)
		var limited int
		for i := range 9 {
			if err := h.svc.Request(ipCtx("203.0.113.9"), fmt.Sprintf("u%d@example.org", i), ""); err != nil {
				if _, ok := guard.AsRateLimited(err); !ok {
					t.Fatalf("err = %v", err)
				}
				limited++
			}
		}
		if limited != 4 {
			t.Errorf("ditolak = %d, want 4 (burst 5 dari 9)", limited)
		}
		if err := h.svc.Request(ipCtx("203.0.113.10"), "lain@example.org", ""); err != nil {
			t.Errorf("IP lain harus lolos: %v", err)
		}
		h.svc.Wait()
	})
	t.Run("Retry-After melalui httpx", func(t *testing.T) {
		h := newLimited(t)
		var err error
		for range 4 {
			err = h.svc.Request(ipCtx("198.51.100.1"), userEmail, "")
		}
		retry, ok := guard.AsRateLimited(err)
		if !ok {
			t.Fatalf("err = %v", err)
		}
		if retry < time.Minute {
			t.Errorf("Retry-After = %v, want sekitar 10 menit", retry)
		}
		h.svc.Wait()
	})
}

func TestRequestLocaleFromPreferencesThenParameterThenDefault(t *testing.T) {
	type tc struct {
		name, pref, param string
		wantEN            bool
	}
	cases := []tc{
		{"preferensi en", "en", "id", true},
		{"preferensi id mengalahkan parameter en", "id", "en", false},
		{"tanpa preferensi: parameter en", "", "en", true},
		{"parameter bervarian dinormalkan", "", "EN-us", true},
		{"tak ada keduanya: id", "", "", false},
		{"locale tak didukung: id", "fr", "de", false},
		{"locale jahat tidak jadi path", "../../etc/passwd", "..\\x", false},
	}
	var subjects = map[bool]string{}
	for _, c := range cases {
		h := newHarness(t, hopts{})
		u := h.st.addUser(t, userEmail, store.UserActive, "$old")
		h.st.prefs[u.ID] = store.Preferences{Locale: c.pref}
		if err := h.request(ipCtx("198.51.100.7"), userEmail, c.param); err != nil {
			t.Fatal(err)
		}
		msgs := h.sender.messages()
		if len(msgs) != 1 {
			t.Fatalf("%s: surat = %d", c.name, len(msgs))
		}
		subjects[c.wantEN] = msgs[0].Subject
		isEN := strings.Contains(msgs[0].Text, "reset your password") || strings.Contains(msgs[0].Text, "Reset your password")
		if isEN != c.wantEN {
			t.Errorf("%s: bahasa Inggris = %v, want %v\n%s", c.name, isEN, c.wantEN, msgs[0].Text)
		}
	}
	if subjects[true] == subjects[false] || subjects[true] == "" || subjects[false] == "" {
		t.Errorf("subjek id dan en harus berbeda dan terisi: %v", subjects)
	}
}

func TestTemplatesRenderValidAndEscapeHTML(t *testing.T) {
	for _, loc := range reset.SupportedLocales() {
		h := newHarness(t, hopts{})
		u := h.st.addUser(t, userEmail, store.UserActive, "$old")
		h.st.mu.Lock()
		h.st.byID[u.ID].Name = `<script>alert(1)</script> & "x"`
		h.st.mu.Unlock()
		if err := h.request(ipCtx("198.51.100.7"), userEmail, loc); err != nil {
			t.Fatal(err)
		}
		msgs := h.sender.messages()
		if len(msgs) != 1 {
			t.Fatalf("%s: surat = %d", loc, len(msgs))
		}
		m := msgs[0]
		if err := sdkmail.Validate(m); err != nil {
			t.Errorf("%s: %v", loc, err)
		}
		if m.Text == "" || m.HTML == "" || m.Subject == "" {
			t.Errorf("%s: subjek/teks/HTML wajib semua terisi", loc)
		}
		if strings.Contains(m.HTML, "<script>") {
			t.Errorf("%s: nama pengguna tidak di-escape di HTML:\n%s", loc, m.HTML)
		}
		raw := tokenFrom(t, m)
		if !strings.Contains(m.HTML, raw) || !strings.Contains(m.Text, "30") || !strings.Contains(m.HTML, "30") {
			t.Errorf("%s: tautan/masa berlaku 30 menit harus tampil di teks dan HTML", loc)
		}
	}
	if got := reset.SupportedLocales(); len(got) < 2 {
		t.Errorf("locale didukung = %v, want id dan en", got)
	}
}

// Host header injection: tautan dibangun hanya dari konfigurasi; layanan bahkan tidak menerima Host.
func TestHostHeaderInjectionDoesNotChangeLink(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := reset.WithClientIP(tctx(), "198.51.100.7")
		_ = h.svc.Request(ctx, r.FormValue("email"), r.Header.Get("Accept-Language"))
		w.WriteHeader(http.StatusAccepted)
	})
	body := url.Values{"email": {userEmail}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "http://evil.example/api/core/auth/password/forgot", strings.NewReader(body))
	req.Host = "evil.example"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Host", "evil.example")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("Forwarded", "host=evil.example")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Referer", "https://evil.example/")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	h.svc.Wait()
	msgs := h.sender.messages()
	if len(msgs) != 1 {
		t.Fatalf("surat = %d", len(msgs))
	}
	all := msgs[0].Subject + msgs[0].Text + msgs[0].HTML
	if strings.Contains(all, "evil") {
		t.Errorf("host dari permintaan masuk ke surat:\n%s", all)
	}
	if !strings.Contains(msgs[0].Text, baseURL+"/reset-password#token=") {
		t.Errorf("tautan harus memakai basis terkonfigurasi:\n%s", msgs[0].Text)
	}
}

// Basis tautan berasal dari PublicURLSource (settings platform) dan divalidasi tiap pemakaian.
func TestLinkBaseFromPublicURLSource(t *testing.T) {
	ok := map[string]string{
		"https://erp.example.com":      "https://erp.example.com/reset-password#token=",
		"https://erp.example.com/":     "https://erp.example.com/reset-password#token=",
		"https://erp.example.com/app/": "https://erp.example.com/app/reset-password#token=",
		"http://localhost:5173":        "http://localhost:5173/reset-password#token=",
	}
	for in, prefix := range ok {
		h := newHarness(t, hopts{})
		h.url.set(in, nil)
		h.st.addUser(t, userEmail, store.UserActive, "$old")
		if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
			t.Fatal(err)
		}
		if m := h.sender.messages(); len(m) != 1 || !strings.Contains(m[0].Text, prefix) {
			t.Errorf("%q: tautan tidak berawalan %q:\n%v", in, prefix, m)
		}
	}
}

// URL yang tidak sah dari sumber (settings dapat berisi nilai apa pun): tidak ada email, tidak ada tautan tersimpan,
// jawaban tetap seragam, peringatan tercatat tanpa data sensitif.
func TestRequestWithBadOrMissingPublicURLSendsNothingAndAnswersUniformly(t *testing.T) {
	bad := []string{"", "erp.example.com", "ftp://erp.example.com", "https://u:p@erp.example.com", "https://erp.example.com/?x=1",
		"https://erp.example.com/#a", "https://", "https://erp.example.com\r\nX: y", "javascript:alert(1)"}
	type src struct {
		name string
		url  string
		err  error
	}
	cases := []src{{"belum dikonfigurasi", "", errNotConfigured}, {"galat sumber", "", errors.New("db mati")}}
	for _, b := range bad {
		cases = append(cases, src{"nilai tidak sah " + b, b, nil})
	}
	for _, c := range cases {
		h := newHarness(t, hopts{})
		h.url.set(c.url, c.err)
		h.st.addUser(t, userEmail, store.UserActive, "$old")
		if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
			t.Errorf("%s: err = %v, harus nil (jawaban seragam)", c.name, err)
		}
		// akun tak dikenal dijawab identik
		if err := h.request(ipCtx("198.51.100.8"), "tidak-ada@example.org", ""); err != nil {
			t.Errorf("%s (tak dikenal): err = %v", c.name, err)
		}
		if n := len(h.sender.messages()); n != 0 {
			t.Errorf("%s: surat = %d, want 0", c.name, n)
		}
		if h.st.callCount("InsertPasswordReset") != 0 {
			t.Errorf("%s: tautan disimpan padahal tidak dapat dikirim", c.name)
		}
		out := h.logs.String()
		if !strings.Contains(out, "WARN") || !strings.Contains(out, "URL publik") {
			t.Errorf("%s: peringatan terstruktur harus tercatat:\n%s", c.name, out)
		}
		for _, secret := range []string{userEmail, "tidak-ada@example.org", "u:p@", "X: y"} {
			if strings.Contains(out, secret) {
				t.Errorf("%s: log memuat %q:\n%s", c.name, secret, out)
			}
		}
	}
}

func TestPublicURLChangeTakesEffectOnNextRequest(t *testing.T) {
	h := newHarness(t, hopts{mod: func(c *reset.Config) {
		c.Email, _ = guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 100, Every: time.Millisecond})
	}})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	h.url.set("", errNotConfigured)
	_ = h.request(ipCtx("198.51.100.7"), userEmail, "")
	h.url.set("https://baru.example.com", nil)
	if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
		t.Fatal(err)
	}
	m := h.sender.messages()
	if len(m) != 1 || !strings.Contains(m[0].Text, "https://baru.example.com/reset-password#token=") {
		t.Errorf("surat = %v", m)
	}
}

func TestNewRequiresDependencies(t *testing.T) {
	good := reset.Config{Store: newFakeStore(), Sessions: &fakeSessions{}, Gate: &countingGate{}, Sender: &fakeSender{}, PublicURL: &fakeURL{url: baseURL}}
	for name, mod := range map[string]func(*reset.Config){
		"store": func(c *reset.Config) { c.Store = nil }, "sessions": func(c *reset.Config) { c.Sessions = nil },
		"gate": func(c *reset.Config) { c.Gate = nil }, "sender": func(c *reset.Config) { c.Sender = nil },
		"public url": func(c *reset.Config) { c.PublicURL = nil },
	} {
		c := good
		mod(&c)
		if _, err := reset.New(c); err == nil {
			t.Errorf("tanpa %s diterima", name)
		}
	}
	if _, err := reset.New(good); err != nil {
		t.Errorf("konfigurasi sah ditolak: %v", err)
	}
}

func TestRequestWithoutTenantFailsClosed(t *testing.T) {
	h := newHarness(t, hopts{})
	err := h.svc.Request(context.Background(), userEmail, "")
	h.svc.Wait()
	if !errors.Is(err, reset.ErrNoTenant) {
		t.Errorf("err = %v, want ErrNoTenant", err)
	}
}

func TestRequestMailFailureStillSucceedsAndLogsNoSecrets(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	h.sender.err = errors.New("smtp mati")
	if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
		t.Fatalf("kegagalan kirim tidak boleh terlihat dari luar: %v", err)
	}
	out := h.logs.String()
	if !strings.Contains(out, "smtp mati") {
		t.Errorf("kegagalan harus dicatat:\n%s", out)
	}
	h.sender.err = nil
	if err := h.request(ipCtx("198.51.100.8"), userEmail, ""); err != nil {
		t.Fatal(err)
	}
	m := h.sender.messages()[0]
	raw := tokenFrom(t, m)
	all := h.logs.String()
	for _, secret := range []string{raw, "#token=", userEmail, "reset-password"} {
		if strings.Contains(all, secret) {
			t.Errorf("log memuat %q:\n%s", secret, all)
		}
	}
}

func TestRequestInFlightBoundedDropsExcess(t *testing.T) {
	h := newHarness(t, hopts{mod: func(c *reset.Config) {
		c.MaxInFlight = 2
		c.Email, _ = guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 1000, Every: time.Millisecond})
		c.IP, _ = guard.NewIPLimiter(guard.LimiterConfig{Burst: 1000, Every: time.Millisecond})
	}})
	h.sender.block = make(chan struct{})
	for i := range 6 {
		h.st.addUser(t, fmt.Sprintf("u%d@example.org", i), store.UserActive, "$old")
	}
	for i := range 6 {
		if err := h.svc.Request(ipCtx("198.51.100.7"), fmt.Sprintf("u%d@example.org", i), ""); err != nil {
			t.Fatalf("Request %d: %v (antrean penuh tidak boleh terlihat dari luar)", i, err)
		}
	}
	// Beri waktu pekerja memulai pengiriman yang tertahan.
	deadline := time.Now().Add(2 * time.Second)
	for h.sender.start.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := h.sender.start.Load(); n > 2 {
		t.Errorf("pengiriman serentak = %d, batas 2", n)
	}
	close(h.sender.block)
	h.svc.Wait()
	if n := len(h.sender.messages()); n != 2 {
		t.Errorf("surat = %d, want 2 (sisanya dibuang tanpa galat bagi pemanggil)", n)
	}
}

// --- Complete ---

func requestToken(t *testing.T, h *harness, email string) string {
	t.Helper()
	if err := h.request(ipCtx("198.51.100.7"), email, ""); err != nil {
		t.Fatal(err)
	}
	msgs := h.sender.messages()
	if len(msgs) == 0 {
		t.Fatal("tidak ada surat")
	}
	return tokenFrom(t, msgs[len(msgs)-1])
}

func TestCompleteResetsPasswordRevokesSessionsUnlocksAndIsSingleUse(t *testing.T) {
	h := newHarness(t, hopts{})
	u := lockedUser(h)
	h.st.sessions[u.ID] = []string{"sesi-1", "sesi-2", "sesi-3"}
	raw := requestToken(t, h, userEmail)
	if err := h.svc.Complete(tctx(), raw, goodPass); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := h.st.user(userEmail)
	if got.PasswordHash == "$old" {
		t.Fatal("hash sandi tidak berubah (sandi lama masih berlaku)")
	}
	ok, _, err := (&password.Hasher{Params: testParams}).Verify(goodPass, got.PasswordHash)
	if err != nil || !ok {
		t.Errorf("sandi baru tidak terverifikasi: %v %v", ok, err)
	}
	if ok, _, _ := (&password.Hasher{Params: testParams}).Verify(oldPass, got.PasswordHash); ok {
		t.Error("sandi lama masih berlaku")
	}
	if got.Status != store.UserActive {
		t.Errorf("status = %s, akun locked harus terbuka setelah reset", got.Status)
	}
	revoked := h.sess.got()
	sort.Strings(revoked)
	if strings.Join(revoked, ",") != "sesi-1,sesi-2,sesi-3" {
		t.Errorf("MarkRevoked = %v, want semua sesi yang dicabut store", revoked)
	}
	// sekali pakai
	if err := h.svc.Complete(tctx(), raw, "kata sandi lain yang kuat 777"); !errors.Is(err, reset.ErrInvalidToken) {
		t.Errorf("pemakaian kedua: err = %v, want ErrInvalidToken", err)
	}
}

func TestCompleteConcurrentExactlyOneWins(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	raw := requestToken(t, h, userEmail)
	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = h.svc.Complete(tctx(), raw, goodPass)
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, reset.ErrInvalidToken):
			t.Errorf("galat tak terduga: %v", err)
		}
	}
	if wins != 1 {
		t.Errorf("sukses = %d, want tepat 1", wins)
	}
}

func TestCompleteExpiredTokenRejected(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	raw := requestToken(t, h, userEmail)
	h.clk.Advance(30*time.Minute + time.Second)
	err := h.svc.Complete(tctx(), raw, goodPass)
	if !errors.Is(err, reset.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
	if h.st.user(userEmail).PasswordHash != "$old" {
		t.Error("sandi berubah oleh token kedaluwarsa")
	}
	// tepat di dalam batas: masih berlaku
	h2 := newHarness(t, hopts{})
	h2.st.addUser(t, userEmail, store.UserActive, "$old")
	raw2 := requestToken(t, h2, userEmail)
	h2.clk.Advance(29*time.Minute + 59*time.Second)
	if err := h2.svc.Complete(tctx(), raw2, goodPass); err != nil {
		t.Errorf("29m59s: %v", err)
	}
}

func TestCompleteWeakPasswordDoesNotConsumeToken(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	raw := requestToken(t, h, userEmail)
	for _, weak := range []string{"pendek", "password1234", strings.Repeat("a", 1025)} {
		err := h.svc.Complete(tctx(), raw, weak)
		var pe *reset.PolicyError
		if !errors.As(err, &pe) || len(pe.Violations) == 0 {
			t.Fatalf("%.12q: err = %v, want *PolicyError dengan Violation", weak, err)
		}
		if strings.Contains(err.Error(), weak) {
			t.Error("galat memuat sandi")
		}
	}
	if h.gate.n.Load() != 0 {
		t.Error("hash argon2 dijalankan untuk sandi yang ditolak kebijakan")
	}
	if h.st.callCount("ResetPassword") != 0 {
		t.Error("ResetPassword dipanggil untuk sandi lemah")
	}
	if err := h.svc.Complete(tctx(), raw, goodPass); err != nil {
		t.Errorf("token harus masih berlaku setelah sandi lemah ditolak: %v", err)
	}
}

func TestCompleteMalformedAndUnknownTokensUniform(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	_ = requestToken(t, h, userEmail)
	random := base64.RawURLEncoding.EncodeToString(make([]byte, 32)) // 32 byte nol: bentuk sah, tak dikenal
	bad := []string{
		"", " ", "abc", random, random + "=", random + "A", strings.Repeat("A", 100000), "../../etc/passwd",
		"AAAA\x00AAAA", strings.Repeat("%", 43), random[:42] + "!",
	}
	var msgs []string
	for _, tok := range bad {
		err := h.svc.Complete(tctx(), tok, goodPass)
		if !errors.Is(err, reset.ErrInvalidToken) {
			t.Errorf("%.20q: err = %v, want ErrInvalidToken", tok, err)
			continue
		}
		msgs = append(msgs, err.Error())
	}
	for _, m := range msgs {
		if m != msgs[0] {
			t.Errorf("pesan galat berbeda antar sebab: %q vs %q", m, msgs[0])
		}
	}
	if h.st.callCount("ResetPassword") != 0 || h.gate.n.Load() != 0 {
		t.Error("token tak sah tidak boleh menjalankan hash atau reset")
	}
}

func TestCompleteHashBusySurfacesRateLimitAndKeepsToken(t *testing.T) {
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	raw := requestToken(t, h, userEmail)
	h.gate.busy = true
	err := h.svc.Complete(tctx(), raw, goodPass)
	if _, ok := guard.AsRateLimited(err); !ok {
		t.Fatalf("err = %v, want RateLimitedError", err)
	}
	h.gate.busy = false
	if err := h.svc.Complete(tctx(), raw, goodPass); err != nil {
		t.Errorf("token harus tetap berlaku setelah 429: %v", err)
	}
}

func TestCompleteWithoutTenantFailsClosed(t *testing.T) {
	h := newHarness(t, hopts{})
	if err := h.svc.Complete(context.Background(), "x", goodPass); !errors.Is(err, reset.ErrNoTenant) {
		t.Errorf("err = %v, want ErrNoTenant", err)
	}
}

// Token tenant lain tidak berlaku (store menyaring tenant; fake di sini tidak, jadi diuji lewat integrasi).
func TestCompleteMarkRevokedOnlyOnSuccess(t *testing.T) {
	h := newHarness(t, hopts{})
	u := h.st.addUser(t, userEmail, store.UserActive, "$old")
	h.st.sessions[u.ID] = []string{"s1"}
	raw := requestToken(t, h, userEmail)
	_ = h.svc.Complete(tctx(), raw, "pendek")
	if len(h.sess.got()) != 0 {
		t.Error("MarkRevoked dipanggil padahal reset gagal")
	}
}
