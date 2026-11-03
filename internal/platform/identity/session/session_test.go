package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

type fakeBackend struct{ *fakeStore }

func (b fakeBackend) NewUser(t *testing.T, ctx context.Context) string {
	tn, _ := tenancy.FromContext(ctx)
	return b.addUser(tn, store.UserActive)
}

func (b fakeBackend) SetStatus(_ *testing.T, _ context.Context, userID, status string) {
	b.setStatus(userID, status)
}

func newFakeWorld(t *testing.T) *world {
	t.Helper()
	return newWorld(t, fakeBackend{newFakeStore()}, t0)
}

// Satu fungsi uji unit per skenario suite (backend fake). Padanan integrasi ada di session_integration_test.go.

func TestRotateIssuesNewTokenAndMarksParentUsed(t *testing.T) {
	suiteRotateIssuesNewTokenAndMarksParentUsed(t, newFakeWorld(t))
}

func TestReuseOfUsedTokenRevokesFamilyAndSession(t *testing.T) {
	suiteReuseOfUsedTokenRevokesFamilyAndSession(t, newFakeWorld(t))
}

func TestConcurrentRefreshSameTokenExactlyOneSucceeds(t *testing.T) {
	suiteConcurrentRefreshSameTokenExactlyOneSucceeds(t, newFakeWorld(t))
}

func TestAbsoluteLifetime90DaysEnforced(t *testing.T) {
	suiteAbsoluteLifetime90DaysEnforced(t, newFakeWorld(t))
}

func TestSlidingExpiry14Days(t *testing.T) { suiteSlidingExpiry14Days(t, newFakeWorld(t)) }

func TestRevocationVisibleToOtherInstanceViaNotifier(t *testing.T) {
	suiteRevocationVisibleToOtherInstanceViaNotifier(t, newFakeWorld(t))
}

func TestRevocationListLoadedOnStartup(t *testing.T) {
	suiteRevocationListLoadedOnStartup(t, newFakeWorld(t))
}

func TestRevocationEntriesRetainedForAccessTokenTTLPlusLeeway(t *testing.T) {
	suiteRevocationEntriesRetainedForAccessTokenTTLPlusLeeway(t, newFakeWorld(t))
}

func TestLostNotificationBoundedAndRecoveredByReload(t *testing.T) {
	suiteLostNotificationBoundedAndRecoveredByReload(t, newFakeWorld(t))
}

func TestRefreshRejectsMissingOrCrossOriginOrigin(t *testing.T) {
	suiteRefreshRejectsMissingOrCrossOriginOrigin(t, newFakeWorld(t))
}

func TestDisabledUserSessionsRevokedImmediately(t *testing.T) {
	suiteDisabledUserSessionsRevokedImmediately(t, newFakeWorld(t))
}

func TestIDORRevokeOwnAndCrossTenant(t *testing.T) { suiteIDOR(t, newFakeWorld(t)) }

func TestMalformedTokensRejectedWithoutPanic(t *testing.T) {
	suiteMalformedTokensRejectedWithoutPanic(t, newFakeWorld(t))
}

func TestStartRules(t *testing.T) { suiteStartRules(t, newFakeWorld(t)) }

func TestRevokeIsIdempotentAndBroadcastsOnce(t *testing.T) {
	suiteRevokeIsIdempotentAndBroadcastsOnce(t, newFakeWorld(t))
}

// ---- uji khusus unit ----

func TestSubscribeGoroutineDoesNotLeakOnClose(t *testing.T) {
	w := newFakeWorld(t)
	runtime.GC()
	before := runtime.NumGoroutine()
	var cancels []func()
	for i := 0; i < 5; i++ {
		in := w.instance()
		cancels = append(cancels, in.list.Subscribe(w.ctx, w.notifier, 10*time.Millisecond))
	}
	if got := w.notifier.subscribers(session.ChannelSessionRevoked); got != 5 {
		t.Fatalf("5 langganan diharapkan, dapat %d", got)
	}
	for _, c := range cancels {
		c()
		c() // idempoten
	}
	if got := w.notifier.subscribers(session.ChannelSessionRevoked); got != 0 {
		t.Fatalf("Close harus berhenti berlangganan, sisa %d", got)
	}
	eventually(t, "goroutine pekerja berhenti", func() bool { runtime.GC(); return runtime.NumGoroutine() <= before })

	// setelah Close, sinyal tidak memicu muat ulang lagi
	calls := w.be.(fakeBackend).listCalls.Load()
	w.notifier.Publish(w.ctx, session.ChannelSessionRevoked)
	time.Sleep(50 * time.Millisecond)
	if got := w.be.(fakeBackend).listCalls.Load(); got != calls {
		t.Fatalf("tidak boleh ada muat ulang setelah Close: %d -> %d", calls, got)
	}
}

