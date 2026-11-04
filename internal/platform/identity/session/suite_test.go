package session_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

const goodOrigin = "https://app.example.test"

// backend adalah persistensi di bawah suite: fake (unit) atau *store.Store + PostgreSQL (integrasi).
type backend interface {
	session.Store
	// NewUser membuat pengguna aktif di tenant context dan mengembalikan id-nya.
	NewUser(t *testing.T, ctx context.Context) string
	// SetStatus mengubah status pengguna (store.UserActive/UserDisabled/UserLocked).
	SetStatus(t *testing.T, ctx context.Context, userID, status string)
}

// world adalah satu lingkungan uji: backend, jam, notifier bersama, dan konteks dua tenant.
type world struct {
	t        *testing.T
	be       backend
	ctx      context.Context // tenant A
	otherCtx context.Context // tenant B
	clock    *fakeClock
	notifier *fakeNotifier
	logs     *syncBuf
}

func newWorld(t *testing.T, be backend, start time.Time) *world {
	t.Helper()
	return &world{
		t: t, be: be,
		ctx:      tenancy.WithTenant(context.Background(), tenantA),
		otherCtx: tenancy.WithTenant(context.Background(), tenantB),
		clock:    &fakeClock{t: start.UTC().Truncate(time.Microsecond)},
		notifier: newFakeNotifier(),
		logs:     &syncBuf{},
	}
}

// instance adalah satu proses serve: daftar tercabut sendiri dan Service di backend, jam, dan notifier bersama.
type instance struct {
	svc  *session.Service
	list *session.RevocationList
}

func (w *world) instance() *instance {
	w.t.Helper()
	log := slog.New(slog.NewTextHandler(w.logs, nil))
	list := session.NewRevocationList(session.RevocationConfig{Loader: w.be, Now: w.clock.Now, Log: log})
	origins, err := session.NewOriginChecker([]string{goodOrigin})
	must(w.t, err)
	// suite menguji logika rotasi, bukan batas laju: bucket longgar (batas laju: TestRotateRateLimitedPerFamily)
	lim, err := guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 1000, Every: time.Millisecond})
	must(w.t, err)
	svc, err := session.New(session.Config{Store: w.be, Revocations: list, Notifier: w.notifier, Origin: origins, Now: w.clock.Now, Log: log, RefreshLimiter: lim})
	must(w.t, err)
	return &instance{svc: svc, list: list}
}

// subscribed membuat instance yang berlangganan notifier bersama (interval muat ulang berkala = every).
func (w *world) subscribed(every time.Duration) *instance {
	w.t.Helper()
	in := w.instance()
	cancel := in.list.Subscribe(w.ctx, w.notifier, every)
	w.t.Cleanup(cancel)
	return in
}

func (w *world) start(in *instance, userID string) (session.Session, session.RefreshToken) {
	w.t.Helper()
	s, rt, err := in.svc.Start(w.ctx, session.StartInput{UserID: userID, ClientKind: "web", UserAgent: "uji", IP: "203.0.113.7"})
	must(w.t, err)
	return s, rt
}

// hashOf menghitung hash yang disimpan untuk token mentah (SHA-256 dari 32 byte hasil dekode base64url).
func hashOf(t testing.TB, raw string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(b) != 32 {
		t.Fatalf("token mentah bukan 32 byte base64url: %v", err)
	}
	sum := sha256.Sum256(b)
	return sum[:]
}

func (w *world) tokenRow(raw string) store.RefreshToken {
	w.t.Helper()
	rt, err := w.be.FindRefreshTokenByHash(w.ctx, tenantA, hashOf(w.t, raw))
	must(w.t, err)
	return rt
}

func (w *world) sessionRow(sid string) store.Session {
	w.t.Helper()
	s, err := w.be.GetSession(w.ctx, tenantA, sid)
	must(w.t, err)
	return s
}

// eventually mengulang cond sampai benar atau batas waktu (untuk pekerjaan asinkron Subscribe).
func eventually(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("batas waktu menunggu: %s", what)
}

func randomRaw(t testing.TB) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ---- suite (dijalankan terhadap fake dan terhadap PostgreSQL sungguhan) ----

