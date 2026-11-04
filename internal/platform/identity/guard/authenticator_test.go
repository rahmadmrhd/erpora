package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/store"
)

// testParams kecil agar tes cepat; bukan default OWASP (Authenticator tidak menolak parameter kecil, config.Load yang
// menegakkan batas bawah).
var testParams = password.Params{MemoryKiB: 8192, Time: 1, Parallelism: 1}

const goodPassword = "kata sandi yang benar 123"

var (
	goodHashOnce sync.Once
	goodHashPHC  string
)

func goodHash(t testing.TB) string {
	t.Helper()
	goodHashOnce.Do(func() {
		var err error
		if goodHashPHC, err = (&password.Hasher{Params: testParams}).Hash(goodPassword); err != nil {
			panic(err)
		}
	})
	return goodHashPHC
}

// countingRunner membungkus HashGate: menghitung pemanggilan Do dan puncak konkurensi, dan dapat memaksa "penuh".
type countingRunner struct {
	inner   HashRunner
	calls   atomic.Int32
	running atomic.Int32
	peak    atomic.Int32
	busy    atomic.Bool
	panics  atomic.Bool // memaksa panic di dalam Do (mensimulasikan bug/panic di jalur verifikasi)
}

func (r *countingRunner) Do(ctx context.Context, fn func() error) error {
	r.calls.Add(1)
	if r.panics.Load() {
		panic("boom: panic uji di jalur verifikasi")
	}
	if r.busy.Load() {
		return &RateLimitedError{RetryAfter: time.Second, Cause: ErrHashBusy}
	}
	return r.inner.Do(ctx, func() error {
		n := r.running.Add(1)
		for {
			p := r.peak.Load()
			if n <= p || r.peak.CompareAndSwap(p, n) {
				break
			}
		}
		defer r.running.Add(-1)
		return fn()
	})
}

type harness struct {
	fs     *fakeStore
	c      *clock
	a      *Authenticator
	runner *countingRunner
	sleeps []time.Duration
	mu     sync.Mutex
}

type harnessOpts struct {
	realClock bool
	gateSize  int
	ip        *LimiterConfig
	target    *LimiterConfig
	floor     time.Duration
}

