package guard

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"erpora/internal/platform/identity/store"
)

func newTestLockout(t *testing.T, fs *fakeStore, c *clock) *Lockout {
	t.Helper()
	l, err := NewLockout(fs, DefaultLockoutConfig(), c.Now)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// failN mendaftarkan n kegagalan untuk (user, ip) tanpa menunggu jeda (RegisterFailure hanya mencatat; penolakan
// selama jeda adalah tugas Check/Authenticator) dan mengembalikan State terakhir.
func failN(t *testing.T, l *Lockout, c *clock, userID, ip string, n int) State {
	t.Helper()
	var st State
	var err error
	for range n {
		if st, err = l.RegisterFailure(tctx(), userID, ip); err != nil {
			t.Fatal(err)
		}
		c.Advance(time.Second)
	}
	return st
}

func TestPairPenaltySchedule(t *testing.T) {
	cfg := DefaultLockoutConfig()
	cases := []struct {
		n    int
		want time.Duration
	}{
		{0, 0}, {1, 0}, {4, 0},
		{5, 30 * time.Second}, {6, 60 * time.Second}, {7, 120 * time.Second}, {8, 240 * time.Second}, {9, 480 * time.Second},
		{10, 15 * time.Minute}, {11, 15 * time.Minute}, {19, 15 * time.Minute},
		{20, time.Hour}, {21, time.Hour}, {1000, time.Hour},
	}
	for _, tc := range cases {
		if got := cfg.pairPenalty(tc.n); got != tc.want {
			t.Errorf("pairPenalty(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestPairDelayStartsExactlyAtFifthFailure(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	for i := 1; i <= 4; i++ {
		st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
		if err != nil {
			t.Fatal(err)
		}
		if st.PairBlocked || st.RetryAfter != 0 {
			t.Fatalf("gagal ke-%d: %+v, want tanpa jeda", i, st)
		}
	}
	st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.PairBlocked || st.RetryAfter != 30*time.Second || st.AccountLocked {
		t.Fatalf("gagal ke-5: %+v, want jeda 30s", st)
	}
	// Check membaca jeda yang sama dan menyusut seiring waktu.
	c.Advance(10 * time.Second)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); !st.PairBlocked || st.RetryAfter != 20*time.Second {
		t.Fatalf("Check setelah 10s: %+v, want jeda 20s", st)
	}
	c.Advance(20 * time.Second)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); st.PairBlocked || st.RetryAfter != 0 {
		t.Fatalf("Check setelah 30s: %+v, want tidak diblokir", st)
	}
	// Gagal ke-6 menggandakan jeda.
	st, _ = l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if !st.PairBlocked || st.RetryAfter != 60*time.Second {
		t.Fatalf("gagal ke-6: %+v, want jeda 60s", st)
	}
	// Pasangan lain (IP lain) tidak terkena jeda.
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.2"); st.PairBlocked {
		t.Fatalf("IP lain terkena jeda: %+v", st)
	}
}

func TestPairBlockedForAnHourAtTwentiethFailureWithoutLockingAccount(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	st := failN(t, l, c, u.ID, "198.51.100.1", 19)
	if st.RetryAfter > 15*time.Minute {
		t.Fatalf("gagal ke-19: %+v, jeda maksimum 15 menit", st)
	}
	c.Advance(-time.Second) // failN memajukan 1s setelah gagal terakhir; kembali ke saat gagal ke-19
	st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.PairBlocked || st.RetryAfter != time.Hour {
		t.Fatalf("gagal ke-20: %+v, want pasangan diblokir 1 jam", st)
	}
	if st.AccountLocked || fs.status(u.ID) != store.UserActive {
		t.Fatal("satu pasangan tidak boleh mengunci akun")
	}
	// Selama blokir, IP lain tetap bebas.
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.2"); st.PairBlocked {
		t.Fatalf("IP lain ikut diblokir: %+v", st)
	}
	c.Advance(59 * time.Minute)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); !st.PairBlocked {
		t.Fatal("blokir harus bertahan sampai 1 jam penuh")
	}
	c.Advance(time.Minute)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); st.PairBlocked {
		t.Fatalf("blokir 1 jam harus berakhir: %+v", st)
	}
}