func suiteRotateIssuesNewTokenAndMarksParentUsed(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, root := w.start(in, user)

	if len(root.Value) != 43 {
		t.Fatalf("token mentah harus 43 karakter base64url, dapat %d", len(root.Value))
	}
	row := w.tokenRow(root.Value)
	if row.SessionID != sess.ID || row.FamilyID != root.FamilyID || row.ParentID != "" || row.UsedAt != nil {
		t.Fatalf("akar family salah: %+v", row)
	}
	if string(row.TokenHash) == root.Value || len(row.TokenHash) != 32 {
		t.Fatal("yang disimpan harus hash 32 byte, bukan token mentah")
	}
	if !row.ExpiresAt.Equal(w.clock.Now().Add(session.RefreshSlidingTTL)) {
		t.Fatalf("umur token akar harus 14 hari, dapat %v", row.ExpiresAt)
	}
	if !sess.AbsoluteExpiresAt.Equal(w.clock.Now().Add(session.SessionAbsoluteTTL)) {
		t.Fatalf("umur absolut sesi harus 90 hari, dapat %v", sess.AbsoluteExpiresAt)
	}

	w.clock.Advance(time.Minute)
	got, next, err := in.svc.Rotate(w.ctx, root.Value, goodOrigin)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if got.ID != sess.ID || got.UserID != user {
		t.Fatalf("sesi salah: %+v", got)
	}
	if next.Value == root.Value || len(next.Value) != 43 || next.FamilyID != root.FamilyID {
		t.Fatalf("token pengganti harus baru dan satu family: %+v", next)
	}
	parent := w.tokenRow(root.Value)
	if parent.UsedAt == nil {
		t.Fatal("induk harus ditandai terpakai")
	}
	child := w.tokenRow(next.Value)
	if child.ParentID != parent.ID || child.FamilyID != parent.FamilyID || child.SessionID != sess.ID || child.UsedAt != nil {
		t.Fatalf("anak salah: %+v (induk %+v)", child, parent)
	}
	// rantai berlanjut
	if _, _, err := in.svc.Rotate(w.ctx, next.Value, goodOrigin); err != nil {
		t.Fatalf("Rotate anak: %v", err)
	}
	if in.list.IsRevoked(sess.ID) {
		t.Fatal("rotasi normal tidak mencabut sesi")
	}
}

func suiteReuseOfUsedTokenRevokesFamilyAndSession(t *testing.T, w *world) {
	a := w.instance()
	b := w.subscribed(-1)
	user := w.be.NewUser(t, w.ctx)
	sess, root := w.start(a, user)
	_, second, err := a.svc.Rotate(w.ctx, root.Value, goodOrigin)
	must(t, err)

	before := w.notifier.count(session.ChannelSessionRevoked)
	if _, _, err := a.svc.Rotate(w.ctx, root.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("reuse harus ErrInvalidRefresh, dapat %v", err)
	}
	row := w.sessionRow(sess.ID)
	if row.RevokedAt == nil || row.RevokeReason != session.ReasonRefreshReuse {
		t.Fatalf("sesi harus tercabut karena reuse: %+v", row)
	}
	if !a.list.IsRevoked(sess.ID) {
		t.Fatal("daftar tercabut lokal harus memuat sesi")
	}
	if w.notifier.count(session.ChannelSessionRevoked) != before+1 {
		t.Fatal("pencabutan harus disiarkan sekali")
	}
	eventually(t, "instance lain menerima sinyal reuse", func() bool { return b.list.IsRevoked(sess.ID) })
	// token pengganti (milik pemenang/korban) ikut mati
	if _, _, err := a.svc.Rotate(w.ctx, second.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("token family harus mati: %v", err)
	}
	logs := w.logs.String()
	if !strings.Contains(logs, "identity.refresh_reuse") || !strings.Contains(logs, sess.ID) {
		t.Fatalf("event keamanan harus tercatat: %q", logs)
	}
	for _, raw := range []string{root.Value, second.Value} {
		if strings.Contains(logs, raw) {
			t.Fatal("token mentah tidak boleh masuk log")
		}
	}
}

func suiteConcurrentRefreshSameTokenExactlyOneSucceeds(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, root := w.start(in, user)

	const n = 20
	type result struct {
		rt  session.RefreshToken
		err error
	}
	results := make([]result, n)
	var wg sync.WaitGroup
	startGate := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-startGate
			_, rt, err := in.svc.Rotate(w.ctx, root.Value, goodOrigin)
			results[i] = result{rt, err}
		}(i)
	}
	close(startGate)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("deadlock: rotasi serentak tidak selesai dalam 60 detik")
	}

	wins := 0
	var winner session.RefreshToken
	for i, r := range results {
		switch {
		case r.err == nil:
			wins++
			winner = r.rt
		case !errors.Is(r.err, session.ErrInvalidRefresh):
			t.Fatalf("goroutine %d: yang kalah harus ErrInvalidRefresh, dapat %v", i, r.err)
		}
	}
	if wins != 1 {
		t.Fatalf("tepat satu rotasi harus sukses, dapat %d", wins)
	}
	row := w.sessionRow(sess.ID)
	if row.RevokedAt == nil || row.RevokeReason != session.ReasonRefreshReuse {
		t.Fatalf("family harus tercabut untuk yang kalah: %+v", row)
	}
	if !in.list.IsRevoked(sess.ID) {
		t.Fatal("sesi harus ada di daftar tercabut")
	}
	// token pemenang tidak dapat dipakai lagi (family tercabut) dan hanya satu anak yang pernah ada
	if _, _, err := in.svc.Rotate(w.ctx, winner.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("token pemenang harus mati: %v", err)
	}
	if w.tokenRow(root.Value).UsedAt == nil {
		t.Fatal("akar harus terpakai")
	}
}