func newHarness(t *testing.T, o harnessOpts) *harness {
	t.Helper()
	h := &harness{fs: newFakeStore(), c: newClock()}
	now := h.c.Now
	if o.realClock {
		now = time.Now
	}
	if o.gateSize == 0 {
		o.gateSize = 4
	}
	gate, err := NewHashGate(o.gateSize, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	h.runner = &countingRunner{inner: gate}
	lo, err := NewLockout(h.fs, DefaultLockoutConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	tcfg := LimiterConfig{Burst: 1000, Every: time.Millisecond, Now: now}
	if o.target != nil {
		tcfg = *o.target
		tcfg.Now = now
	}
	target, err := NewPrincipalLimiter(tcfg)
	if err != nil {
		t.Fatal(err)
	}
	var ipl *IPLimiter
	if o.ip != nil {
		cfg := *o.ip
		cfg.Now = now
		if ipl, err = NewIPLimiter(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if o.floor == 0 {
		o.floor = time.Nanosecond
	}
	a, err := NewAuthenticator(AuthenticatorConfig{
		Accounts: h.fs, Lockout: lo, Gate: h.runner, Argon2: testParams, IP: ipl, Target: target,
		Now: now, MinFailureDuration: o.floor, Logger: slog.New(slog.DiscardHandler),
		Sleep: func(_ context.Context, d time.Duration) error {
			h.mu.Lock()
			h.sleeps = append(h.sleeps, d)
			h.mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	return h
}

func (h *harness) login(email, pass, ip string) Outcome {
	out, err := h.a.Verify(tctx(), LoginInput{Email: email, Password: pass, IP: ip})
	if err != nil {
		panic(err)
	}
	return out
}

func ipN(i int) string { return fmt.Sprintf("198.51.%d.%d", i/250, i%250+1) }

func TestUnknownEmailUsesDummyWithConfiguredParams(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	// Hash tiruan memakai parameter yang dikonfigurasi (bukan password.Dummy() berparameter default), supaya biayanya
	// identik dengan Hasher sungguhan.
	if h.a.dummy.Params != testParams || h.a.hasher.Params != testParams {
		t.Fatalf("dummy=%+v hasher=%+v, want %+v", h.a.dummy.Params, h.a.hasher.Params, testParams)
	}
	if want := "$argon2id$v=19$m=8192,t=1,p=1$"; !strings.HasPrefix(h.a.dummy.DummyHash(), want) {
		t.Fatalf("hash tiruan %q tidak berparameter terkonfigurasi (%s)", h.a.dummy.DummyHash(), want)
	}
	if testParams == password.DefaultParams() {
		t.Fatal("tes tidak bermakna: parameter uji sama dengan default")
	}
	out := h.login("tidak-ada@example.com", goodPassword, "198.51.100.1")
	if out.Kind != InvalidCredentials || out.Reason != ReasonUnknownEmail {
		t.Fatalf("outcome = %+v", out)
	}
	if got := h.runner.calls.Load(); got != 1 {
		t.Fatalf("verifikasi tiruan lewat gerbang = %d kali, want 1", got)
	}
	// Zero Argon2 = default OWASP.
	a, err := NewAuthenticator(AuthenticatorConfig{Accounts: h.fs, Lockout: h.a.lockout, Gate: h.runner, Target: h.a.target})
	if err != nil {
		t.Fatal(err)
	}
	if a.dummy.Params != password.DefaultParams() {
		t.Fatalf("zero Argon2 -> %+v, want default", a.dummy.Params)
	}
}

func TestInvalidHashOnRealAccountTreatedAsWrongPassword(t *testing.T) {
	for name, bad := range map[string]string{
		"kosong (SSO-only)": "", "sampah": "bukan-phc", "algoritma lain": "$argon2i$v=19$m=8192,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"parameter melewati batas": "$argon2id$v=19$m=99999999,t=1,p=1$c2FsdHNhbHRzYWx0$aGFzaGhhc2hoYXNoaGFzaGhhc2g",
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, harnessOpts{})
			u := h.fs.addUser("a@example.com", bad, store.UserActive)
			out, err := h.a.Verify(tctx(), LoginInput{Email: "a@example.com", Password: goodPassword, IP: "198.51.100.1"})
			if err != nil {
				t.Fatalf("hash cacat bukan galat server: %v", err)
			}
			if out.Kind != InvalidCredentials || out.Reason != ReasonInvalidHash {
				t.Fatalf("outcome = %+v, want InvalidCredentials/InvalidHash", out)
			}
			// ErrInvalidHash gagal sebelum argon2 berjalan: verifikasi tiruan menutup selisih waktu (2 pemanggilan gerbang).
			if got := h.runner.calls.Load(); got != 2 {
				t.Fatalf("pemanggilan gerbang = %d, want 2 (verifikasi gagal cepat + tiruan)", got)
			}
			// Dihitung sebagai kegagalan sandi pada pasangan (akun, IP).
			rows := h.fs.rows(u.ID)
			if len(rows) != 1 || rows[0].success {
				t.Fatalf("catatan percobaan = %+v, want satu kegagalan", rows)
			}
		})
	}
}

func TestAuthenticatorSuccessAndRehash(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	out := h.login(" A@Example.com ", goodPassword, "198.51.100.1")
	if out.Kind != Success || out.User.ID != u.ID || out.NeedsRehash {
		t.Fatalf("outcome = %+v", out)
	}
	if out.User.PasswordHash != "" {
		t.Fatal("hash sandi tidak boleh keluar dari Authenticator")
	}
	if len(h.fs.rows(u.ID)) != 0 {
		t.Fatal("sukses sandi tidak boleh dicatat sebelum autentikasi selesai penuh (MFA)")
	}
	if err := h.a.RegisterSuccess(tctx(), u.ID, "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	if rows := h.fs.rows(u.ID); len(rows) != 1 || !rows[0].success {
		t.Fatalf("rows = %+v, want satu sukses", rows)
	}

	// Hash tersimpan berparameter lama: sandi benar, NeedsRehash true.
	old, _ := (&password.Hasher{Params: password.Params{MemoryKiB: 8192, Time: 2, Parallelism: 1}}).Hash(goodPassword)
	h.fs.addUser("old@example.com", old, store.UserActive)
	if out := h.login("old@example.com", goodPassword, "198.51.100.2"); out.Kind != Success || !out.NeedsRehash || out.RehashFrom != old || out.User.PasswordHash != "" {
		t.Fatalf("outcome = %+v, want sukses + NeedsRehash + RehashFrom = hash lama (PasswordHash user kosong)", out)
	}
}

// failurePathTrace menjalankan satu login gagal untuk setiap jenis akun dan mengembalikan jejak pemanggilan store.
func failureTraces(t *testing.T) map[string][]string {
	t.Helper()
	traces := map[string][]string{}
	cases := map[string]struct{ email, status, hash, pass string }{
		"sandi salah":              {"salah@example.com", store.UserActive, goodHash(t), "bukan sandinya"},
		"email tak dikenal":        {"", "", "", goodPassword},
		"terkunci, sandi benar":    {"kunci@example.com", store.UserLocked, goodHash(t), goodPassword},
		"terkunci, sandi salah":    {"kunci2@example.com", store.UserLocked, goodHash(t), "bukan sandinya"},
		"dinonaktifkan":            {"off@example.com", store.UserDisabled, goodHash(t), goodPassword},
		"hash tersimpan cacat":     {"cacat@example.com", store.UserActive, "bukan-phc", goodPassword},
		"hash tersimpan kosong":    {"kosong@example.com", store.UserActive, "", goodPassword},
		"sandi melebihi 1024 byte": {"panjang@example.com", store.UserActive, goodHash(t), strings.Repeat("a", 2000)},
	}
	for name, tc := range cases {
		h := newHarness(t, harnessOpts{})
		email := tc.email
		if email == "" {
			email = "tidak-ada@example.com"
		} else {
			h.fs.addUser(email, tc.hash, tc.status)
		}
		h.fs.resetCalls()
		out := h.login(email, tc.pass, "198.51.100.1")
		if out.Kind != InvalidCredentials {
			t.Fatalf("%s: outcome = %+v, want InvalidCredentials", name, out)
		}
		traces[name] = h.fs.trace()
	}
	return traces
}

// TestFailureCallTraceIdenticalAcrossPaths: urutan panggilan database identik pada semua jalur gagal (email tak dikenal,
// sandi salah, terkunci, dinonaktifkan, hash cacat): tidak ada jalur yang lebih murah, sehingga latensi DB tidak
// membedakan jalur (jalur akun tak dikenal memakai UUID acak untuk panggilan tiruan).
func TestFailureCallTraceIdenticalAcrossPaths(t *testing.T) {
	traces := failureTraces(t)
	want := []string{"FindUserByEmail", "ReserveLoginAttempt", "CrossIPFailures"}
	for name, got := range traces {
		if !slices.Equal(got, want) {
			t.Errorf("%s: jejak = %v, want %v", name, got, want)
		}
	}
}

func TestPairBlockedAttemptsRejectedBeforeVerifyAndNotRecorded(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(fmt.Sprintf("akun dikenal=%v", known), func(t *testing.T) {
			h := newHarness(t, harnessOpts{})
			email := "tidak-ada@example.com"
			var u store.User
			if known {
				email = "a@example.com"
				u = h.fs.addUser(email, goodHash(t), store.UserActive)
			}
			for i := 1; i <= 5; i++ {
				if out := h.login(email, "salah", "198.51.100.1"); out.Kind != InvalidCredentials {
					t.Fatalf("gagal %d: %+v, want InvalidCredentials (jeda baru berlaku untuk permintaan berikutnya)", i, out)
				}
			}
			calls := h.runner.calls.Load()
			h.fs.resetCalls()
			out := h.login(email, goodPassword, "198.51.100.1") // sandi benar pun ditolak selama jeda
			if out.Kind != RateLimited || out.Reason != ReasonPairBlocked || out.RetryAfter != 30*time.Second {
				t.Fatalf("selama jeda: %+v, want RateLimited 30s", out)
			}
			if h.runner.calls.Load() != calls {
				t.Fatal("verifikasi sandi tidak boleh berjalan selama jeda")
			}
			if got, want := h.fs.trace(), []string{"FindUserByEmail", "ReserveLoginAttempt"}; !slices.Equal(got, want) {
				t.Fatalf("jejak selama jeda = %v, want %v (identik untuk akun dikenal dan tak dikenal)", got, want)
			}
			if known && len(h.fs.rows(u.ID)) != 5 {
				t.Fatalf("percobaan selama jeda tercatat: %d baris, want 5", len(h.fs.rows(u.ID)))
			}
		})
	}
}

// outcomeSequence menjalankan n percobaan berturut-turut dengan jam maju seperlunya dan mengembalikan urutan hasilnya.
func outcomeSequence(h *harness, email, pass, ip string, n int) []string {
	var seq []string
	for range n {
		out := h.login(email, pass, ip)
		seq = append(seq, fmt.Sprintf("%v/%v", out.Kind, out.RetryAfter))
		if out.Kind == RateLimited {
			h.c.Advance(out.RetryAfter)
		}
	}
	return seq
}

// TestBlockedResponseDoesNotRevealAccountExistence: urutan hasil (jenis dan Retry-After) identik untuk email yang ada,
// email tak dikenal, akun terkunci, dan akun dinonaktifkan, sepanjang seluruh jadwal jeda sampai blokir 1 jam. Tanpa
// pelacak email tak dikenal, akun nyata menjawab 429 sedangkan email tak dikenal terus 401 (oracle enumerasi).
func TestBlockedResponseDoesNotRevealAccountExistence(t *testing.T) {
	const attempts = 50
	run := func(email, status, pass string) []string {
		h := newHarness(t, harnessOpts{})
		if status != "" {
			h.fs.addUser(email, goodHash(t), status)
		}
		return outcomeSequence(h, email, pass, "198.51.100.1", attempts)
	}
	baseline := run("tidak-ada@example.com", "", goodPassword)
	if !strings.Contains(strings.Join(baseline, " "), "RateLimited/1h0m0s") {
		t.Fatalf("prasyarat: urutan dasar tidak mencapai blokir 1 jam: %v", baseline)
	}
	for name, got := range map[string][]string{
		"akun aktif, sandi salah":       run("a@example.com", store.UserActive, "salah"),
		"akun aktif, sandi benar":       run("b@example.com", store.UserActive, goodPassword), // sukses tidak terjadi: lihat di bawah
		"akun terkunci":                 run("c@example.com", store.UserLocked, goodPassword),
		"akun dinonaktifkan":            run("d@example.com", store.UserDisabled, goodPassword),
		"akun aktif, hash cacat":        run("e@example.com", store.UserActive, "salah"),
		"email tak dikenal, salah":      run("tidak-ada-2@example.com", "", "salah"),
		"email tak dikenal, benar":      run("tidak-ada-3@example.com", "", goodPassword),
		"akun aktif dengan huruf besar": run("F@Example.com", store.UserActive, "salah"),
	} {
		if name == "akun aktif, sandi benar" {
			// Sandi benar pada akun aktif sukses di percobaan pertama: bukan bagian dari perbandingan kegagalan.
			if !strings.HasPrefix(got[0], "Success") {
				t.Errorf("%s: %v, want sukses di awal", name, got)
			}
			continue
		}
		if !slices.Equal(got, baseline) {
			t.Errorf("%s: urutan hasil berbeda dari email tak dikenal (membocorkan keberadaan akun)\n got  %v\n want %v", name, got, baseline)
		}
	}
}

// Jendela 24 jam pasangan berlaku sama untuk email tak dikenal (pelacak memori): urutan hasil identik dengan akun nyata.
func TestPairWindowAppliesIdenticallyToUnknownEmail(t *testing.T) {
	run := func(email string, known bool) []string {
		h := newHarness(t, harnessOpts{})
		if known {
			h.fs.addUser(email, goodHash(t), store.UserActive)
		}
		var seq []string
		step := func() {
			out := h.login(email, "salah", "198.51.100.1")
			seq = append(seq, fmt.Sprintf("%v/%v", out.Kind, out.RetryAfter))
		}
		for range 5 {
			step()
		}
		step() // selama jeda
		h.c.Advance(24*time.Hour + time.Second)
		for range 5 { // jendela sudah bersih: 4 gagal tanpa jeda, gagal ke-5 baru memicu jeda pada permintaan berikutnya
			step()
		}
		step()
		return seq
	}
	want := run("tidak-ada@example.com", false)
	got := run("a@example.com", true)
	if !slices.Equal(got, want) {
		t.Fatalf("akun nyata dan tak dikenal berbeda: got %v want %v", got, want)
	}
	if want[5] != "RateLimited/30s" || want[6] != "InvalidCredentials/0s" || want[10] != "InvalidCredentials/0s" || want[11] != "RateLimited/30s" {
		t.Fatalf("urutan tak terduga: %v", want)
	}
}

func TestAccountLockedByCrossIPFailuresRejectsCorrectPasswordUniformlyUntilUnlocked(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	lockedNow := 0
	for round := range 10 {
		for i := range 5 {
			ip := ipN(i)
			out := h.login("a@example.com", "salah", ip)
			for out.Kind == RateLimited { // tunggu jeda pasangan, lalu ulangi
				h.c.Advance(out.RetryAfter)
				out = h.login("a@example.com", "salah", ip)
			}
			if out.Kind != InvalidCredentials {
				t.Fatalf("putaran %d IP %d: %+v", round, i, out)
			}
			if out.LockedNow {
				lockedNow++
			}
		}
	}
	if h.fs.status(u.ID) != store.UserLocked || lockedNow != 1 {
		t.Fatalf("status=%s lockedNow=%d, want locked tepat satu kali (pada gagal ke-50)", h.fs.status(u.ID), lockedNow)
	}
	// Sandi benar dari IP baru ditolak seragam (tidak mengungkap bahwa akunnya terkunci).
	out := h.login("a@example.com", goodPassword, "203.0.113.50")
	if out.Kind != InvalidCredentials || out.Reason != ReasonAccountLocked || out.User.ID != "" {
		t.Fatalf("akun terkunci + sandi benar: %+v, want InvalidCredentials", out)
	}
	// Tidak terbuka sendiri.
	h.c.Advance(30 * 24 * time.Hour)
	if out := h.login("a@example.com", goodPassword, "203.0.113.50"); out.Kind != InvalidCredentials {
		t.Fatalf("akun terkunci terbuka sendiri: %+v", out)
	}
	// Unlock (reset sandi / core:unlock / admin) membuka kembali.
	if err := h.a.lockout.Unlock(tctx(), u.ID); err != nil {
		t.Fatal(err)
	}
	if out := h.login("a@example.com", goodPassword, "203.0.113.50"); out.Kind != Success {
		t.Fatalf("setelah Unlock: %+v, want sukses", out)
	}
}

func TestDoSLockoutOtherIPLogsInWithoutDelay(t *testing.T) {
	// Varian 1: IP A menembak 25 kali tanpa menunggu (jeda berlaku), memakai pembatas sasaran bawaan.
	t.Run("tanpa menunggu", func(t *testing.T) {
		tc := DefaultTargetLimiterConfig()
		h := newHarness(t, harnessOpts{target: &tc})
		u := h.fs.addUser("korban@example.com", goodHash(t), store.UserActive)
		var kinds []OutcomeKind
		for range 25 {
			kinds = append(kinds, h.login("korban@example.com", "tebakan", "203.0.113.9").Kind)
		}
		if got := h.fs.rows(u.ID); len(got) != 5 {
			t.Fatalf("kegagalan tercatat = %d, want 5 (sisanya ditolak oleh jeda tanpa dicatat)", len(got))
		}
		if kinds[5] != RateLimited {
			t.Fatalf("permintaan ke-6 = %v, want RateLimited", kinds[5])
		}
		out := h.login("korban@example.com", goodPassword, "198.51.100.77")
		if out.Kind != Success {
			t.Fatalf("pemilik akun dari IP lain: %+v, want sukses tanpa jeda", out)
		}
		if h.fs.status(u.ID) != store.UserActive {
			t.Fatal("satu IP tidak boleh mengunci akun")
		}
	})
	// Varian 2: IP A terus menembak sepanjang jadwal (menunggu tiap jeda): 20 kegagalan, lalu blokir pasangan 1 jam.
	t.Run("menunggu tiap jeda", func(t *testing.T) {
		h := newHarness(t, harnessOpts{})
		u := h.fs.addUser("korban@example.com", goodHash(t), store.UserActive)
		failures := func() int {
			n := 0
			for _, r := range h.fs.rows(u.ID) {
				if !r.success {
					n++
				}
			}
			return n
		}
		for range 200 {
			if failures() == 20 {
				break
			}
			out := h.login("korban@example.com", "tebakan", "203.0.113.9")
			if out.Kind == RateLimited {
				h.c.Advance(out.RetryAfter)
			}
		}
		if failures() != 20 {
			t.Fatalf("kegagalan tercatat = %d, want 20", failures())
		}
		for range 5 { // permintaan berikutnya ditolak oleh blokir 1 jam dan tidak dicatat
			if out := h.login("korban@example.com", "tebakan", "203.0.113.9"); out.Kind != RateLimited || out.RetryAfter != time.Hour {
				t.Fatalf("%+v, want RateLimited 1h", out)
			}
		}
		if failures() != 20 {
			t.Fatalf("kegagalan tercatat = %d setelah blokir, want tetap 20", failures())
		}
		if h.fs.status(u.ID) != store.UserActive {
			t.Fatal("satu IP tidak boleh mengunci akun")
		}
		if out := h.login("korban@example.com", goodPassword, "198.51.100.77"); out.Kind != Success {
			t.Fatalf("pemilik akun dari IP B: %+v, want sukses", out)
		}
	})
}

func TestIPLimiterAndTargetLimiterAnswer429BeforeAnyWork(t *testing.T) {
	t.Run("per IP", func(t *testing.T) {
		h := newHarness(t, harnessOpts{ip: &LimiterConfig{Burst: 2, Every: 10 * time.Second}})
		h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
		h.login("a@example.com", "x", "198.51.100.1")
		h.login("b@example.com", "x", "198.51.100.1")
		h.fs.resetCalls()
		calls := h.runner.calls.Load()
		out := h.login("c@example.com", "x", "198.51.100.1")
		if out.Kind != RateLimited || out.Reason != ReasonIPLimited || out.RetryAfter != 10*time.Second {
			t.Fatalf("%+v, want RateLimited 10s", out)
		}
		if len(h.fs.trace()) != 0 || h.runner.calls.Load() != calls {
			t.Fatal("batas IP harus menolak sebelum kueri atau hash apa pun")
		}
		if out := h.login("c@example.com", "x", "198.51.100.2"); out.Kind == RateLimited {
			t.Fatal("IP lain tidak boleh terkena batas IP pertama")
		}
	})
	t.Run("per sasaran", func(t *testing.T) {
		h := newHarness(t, harnessOpts{target: &LimiterConfig{Burst: 2, Every: 20 * time.Second}})
		h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
		for i, email := range []string{"a@example.com", "tidak-ada@example.com"} {
			for j := range 2 {
				if out := h.login(email, "x", ipN(i*10+j)); out.Kind != InvalidCredentials {
					t.Fatalf("%s #%d: %+v", email, j, out)
				}
			}
			calls := h.runner.calls.Load()
			out := h.login(email, "x", ipN(i*10+5)) // IP lain, sasaran sama
			if out.Kind != RateLimited || out.Reason != ReasonTargetLimited || out.RetryAfter != 20*time.Second {
				t.Fatalf("%s ketiga: %+v, want RateLimited 20s (sama untuk akun dikenal dan tak dikenal)", email, out)
			}
			if h.runner.calls.Load() != calls {
				t.Fatalf("%s: hash berjalan padahal sasaran dibatasi", email)
			}
		}
		// Sasaran lain tidak terdampak.
		if out := h.login("lain@example.com", "x", ipN(99)); out.Kind != InvalidCredentials {
			t.Fatalf("sasaran lain: %+v", out)
		}
	})
}

func TestRateLimitedWhenHashGateBusyIdenticalForKnownAndUnknown(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	h.runner.busy.Store(true)
	for _, email := range []string{"a@example.com", "tidak-ada@example.com"} {
		out := h.login(email, goodPassword, "198.51.100.1")
		if out.Kind != RateLimited || out.Reason != ReasonHashBusy || out.RetryAfter != time.Second {
			t.Fatalf("%s: %+v, want RateLimited 1s", email, out)
		}
	}
	if len(h.fs.rows(u.ID)) != 0 {
		t.Fatal("gerbang penuh bukan kegagalan sandi: tidak boleh dicatat")
	}
	h.runner.busy.Store(false)
	if out := h.login("a@example.com", goodPassword, "198.51.100.1"); out.Kind != Success {
		t.Fatalf("setelah gerbang lega: %+v", out)
	}
}

func TestMinFailureDurationFloorOnFailuresOnly(t *testing.T) {
	h := newHarness(t, harnessOpts{floor: 200 * time.Millisecond})
	h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	h.login("a@example.com", "salah", "198.51.100.1")
	h.login("tidak-ada@example.com", "salah", "198.51.100.2")
	if len(h.sleeps) != 2 || h.sleeps[0] != 200*time.Millisecond || h.sleeps[1] != 200*time.Millisecond {
		t.Fatalf("tidur = %v, want dua kali 200ms (jam palsu tidak maju selama pemrosesan)", h.sleeps)
	}
	h.sleeps = nil
	h.login("a@example.com", goodPassword, "198.51.100.3")
	if len(h.sleeps) != 0 {
		t.Fatalf("sukses tidak boleh ditunda: %v", h.sleeps)
	}
	// 429 jeda juga tidak ditunda (cepat dan identik untuk akun dikenal/tak dikenal).
	for range 5 {
		h.login("a@example.com", "salah", "198.51.100.4")
	}
	h.sleeps = nil
	if out := h.login("a@example.com", "salah", "198.51.100.4"); out.Kind != RateLimited || len(h.sleeps) != 0 {
		t.Fatalf("out=%+v tidur=%v", out, h.sleeps)
	}
}

func TestAuthenticatorReturnsStoreErrorsAndRequiresTenant(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	h.fs.failOn["FindUserByEmail"] = errBoom
	if _, err := h.a.Verify(tctx(), LoginInput{Email: "a@example.com", Password: "x", IP: "198.51.100.1"}); !errors.Is(err, errBoom) {
		t.Fatalf("galat store harus dirambatkan (500), bukan dianggap email tak dikenal: %v", err)
	}
	delete(h.fs.failOn, "FindUserByEmail")
	if _, err := h.a.Verify(context.Background(), LoginInput{Email: "a@example.com", Password: "x", IP: "198.51.100.1"}); !errors.Is(err, ErrNoTenant) {
		t.Fatalf("tanpa tenant: %v", err)
	}
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	h.fs.failOn["ReserveLoginAttempt"] = errBoom
	if _, err := h.a.Verify(tctx(), LoginInput{Email: "a@example.com", Password: "salah", IP: "198.51.100.1"}); !errors.Is(err, errBoom) {
		t.Fatalf("gagal mencatat kegagalan harus galat (gagal tertutup): %v", err)
	}
	_ = u
}

func TestAuthenticatorConfigValidation(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	base := AuthenticatorConfig{Accounts: h.fs, Lockout: h.a.lockout, Gate: h.runner, Target: h.a.target}
	for name, mut := range map[string]func(*AuthenticatorConfig){
		"tanpa Accounts": func(c *AuthenticatorConfig) { c.Accounts = nil },
		"tanpa Lockout":  func(c *AuthenticatorConfig) { c.Lockout = nil },
		"tanpa Gate":     func(c *AuthenticatorConfig) { c.Gate = nil },
		"tanpa Target":   func(c *AuthenticatorConfig) { c.Target = nil },
	} {
		cfg := base
		mut(&cfg)
		if _, err := NewAuthenticator(cfg); err == nil {
			t.Errorf("%s: konfigurasi tidak valid diterima", name)
		}
	}
	if _, err := NewAuthenticator(base); err != nil {
		t.Fatalf("konfigurasi valid ditolak: %v", err)
	}
}

// TestAuthenticatorVerifyBoundedBySemaphore: login serentak (akun dikenal dan tak dikenal) tidak pernah menjalankan
// lebih banyak Verify daripada kapasitas gerbang.
func TestAuthenticatorVerifyBoundedBySemaphore(t *testing.T) {
	const capacity = 2
	h := newHarness(t, harnessOpts{gateSize: capacity})
	for i := range 8 {
		h.fs.addUser(fmt.Sprintf("u%d@example.com", i), goodHash(t), store.UserActive)
	}
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			email := fmt.Sprintf("u%d@example.com", i%16) // separuhnya tak dikenal
			if _, err := h.a.Verify(tctx(), LoginInput{Email: email, Password: "salah", IP: ipN(i)}); err != nil {
				t.Errorf("Verify: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := h.runner.peak.Load(); got > capacity {
		t.Fatalf("Verify bersamaan mencapai %d, melebihi kapasitas %d", got, capacity)
	}
	if got := h.runner.peak.Load(); got < 1 {
		t.Fatal("tidak ada Verify yang berjalan")
	}
}

func median(ds []time.Duration) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[len(s)/2]
}

// TestLoginTimingUnknownEmailMatchesKnownEmailFailure (Review Focus 1): waktu respons jalur gagal akun-tak-dikenal,
// terkunci, dinonaktifkan, dan hash-cacat tidak lebih cepat secara signifikan dari sandi salah pada akun nyata (rasio
// median <= 1,2). Parameter argon2 kecil, latensi DB tiruan, pemanasan, urutan bergantian, jam nyata. Tiap iterasi
// memakai akun dan IP baru sehingga tak ada jeda yang ikut berlaku.
func TestLoginTimingUnknownEmailMatchesKnownEmailFailure(t *testing.T) {
	h := newHarness(t, harnessOpts{realClock: true})
	h.fs.delay = 300 * time.Microsecond
	const iterations = 60
	type path struct {
		name   string
		status string
		hash   string
		known  bool
		pass   string
		member string // status keanggotaan tenant; kosong = active
	}
	paths := []path{
		{"sandi salah (dasar)", store.UserActive, goodHash(t), true, "bukan sandinya", ""},
		{"email tak dikenal", "", "", false, goodPassword, ""},
		{"akun terkunci, sandi benar", store.UserLocked, goodHash(t), true, goodPassword, ""},
		{"akun dinonaktifkan, sandi benar", store.UserDisabled, goodHash(t), true, goodPassword, ""},
		{"hash tersimpan cacat", store.UserActive, "bukan-phc", true, goodPassword, ""},
		{"keanggotaan nonaktif, sandi benar", store.UserActive, goodHash(t), true, goodPassword, store.MembershipSuspended},
	}
	samples := make([][]time.Duration, len(paths))
	counter := 0
	once := func(p path) time.Duration {
		counter++
		email := fmt.Sprintf("u%d@example.com", counter)
		if p.known {
			u := h.fs.addUser(email, p.hash, p.status)
			if p.member != "" {
				h.fs.mu.Lock()
				h.fs.users[u.ID].MembershipStatus = p.member
				h.fs.mu.Unlock()
			}
		}
		start := time.Now()
		out, err := h.a.Verify(tctx(), LoginInput{Email: email, Password: p.pass, IP: ipN(counter)})
		d := time.Since(start)
		if err != nil || out.Kind != InvalidCredentials {
			t.Fatalf("%s: out=%+v err=%v", p.name, out, err)
		}
		return d
	}
	for range 10 { // pemanasan: alokasi memori argon2 dan cache
		for _, p := range paths {
			once(p)
		}
	}
	for i := range iterations {
		// Urutan bergantian (maju/mundur): biaya GC dari pemanggilan sebelumnya tidak selalu membebani satu jalur.
		for k := range paths {
			idx := k
			if i%2 == 1 {
				idx = len(paths) - 1 - k
			}
			samples[idx] = append(samples[idx], once(paths[idx]))
		}
	}
	base := median(samples[0])
	for i, p := range paths[1:] {
		m := median(samples[i+1])
		hi, lo := base, m
		if lo > hi {
			hi, lo = lo, hi
		}
		ratio := float64(hi) / float64(lo)
		t.Logf("%-34s median=%v dasar=%v rasio=%.3f", p.name, m, base, ratio)
		if ratio > 1.2 {
			t.Errorf("%s: rasio median %.3f > 1,2: jalur dapat dibedakan lewat waktu", p.name, ratio)
		}
	}
}
