package guard

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"erpora/internal/platform/identity/store"
)

const (
	v6a1 = "2001:db8:1:2:aaaa:bbbb:cccc:1"  // /64-A
	v6a2 = "2001:db8:1:2:1111:2222:3333:99" // /64-A (host lain)
	v6b  = "2001:db8:1:3::5"                // /64-B
	keyA = "2001:db8:1:2::/64"
)

// A-1: kunci pasangan IPv6 harus sama di semua jalur (sandi, sukses, MFA, pelacak email tak dikenal).

func TestIPv6FailureDoesNotBlockOtherPrefix(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	for i := range 25 {
		out := h.login("a@example.com", "salah", fmt.Sprintf("2001:db8:1:2::%x", i+1))
		if out.Kind == Success {
			t.Fatal("sandi salah tidak boleh sukses")
		}
		if out.Kind == RateLimited {
			h.c.Advance(out.RetryAfter)
		}
	}
	for _, r := range h.fs.rows(u.ID) {
		if r.ip != keyA {
			t.Fatalf("baris tercatat pada kunci %q, want %q (satu kunci per /64)", r.ip, keyA)
		}
	}
	// Penyerang di /64-A sedang diblokir; pengguna sah dari /64-B masuk tanpa jeda.
	if out := h.login("a@example.com", goodPassword, v6b); out.Kind != Success {
		t.Fatalf("pengguna dari /64 lain: %+v, want Success", out)
	}
	if out := h.login("a@example.com", goodPassword, v6a1); out.Kind != RateLimited {
		t.Fatalf("penyerang /64-A: %+v, want RateLimited", out)
	}
}

func TestIPv6SuccessResetsSamePair(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	for range 4 {
		h.login("a@example.com", "salah", v6a1)
	}
	if err := h.a.RegisterSuccess(tctx(), u.ID, v6a2); err != nil { // IP mentah lain di /64 yang sama
		t.Fatal(err)
	}
	ps, err := h.fs.PairFailureStateSince(tctx(), testTenant, u.ID, keyA, time.Time{})
	if err != nil || ps.ConsecutiveFailures != 0 {
		t.Fatalf("setelah sukses: %+v %v, want 0 kegagalan pada kunci %s", ps, err, keyA)
	}
	if out := h.login("a@example.com", "salah", v6a1); out.Kind != InvalidCredentials {
		t.Fatalf("setelah reset: %+v, want InvalidCredentials (bukan jeda)", out)
	}
}

func TestIPv6PasswordAndMFAFailuresShareOnePair(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	for range 3 {
		h.login("a@example.com", "salah", v6a1)
	}
	for range 1 {
		res, st, err := h.a.lockout.Reserve(tctx(), store.AttemptMFA, u.ID, v6a2) // IP mentah, seperti adaptor MFA
		if err != nil || st.PairBlocked || !res.Reserved() {
			t.Fatalf("Reserve MFA: %+v %+v %v", res, st, err)
		}
		if _, err := h.a.lockout.Fail(tctx(), res); err != nil {
			t.Fatal(err)
		}
	}
	// 3 sandi + 1 MFA = 4 pada pasangan yang sama; satu kegagalan sandi lagi menjadi yang ke-5 -> jeda.
	if out := h.login("a@example.com", "salah", v6a1); out.Kind != InvalidCredentials {
		t.Fatalf("gagal ke-5: %+v", out)
	}
	if out := h.login("a@example.com", goodPassword, v6a2); out.Kind != RateLimited || out.Reason != ReasonPairBlocked {
		t.Fatalf("pasangan harus dijeda setelah 5 gagal campuran sandi+MFA: %+v", out)
	}
	for _, r := range h.fs.rows(u.ID) {
		if r.ip != keyA {
			t.Fatalf("kunci baris = %q, want %q", r.ip, keyA)
		}
	}
}

func TestIPv6UnknownEmailIndistinguishableFromKnown(t *testing.T) {
	run := func(email string, addUser bool) []string {
		h := newHarness(t, harnessOpts{})
		if addUser {
			h.fs.addUser(email, goodHash(t), store.UserActive)
		}
		return outcomeSequence(h, email, "salah", v6a1, 30)
	}
	known, unknown := run("a@example.com", true), run("tidak-ada@example.com", false)
	if !slices.Equal(known, unknown) {
		t.Fatalf("urutan hasil berbeda (oracle enumerasi):\nkenal   = %v\ntakdikenal = %v", known, unknown)
	}
	if !slices.ContainsFunc(known, func(s string) bool { return s != known[0] }) {
		t.Fatal("tes tidak bermakna: tidak pernah ada jeda")
	}
}

// A-2: reservasi atomik per pasangan.