func suiteAbsoluteLifetime90DaysEnforced(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	start := w.clock.Now()
	sess, rt := w.start(in, user)
	abs := start.Add(90 * 24 * time.Hour)
	if !sess.AbsoluteExpiresAt.Equal(abs) {
		t.Fatalf("batas absolut = %v, harapan %v", sess.AbsoluteExpiresAt, abs)
	}
	// rotasi tiap 13 hari tetap dalam jendela sliding 14 hari
	for day := 13; day <= 78; day += 13 {
		w.clock.Set(start.Add(time.Duration(day) * 24 * time.Hour))
		_, next, err := in.svc.Rotate(w.ctx, rt.Value, goodOrigin)
		if err != nil {
			t.Fatalf("hari %d: %v", day, err)
		}
		if next.ExpiresAt.After(abs) {
			t.Fatalf("hari %d: token melewati batas absolut: %v", day, next.ExpiresAt)
		}
		rt = next
	}
	// hari 89: sliding akan melewati hari 90, jadi umur token dipotong di batas absolut
	w.clock.Set(start.Add(89 * 24 * time.Hour))
	_, next, err := in.svc.Rotate(w.ctx, rt.Value, goodOrigin)
	if err != nil {
		t.Fatalf("hari 89: %v", err)
	}
	if !next.ExpiresAt.Equal(abs) {
		t.Fatalf("token terakhir harus dipotong di batas absolut %v, dapat %v", abs, next.ExpiresAt)
	}
	// tepat pada batas absolut: ditolak, dan token tidak terbakar secara tidak sengaja
	w.clock.Set(abs)
	if _, _, err := in.svc.Rotate(w.ctx, next.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("pada batas absolut harus ditolak: %v", err)
	}
	w.clock.Set(abs.Add(24 * time.Hour))
	if _, _, err := in.svc.Rotate(w.ctx, next.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("setelah batas absolut harus ditolak: %v", err)
	}
}

func suiteSlidingExpiry14Days(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)

	// token yang tidak dipakai 14 hari mati
	start := w.clock.Now()
	_, rt1 := w.start(in, user)
	w.clock.Set(start.Add(14*24*time.Hour - time.Second))
	_, next, err := in.svc.Rotate(w.ctx, rt1.Value, goodOrigin)
	if err != nil {
		t.Fatalf("tepat sebelum 14 hari harus sah: %v", err)
	}
	// sliding: umur token baru dihitung dari saat rotasi
	if want := w.clock.Now().Add(14 * 24 * time.Hour); !next.ExpiresAt.Equal(want) {
		t.Fatalf("token baru harus berumur 14 hari dari rotasi: %v != %v", next.ExpiresAt, want)
	}
	w.clock.Set(next.ExpiresAt)
	if _, _, err := in.svc.Rotate(w.ctx, next.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("pada saat kedaluwarsa harus ditolak: %v", err)
	}

	// sesi lain: lewat 14 hari tanpa rotasi
	w.clock.Set(start)
	_, rt2 := w.start(in, user)
	w.clock.Advance(14*24*time.Hour + time.Second)
	if _, _, err := in.svc.Rotate(w.ctx, rt2.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("lewat 14 hari harus ditolak: %v", err)
	}
}

func suiteRevocationVisibleToOtherInstanceViaNotifier(t *testing.T, w *world) {
	a := w.instance()
	b := w.subscribed(-1) // tanpa muat ulang berkala: hanya notifier
	user := w.be.NewUser(t, w.ctx)
	sess, _ := w.start(a, user)
	other, _ := w.start(a, user)

	if b.list.IsRevoked(sess.ID) {
		t.Fatal("belum dicabut")
	}
	must(t, a.svc.Revoke(w.ctx, sess.ID, session.ReasonLogout))
	if !a.list.IsRevoked(sess.ID) {
		t.Fatal("instance pencabut harus langsung menolak")
	}
	eventually(t, "instance lain menolak sesi tercabut", func() bool { return b.list.IsRevoked(sess.ID) })
	if b.list.IsRevoked(other.ID) {
		t.Fatal("sesi lain tidak boleh ikut tercabut")
	}
}