func TestAccountLockedOnlyAtFiftyFailuresAcrossFiveIPs(t *testing.T) {
	ips := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("198.51.100.%d", i+1)
		}
		return out
	}
	run := func(t *testing.T, nIPs, failures int) (State, string) {
		t.Helper()
		fs, c := newFakeStore(), newClock()
		u := fs.addUser("a@example.com", "x", store.UserActive)
		l := newTestLockout(t, fs, c)
		var st State
		for i := range failures {
			var err error
			if st, err = l.RegisterFailure(tctx(), u.ID, ips(nIPs)[i%nIPs]); err != nil {
				t.Fatal(err)
			}
			c.Advance(time.Second) // seluruhnya dalam satu jam
		}
		return st, fs.status(u.ID)
	}
	// Tepat di bawah dan di atas kedua ambang.
	if st, status := run(t, 5, 49); st.AccountLocked || status != store.UserActive {
		t.Fatalf("49 gagal lintas 5 IP: %+v status=%s, tidak boleh terkunci", st, status)
	}
	if st, status := run(t, 5, 50); !st.AccountLocked || status != store.UserLocked {
		t.Fatalf("50 gagal lintas 5 IP: %+v status=%s, harus terkunci", st, status)
	}
	if st, status := run(t, 4, 50); st.AccountLocked || status != store.UserActive {
		t.Fatalf("50 gagal lintas 4 IP: %+v status=%s, tidak boleh terkunci (kurang dari 5 IP)", st, status)
	}
	if st, status := run(t, 4, 400); st.AccountLocked || status != store.UserActive {
		t.Fatalf("400 gagal lintas 4 IP: %+v status=%s, jumlah IP tetap di bawah ambang", st, status)
	}
	if st, status := run(t, 1, 200); st.AccountLocked || status != store.UserActive {
		t.Fatalf("200 gagal dari 1 IP: %+v status=%s, satu IP tidak boleh mengunci akun", st, status)
	}
	if st, status := run(t, 10, 50); !st.AccountLocked || status != store.UserLocked {
		t.Fatalf("50 gagal lintas 10 IP: %+v status=%s, harus terkunci", st, status)
	}
}

func TestAccountLockWindowIsOneHour(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	// 49 gagal lintas 7 IP, lalu 1 jam berlalu: kegagalan lama keluar dari jendela, satu gagal baru tidak mengunci.
	for i := range 49 {
		if _, err := l.RegisterFailure(tctx(), u.ID, fmt.Sprintf("198.51.100.%d", i%7+1)); err != nil {
			t.Fatal(err)
		}
	}
	c.Advance(time.Hour + time.Second)
	st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if st.AccountLocked || fs.status(u.ID) != store.UserActive {
		t.Fatalf("kegagalan di luar jendela 1 jam tidak boleh dihitung: %+v", st)
	}
}

func TestAccountLockDoesNotReleaseItself(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	for i := range 50 {
		if _, err := l.RegisterFailure(tctx(), u.ID, fmt.Sprintf("198.51.100.%d", i%5+1)); err != nil {
			t.Fatal(err)
		}
	}
	if fs.status(u.ID) != store.UserLocked {
		t.Fatal("akun seharusnya terkunci")
	}
	c.Advance(30 * 24 * time.Hour)
	if _, err := l.Check(tctx(), u.ID, "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	if fs.status(u.ID) != store.UserLocked {
		t.Fatal("akun locked tidak boleh terbuka sendiri seiring waktu")
	}
}