func TestSubscribeStopsWhenContextCancelled(t *testing.T) {
	w := newFakeWorld(t)
	runtime.GC()
	before := runtime.NumGoroutine()
	ctx, stop := context.WithCancel(w.ctx)
	in := w.instance()
	cancel := in.list.Subscribe(ctx, w.notifier, 5*time.Millisecond)
	stop()
	eventually(t, "goroutine berhenti saat ctx dibatalkan", func() bool { runtime.GC(); return runtime.NumGoroutine() <= before })
	cancel()
}

func TestSubscribeReloadErrorsAreLoggedNotFatal(t *testing.T) {
	buf := &syncBuf{}
	fl := &failingLoader{}
	list := session.NewRevocationList(session.RevocationConfig{Loader: fl, Log: slog.New(slog.NewTextHandler(buf, nil))})
	ctx := tenancy.WithTenant(context.Background(), tenantA)
	cancel := list.Subscribe(ctx, nil, 5*time.Millisecond)
	defer cancel()
	eventually(t, "muat ulang berulang meski galat", func() bool { return fl.calls.Load() >= 3 })
	if !strings.Contains(buf.String(), "gagal") {
		t.Fatalf("galat muat ulang harus dicatat: %q", buf.String())
	}
	if err := list.Load(ctx); err == nil {
		t.Fatal("Load harus mengembalikan galat Loader")
	}
}

func TestRevocationListBasics(t *testing.T) {
	clk := &fakeClock{t: t0}
	l := session.NewRevocationList(session.RevocationConfig{Now: clk.Now})

	if l.IsRevoked("") || l.IsRevoked("tidak-ada") {
		t.Fatal("daftar kosong")
	}
	l.Add("", t0.Add(time.Hour)) // sid kosong diabaikan
	l.Add("lampau", t0.Add(-time.Second))
	if l.Len() != 0 {
		t.Fatalf("entri kosong/lampau diabaikan, dapat %d", l.Len())
	}
	l.Add("s1", t0.Add(time.Hour))
	l.Add("s1", t0.Add(time.Minute)) // tidak pernah dipersingkat
	clk.Advance(30 * time.Minute)
	if !l.IsRevoked("s1") {
		t.Fatal("Add tidak boleh mempersingkat entri")
	}
	l.Add("s1", t0.Add(2*time.Hour)) // dapat diperpanjang
	clk.Advance(45 * time.Minute)
	if !l.IsRevoked("s1") {
		t.Fatal("entri harus dapat diperpanjang")
	}
	clk.Advance(time.Hour)
	if l.IsRevoked("s1") {
		t.Fatal("entri harus lewat")
	}
	// Add memangkas entri lama secara berkala
	l.Add("s2", clk.Now().Add(time.Hour))
	clk.Advance(2 * time.Minute)
	l.Add("s3", clk.Now().Add(time.Hour))
	if l.Len() != 2 {
		t.Fatalf("s1 lewat harus terpangkas oleh Add berkala, Len=%d", l.Len())
	}

	// Load tanpa tenant atau tanpa loader
	if err := l.Load(context.Background()); err == nil {
		t.Fatal("Load tanpa Loader harus galat")
	}
	l2 := session.NewRevocationList(session.RevocationConfig{Loader: newFakeStore()})
	if err := l2.Load(context.Background()); !errors.Is(err, session.ErrNoTenant) {
		t.Fatalf("Load tanpa tenant harus ErrNoTenant: %v", err)
	}
}