func suiteRevocationListLoadedOnStartup(t *testing.T, w *world) {
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	revoked, _ := w.start(a, user)
	alive, _ := w.start(a, user)
	must(t, a.svc.Revoke(w.ctx, revoked.ID, session.ReasonLogout))

	c := w.instance() // instance baru (restart): daftar kosong sampai Load
	if c.list.IsRevoked(revoked.ID) {
		t.Fatal("daftar baru harus kosong sebelum Load")
	}
	must(t, c.list.Load(w.ctx))
	if !c.list.IsRevoked(revoked.ID) {
		t.Fatal("Load harus memuat sesi tercabut dari DB")
	}
	if c.list.IsRevoked(alive.ID) {
		t.Fatal("sesi aktif tidak boleh tercabut")
	}
}

func suiteRevocationEntriesRetainedForAccessTokenTTLPlusLeeway(t *testing.T, w *world) {
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, _ := w.start(a, user)
	revokedAt := w.clock.Now()
	must(t, a.svc.Revoke(w.ctx, sess.ID, session.ReasonLogout))

	const retention = 10*time.Minute + 30*time.Second
	if session.RevocationRetention != retention {
		t.Fatalf("retensi harus 10 menit 30 detik, dapat %v", session.RevocationRetention)
	}
	w.clock.Set(revokedAt.Add(retention - time.Nanosecond))
	a.list.Prune()
	if !a.list.IsRevoked(sess.ID) {
		t.Fatal("entri tidak boleh dibuang sebelum 10 menit 30 detik")
	}
	w.clock.Set(revokedAt.Add(retention))
	a.list.Prune()
	if !a.list.IsRevoked(sess.ID) {
		t.Fatal("entri masih berlaku tepat pada batas retensi (inklusif)")
	}
	// instance yang start pada menit ke-10 masih memuat entri itu dari DB
	w.clock.Set(revokedAt.Add(10 * time.Minute))
	c := w.instance()
	must(t, c.list.Load(w.ctx))
	if !c.list.IsRevoked(sess.ID) {
		t.Fatal("Load pada menit ke-10 harus memuat entri")
	}
	w.clock.Set(revokedAt.Add(retention + time.Nanosecond))
	if a.list.IsRevoked(sess.ID) || c.list.IsRevoked(sess.ID) {
		t.Fatal("setelah retensi entri tidak lagi berlaku (batas atas keberlakuan)")
	}
	a.list.Prune()
	if a.list.Len() != 0 {
		t.Fatalf("Prune harus membuang entri yang lewat retensi, sisa %d", a.list.Len())
	}
	// instance yang start setelah retensi tidak memuatnya
	d := w.instance()
	must(t, d.list.Load(w.ctx))
	if d.list.Len() != 0 {
		t.Fatal("Load setelah retensi tidak boleh memuat entri")
	}
}

func suiteLostNotificationBoundedAndRecoveredByReload(t *testing.T, w *world) {
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, _ := w.start(a, user)

	// B berlangganan hanya lewat muat ulang berkala; notifikasi tidak pernah diterimanya (notifier = nil)
	b := w.instance()
	cancel := b.list.Subscribe(w.ctx, nil, 10*time.Millisecond)
	defer cancel()
	revokedAt := w.clock.Now()
	must(t, a.svc.Revoke(w.ctx, sess.ID, session.ReasonLogout))
	eventually(t, "muat ulang berkala menutup celah notifikasi hilang", func() bool { return b.list.IsRevoked(sess.ID) })

	// entri hasil muat ulang tetap dibatasi retensi
	w.clock.Set(revokedAt.Add(session.RevocationRetention + time.Nanosecond))
	if b.list.IsRevoked(sess.ID) {
		t.Fatal("entri tidak boleh berlaku lebih lama dari retensi")
	}
}

func suiteRefreshRejectsMissingOrCrossOriginOrigin(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, rt := w.start(in, user)
	for _, origin := range []string{
		"", "null", "https://evil.example.test", "http://app.example.test", "https://app.example.test.evil.test",
		"https://app.example.test/", "https://app.example.test:8443", "app.example.test", "https://user@app.example.test",
	} {
		if _, _, err := in.svc.Rotate(w.ctx, rt.Value, origin); !errors.Is(err, session.ErrOriginNotAllowed) {
			t.Fatalf("origin %q harus ditolak, dapat %v", origin, err)
		}
	}
	// permintaan lintas-origin tidak membakar token dan tidak mencabut apa pun
	if w.tokenRow(rt.Value).UsedAt != nil || w.sessionRow(sess.ID).RevokedAt != nil {
		t.Fatal("penolakan Origin tidak boleh menyentuh token atau sesi")
	}
	if _, _, err := in.svc.Rotate(w.ctx, rt.Value, goodOrigin); err != nil {
		t.Fatalf("origin sah harus diterima: %v", err)
	}
}