func TestSuccessResetsPairCountButNotCrossIPWindow(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	failN(t, l, c, u.ID, "198.51.100.1", 4)
	if err := l.RegisterSuccess(tctx(), u.ID, "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	c.Advance(time.Second)
	st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if ps, _ := fs.PairFailureState(tctx(), testTenant, u.ID, "198.51.100.1"); ps.ConsecutiveFailures != 1 || st.PairBlocked {
		t.Fatalf("setelah sukses: %+v %+v, hitungan pasangan harus mulai dari 1", st, ps)
	}
	// Sukses tidak mengosongkan jendela lintas IP: 4 gagal (sebelum sukses) + 1 tetap terhitung.
	cs, _ := fs.CrossIPFailures(tctx(), testTenant, u.ID, c.Now().Add(-time.Hour))
	if cs.Failures != 5 {
		t.Fatalf("hitungan lintas IP = %d, want 5 (tidak direset sukses)", cs.Failures)
	}
	// Sukses dari IP lain tidak mereset pasangan ini.
	failN(t, l, c, u.ID, "198.51.100.1", 3) // sekarang 4 gagal berturut-turut
	if err := l.RegisterSuccess(tctx(), u.ID, "198.51.100.2"); err != nil {
		t.Fatal(err)
	}
	if ps, _ := fs.PairFailureState(tctx(), testTenant, u.ID, "198.51.100.1"); ps.ConsecutiveFailures != 4 {
		t.Fatalf("sukses dari IP lain mereset pasangan: %+v", ps)
	}
}

func TestIPv6PairsUse64Prefix(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	// 5 alamat berbeda dalam satu /64: satu pasangan (jeda pada gagal ke-5) dan satu "IP" bagi ambang lintas IP.
	var st State
	for i := range 5 {
		var err error
		if st, err = l.RegisterFailure(tctx(), u.ID, fmt.Sprintf("2001:db8:1:2::%x", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if !st.PairBlocked {
		t.Fatalf("5 alamat satu /64 harus satu pasangan dan terkena jeda: %+v", st)
	}
	cs, _ := fs.CrossIPFailures(tctx(), testTenant, u.ID, c.Now().Add(-time.Hour))
	if cs.DistinctIPs != 1 {
		t.Fatalf("IP berbeda = %d, want 1 (satu /64)", cs.DistinctIPs)
	}
	for _, r := range fs.rows(u.ID) {
		if r.ip != "2001:db8:1:2::/64" {
			t.Fatalf("IP tersimpan %q, want prefix /64", r.ip)
		}
	}
}

func TestUnlockIsIdempotent(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	if err := l.Unlock(tctx(), u.ID); err != nil {
		t.Fatalf("Unlock akun yang tidak terkunci: %v", err)
	}
	for i := range 50 {
		if _, err := l.RegisterFailure(tctx(), u.ID, fmt.Sprintf("198.51.100.%d", i%5+1)); err != nil {
			t.Fatal(err)
		}
	}
	if fs.status(u.ID) != store.UserLocked {
		t.Fatal("prasyarat: akun terkunci")
	}
	if err := l.Unlock(tctx(), u.ID); err != nil {
		t.Fatal(err)
	}
	if fs.status(u.ID) != store.UserActive || len(fs.rows(u.ID)) != 0 {
		t.Fatalf("setelah Unlock: status=%s baris=%d, want active tanpa catatan percobaan", fs.status(u.ID), len(fs.rows(u.ID)))
	}
	if err := l.Unlock(tctx(), u.ID); err != nil {
		t.Fatalf("Unlock kedua: %v", err)
	}
}

func TestLockoutFailsClosedWithoutTenantAndPropagatesStoreErrors(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	if _, err := l.RegisterFailure(context.Background(), u.ID, "198.51.100.1"); !errors.Is(err, ErrNoTenant) {
		t.Fatalf("tanpa tenant: err = %v, want ErrNoTenant", err)
	}
	if _, err := l.Check(context.Background(), u.ID, "198.51.100.1"); !errors.Is(err, ErrNoTenant) {
		t.Fatalf("Check tanpa tenant: err = %v", err)
	}
	if err := l.RegisterSuccess(context.Background(), u.ID, "198.51.100.1"); !errors.Is(err, ErrNoTenant) {
		t.Fatalf("RegisterSuccess tanpa tenant: err = %v", err)
	}
	if err := l.Unlock(context.Background(), u.ID); !errors.Is(err, ErrNoTenant) {
		t.Fatalf("Unlock tanpa tenant: err = %v", err)
	}
	fs.failOn["RecordLoginAttempt"] = errBoom
	if _, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1"); !errors.Is(err, errBoom) {
		t.Fatalf("galat store harus dirambatkan: %v", err)
	}
}

// TestCheckIsReadOnlyAndReportsPairAndAccountState: Check (dipakai MFA dan Authenticator sebelum memverifikasi
// kredensial) tidak menambah catatan percobaan, mengembalikan jeda setelah 5 gagal, bersih untuk pasangan lain, dan
// melaporkan akun locked.
func TestCheckIsReadOnlyAndReportsPairAndAccountState(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	if st, err := l.Check(tctx(), u.ID, "198.51.100.1"); err != nil || st != (State{}) {
		t.Fatalf("pasangan bersih: %+v err=%v", st, err)
	}
	failN(t, l, c, u.ID, "198.51.100.1", 5)
	c.Advance(-time.Second) // kembali ke saat gagal ke-5
	before := len(fs.rows(u.ID))
	for range 3 {
		st, err := l.Check(tctx(), u.ID, "198.51.100.1")
		if err != nil {
			t.Fatal(err)
		}
		if !st.PairBlocked || st.RetryAfter != 30*time.Second || st.AccountLocked {
			t.Fatalf("Check setelah 5 gagal: %+v, want jeda 30s", st)
		}
	}
	if got := len(fs.rows(u.ID)); got != before {
		t.Fatalf("Check menambah catatan percobaan: %d -> %d", before, got)
	}
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.2"); st != (State{}) {
		t.Fatalf("pasangan lain harus bersih: %+v", st)
	}
	for i := range 50 {
		if _, err := l.RegisterFailure(tctx(), u.ID, fmt.Sprintf("198.51.100.%d", i%5+10)); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := l.Check(tctx(), u.ID, "203.0.113.1"); !st.AccountLocked || st.PairBlocked {
		t.Fatalf("akun locked harus terbaca dari IP mana pun: %+v", st)
	}
	// Konversi struktural ke mfa.LockState: field dan urutan identik.
	type lockState struct {
		PairBlocked, AccountLocked bool
		RetryAfter                 time.Duration
	}
	_ = lockState(State{})
}

// Q19 (3 Nov 2026): hanya kegagalan pasangan dalam 24 jam terakhir yang dihitung.
func TestPairFailuresOlderThan24HoursAreNotCounted(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	if DefaultLockoutConfig().PairWindow != 24*time.Hour {
		t.Fatalf("PairWindow bawaan = %v, want 24 jam", DefaultLockoutConfig().PairWindow)
	}
	// 4 gagal, lalu >24 jam: tidak dihitung; gagal kelima bukan jeda.
	for range 4 {
		if _, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1"); err != nil {
			t.Fatal(err)
		}
	}
	c.Advance(24*time.Hour + time.Second)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); st != (State{}) {
		t.Fatalf("Check setelah >24 jam: %+v", st)
	}
	st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if st.PairBlocked {
		t.Fatalf("kegagalan >24 jam tidak boleh dihitung: %+v", st)
	}
	// Tepat di batas jendela (24 jam persis) masih dihitung; <24 jam dihitung.
	fs2, c2 := newFakeStore(), newClock()
	u2 := fs2.addUser("b@example.com", "x", store.UserActive)
	l2 := newTestLockout(t, fs2, c2)
	for range 4 {
		if _, err := l2.RegisterFailure(tctx(), u2.ID, "198.51.100.1"); err != nil {
			t.Fatal(err)
		}
	}
	c2.Advance(24 * time.Hour)
	st, _ = l2.RegisterFailure(tctx(), u2.ID, "198.51.100.1")
	if !st.PairBlocked || st.RetryAfter != 30*time.Second {
		t.Fatalf("4 gagal tepat 24 jam lalu + 1 baru: %+v, want jeda 30s", st)
	}
	// Jendela bergeser per kegagalan: 2 gagal lama (keluar jendela) + 3 baru = 3, bukan 5.
	fs3, c3 := newFakeStore(), newClock()
	u3 := fs3.addUser("c@example.com", "x", store.UserActive)
	l3 := newTestLockout(t, fs3, c3)
	for range 2 {
		l3.RegisterFailure(tctx(), u3.ID, "198.51.100.1") //nolint:errcheck
	}
	c3.Advance(25 * time.Hour)
	for range 3 {
		if st, _ = l3.RegisterFailure(tctx(), u3.ID, "198.51.100.1"); st.PairBlocked {
			t.Fatalf("2 lama + 3 baru tidak boleh memicu jeda: %+v", st)
		}
	}
}

// Blokir 20 gagal tidak memberi jatah baru penuh setelah 1 jam: hitungan masih 20 dalam jendela 24 jam, jadi satu
// kegagalan lagi memblokir 1 jam lagi; baru setelah semua kegagalan keluar jendela, pasangan bersih.
func TestPairBlockDoesNotGrantFreshAllowanceAfterOneHour(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	for range 20 {
		if _, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1"); err != nil {
			t.Fatal(err)
		}
	}
	c.Advance(time.Hour + time.Second)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); st.PairBlocked {
		t.Fatalf("blokir 1 jam harus berakhir: %+v", st)
	}
	if ps, _ := fs.PairFailureStateSince(tctx(), testTenant, u.ID, "198.51.100.1", c.Now().Add(-24*time.Hour)); ps.ConsecutiveFailures != 20 {
		t.Fatalf("hitungan dalam jendela = %d, want 20", ps.ConsecutiveFailures)
	}
	st, err := l.RegisterFailure(tctx(), u.ID, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.PairBlocked || st.RetryAfter != time.Hour {
		t.Fatalf("satu gagal setelah blokir: %+v, want diblokir 1 jam lagi (bukan jatah baru 20)", st)
	}
	// Setelah seluruh kegagalan keluar dari jendela 24 jam, pasangan bersih lagi.
	c.Advance(24*time.Hour + time.Second)
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); st != (State{}) {
		t.Fatalf("setelah 24 jam: %+v, want bersih", st)
	}
	// Sukses tetap mereset hitungan di dalam jendela.
	for range 20 {
		l.RegisterFailure(tctx(), u.ID, "198.51.100.1") //nolint:errcheck
	}
	c.Advance(2 * time.Hour)
	if err := l.RegisterSuccess(tctx(), u.ID, "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	c.Advance(time.Second)
	if st, _ := l.RegisterFailure(tctx(), u.ID, "198.51.100.1"); st.PairBlocked {
		t.Fatalf("sukses harus mereset hitungan: %+v", st)
	}
}

