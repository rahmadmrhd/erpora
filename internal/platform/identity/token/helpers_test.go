package token

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"erpora/internal/platform/crypto"
)

const (
	testIssuer   = "https://erpora.test"
	testAudience = "erpora-api"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

// fakeClock adalah jam palsu yang aman dipakai bersamaan.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
func (c *fakeClock) Set(t time.Time) { c.mu.Lock(); defer c.mu.Unlock(); c.t = t }

// fakeStore memenuhi KeyStore dan menegakkan aturan yang sama dengan store.Store sungguhan (kid, panjang kunci
// publik, cap waktu per status, tepi transisi sah, transisi bersyarat atomik); fake yang longgar menyembunyikan bug.
type fakeStore struct {
	mu        sync.Mutex
	recs      map[string]KeyRecord
	listCalls atomic.Int64
	listErr   error
}

var (
	errConflict          = errors.New("fake: conflict")
	errInvalidArgument   = errors.New("fake: invalid argument")
	errInvalidTransition = errors.New("fake: invalid transition")
)

func newFakeStore() *fakeStore { return &fakeStore{recs: map[string]KeyRecord{}} }

func cp(r KeyRecord) KeyRecord {
	r.PublicKey = append([]byte(nil), r.PublicKey...)
	r.PrivateKeySealed = append([]byte(nil), r.PrivateKeySealed...)
	return r
}

func (s *fakeStore) ListSigningKeys(context.Context) ([]KeyRecord, error) {
	s.listCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]KeyRecord, 0, len(s.recs))
	for _, r := range s.recs {
		out = append(out, cp(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *fakeStore) InsertSigningKey(_ context.Context, k KeyRecord) error {
	c := k.CreatedAt.UTC()
	switch k.Status {
	case StatusActive:
		if k.ActivatedAt == nil {
			k.ActivatedAt = &c
		}
	case StatusPending:
		if k.ActivatedAt != nil || k.RetiringAt != nil || k.RetiredAt != nil {
			return errInvalidArgument
		}
	case StatusRetiring, StatusRetired:
	default:
		return errInvalidArgument
	}
	if !ValidKID(k.ID) || len(k.PublicKey) != 32 || len(k.PrivateKeySealed) == 0 || len(k.PrivateKeySealed) > 4096 {
		return errInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.recs[k.ID]; dup {
		return errConflict
	}
	s.recs[k.ID] = cp(k)
	return nil
}

func (s *fakeStore) TransitionSigningKey(_ context.Context, id, from, to string, at time.Time) (bool, error) {
	okEdge := (from == StatusPending && (to == StatusActive || to == StatusRetired)) ||
		(from == StatusActive && to == StatusRetiring) || (from == StatusRetiring && to == StatusRetired)
	if !okEdge {
		return false, errInvalidTransition
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[id]
	if !ok || r.Status != from {
		return false, nil
	}
	at = at.UTC()
	r.Status = to
	switch to {
	case StatusActive:
		r.ActivatedAt = &at
	case StatusRetiring:
		r.RetiringAt = &at
	case StatusRetired:
		r.RetiredAt = &at
	}
	s.recs[id] = r
	return true, nil
}

func (s *fakeStore) get(id string) KeyRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cp(s.recs[id])
}

func (s *fakeStore) statuses() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]string{}
	for id, r := range s.recs {
		m[id] = r.Status
	}
	return m
}

// fakeNotifier adalah Notifier in-process bernama kanal.
type fakeNotifier struct {
	mu        sync.Mutex
	next      int
	subs      map[string]map[int]func(context.Context)
	published map[string]int
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{subs: map[string]map[int]func(context.Context){}, published: map[string]int{}}
}

func (n *fakeNotifier) Publish(ctx context.Context, channel string) {
	n.mu.Lock()
	n.published[channel]++
	var fns []func(context.Context)
	for _, f := range n.subs[channel] {
		fns = append(fns, f)
	}
	n.mu.Unlock()
	for _, f := range fns {
		f(ctx)
	}
}

func (n *fakeNotifier) Subscribe(channel string, fn func(context.Context)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.subs[channel] == nil {
		n.subs[channel] = map[int]func(context.Context){}
	}
	id := n.next
	n.next++
	n.subs[channel][id] = fn
	return func() { n.mu.Lock(); defer n.mu.Unlock(); delete(n.subs[channel], id) }
}

func (n *fakeNotifier) count(channel string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.published[channel]
}

func testSealer(t testing.TB) *crypto.Sealer {
	t.Helper()
	m := make([]byte, crypto.KeySize)
	if _, err := rand.Read(m); err != nil {
		t.Fatal(err)
	}
	s, err := crypto.NewSealerFromMaterial(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type env struct {
	t        testing.TB
	store    *fakeStore
	sealer   *crypto.Sealer
	notifier *fakeNotifier
	clk      *fakeClock
	ring     *Keyring
	issuer   *Issuer
	verifier *Verifier
	prom     *Promoter
}

// newEnv membuat keyring dengan satu kunci active (bootstrap) dan jam di t0.
func newEnv(t testing.TB) *env {
	t.Helper()
	e := newBareEnv(t)
	if created, err := e.ring.EnsureActive(context.Background()); err != nil || !created {
		t.Fatalf("EnsureActive: created=%v err=%v", created, err)
	}
	return e
}

func newBareEnv(t testing.TB) *env {
	t.Helper()
	e := &env{t: t, store: newFakeStore(), sealer: testSealer(t), notifier: newFakeNotifier(), clk: &fakeClock{t: t0}}
	e.ring = e.newRing()
	var err error
	if e.issuer, err = NewIssuer(IssuerConfig{Keyring: e.ring, Issuer: testIssuer, Audience: testAudience, Now: e.clk.Now}); err != nil {
		t.Fatal(err)
	}
	if e.verifier, err = NewVerifier(VerifierConfig{Keyring: e.ring, Issuer: testIssuer, Audience: testAudience, Now: e.clk.Now}); err != nil {
		t.Fatal(err)
	}
	if e.prom, err = NewPromoter(PromoterConfig{Keyring: e.ring, Now: e.clk.Now, Log: quietLog()}); err != nil {
		t.Fatal(err)
	}
	return e
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newRing membuat Keyring lain (instance lain) di store, sealer, notifier, dan jam yang sama.
func (e *env) newRing() *Keyring {
	r, err := NewKeyring(KeyringConfig{Store: e.store, Sealer: e.sealer, Notifier: e.notifier, Now: e.clk.Now, Log: quietLog()})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(r.Close)
	return r
}

func (e *env) tick() {
	e.t.Helper()
	if err := e.prom.Tick(context.Background(), e.clk.Now()); err != nil {
		e.t.Fatalf("Tick: %v", err)
	}
}

func (e *env) issue() string {
	e.t.Helper()
	tok, err := e.issuer.Issue(Claims{Subject: "user-1", SessionID: "sess-1", TenantID: "tenant-1", PermRev: 7, AMR: []string{"pwd", "otp"}, Scope: []string{"read", "write"}})
	if err != nil {
		e.t.Fatalf("Issue: %v", err)
	}
	return tok
}

func (e *env) mustVerify(raw string) Claims {
	e.t.Helper()
	c, err := e.verifier.Verify(raw)
	if err != nil {
		e.t.Fatalf("Verify: %v (reason %q)", err, Reason(err))
	}
	return c
}

func (e *env) wantReject(raw, reason string) {
	e.t.Helper()
	_, err := e.verifier.Verify(raw)
	if !errors.Is(err, ErrInvalidToken) {
		e.t.Fatalf("Verify harus ditolak sebagai ErrInvalidToken, dapat %v", err)
	}
	if got := Reason(err); reason != "" && got != reason {
		e.t.Fatalf("reason = %q, ingin %q", got, reason)
	}
	if err.Error() != ErrInvalidToken.Error() {
		e.t.Fatalf("pesan galat harus generik, dapat %q", err.Error())
	}
}

// seedOf membuka seed kunci dari rekaman tersegel (hanya untuk tes: memalsukan token dengan kunci pending).
func (e *env) seedOf(kid string) []byte {
	e.t.Helper()
	seed, err := e.sealer.Open(e.store.get(kid).PrivateKeySealed, sealAAD(kid))
	if err != nil {
		e.t.Fatal(err)
	}
	return seed
}

func (e *env) activeKid() string { return e.ring.Active().ID }

// forge menyusun token dari header dan klaim apa pun (string = JSON mentah, selain itu di-marshal) dan
// menandatanganinya dengan priv.
func forge(t testing.TB, priv ed25519.PrivateKey, hdr, claims any) string {
	t.Helper()
	enc := func(v any) string {
		switch x := v.(type) {
		case string:
			return b64.EncodeToString([]byte(x))
		case []byte:
			return b64.EncodeToString(x)
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b64.EncodeToString(b)
	}
	in := enc(hdr) + "." + enc(claims)
	return in + "." + b64.EncodeToString(ed25519.Sign(priv, []byte(in)))
}

func (e *env) goodClaims(mut func(map[string]any)) map[string]any {
	now := e.clk.Now()
	m := map[string]any{
		"iss": testIssuer, "sub": "user-1", "sid": "sess-1", "tid": "tenant-1", "aud": testAudience,
		"exp": now.Add(AccessTokenTTL).Unix(), "iat": now.Unix(), "jti": "jti-1", "perm_rev": 1, "amr": []string{"pwd"},
	}
	if mut != nil {
		mut(m)
	}
	return m
}

func (e *env) forgeActive(hdrMut func(map[string]any), claimMut func(map[string]any)) string {
	e.t.Helper()
	k := e.ring.Active()
	h := map[string]any{"alg": "EdDSA", "kid": k.ID}
	if hdrMut != nil {
		hdrMut(h)
	}
	return forge(e.t, k.priv, h, e.goodClaims(claimMut))
}

func mustHex(t testing.TB, s string) []byte {
	t.Helper()
	out, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func containsBytes(hay, needle []byte) bool { return bytes.Contains(hay, needle) }