func suiteDisabledUserSessionsRevokedImmediately(t *testing.T, w *world) {
	a := w.instance()
	b := w.subscribed(-1)

	// hook OnUserDisabled mencabut semua sesi segera
	user := w.be.NewUser(t, w.ctx)
	s1, _ := w.start(a, user)
	s2, _ := w.start(a, user)
	w.be.SetStatus(t, w.ctx, user, store.UserDisabled)
	must(t, a.svc.OnUserDisabled(w.ctx, user))
	for _, s := range []session.Session{s1, s2} {
		row := w.sessionRow(s.ID)
		if row.RevokedAt == nil || row.RevokeReason != session.ReasonUserDisabled {
			t.Fatalf("sesi %s harus tercabut: %+v", s.ID, row)
		}
		if !a.list.IsRevoked(s.ID) {
			t.Fatalf("sesi %s harus langsung tercabut di daftar lokal", s.ID)
		}
		sid := s.ID
		eventually(t, "instance lain menolak sesi pengguna nonaktif", func() bool { return b.list.IsRevoked(sid) })
	}

	// pengaman: hook terlewat, Rotate menolak pengguna nonaktif (disabled) dan mencabut semua sesinya
	{
		u := w.be.NewUser(t, w.ctx)
		x1, rt1 := w.start(a, u)
		x2, _ := w.start(a, u)
		w.be.SetStatus(t, w.ctx, u, store.UserDisabled)
		if _, _, err := a.svc.Rotate(w.ctx, rt1.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
			t.Fatalf("pengguna disabled: Rotate harus ErrInvalidRefresh, dapat %v", err)
		}
		for _, s := range []session.Session{x1, x2} {
			if w.sessionRow(s.ID).RevokedAt == nil || !a.list.IsRevoked(s.ID) {
				t.Fatalf("pengguna disabled: sesi %s harus tercabut", s.ID)
			}
		}
		// dan Start untuk pengguna itu ditolak
		if _, _, err := a.svc.Start(w.ctx, session.StartInput{UserID: u}); !errors.Is(err, session.ErrUserNotActive) {
			t.Fatalf("pengguna disabled: Start harus ErrUserNotActive, dapat %v", err)
		}
	}

	// Q30 butir 4: locked hanya menolak login baru; sesi yang ada tetap valid dan dapat di-refresh, TIDAK dicabut.
	{
		u := w.be.NewUser(t, w.ctx)
		x1, rt1 := w.start(a, u)
		x2, _ := w.start(a, u)
		w.be.SetStatus(t, w.ctx, u, store.UserLocked)
		if _, _, err := a.svc.Rotate(w.ctx, rt1.Value, goodOrigin); err != nil {
			t.Fatalf("pengguna locked: Rotate harus berhasil (sesi tidak dicabut), dapat %v", err)
		}
		for _, s := range []session.Session{x1, x2} {
			if w.sessionRow(s.ID).RevokedAt != nil || a.list.IsRevoked(s.ID) {
				t.Fatalf("pengguna locked: sesi %s tidak boleh tercabut", s.ID)
			}
		}
		if _, _, err := a.svc.Start(w.ctx, session.StartInput{UserID: u}); !errors.Is(err, session.ErrUserNotActive) {
			t.Fatalf("pengguna locked: Start (login baru) harus ErrUserNotActive, dapat %v", err)
		}
	}

	// RevokeAllForUser pada pengguna aktif
	u := w.be.NewUser(t, w.ctx)
	y1, _ := w.start(a, u)
	y2, _ := w.start(a, u)
	keep := w.be.NewUser(t, w.ctx)
	k1, _ := w.start(a, keep)
	must(t, a.svc.RevokeAllForUser(w.ctx, u))
	if !a.list.IsRevoked(y1.ID) || !a.list.IsRevoked(y2.ID) || a.list.IsRevoked(k1.ID) {
		t.Fatal("RevokeAllForUser hanya mencabut sesi pengguna itu")
	}
}

