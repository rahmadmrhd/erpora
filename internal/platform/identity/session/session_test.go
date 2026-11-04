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

	"erpora/internal/platform/identity/guard"
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

func TestUpgradeReplacesRefreshTokenWithoutRevokingSession(t *testing.T) {
	suiteUpgradeReplacesRefreshTokenWithoutRevokingSession(t, newFakeWorld(t))
}

func TestUpgradeOwnershipAndPreconditions(t *testing.T) {
	suiteUpgradeOwnershipAndPreconditions(t, newFakeWorld(t))
}

func TestConcurrentUpgradeExactlyOneSucceeds(t *testing.T) {
	suiteConcurrentUpgradeExactlyOneSucceeds(t, newFakeWorld(t))
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

// racingBackend menjalankan hook tepat sebelum RotateRefreshToken: mensimulasikan Upgrade yang menggantikan token di
// antara pemuatan token dan rotasi.
type racingBackend struct {
	fakeBackend
	before func()
}

func (r *racingBackend) RotateRefreshToken(ctx context.Context, t tenancy.TenantID, usedID string, at time.Time, next store.RefreshToken) (bool, error) {
	if r.before != nil {
		f := r.before
		r.before = nil
		f()
	}
	return r.fakeBackend.RotateRefreshToken(ctx, t, usedID, at, next)
}

// Rotasi yang kalah karena token digantikan Upgrade (token lenyap) bukan reuse: ditolak seragam, sesi tidak dicabut.
func TestRotateLosingToUpgradeDoesNotRevokeSession(t *testing.T) {
	rb := &racingBackend{fakeBackend: fakeBackend{newFakeStore()}}
	w := newWorld(t, rb, t0)
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, tok := w.start(a, user)
	rb.before = func() {
		if _, _, err := a.svc.Upgrade(w.ctx, user, sess.ID, "otp"); err != nil {
			t.Errorf("Upgrade: %v", err)
		}
	}
	if _, _, err := a.svc.Rotate(w.ctx, tok.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("err = %v, want ErrInvalidRefresh", err)
	}
	if row := w.sessionRow(sess.ID); row.RevokedAt != nil || a.list.IsRevoked(sess.ID) {
		t.Fatal("sesi tidak boleh dicabut: ini bukan reuse")
	}
}

// ADR-0013 §4: batas laju per identitas sasaran (family) pada refresh. Token dikenal lebih dulu, lalu bucket family
// diperiksa SEBELUM rotasi; token yang ditolak batas laju tidak terpakai (bukan reuse).
func TestRotateRateLimitedPerFamily(t *testing.T) {
	w := newFakeWorld(t)
	limiter, err := guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 2, Every: time.Hour})
	must(t, err)
	log := slog.New(slog.NewTextHandler(w.logs, nil))
	list := session.NewRevocationList(session.RevocationConfig{Loader: w.be, Now: w.clock.Now, Log: log})
	origins, err := session.NewOriginChecker([]string{goodOrigin})
	must(t, err)
	svc, err := session.New(session.Config{Store: w.be, Revocations: list, Notifier: w.notifier, Origin: origins,
		Now: w.clock.Now, Log: log, RefreshLimiter: limiter})
	must(t, err)
	in := &instance{svc: svc, list: list}

	user := w.be.NewUser(t, w.ctx)
	_, cur := w.start(in, user)
	other := w.be.NewUser(t, w.ctx)
	_, otherTok := w.start(in, other)

	for i := 0; i < 2; i++ { // refresh normal: dalam burst tetap berhasil
		w.clock.Advance(time.Minute)
		_, next, err := svc.Rotate(w.ctx, cur.Value, goodOrigin)
		if err != nil {
			t.Fatalf("rotasi ke-%d harus berhasil: %v", i+1, err)
		}
		cur = next
	}
	w.clock.Advance(time.Minute)
	_, _, err = svc.Rotate(w.ctx, cur.Value, goodOrigin)
	retry, ok := guard.AsRateLimited(err)
	if !ok || retry <= 0 {
		t.Fatalf("melebihi batas family harus *guard.RateLimitedError dengan Retry-After > 0, dapat %v", err)
	}
	if errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("batas laju bukan ErrInvalidRefresh: %v", err)
	}
	// token yang ditolak batas laju tidak terpakai: sesi dan family tetap hidup (bukan reuse)
	if row := w.tokenRow(cur.Value); row.UsedAt != nil {
		t.Fatal("token yang ditolak batas laju tidak boleh ditandai terpakai")
	}
	if w.sessionRow(row0(t, w, cur.Value)).RevokedAt != nil {
		t.Fatal("batas laju tidak boleh mencabut sesi")
	}
	// family lain tidak terpengaruh (kunci per family)
	if _, _, err := svc.Rotate(w.ctx, otherTok.Value, goodOrigin); err != nil {
		t.Fatalf("family lain tidak boleh ikut dibatasi: %v", err)
	}
	// token tak dikenal tidak mengonsumsi bucket mana pun dan tetap ErrInvalidRefresh seragam
	if _, _, err := svc.Rotate(w.ctx, randomRaw(t), goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("token tak dikenal harus ErrInvalidRefresh, dapat %v", err)
	}
}

func row0(t *testing.T, w *world, raw string) string { t.Helper(); return w.tokenRow(raw).SessionID }

// ADR-0013 §2/§4: deteksi reuse mendahului batas laju family. Penyerang yang merotasi token curian pada laju isi ulang
// menjaga bucket family hampir kosong; pengguna sah yang memakai token LAMA harus tetap memicu pencabutan family
// (bukan 429 yang dicoba ulang klien tanpa henti).
func TestReuseDetectionPrecedesFamilyRateLimit(t *testing.T) {
	w := newFakeWorld(t)
	limiter, err := guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 2, Every: time.Hour})
	must(t, err)
	log := slog.New(slog.NewTextHandler(w.logs, nil))
	list := session.NewRevocationList(session.RevocationConfig{Loader: w.be, Now: w.clock.Now, Log: log})
	origins, err := session.NewOriginChecker([]string{goodOrigin})
	must(t, err)
	svc, err := session.New(session.Config{Store: w.be, Revocations: list, Notifier: w.notifier, Origin: origins,
		Now: w.clock.Now, Log: log, RefreshLimiter: limiter})
	must(t, err)
	in := &instance{svc: svc, list: list}

	user := w.be.NewUser(t, w.ctx)
	sess, root := w.start(in, user)
	_, t1, err := svc.Rotate(w.ctx, root.Value, goodOrigin)
	must(t, err)
	_, t2, err := svc.Rotate(w.ctx, t1.Value, goodOrigin) // bucket family kini kosong
	must(t, err)
	if _, _, err := svc.Rotate(w.ctx, t2.Value, goodOrigin); !isRateLimited(err) {
		t.Fatalf("token aktif dengan bucket kosong harus 429, dapat %v", err)
	}
	// token lama (sudah terpakai) = reuse: family dicabut walau bucket kosong
	if _, _, err := svc.Rotate(w.ctx, root.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) || isRateLimited(err) {
		t.Fatalf("reuse harus ErrInvalidRefresh (bukan 429), dapat %v", err)
	}
	if row := w.sessionRow(sess.ID); row.RevokedAt == nil || row.RevokeReason != session.ReasonRefreshReuse {
		t.Fatalf("family harus tercabut karena reuse: %+v", row)
	}
}

func isRateLimited(err error) bool { _, ok := guard.AsRateLimited(err); return ok }
