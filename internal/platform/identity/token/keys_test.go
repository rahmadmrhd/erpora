package token

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/crypto"
)

var bg = context.Background()

func TestRotateCreatesOnlyPendingKey(t *testing.T) {
	e := newEnv(t)
	activeBefore := e.activeKid()
	k, err := e.ring.Rotate(bg)
	if err != nil {
		t.Fatal(err)
	}
	if k.Status != StatusPending || !ValidKID(k.ID) || len(k.Public) != ed25519.PublicKeySize {
		t.Fatalf("kunci hasil Rotate: %v", k)
	}
	if !k.ActivatesAt().Equal(k.CreatedAt.Add(JWKSMaxAge)) || !k.CreatedAt.Equal(t0) {
		t.Fatalf("ActivatesAt = %v, CreatedAt = %v", k.ActivatesAt(), k.CreatedAt)
	}
	if info := k.Info(); info.Status != StatusPending || info.KeyID != k.ID || info.Algorithm != "EdDSA" || !info.ActivatesAt.Equal(t0.Add(JWKSMaxAge)) {
		t.Fatalf("Info = %+v", info)
	}
	if e.activeKid() != activeBefore {
		t.Fatal("Rotate tidak boleh mengganti kunci aktif")
	}
	if got := e.store.statuses()[k.ID]; got != StatusPending {
		t.Fatalf("status tersimpan = %s", got)
	}
	// 2 publish: bootstrap + rotate
	if n := e.notifier.count(ChannelKeysChanged); n != 2 {
		t.Fatalf("keys_changed terbit %d kali", n)
	}
}

func TestPendingKeyPublishedInJWKSButNotUsedToSign(t *testing.T) {
	e := newEnv(t)
	oldKid := e.activeKid()
	pend, err := e.ring.Rotate(bg)
	if err != nil {
		t.Fatal(err)
	}
	// JWKS memuat keduanya
	ids := jwksKids(t, e.ring)
	if !contains(ids, oldKid) || !contains(ids, pend.ID) {
		t.Fatalf("JWKS = %v", ids)
	}
	// penandatanganan tetap kunci lama, header kid = lama
	tok := e.issue()
	hb, _ := b64.DecodeString(strings.Split(tok, ".")[0])
	if !strings.Contains(string(hb), oldKid) || strings.Contains(string(hb), pend.ID) {
		t.Fatalf("header: %s", hb)
	}
	if e.ring.Active().ID != oldKid {
		t.Fatal("kunci pending dipakai menandatangani")
	}
	// kunci pending tidak memegang material privat di memori
	if p, _ := e.ring.ByKID(pend.ID); len(p.priv) != 0 {
		t.Fatal("kunci pending tidak boleh memuat kunci privat")
	}
}

func TestPromoterActivatesPendingAfterJWKSMaxAge(t *testing.T) {
	e := newEnv(t)
	oldKid := e.activeKid()
	pend, _ := e.ring.Rotate(bg)
	published := e.notifier.count(ChannelKeysChanged)

	e.clk.Advance(JWKSMaxAge - time.Second)
	e.tick()
	if e.activeKid() != oldKid || e.store.statuses()[pend.ID] != StatusPending {
		t.Fatal("belum boleh dipromosikan sebelum 300 detik")
	}
	if e.notifier.count(ChannelKeysChanged) != published {
		t.Fatal("tanpa perubahan tidak boleh ada notifikasi")
	}
	e.clk.Advance(time.Second)
	e.tick()
	st := e.store.statuses()
	if st[pend.ID] != StatusActive || st[oldKid] != StatusRetiring {
		t.Fatalf("status setelah promosi: %v", st)
	}
	if e.activeKid() != pend.ID {
		t.Fatal("kunci baru harus menjadi penandatangan")
	}
	if e.notifier.count(ChannelKeysChanged) != published+1 {
		t.Fatalf("promosi harus memicu keys_changed tepat sekali")
	}
	if rec := e.store.get(oldKid); rec.RetiringAt == nil || !rec.RetiringAt.Equal(e.clk.Now()) {
		t.Fatalf("RetiringAt = %v", rec.RetiringAt)
	}
	// idempoten: tick lagi tidak mengubah apa pun
	e.tick()
	if e.notifier.count(ChannelKeysChanged) != published+1 {
		t.Fatal("tick kedua tidak boleh mempublikasikan lagi")
	}
}