func suiteIDOR(t *testing.T, w *world) {
	in := w.instance()
	ua, ub := w.be.NewUser(t, w.ctx), w.be.NewUser(t, w.ctx)
	sa, rta := w.start(in, ua)
	sb, _ := w.start(in, ub)

	// pengguna A tidak dapat mencabut sesi B lewat RevokeOwn (404 generik) dan tidak ada efek
	for _, sid := range []string{sb.ID, uuid.NewString(), "bukan-uuid", "", strings.Repeat("x", 5000)} {
		if err := in.svc.RevokeOwn(w.ctx, ua, sid); !errors.Is(err, session.ErrSessionNotFound) {
			t.Fatalf("RevokeOwn(A, %q) harus ErrSessionNotFound, dapat %v", sid, err)
		}
	}
	if w.sessionRow(sb.ID).RevokedAt != nil || in.list.IsRevoked(sb.ID) {
		t.Fatal("sesi B tidak boleh tercabut oleh A")
	}
	if err := in.svc.RevokeOwn(w.ctx, "", sb.ID); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("userID kosong harus ditolak: %v", err)
	}
	must(t, in.svc.RevokeOwn(w.ctx, ub, sb.ID))
	if !in.list.IsRevoked(sb.ID) {
		t.Fatal("pemilik dapat mencabut sesinya sendiri")
	}
	if w.sessionRow(sb.ID).RevokeReason != session.ReasonUserRevoked {
		t.Fatal("alasan harus user_revoked")
	}

	// lintas tenant: token dan sesi tenant A tidak terlihat dari konteks tenant B
	if _, _, err := in.svc.Rotate(w.otherCtx, rta.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("Rotate lintas tenant harus ErrInvalidRefresh, dapat %v", err)
	}
	if err := in.svc.Revoke(w.otherCtx, sa.ID, session.ReasonLogout); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("Revoke lintas tenant harus ErrSessionNotFound, dapat %v", err)
	}
	if err := in.svc.RevokeOwn(w.otherCtx, ua, sa.ID); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("RevokeOwn lintas tenant harus ErrSessionNotFound, dapat %v", err)
	}
	if w.tokenRow(rta.Value).UsedAt != nil || w.sessionRow(sa.ID).RevokedAt != nil {
		t.Fatal("akses lintas tenant tidak boleh menyentuh token atau sesi")
	}
	// tenant yang benar tetap berfungsi
	if _, _, err := in.svc.Rotate(w.ctx, rta.Value, goodOrigin); err != nil {
		t.Fatalf("Rotate sah: %v", err)
	}
}

func suiteMalformedTokensRejectedWithoutPanic(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	_, rt := w.start(in, user)
	valid := rt.Value

	bad := []string{
		"", "\x00", strings.Repeat("A", 43) + "\x00", strings.Repeat("\x00", 43), strings.Repeat("A", 10000),
		valid[:42], valid + "A", valid[:42] + "+", valid[:42] + "/", valid[:21] + "\n" + valid[22:], " " + valid[1:],
		valid[:42] + "=", strings.Repeat("A", 42) + "B", // sisa bit tak nol (tidak kanonik)
		"' OR 1=1 --" + strings.Repeat("x", 32), strings.Repeat("é", 43), "../../etc/passwd",
	}
	for _, in2 := range bad {
		_, _, err := in.svc.Rotate(w.ctx, in2, goodOrigin)
		if !errors.Is(err, session.ErrInvalidRefresh) {
			t.Fatalf("token %q harus ErrInvalidRefresh, dapat %v", trunc(in2), err)
		}
		if len(in2) > 8 && strings.Contains(err.Error(), in2) {
			t.Fatal("galat tidak boleh memuat token masukan")
		}
	}
	// berbentuk benar tetapi tidak dikenal: 401 generik yang sama, tanpa efek apa pun
	unknown := randomRaw(t)
	if _, _, err := in.svc.Rotate(w.ctx, unknown, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("token tak dikenal harus ErrInvalidRefresh: %v", err)
	}
	if strings.Contains(w.logs.String(), unknown) {
		t.Fatal("token masukan tidak boleh masuk log")
	}
	// token sah tetap utuh
	if _, _, err := in.svc.Rotate(w.ctx, valid, goodOrigin); err != nil {
		t.Fatalf("token sah harus tetap berfungsi: %v", err)
	}
}

