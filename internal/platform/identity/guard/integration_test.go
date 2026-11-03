//go:build integration

package guard

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

type oneTenant struct{ db database.TenantDB }

func (c oneTenant) For(_ context.Context, id tenancy.TenantID) (database.TenantDB, error) {
	if id != testTenant {
		return database.TenantDB{}, database.ErrUnknownTenant
	}
	return c.db, nil
}

func setupDB(t *testing.T) *store.Store {
	t.Helper()
	dsn := pgtest.Start(t)
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	return store.New(database.ControlDB{DB: db}, oneTenant{db: database.TenantDB{DB: db}})
}

var _ = gorm.ErrRecordNotFound

type intHarness struct {
	st *store.Store
	c  *clock
	lo *Lockout
	a  *Authenticator
}

func newIntHarness(t *testing.T) *intHarness {
	t.Helper()
	st := setupDB(t)
	c := newClock()
	lo, err := NewLockout(st, DefaultLockoutConfig(), c.Now)
	if err != nil {
		t.Fatal(err)
	}
	gate, _ := NewHashGate(2, 5*time.Second)
	target, _ := NewPrincipalLimiter(LimiterConfig{Burst: 1000, Every: time.Millisecond, Now: c.Now})
	a, err := NewAuthenticator(AuthenticatorConfig{
		Accounts: st, Lockout: lo, Gate: gate, Target: target, Argon2: testParams,
		Now: c.Now, MinFailureDuration: time.Nanosecond, Logger: slog.New(slog.DiscardHandler),
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &intHarness{st: st, c: c, lo: lo, a: a}
}

func (h *intHarness) addUser(t *testing.T, email string) store.User {
	t.Helper()
	phc, err := (&password.Hasher{Params: testParams}).Hash(goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	u, err := h.st.CreateUser(tctx(), testTenant, store.NewUser{Email: email, Name: "Uji", PasswordHash: phc})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (h *intHarness) login(email, pass, ip string) Outcome {
	out, err := h.a.Verify(tctx(), LoginInput{Email: email, Password: pass, IP: ip})
	if err != nil {
		panic(err)
	}
	return out
}

// TestIntegrationPairDelayVisibleToOtherInstance: penghitung hidup di database, jadi instance lain melihat jeda yang
// sama (berlaku lintas instance).
func TestIntegrationPairDelayVisibleToOtherInstance(t *testing.T) {
	h := newIntHarness(t)
	u := h.addUser(t, "a@example.com")
	for range 5 {
		if out := h.login("a@example.com", "salah", "198.51.100.1"); out.Kind != InvalidCredentials {
			t.Fatalf("%+v", out)
		}
	}
	other, err := NewLockout(h.st, DefaultLockoutConfig(), h.c.Now) // "instance" kedua: Lockout baru, store sama
	if err != nil {
		t.Fatal(err)
	}
	st, err := other.Check(tctx(), u.ID, "198.51.100.1")
	if err != nil || !st.PairBlocked || st.RetryAfter != 30*time.Second || st.AccountLocked {
		t.Fatalf("Check instance lain: %+v err=%v, want jeda 30s", st, err)
	}
	if st, _ := other.Check(tctx(), u.ID, "198.51.100.2"); st != (State{}) {
		t.Fatalf("IP lain harus bersih: %+v", st)
	}
	if out := h.login("a@example.com", goodPassword, "198.51.100.1"); out.Kind != RateLimited {
		t.Fatalf("sandi benar selama jeda: %+v, want RateLimited", out)
	}
	if out := h.login("a@example.com", goodPassword, "198.51.100.2"); out.Kind != Success {
		t.Fatalf("IP lain: %+v, want sukses", out)
	}
}

func TestIntegrationDoSLockoutAndAccountLockAndUnlock(t *testing.T) {
	h := newIntHarness(t)
	u := h.addUser(t, "korban@example.com")
	// IP A: 25 percobaan menunggu jeda sampai blokir 1 jam; akun tidak terkunci, IP B tetap bisa masuk.
	for range 200 {
		out := h.login("korban@example.com", "tebakan", "203.0.113.9")
		if out.Kind == RateLimited && out.RetryAfter == time.Hour {
			break
		}
		if out.Kind == RateLimited {
			h.c.Advance(out.RetryAfter)
		}
	}
	got, _ := h.st.GetUser(tctx(), testTenant, u.ID)
	if got.Status != store.UserActive {
		t.Fatalf("satu IP mengunci akun: %s", got.Status)
	}
	if out := h.login("korban@example.com", goodPassword, "198.51.100.77"); out.Kind != Success {
		t.Fatalf("IP B: %+v, want sukses tanpa jeda", out)
	}
	if err := h.a.RegisterSuccess(tctx(), u.ID, "198.51.100.77"); err != nil {
		t.Fatal(err)
	}
	// Botnet: kegagalan lintas >= 5 IP melampaui 50 dalam 1 jam mengunci akun (risiko residual yang diterima).
	h.c.Advance(2 * time.Hour) // keluar dari jendela kegagalan IP A
	for i := range 60 {
		ip := fmt.Sprintf("192.0.2.%d", i%6+1)
		out := h.login("korban@example.com", "tebakan", ip)
		for out.Kind == RateLimited {
			h.c.Advance(out.RetryAfter)
			out = h.login("korban@example.com", "tebakan", ip)
		}
		if out.LockedNow {
			break
		}
	}
	if got, _ = h.st.GetUser(tctx(), testTenant, u.ID); got.Status != store.UserLocked {
		t.Fatalf("status = %s, want locked setelah >= 50 gagal lintas >= 5 IP", got.Status)
	}
	if out := h.login("korban@example.com", goodPassword, "198.51.100.77"); out.Kind != InvalidCredentials || out.Reason != ReasonAccountLocked {
		t.Fatalf("akun terkunci + sandi benar: %+v", out)
	}
	if st, _ := h.lo.Check(tctx(), u.ID, "198.51.100.77"); !st.AccountLocked {
		t.Fatalf("Check harus melaporkan akun locked: %+v", st)
	}
	if err := h.lo.Unlock(tctx(), u.ID); err != nil {
		t.Fatal(err)
	}
	if out := h.login("korban@example.com", goodPassword, "198.51.100.77"); out.Kind != Success {
		t.Fatalf("setelah Unlock: %+v, want sukses", out)
	}
	if err := h.lo.Unlock(tctx(), u.ID); err != nil {
		t.Fatalf("Unlock kedua (idempoten): %v", err)
	}
}

// Jendela 24 jam pasangan dan Unlock pada pasangan terblokir pada akun active, dengan store sungguhan.
func TestIntegrationPairWindowAndUnlockOfBlockedPair(t *testing.T) {
	h := newIntHarness(t)
	u := h.addUser(t, "a@example.com")
	const ip = "198.51.100.1"
	for range 4 {
		if _, err := h.lo.RegisterFailure(tctx(), u.ID, ip); err != nil {
			t.Fatal(err)
		}
	}
	h.c.Advance(24*time.Hour + time.Second)
	if st, err := h.lo.RegisterFailure(tctx(), u.ID, ip); err != nil || st.PairBlocked {
		t.Fatalf("kegagalan >24 jam tidak boleh dihitung: %+v err=%v", st, err)
	}
	// 20 gagal -> blokir 1 jam; setelah 1 jam hitungan masih 20 dalam jendela: satu gagal memblokir lagi.
	for range 19 {
		h.lo.RegisterFailure(tctx(), u.ID, ip) //nolint:errcheck
	}
	if st, _ := h.lo.Check(tctx(), u.ID, ip); !st.PairBlocked || st.RetryAfter != time.Hour {
		t.Fatalf("blokir 1 jam: %+v", st)
	}
	h.c.Advance(time.Hour + time.Second)
	if st, _ := h.lo.Check(tctx(), u.ID, ip); st.PairBlocked {
		t.Fatalf("blokir berakhir: %+v", st)
	}
	if st, _ := h.lo.RegisterFailure(tctx(), u.ID, ip); !st.PairBlocked || st.RetryAfter != time.Hour {
		t.Fatalf("satu gagal setelah blokir harus memblokir lagi: %+v", st)
	}
	// Unlock pada akun active yang terblokir per pasangan menghapus blokir (store.UnlockUser selalu menghapus catatan).
	if err := h.lo.Unlock(tctx(), u.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := h.lo.Check(tctx(), u.ID, ip); st != (State{}) {
		t.Fatalf("setelah Unlock: %+v, want bersih", st)
	}
	if out := h.login("a@example.com", goodPassword, ip); out.Kind != Success {
		t.Fatalf("login setelah Unlock: %+v", out)
	}
}

// TestIntegrationDummyOperationsAreHarmless: operasi tiruan untuk email tak dikenal tidak menulis baris apa pun dan
// tidak menimbulkan galat pada store sungguhan (UUID acak bukan anggota tenant).
func TestIntegrationDummyOperationsAreHarmless(t *testing.T) {
	h := newIntHarness(t)
	for range 3 {
		if out := h.login("tidak-ada@example.com", "salah", "198.51.100.1"); out.Kind != InvalidCredentials || out.Reason != ReasonUnknownEmail {
			t.Fatalf("%+v", out)
		}
	}
	cs, err := h.st.CrossIPFailures(tctx(), testTenant, "00000000-0000-4000-8000-000000000000", h.c.Now().Add(-time.Hour))
	if err != nil || cs.Failures != 0 {
		t.Fatalf("cs=%+v err=%v", cs, err)
	}
}

// TestIntegrationLoginTimingKnownVsUnknown: dengan store sungguhan, jalur email tak dikenal tidak lebih cepat secara
// signifikan dari sandi salah pada akun nyata (jumlah dan bentuk round-trip DB sama). Latensi DB (tunnel) mendominasi,
// jadi toleransi lebih longgar daripada tes unit.
func TestIntegrationLoginTimingKnownVsUnknown(t *testing.T) {
	h := newIntHarness(t)
	var known, unknown []time.Duration
	for i := range 40 {
		email := fmt.Sprintf("k%d@example.com", i)
		h.addUser(t, email)
		ip := fmt.Sprintf("192.0.2.%d", i+1)
		start := time.Now()
		h.login(email, "salah", ip)
		k := time.Since(start)
		start = time.Now()
		h.login(fmt.Sprintf("tidak-ada%d@example.com", i), "salah", ip)
		u := time.Since(start)
		if i >= 5 { // pemanasan
			known, unknown = append(known, k), append(unknown, u)
		}
	}
	mk, mu := median(known), median(unknown)
	hi, lo := mk, mu
	if lo > hi {
		hi, lo = lo, hi
	}
	t.Logf("median dikenal=%v tak dikenal=%v rasio=%.3f", mk, mu, float64(hi)/float64(lo))
	if float64(hi)/float64(lo) > 1.25 {
		t.Fatalf("rasio median %.3f > 1,25", float64(hi)/float64(lo))
	}
}