// seedFailures mengisi n kegagalan pasangan (akun, kunci IP) berselang agar lastAgo lalu terjadi terakhir.
func seedFailures(h *harness, userID, ipKey string, n int, lastAgo time.Duration) {
	now := h.c.Now()
	for i := range n {
		at := now.Add(-lastAgo - time.Duration(n-1-i)*time.Second)
		_ = h.fs.RecordLoginAttempt(tctx(), testTenant, userID, ipKey, store.AttemptPassword, false, at)
	}
}

func TestConcurrentGuessesAtThresholdReachVerifyAtMostOnce(t *testing.T) {
	const burst = 24
	cases := []struct {
		name    string
		n       int
		lastAgo time.Duration
		known   bool
	}{
		{"akun dikenal, 4 gagal (gagal ke-5 masih dijatah)", 4, 5 * time.Second, true},
		{"akun dikenal, jeda 30 detik baru habis", 5, 31 * time.Second, true},
		{"akun dikenal, blokir 1 jam baru habis", 20, time.Hour + time.Minute, true},
		{"email tak dikenal, 4 gagal", 4, 5 * time.Second, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, harnessOpts{gateSize: burst})
			email := "tidak-ada@example.com"
			if tc.known {
				email = "a@example.com"
				u := h.fs.addUser(email, goodHash(t), store.UserActive)
				seedFailures(h, u.ID, "198.51.100.1", tc.n, tc.lastAgo)
			} else {
				for range tc.n { // pelacak memori: gagal berurutan dengan jam maju sehingga gagal terakhir tc.lastAgo lalu
					h.login(email, "salah", "198.51.100.1")
					h.c.Advance(time.Second)
				}
				h.c.Advance(tc.lastAgo - time.Second)
			}
			before := h.runner.calls.Load()
			var wg sync.WaitGroup
			var limitedN, invalidN atomic.Int32
			start := make(chan struct{})
			for range burst {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					out, err := h.a.Verify(tctx(), LoginInput{Email: email, Password: "salah", IP: "198.51.100.1"})
					if err != nil {
						t.Errorf("Verify: %v", err)
						return
					}
					switch out.Kind {
					case RateLimited:
						limitedN.Add(1)
					case InvalidCredentials:
						invalidN.Add(1)
					default:
						t.Errorf("outcome = %+v", out)
					}
				}()
			}
			close(start)
			wg.Wait()
			if got := h.runner.calls.Load() - before; got != 1 {
				t.Fatalf("verifikasi dijalankan %d kali, want tepat 1 (jadwal jeda hanya mengizinkan satu tebakan)", got)
			}
			if invalidN.Load() != 1 || limitedN.Load() != burst-1 {
				t.Fatalf("invalid=%d limited=%d, want 1 dan %d", invalidN.Load(), limitedN.Load(), burst-1)
			}
		})
	}
}

func TestConcurrentMFAGuessesReserveAtMostOnce(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	seedFailures(h, u.ID, "198.51.100.1", 4, 5*time.Second)
	var wg sync.WaitGroup
	var admitted atomic.Int32
	start := make(chan struct{})
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, st, err := h.a.lockout.Reserve(tctx(), store.AttemptMFA, u.ID, "198.51.100.1")
			if err != nil {
				t.Errorf("Reserve: %v", err)
				return
			}
			if res.Reserved() {
				admitted.Add(1)
			} else if !st.PairBlocked {
				t.Errorf("tidak dicadangkan dan tidak diblokir: %+v", st)
			}
		}()
	}
	close(start)
	wg.Wait()
	if admitted.Load() != 1 {
		t.Fatalf("reservasi MFA = %d, want 1", admitted.Load())
	}
}

func TestReservationReleasedOnEveryNonFailureOutcome(t *testing.T) {
	// Sandi benar: bukan kegagalan.
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	if out := h.login("a@example.com", goodPassword, "198.51.100.1"); out.Kind != Success {
		t.Fatalf("%+v", out)
	}
	if n := len(h.fs.rows(u.ID)); n != 0 {
		t.Fatalf("sukses sandi meninggalkan %d baris reservasi", n)
	}
	// Pembatas hash penuh: dibatasi laju, bukan kegagalan.
	h.runner.busy.Store(true)
	if out := h.login("a@example.com", goodPassword, "198.51.100.1"); out.Kind != RateLimited || out.Reason != ReasonHashBusy {
		t.Fatalf("%+v", out)
	}
	if n := len(h.fs.rows(u.ID)); n != 0 {
		t.Fatalf("hash_busy meninggalkan %d baris reservasi", n)
	}
	// Pembatas per email: dibatasi laju, bukan kegagalan.
	h2 := newHarness(t, harnessOpts{target: &LimiterConfig{Burst: 1, Every: time.Hour}})
	u2 := h2.fs.addUser("b@example.com", goodHash(t), store.UserActive)
	h2.login("b@example.com", goodPassword, "198.51.100.1")
	if out := h2.login("b@example.com", goodPassword, "198.51.100.1"); out.Kind != RateLimited || out.Reason != ReasonTargetLimited {
		t.Fatalf("%+v", out)
	}
	if n := len(h2.fs.rows(u2.ID)); n != 0 {
		t.Fatalf("target_limited meninggalkan %d baris reservasi", n)
	}
	// Galat infrastruktur setelah reservasi (kueri lintas IP gagal pada jalur gagal) mempertahankan kegagalan: konservatif.
	// Email tak dikenal: reservasi memori dilepas pada jalur non-kegagalan yang sama.
	h3 := newHarness(t, harnessOpts{})
	h3.runner.busy.Store(true)
	for range 10 {
		if out := h3.login("tidak-ada@example.com", "x", "198.51.100.1"); out.Kind != RateLimited || out.Reason != ReasonHashBusy {
			t.Fatalf("%+v", out)
		}
	}
	h3.runner.busy.Store(false)
	for i := 1; i <= 5; i++ {
		if out := h3.login("tidak-ada@example.com", "x", "198.51.100.1"); out.Kind != InvalidCredentials {
			t.Fatalf("gagal %d: %+v, want InvalidCredentials (reservasi memori harus dilepas saat hash_busy)", i, out)
		}
	}
}