func trunc(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func suiteStartRules(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)

	long := strings.Repeat("é", 700) // 1400 byte, UTF-8 ganda-byte
	sess, rt, err := in.svc.Start(w.ctx, session.StartInput{UserID: user, UserAgent: long, IP: strings.Repeat("1", 100)})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(sess.UserAgent) > 512 || !strings.HasPrefix(long, sess.UserAgent) || sess.UserAgent == "" {
		t.Fatalf("user agent harus dipotong di batas rune: %d byte", len(sess.UserAgent))
	}
	if sess.IP != "" {
		t.Fatalf("IP terlalu panjang harus dikosongkan: %q", sess.IP)
	}
	if sess.ClientKind != "web" || sess.AuthMethod != "password" || len(sess.AMR) != 1 || sess.AMR[0] != "pwd" {
		t.Fatalf("nilai bawaan salah: %+v", sess)
	}
	if got := w.sessionRow(sess.ID); got.UserID != user || got.RevokedAt != nil {
		t.Fatalf("sesi tersimpan salah: %+v", got)
	}
	if len(rt.Value) != 43 || rt.FamilyID == "" {
		t.Fatalf("token salah: %+v", rt)
	}

	// token acak: dua Start tidak pernah menghasilkan token sama
	_, rt2 := w.start(in, user)
	if rt2.Value == rt.Value {
		t.Fatal("token harus acak")
	}

	// ditolak
	if _, _, err := in.svc.Start(w.ctx, session.StartInput{UserID: uuid.NewString()}); !errors.Is(err, session.ErrUserNotActive) {
		t.Fatalf("pengguna tak dikenal harus ErrUserNotActive: %v", err)
	}
	if _, _, err := in.svc.Start(w.ctx, session.StartInput{UserID: "bukan-uuid"}); !errors.Is(err, session.ErrUserNotActive) {
		t.Fatalf("id bukan UUID harus ErrUserNotActive: %v", err)
	}
	if _, _, err := in.svc.Start(w.otherCtx, session.StartInput{UserID: user}); !errors.Is(err, session.ErrUserNotActive) {
		t.Fatalf("bukan anggota tenant harus ErrUserNotActive: %v", err)
	}
	if _, _, err := in.svc.Start(w.ctx, session.StartInput{UserID: user, ClientKind: "tv"}); !errors.Is(err, session.ErrInvalidInput) {
		t.Fatalf("client kind tak sah harus ErrInvalidInput: %v", err)
	}
	if _, _, err := in.svc.Start(context.Background(), session.StartInput{UserID: user}); !errors.Is(err, session.ErrNoTenant) {
		t.Fatalf("tanpa tenant harus ErrNoTenant: %v", err)
	}
	if _, _, err := in.svc.Rotate(context.Background(), rt.Value, goodOrigin); !errors.Is(err, session.ErrNoTenant) {
		t.Fatalf("Rotate tanpa tenant harus ErrNoTenant: %v", err)
	}
}

func suiteRevokeIsIdempotentAndBroadcastsOnce(t *testing.T, w *world) {
	in := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, _ := w.start(in, user)

	before := w.notifier.count(session.ChannelSessionRevoked)
	must(t, in.svc.Revoke(w.ctx, sess.ID, session.ReasonLogout))
	must(t, in.svc.Revoke(w.ctx, sess.ID, session.ReasonLogout)) // idempoten
	if got := w.notifier.count(session.ChannelSessionRevoked) - before; got != 1 {
		t.Fatalf("pencabutan harus disiarkan sekali, dapat %d", got)
	}
	if w.sessionRow(sess.ID).RevokeReason != session.ReasonLogout {
		t.Fatal("alasan pertama harus bertahan")
	}
	if err := in.svc.Revoke(w.ctx, uuid.NewString(), session.ReasonLogout); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("sesi tak ada harus ErrSessionNotFound: %v", err)
	}
	if err := in.svc.Revoke(w.ctx, sess.ID, ""); !errors.Is(err, session.ErrInvalidInput) {
		t.Fatalf("alasan kosong harus ErrInvalidInput: %v", err)
	}
	// sesi tercabut tidak dapat dirotasi
	// (token akar belum dipakai, tetapi sesinya tercabut)
	s2, rt2 := w.start(in, user)
	must(t, in.svc.Revoke(w.ctx, s2.ID, session.ReasonLogout))
	if _, _, err := in.svc.Rotate(w.ctx, rt2.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("sesi tercabut: Rotate harus ErrInvalidRefresh, dapat %v", err)
	}

	// MarkRevoked: id yang dicabut pihak lain lewat store dicatat dan disiarkan
	s3, _ := w.start(in, user)
	ids, err := w.be.RevokeUserSessions(w.ctx, tenantA, user, "", "password_changed", w.clock.Now())
	must(t, err)
	if len(ids) == 0 {
		t.Fatal("harus ada sesi aktif yang tercabut")
	}
	other := w.subscribed(-1)
	in.svc.MarkRevoked(w.ctx, ids...)
	if !in.list.IsRevoked(s3.ID) {
		t.Fatal("MarkRevoked harus mencatat di daftar lokal")
	}
	eventually(t, "MarkRevoked disiarkan", func() bool { return other.list.IsRevoked(s3.ID) })
}