// Unlock menghapus catatan percobaan juga pada akun active yang hanya terblokir per pasangan.
func TestUnlockClearsPairBlockOnActiveAccount(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	for range 20 {
		l.RegisterFailure(tctx(), u.ID, "198.51.100.1") //nolint:errcheck
	}
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); !st.PairBlocked {
		t.Fatal("prasyarat: pasangan terblokir")
	}
	if err := l.Unlock(tctx(), u.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := l.Check(tctx(), u.ID, "198.51.100.1"); st != (State{}) {
		t.Fatalf("setelah Unlock: %+v, want bersih", st)
	}
	if fs.status(u.ID) != store.UserActive {
		t.Fatal("status tetap active")
	}
}

func TestMFAFailuresCountTowardPair(t *testing.T) {
	fs, c := newFakeStore(), newClock()
	u := fs.addUser("a@example.com", "x", store.UserActive)
	l := newTestLockout(t, fs, c)
	var st State
	for range 5 {
		var err error
		if st, err = l.RegisterMFAFailure(tctx(), u.ID, "198.51.100.1"); err != nil {
			t.Fatal(err)
		}
	}
	if !st.PairBlocked || st.RetryAfter != 30*time.Second {
		t.Fatalf("5 gagal MFA: %+v, want jeda 30s", st)
	}
	for _, r := range fs.rows(u.ID) {
		if r.kind != store.AttemptMFA {
			t.Fatalf("kind = %q, want mfa", r.kind)
		}
	}
}