// Saat serve start dan waktu aktivasi sudah lewat, Run langsung mempromosikan.
func TestPromoterRunPromotesImmediatelyAtStartWhenDue(t *testing.T) {
	e := newEnv(t)
	oldKid := e.activeKid()
	pend, _ := e.ring.Rotate(bg)
	e.clk.Advance(JWKSMaxAge + time.Hour) // serve baru menyala jauh setelah waktu aktivasi
	p, err := NewPromoter(PromoterConfig{Keyring: e.ring, Now: e.clk.Now, Interval: time.Hour, Log: quietLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for e.store.statuses()[pend.ID] != StatusActive {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("Run tidak mempromosikan saat start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if e.store.statuses()[oldKid] != StatusRetiring {
		t.Fatalf("status lama = %s", e.store.statuses()[oldKid])
	}
}

func TestRotateKeepsOldTokensValidWhileRetiring(t *testing.T) {
	e := newEnv(t)
	oldTok := e.issue()
	oldKid := e.activeKid()
	pend, _ := e.ring.Rotate(bg)
	e.mustVerify(oldTok)
	e.clk.Advance(JWKSMaxAge)
	e.tick()
	// token lama tetap valid (kunci retiring), token baru memakai kunci baru dan valid
	e.mustVerify(oldTok)
	newTok := e.issue()
	if !strings.Contains(headerOf(t, newTok), pend.ID) {
		t.Fatal("token baru harus bertanda kunci baru")
	}
	e.mustVerify(newTok)
	// token lama yang diterbitkan tepat sebelum rotasi tetap diterima sampai exp+leeway
	if !strings.Contains(headerOf(t, oldTok), oldKid) {
		t.Fatal("token lama bertanda kunci lama")
	}
	// instance lain (keyring baru dari store) juga menerima keduanya
	other := e.newRing()
	if err := other.Reload(bg); err != nil {
		t.Fatal(err)
	}
	v2, _ := NewVerifier(VerifierConfig{Keyring: other, Issuer: testIssuer, Audience: testAudience, Now: e.clk.Now})
	for _, tok := range []string{oldTok, newTok} {
		if _, err := v2.Verify(tok); err != nil {
			t.Fatalf("instance lain: %v", err)
		}
	}
}

func TestRetiringBecomesRetiredOnlyAfterTokenTTLPlusLeeway(t *testing.T) {
	e := newEnv(t)
	oldKid := e.activeKid()
	e.ring.Rotate(bg)
	e.clk.Advance(JWKSMaxAge)
	e.tick()
	if e.store.statuses()[oldKid] != StatusRetiring {
		t.Fatal("kunci lama harus retiring")
	}
	if RetireAfter != 10*time.Minute+30*time.Second {
		t.Fatalf("RetireAfter = %v", RetireAfter)
	}
	e.clk.Advance(RetireAfter - time.Nanosecond)
	e.tick()
	if e.store.statuses()[oldKid] != StatusRetiring {
		t.Fatal("belum boleh retired sebelum umur token + leeway")
	}
	// kunci retiring masih di JWKS
	if !contains(jwksKids(t, e.ring), oldKid) {
		t.Fatal("kunci retiring harus ada di JWKS")
	}
	e.clk.Advance(time.Nanosecond)
	e.tick()
	if e.store.statuses()[oldKid] != StatusRetired {
		t.Fatal("harus retired tepat pada RetireAfter")
	}
	if contains(jwksKids(t, e.ring), oldKid) {
		t.Fatal("kunci retired tidak boleh ada di JWKS")
	}
}

func TestUnknownKIDReloadsOnceRateLimited(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	mk := func(i int) string {
		return forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": fmt.Sprintf("unknown-kid-%04d", i)}, e.goodClaims(nil))
	}
	before := e.store.listCalls.Load()
	for i := 0; i < 50; i++ {
		e.wantReject(mk(i), ReasonUnknownKID)
	}
	if n := e.store.listCalls.Load() - before; n != 1 {
		t.Fatalf("muat ulang akibat kid tak dikenal = %d, ingin tepat 1", n)
	}
	e.clk.Advance(defaultUnknownKIDReloadInterval - time.Second)
	e.wantReject(mk(100), ReasonUnknownKID)
	if n := e.store.listCalls.Load() - before; n != 1 {
		t.Fatalf("masih dalam batas laju, tetapi muat ulang = %d", n)
	}
	e.clk.Advance(time.Second)
	e.wantReject(mk(101), ReasonUnknownKID)
	if n := e.store.listCalls.Load() - before; n != 2 {
		t.Fatalf("setelah interval harus muat ulang lagi, total = %d", n)
	}
}

// Instance B belum menerima notifikasi: kid kunci baru (sudah dipromosikan instance A) dikenali lewat muat ulang
// akibat kid tak dikenal, juga bila di snapshot B kunci itu masih pending.
func TestUnknownOrStalePendingKIDFoundAfterReload(t *testing.T) {
	e := newEnv(t)
	// instance A tanpa notifier ke B: pakai Notifier terpisah agar B tidak menerima notifikasi
	stale, _ := NewKeyring(KeyringConfig{Store: e.store, Sealer: e.sealer, Now: e.clk.Now, Log: quietLog()})
	if err := stale.Reload(bg); err != nil {
		t.Fatal(err)
	}
	vStale, _ := NewVerifier(VerifierConfig{Keyring: stale, Issuer: testIssuer, Audience: testAudience, Now: e.clk.Now})

	pend, _ := e.ring.Rotate(bg)
	if _, ok := stale.ByKID(pend.ID); !ok { // reload pertama: kini pending terlihat
		t.Fatal("kunci pending seharusnya terlihat setelah muat ulang")
	}
	e.clk.Advance(JWKSMaxAge + defaultUnknownKIDReloadInterval)
	e.tick() // pending -> active di store; stale masih melihat pending
	tok := e.issue()
	if !strings.Contains(headerOf(t, tok), pend.ID) {
		t.Fatal("token harus bertanda kunci baru")
	}
	if _, err := vStale.Verify(tok); err != nil {
		t.Fatalf("instance basi harus memuat ulang dan menerima: %v (%s)", err, Reason(err))
	}
}

func TestKeyringReloadsOnKeysChangedNotification(t *testing.T) {
	e := newEnv(t)
	other := e.newRing() // berlangganan notifier yang sama
	if err := other.Reload(bg); err != nil {
		t.Fatal(err)
	}
	pend, err := e.ring.Rotate(bg) // memublikasikan keys_changed
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range other.Keys() {
		found = found || k.ID == pend.ID
	}
	if !found {
		t.Fatal("instance lain harus memuat ulang saat keys_changed")
	}
	// Close menghentikan langganan
	other.Close()
	before := e.store.listCalls.Load()
	e.notifier.Publish(bg, ChannelKeysChanged)
	if n := e.store.listCalls.Load() - before; n != 1 { // hanya e.ring
		t.Fatalf("muat ulang setelah Close = %d", n)
	}
}

func TestReloadFailureKeepsOldSnapshot(t *testing.T) {
	e := newEnv(t)
	kid := e.activeKid()
	e.store.listErr = errors.New("db mati")
	if err := e.ring.Reload(bg); err == nil {
		t.Fatal("harus galat")
	}
	e.store.listErr = nil
	if e.activeKid() != kid {
		t.Fatal("snapshot lama harus dipertahankan")
	}
	e.mustVerify(e.issue())
}

func TestReloadFailsClosedWhenActiveKeyCannotBeOpened(t *testing.T) {
	e := newEnv(t)
	wrong := testSealer(t)
	r2, _ := NewKeyring(KeyringConfig{Store: e.store, Sealer: wrong, Log: quietLog()})
	err := r2.Reload(bg)
	if err == nil {
		t.Fatal("kunci active yang tak dapat dibuka harus menggagalkan muat")
	}
	if !r2.Active().IsZero() {
		t.Fatal("tidak boleh ada kunci aktif")
	}
	// kunci publik/privat tidak muncul di galat
	if strings.Contains(err.Error(), fmt.Sprintf("%x", e.ring.Active().Public)) {
		t.Fatal("galat membocorkan material")
	}
}

func TestReloadRejectsMismatchedPublicKey(t *testing.T) {
	e := newBareEnv(t)
	rec, err := e.ring.newRecord(StatusActive, t0)
	if err != nil {
		t.Fatal(err)
	}
	otherPub, _, _ := ed25519.GenerateKey(nil)
	rec.PublicKey = otherPub
	if err := e.store.InsertSigningKey(bg, rec); err != nil {
		t.Fatal(err)
	}
	if err := e.ring.Reload(bg); err == nil {
		t.Fatal("kunci publik tidak cocok harus ditolak")
	}
}

func TestKeyringSkipsCorruptNonActiveRecord(t *testing.T) {
	e := newEnv(t)
	// pending berstatus valid tetapi sealed sembarang: tidak dibuka, jadi tetap dimuat
	rec, _ := e.ring.newRecord(StatusPending, t0)
	rec.PrivateKeySealed = []byte("sampah")
	if err := e.store.InsertSigningKey(bg, rec); err != nil {
		t.Fatal(err)
	}
	if err := e.ring.Reload(bg); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.ring.ByKID(rec.ID); !ok {
		t.Fatal("kunci pending harus terlihat")
	}
}

func TestPrivateKeyStoredSealed(t *testing.T) {
	e := newEnv(t)
	pend, _ := e.ring.Rotate(bg)
	for _, kid := range []string{e.activeKid(), pend.ID} {
		rec := e.store.get(kid)
		seed := e.seedOf(kid)
		if len(seed) != ed25519.SeedSize {
			t.Fatalf("seed %d byte", len(seed))
		}
		priv := ed25519.NewKeyFromSeed(seed)
		for name, raw := range map[string][]byte{"seed": seed, "privat 64 byte": priv} {
			if bytes.Contains(rec.PrivateKeySealed, raw) {
				t.Fatalf("kolom privat %s memuat %s mentah", kid, name)
			}
		}
		if bytes.Contains(rec.PrivateKeySealed, rec.PublicKey) {
			t.Fatal("kolom privat memuat kunci publik mentah")
		}
		if len(rec.PrivateKeySealed) == 0 || len(rec.PrivateKeySealed) > 4096 {
			t.Fatalf("ukuran kolom %d", len(rec.PrivateKeySealed))
		}
		// AAD terikat kid: blob tidak dapat dipindah ke baris lain
		other := "someone-elses-kid-xx"
		if _, err := e.sealer.Open(rec.PrivateKeySealed, sealAAD(other)); !errors.Is(err, crypto.ErrOpen) {
			t.Fatalf("AAD kid lain harus gagal: %v", err)
		}
		if !bytes.Equal(priv.Public().(ed25519.PublicKey), rec.PublicKey) {
			t.Fatal("seed tidak cocok dengan kunci publik")
		}
	}
	// blob kunci A dipindah ke baris kunci B: kunci B (bila active) tidak boleh terbuka
	e2 := newBareEnv(t)
	recA, _ := e2.ring.newRecord(StatusActive, t0)
	recB, _ := e2.ring.newRecord(StatusActive, t0)
	recB.PrivateKeySealed = recA.PrivateKeySealed
	_ = e2.store.InsertSigningKey(bg, recB)
	if err := e2.ring.Reload(bg); err == nil {
		t.Fatal("blob yang dipindah antar kid harus ditolak")
	}
}

func TestKeyRedaction(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	seed := e.seedOf(k.ID)
	var sb strings.Builder
	h := slog.NewJSONHandler(&sb, nil)
	slog.New(h).Info("tes", "key", k)
	out := []string{
		fmt.Sprintf("%v", k), fmt.Sprintf("%+v", k), fmt.Sprintf("%#v", k), fmt.Sprintf("%s", k), fmt.Sprintf("%x", k), k.String(),
		fmt.Sprintf("%v", &k), fmt.Sprintf("%+v", []Key{k}), sb.String(),
	}
	for _, s := range out {
		if strings.Contains(s, fmt.Sprintf("%x", seed)) || strings.Contains(s, fmt.Sprintf("%x", []byte(k.priv))) || strings.Contains(s, fmt.Sprintf("%v", []byte(k.priv))) {
			t.Fatalf("pencetakan membocorkan kunci privat: %s", s)
		}
		if !strings.Contains(s, "****") {
			t.Fatalf("pencetakan tidak tersamar: %s", s)
		}
	}
}

func TestEnsureActive(t *testing.T) {
	e := newBareEnv(t)
	created, err := e.ring.EnsureActive(bg)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if e.ring.Active().IsZero() {
		t.Fatal("harus ada kunci aktif")
	}
	if created, _ := e.ring.EnsureActive(bg); created {
		t.Fatal("kedua kali tidak boleh membuat lagi")
	}
	// hanya pending (mis. rotate-keys sebelum serve): tidak membuat kunci active baru
	e2 := newBareEnv(t)
	if _, err := e2.ring.Rotate(bg); err != nil {
		t.Fatal(err)
	}
	if created, _ := e2.ring.EnsureActive(bg); created {
		t.Fatal("dengan kunci pending tidak boleh membuat kunci active (menunggu promosi)")
	}
	if !e2.ring.Active().IsZero() {
		t.Fatal("belum ada kunci active")
	}
	e2.clk.Advance(JWKSMaxAge)
	e2.tick()
	if e2.ring.Active().IsZero() {
		t.Fatal("pending matang harus dipromosikan walau belum ada active sebelumnya")
	}
}

// Dua instance membuat kunci pertama serentak: Active() sama di semua instance, dan Promoter menurunkan yang lama.
func TestTwoActiveKeysConverge(t *testing.T) {
	e := newBareEnv(t)
	r2 := e.newRing()
	if _, err := e.ring.EnsureActive(bg); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(time.Second)
	// r2 belum melihat kunci pertama (muat sebelum): paksa dengan menyisipkan langsung
	rec, _ := r2.newRecord(StatusActive, e.clk.Now())
	if err := e.store.InsertSigningKey(bg, rec); err != nil {
		t.Fatal(err)
	}
	if err := e.ring.Reload(bg); err != nil {
		t.Fatal(err)
	}
	if err := r2.Reload(bg); err != nil {
		t.Fatal(err)
	}
	if e.ring.Active().ID != r2.Active().ID || e.ring.Active().ID != rec.ID {
		t.Fatalf("Active() berbeda antar instance: %s vs %s", e.ring.Active().ID, r2.Active().ID)
	}
	e.tick()
	st := e.store.statuses()
	n := map[string]int{}
	for _, s := range st {
		n[s]++
	}
	if n[StatusActive] != 1 || n[StatusRetiring] != 1 {
		t.Fatalf("status setelah konvergensi: %v", st)
	}
	if e.ring.Active().ID != rec.ID {
		t.Fatal("kunci terbaru harus tetap aktif")
	}
}

func TestTickPromotesNewestMaturePendingAndCancelsOlder(t *testing.T) {
	e := newEnv(t)
	a, _ := e.ring.Rotate(bg)
	e.clk.Advance(time.Second)
	b, _ := e.ring.Rotate(bg)
	e.clk.Advance(JWKSMaxAge)
	e.tick()
	st := e.store.statuses()
	if st[b.ID] != StatusActive || st[a.ID] != StatusRetired {
		t.Fatalf("status: %v", st)
	}
	// pending yang lebih baru dan belum matang tetap pending
	e2 := newEnv(t)
	c, _ := e2.ring.Rotate(bg)
	e2.clk.Advance(JWKSMaxAge - time.Second)
	d, _ := e2.ring.Rotate(bg)
	e2.clk.Advance(time.Second)
	e2.tick()
	st2 := e2.store.statuses()
	if st2[c.ID] != StatusActive || st2[d.ID] != StatusPending {
		t.Fatalf("status: %v", st2)
	}
}

func TestTickPromotesBeforeDemoting(t *testing.T) {
	// ada saat tanpa kunci active akan membuat Issue gagal; di sini setiap pengamatan sesudah Tick harus punya active
	e := newEnv(t)
	e.ring.Rotate(bg)
	e.clk.Advance(JWKSMaxAge)
	e.tick()
	if e.ring.Active().IsZero() {
		t.Fatal("harus ada kunci active")
	}
}

func TestConcurrentKeyringUse(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	stop := make(chan struct{})
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				tok, err := e.issuer.Issue(Claims{Subject: "u", SessionID: "s", TenantID: "t"})
				if err != nil {
					errs <- err
					return
				}
				// Jam tiruan maju 3 x JWKSMaxAge (> umur access token) selama tes; goroutine yang tertunda (mis. -race)
				// dapat memverifikasi token yang diterbitkan sebelum dua kemajuan jam, sehingga kedaluwarsa itu sah.
				// Tes ini menguji keamanan konkurensi keyring (kid/tanda tangan), bukan masa berlaku.
				if _, err := e.verifier.Verify(tok); err != nil && Reason(err) != ReasonExpired {
					errs <- fmt.Errorf("verify: %w (%s)", err, Reason(err))
					return
				}
				e.ring.Keys()
				e.ring.ByKID("unknown-kid-concurrent")
			}
		}()
	}
	for i := 0; i < 3; i++ {
		if _, err := e.ring.Rotate(bg); err != nil {
			t.Error(err)
		}
		e.clk.Advance(JWKSMaxAge)
		if err := e.prom.Tick(bg, e.clk.Now()); err != nil {
			t.Error(err)
		}
		_ = e.ring.Reload(bg)
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestJWKS(t *testing.T) {
	e := newEnv(t)
	active := e.activeKid()
	pend, _ := e.ring.Rotate(bg)
	e.clk.Advance(JWKSMaxAge)
	e.tick() // pend active, active retiring
	e.clk.Advance(RetireAfter)
	e.tick() // lama retired
	pend2, _ := e.ring.Rotate(bg)

	rec := httptest.NewRecorder()
	JWKSHandler(e.ring).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if strings.Contains(body, active) {
		t.Fatal("kunci retired tidak boleh ada di JWKS")
	}
	var doc struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Keys) != 2 {
		t.Fatalf("keys = %v", doc.Keys)
	}
	seen := map[string]bool{}
	for _, k := range doc.Keys {
		if len(k) != 6 || k["kty"] != "OKP" || k["crv"] != "Ed25519" || k["alg"] != "EdDSA" || k["use"] != "sig" || k["kid"] == "" || k["x"] == "" {
			t.Fatalf("entri JWK tak terduga: %v", k)
		}
		if _, hasD := k["d"]; hasD {
			t.Fatal("JWKS tidak boleh memuat parameter privat d")
		}
		pub, err := b64.DecodeString(k["x"])
		if err != nil || len(pub) != 32 {
			t.Fatalf("x tidak sah: %v", err)
		}
		seen[k["kid"]] = true
	}
	if !seen[pend.ID] || !seen[pend2.ID] {
		t.Fatalf("kid di JWKS = %v", seen)
	}
	// tidak ada seed/privat di badan
	for _, kid := range []string{pend.ID, pend2.ID} {
		if strings.Contains(body, b64.EncodeToString(e.seedOf(kid))) {
			t.Fatal("JWKS membocorkan seed")
		}
	}
	// HEAD dan metode lain
	rec = httptest.NewRecorder()
	JWKSHandler(e.ring).ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/x", nil))
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("HEAD: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	JWKSHandler(e.ring).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
}

func TestJWKSEmptyKeyring(t *testing.T) {
	e := newBareEnv(t)
	rec := httptest.NewRecorder()
	JWKSHandler(e.ring).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if strings.TrimSpace(rec.Body.String()) != `{"keys":[]}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func headerOf(t testing.TB, tok string) string {
	t.Helper()
	b, err := b64.DecodeString(strings.Split(tok, ".")[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func jwksKids(t testing.TB, r *Keyring) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	JWKSHandler(r).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, k := range doc.Keys {
		out = append(out, k.Kid)
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