func TestRevocationListConcurrentUse(t *testing.T) {
	l := session.NewRevocationList(session.RevocationConfig{})
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 500; j++ {
				sid := fmt.Sprintf("s%d", j%50)
				l.Add(sid, time.Now().Add(time.Minute))
				_ = l.IsRevoked(sid)
				if j%100 == 0 {
					l.Prune()
				}
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

func TestOriginChecker(t *testing.T) {
	chk, err := session.NewOriginChecker([]string{"https://App.Example.test", "http://localhost:5173", "https://erp.example.test:443"})
	if err != nil {
		t.Fatal(err)
	}
	for origin, want := range map[string]bool{
		"https://app.example.test": true, "HTTPS://APP.EXAMPLE.TEST": true, "http://localhost:5173": true,
		"https://erp.example.test": true, "https://erp.example.test:443": true,
		"": false, "null": false, "https://app.example.test/": false, "https://app.example.test/x": false,
		"http://app.example.test": false, "https://localhost:5173": false, "http://localhost": false,
		"https://evil.test": false, "https://app.example.test.evil.test": false, "https://app.example.test?x=1": false,
		"https://u:p@app.example.test": false, "https://app.example.test#f": false, "ftp://app.example.test": false,
		"javascript:alert(1)": false, "https://": false, strings.Repeat("a", 600): false, "https://app.example.test\x00": false,
	} {
		if got := chk(origin); got != want {
			t.Errorf("origin %q: dapat %v, harapan %v", trunc(origin), got, want)
		}
	}
	for _, bad := range []string{"", "null", "*", "app.example.test", "https://a.test/path", "https://a.test/", "ftp://a.test"} {
		if _, err := session.NewOriginChecker([]string{bad}); err == nil {
			t.Errorf("entri %q harus ditolak saat konfigurasi", bad)
		}
	}
	none, err := session.NewOriginChecker(nil)
	if err != nil || none(goodOrigin) {
		t.Fatal("daftar kosong menolak semua origin")
	}
}

func TestNewRequiresDependencies(t *testing.T) {
	chk, _ := session.NewOriginChecker([]string{goodOrigin})
	list := session.NewRevocationList(session.RevocationConfig{})
	st := newFakeStore()
	for name, cfg := range map[string]session.Config{
		"tanpa store":       {Revocations: list, Origin: chk},
		"tanpa daftar":      {Store: st, Origin: chk},
		"tanpa origin":      {Store: st, Revocations: list},
		"semua kosong":      {},
		"origin diberi nil": {Store: st, Revocations: list, Origin: nil},
	} {
		if _, err := session.New(cfg); err == nil {
			t.Errorf("%s: New harus galat (gagal tertutup)", name)
		}
	}
}

func TestRefreshTokenNeverFormatsItsValue(t *testing.T) {
	const secret = "SECRETSECRETSECRETSECRETSECRETSECRETSECRET0"
	rt := session.RefreshToken{Value: secret, FamilyID: "fam", ExpiresAt: t0}
	outs := []string{
		fmt.Sprintf("%v|%+v|%#v|%s|%q|%x|%d", rt, rt, rt, rt, rt, rt, rt),
		fmt.Sprint(rt), fmt.Sprintf("%v", &rt), fmt.Sprintf("%+v", []session.RefreshToken{rt}),
		fmt.Sprintf("%+v", struct{ T session.RefreshToken }{rt}),
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "token", rt, slog.Any("t2", &rt))
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "token", rt)
	outs = append(outs, buf.String())
	if b, err := json.Marshal(map[string]any{"s": rt.String()}); err == nil {
		outs = append(outs, string(b))
	}
	for _, o := range outs {
		if strings.Contains(o, secret) {
			t.Fatalf("nilai token bocor: %s", o)
		}
	}
}

func TestRotateErrorsNeverContainRefreshToken(t *testing.T) {
	w := newFakeWorld(t)
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	_, rt := w.start(in, user)
	w.clock.Advance(15 * 24 * time.Hour) // kedaluwarsa
	_, _, err := in.svc.Rotate(w.ctx, rt.Value, goodOrigin)
	if !errors.Is(err, session.ErrInvalidRefresh) || strings.Contains(err.Error(), rt.Value) {
		t.Fatalf("galat generik tanpa token: %v", err)
	}
	if strings.Contains(w.logs.String(), rt.Value) {
		t.Fatal("token tidak boleh masuk log")
	}
}

// Semua kegagalan token tidak dapat dibedakan klien: galat yang sama persis untuk tak dikenal, kedaluwarsa,
// terpakai, dan sesi tercabut.
func TestAllTokenFailuresAreIndistinguishable(t *testing.T) {
	w := newFakeWorld(t)
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)

	_, expired := w.start(in, user)
	_, used := w.start(in, user)
	_, _, err := in.svc.Rotate(w.ctx, used.Value, goodOrigin)
	must(t, err)
	revokedSess, revoked := w.start(in, user)
	must(t, in.svc.Revoke(w.ctx, revokedSess.ID, session.ReasonLogout))

	var msgs []string
	for _, raw := range []string{randomRaw(t), used.Value, revoked.Value} {
		_, _, err := in.svc.Rotate(w.ctx, raw, goodOrigin)
		if !errors.Is(err, session.ErrInvalidRefresh) {
			t.Fatalf("harus ErrInvalidRefresh: %v", err)
		}
		msgs = append(msgs, err.Error())
	}
	w.clock.Advance(15 * 24 * time.Hour)
	_, _, err = in.svc.Rotate(w.ctx, expired.Value, goodOrigin)
	msgs = append(msgs, err.Error())
	for _, m := range msgs {
		if m != msgs[0] {
			t.Fatalf("pesan galat berbeda antar penyebab: %q vs %q", m, msgs[0])
		}
	}
}