func TestLockoutConfigValidation(t *testing.T) {
	if _, err := NewLockout(newFakeStore(), DefaultLockoutConfig(), nil); err != nil {
		t.Fatalf("default: %v", err)
	}
	mutations := map[string]func(*LockoutConfig){
		"ambang jeda nol":              func(c *LockoutConfig) { c.PairThreshold = 0 },
		"jeda dasar nol":               func(c *LockoutConfig) { c.BaseDelay = 0 },
		"jeda maks di bawah dasar":     func(c *LockoutConfig) { c.MaxDelay = time.Second },
		"ambang blokir <= ambang jeda": func(c *LockoutConfig) { c.PairBlockThreshold = c.PairThreshold },
		"durasi blokir nol":            func(c *LockoutConfig) { c.PairBlockDuration = 0 },
		"jendela pasangan < blokir":    func(c *LockoutConfig) { c.PairWindow = c.PairBlockDuration - time.Second },
		"ambang gagal akun nol":        func(c *LockoutConfig) { c.AccountFailures = 0 },
		"ambang IP akun nol":           func(c *LockoutConfig) { c.AccountDistinctIPs = 0 },
		"jendela akun nol":             func(c *LockoutConfig) { c.AccountWindow = 0 },
	}
	for name, mut := range mutations {
		cfg := DefaultLockoutConfig()
		mut(&cfg)
		if _, err := NewLockout(newFakeStore(), cfg, nil); err == nil {
			t.Errorf("%s: konfigurasi tidak valid diterima", name)
		}
	}
	if _, err := NewLockout(nil, DefaultLockoutConfig(), nil); err == nil {
		t.Error("store nil diterima")
	}
}

// Tidak ada pelanggaran pada antarmuka: *store.Store harus memenuhi AttemptStore (agar tes unit memakai fake dan
// produksi memakai store sungguhan).
var _ AttemptStore = (*store.Store)(nil)