// Upgrade (naik ke otp): token refresh pra-MFA diganti dan mati tanpa mencabut sesi (bukan reuse); leluhur yang sudah
// terpakai tetap memicu deteksi reuse; AMR sesi naik.
func suiteUpgradeReplacesRefreshTokenWithoutRevokingSession(t *testing.T, w *world) {
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, root := w.start(a, user)
	_, cur, err := a.svc.Rotate(w.ctx, root.Value, goodOrigin) // root kini leluhur terpakai; cur token saat ini
	must(t, err)

	up, fresh, err := a.svc.Upgrade(w.ctx, user, sess.ID, "otp")
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if strings.Join(up.AMR, ",") != "pwd,otp" || fresh.Value == "" || fresh.Value == cur.Value || fresh.ExpiresAt.IsZero() {
		t.Fatalf("hasil = %+v %+v", up, fresh)
	}
	if row := w.sessionRow(sess.ID); strings.Join(row.AMR, ",") != "pwd,otp" || row.RevokedAt != nil {
		t.Fatalf("sesi = %+v", row)
	}
	if got := w.tokenRow(fresh.Value); got.FamilyID != root.FamilyID || got.UsedAt != nil {
		t.Fatalf("token baru harus satu family dan belum terpakai: %+v", got)
	}
	// token pra-upgrade ditolak seragam, sesi TETAP hidup (bukan serangan reuse)
	before := w.notifier.count(session.ChannelSessionRevoked)
	if _, _, err := a.svc.Rotate(w.ctx, cur.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("token pra-upgrade harus ErrInvalidRefresh: %v", err)
	}
	if row := w.sessionRow(sess.ID); row.RevokedAt != nil || a.list.IsRevoked(sess.ID) || w.notifier.count(session.ChannelSessionRevoked) != before {
		t.Fatal("token pra-upgrade tidak boleh mencabut sesi")
	}
	// token baru bekerja dan AMR dipertahankan
	got, next, err := a.svc.Rotate(w.ctx, fresh.Value, goodOrigin)
	if err != nil || strings.Join(got.AMR, ",") != "pwd,otp" {
		t.Fatalf("rotasi token baru: %+v %v", got, err)
	}
	// leluhur lama yang sudah terpakai tetap memicu reuse (mencabut family termasuk token baru)
	if _, _, err := a.svc.Rotate(w.ctx, root.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("reuse leluhur: %v", err)
	}
	if row := w.sessionRow(sess.ID); row.RevokedAt == nil {
		t.Fatal("reuse leluhur harus mencabut sesi")
	}
	if _, _, err := a.svc.Rotate(w.ctx, next.Value, goodOrigin); !errors.Is(err, session.ErrInvalidRefresh) {
		t.Fatalf("family harus mati: %v", err)
	}
	if strings.Contains(w.logs.String(), fresh.Value) {
		t.Fatal("token mentah tidak boleh masuk log")
	}
}

func suiteUpgradeOwnershipAndPreconditions(t *testing.T, w *world) {
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	other := w.be.NewUser(t, w.ctx)
	sess, _ := w.start(a, user)

	if _, _, err := a.svc.Upgrade(w.ctx, other, sess.ID, "otp"); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("sesi milik pengguna lain: %v", err)
	}
	if _, _, err := a.svc.Upgrade(w.otherCtx, user, sess.ID, "otp"); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("lintas tenant: %v", err)
	}
	if _, _, err := a.svc.Upgrade(w.ctx, user, uuid.NewString(), "otp"); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("sesi tak ada: %v", err)
	}
	if _, _, err := a.svc.Upgrade(w.ctx, user, sess.ID, ""); !errors.Is(err, session.ErrInvalidInput) {
		t.Fatalf("faktor kosong: %v", err)
	}
	if row := w.sessionRow(sess.ID); strings.Join(row.AMR, ",") != "pwd" {
		t.Fatalf("percobaan gagal tidak boleh mengubah sesi: %+v", row)
	}
	if _, _, err := a.svc.Upgrade(w.ctx, user, sess.ID, "otp"); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if _, _, err := a.svc.Upgrade(w.ctx, user, sess.ID, "otp"); !errors.Is(err, session.ErrAlreadyUpgraded) {
		t.Fatalf("upgrade ulang: %v", err)
	}
	must(t, a.svc.Revoke(w.ctx, sess.ID, session.ReasonLogout))
	if _, _, err := a.svc.Upgrade(w.ctx, user, sess.ID, "x"); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("sesi tercabut: %v", err)
	}
}

func suiteConcurrentUpgradeExactlyOneSucceeds(t *testing.T, w *world) {
	a := w.instance()
	user := w.be.NewUser(t, w.ctx)
	sess, _ := w.start(a, user)
	const n = 10
	type res struct {
		rt  session.RefreshToken
		err error
	}
	out := make(chan res, n)
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			_, rt, err := a.svc.Upgrade(w.ctx, user, sess.ID, "otp")
			out <- res{rt, err}
		}()
	}
	close(gate)
	wg.Wait()
	close(out)
	wins := 0
	var winner session.RefreshToken
	for r := range out {
		switch {
		case r.err == nil:
			wins++
			winner = r.rt
		case !errors.Is(r.err, session.ErrAlreadyUpgraded):
			t.Errorf("galat tak terduga: %v", r.err)
		}
	}
	if wins != 1 {
		t.Fatalf("upgrade serentak: %d pemenang, want 1", wins)
	}
	if _, _, err := a.svc.Rotate(w.ctx, winner.Value, goodOrigin); err != nil {
		t.Fatalf("token pemenang harus berlaku: %v", err)
	}
}