// M-4: panic di jalur verifikasi (dipulihkan pemanggil, mis. middleware recover) tidak boleh membocorkan reservasi:
// baris percobaan yang tertinggal akan terhitung sebagai kegagalan dan bisa mengunci akun tanpa tebakan salah.
func TestReservationReleasedWhenVerifyPanics(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	h.runner.panics.Store(true)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic seharusnya merambat ke pemanggil")
			}
		}()
		_, _ = h.a.Verify(tctx(), LoginInput{Email: "a@example.com", Password: goodPassword, IP: "198.51.100.1"})
	}()
	if n := len(h.fs.rows(u.ID)); n != 0 {
		t.Fatalf("panic membocorkan %d baris reservasi", n)
	}
	// Email tak dikenal: reservasi memori juga dilepas.
	func() {
		defer func() { _ = recover() }()
		_, _ = h.a.Verify(tctx(), LoginInput{Email: "tidak-ada@example.com", Password: "x", IP: "198.51.100.1"})
	}()
	h.runner.panics.Store(false)
	for i := 1; i <= 5; i++ {
		if out := h.login("tidak-ada@example.com", "x", "198.51.100.1"); out.Kind != InvalidCredentials {
			t.Fatalf("gagal %d: %+v, want InvalidCredentials (reservasi memori bocor setelah panic)", i, out)
		}
	}
}

func TestReleaseSurvivesCancelledContext(t *testing.T) {
	h := newHarness(t, harnessOpts{})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	res, _, err := h.a.lockout.Reserve(tctx(), store.AttemptMFA, u.ID, "198.51.100.1")
	if err != nil || !res.Reserved() {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(tctx())
	cancel()
	if err := h.a.lockout.Release(ctx, res); err != nil {
		t.Fatal(err)
	}
	if n := len(h.fs.rows(u.ID)); n != 0 {
		t.Fatalf("reservasi tidak dilepas: %d baris", n)
	}
}

// A-4: anggota tenant nonaktif dengan sandi benar diperlakukan seperti sandi salah (tidak ada oracle "sandi benar").

func TestSuspendedMembershipTreatedLikeWrongPassword(t *testing.T) {
	h := newHarness(t, harnessOpts{floor: 50 * time.Millisecond})
	u := h.fs.addUser("a@example.com", goodHash(t), store.UserActive)
	h.fs.users[u.ID].MembershipStatus = store.MembershipSuspended
	h.fs.resetCalls()
	out := h.login("a@example.com", goodPassword, "198.51.100.1")
	if out.Kind != InvalidCredentials || out.Reason != ReasonMembershipInactive {
		t.Fatalf("outcome = %+v, want InvalidCredentials/membership_inactive", out)
	}
	if got, want := h.fs.trace(), []string{"FindUserByEmail", "ReserveLoginAttempt", "CrossIPFailures"}; !slices.Equal(got, want) {
		t.Fatalf("jejak = %v, want %v (identik dengan sandi salah)", got, want)
	}
	if rows := h.fs.rows(u.ID); len(rows) != 1 || rows[0].success {
		t.Fatalf("kegagalan harus tercatat: %+v", rows)
	}
	h.mu.Lock()
	sleeps := slices.Clone(h.sleeps)
	h.mu.Unlock()
	if len(sleeps) != 1 || sleeps[0] <= 0 {
		t.Fatalf("lantai waktu harus berlaku: %v", sleeps)
	}
	if got := h.runner.calls.Load(); got != 1 {
		t.Fatalf("verifikasi argon2 (tiruan) = %d kali, want 1", got)
	}
}
