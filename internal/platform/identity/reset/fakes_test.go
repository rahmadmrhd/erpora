package reset_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	sdkmail "erpora/sdk/mail"
)

var (
	testTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	testParams = password.Params{MemoryKiB: 8192, Time: 1, Parallelism: 1}
)

const (
	baseURL   = "https://erp.example.com"
	goodPass  = "kata sandi baru yang kuat 2026"
	oldPass   = "kata sandi lama yang kuat 123"
	userEmail = "budi@example.org"
)

func tctx() context.Context { return tenancy.WithTenant(context.Background(), testTenant) }

func ipCtx(ip string) context.Context { return reset.WithClientIP(tctx(), ip) }

// clock adalah jam palsu yang aman dipakai serentak.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// fakeStore memenuhi reset.Store dengan semantik atomik yang sama seperti store sungguhan (satu mutex).
type fakeStore struct {
	mu       sync.Mutex
	delay    time.Duration
	users    map[string]*store.User // email -> user
	byID     map[string]*store.User
	prefs    map[string]store.Preferences
	resets   map[string]*store.PasswordReset
	sessions map[string][]string // userID -> sesi aktif
	revoked  []string
	calls    map[string]int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users: map[string]*store.User{}, byID: map[string]*store.User{}, prefs: map[string]store.Preferences{},
		resets: map[string]*store.PasswordReset{}, sessions: map[string][]string{}, calls: map[string]int{},
	}
}

func (f *fakeStore) addUser(t testing.TB, email, status, hash string) store.User {
	t.Helper()
	u := &store.User{ID: uuid.NewString(), Email: email, Name: "Budi Santoso", Status: status, PasswordHash: hash}
	f.users[email], f.byID[u.ID] = u, u
	return *u
}

func (f *fakeStore) setStatus(email, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[store.NormalizeEmail(email)].Status = status
}

func (f *fakeStore) GetUser(_ context.Context, _ tenancy.TenantID, userID string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("GetUser")
	u, ok := f.byID[userID]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return *u, nil
}

func (f *fakeStore) call(name string) {
	f.calls[name]++
	if f.delay > 0 {
		f.mu.Unlock()
		time.Sleep(f.delay)
		f.mu.Lock()
	}
}

func (f *fakeStore) callCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeStore) FindUserByEmail(_ context.Context, _ tenancy.TenantID, email string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("FindUserByEmail")
	u, ok := f.users[store.NormalizeEmail(email)]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return *u, nil
}

func (f *fakeStore) GetPreferences(_ context.Context, _ tenancy.TenantID, userID string) (store.Preferences, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("GetPreferences")
	return f.prefs[userID], nil
}

func (f *fakeStore) InvalidatePasswordResets(_ context.Context, _ tenancy.TenantID, userID string, at time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("InvalidatePasswordResets")
	var n int64
	for _, r := range f.resets {
		if r.UserID == userID && r.UsedAt == nil {
			a := at
			r.UsedAt = &a
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) InsertPasswordReset(_ context.Context, _ tenancy.TenantID, r store.PasswordReset) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("InsertPasswordReset")
	if len(r.TokenHash) != 32 {
		return store.ErrInvalidArgument
	}
	cp := r
	cp.TokenHash = append([]byte(nil), r.TokenHash...)
	f.resets[r.ID] = &cp
	return nil
}

func (f *fakeStore) FindPasswordResetByHash(_ context.Context, _ tenancy.TenantID, hash []byte) (store.PasswordReset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("FindPasswordResetByHash")
	for _, r := range f.resets {
		if bytes.Equal(r.TokenHash, hash) {
			return *r, nil
		}
	}
	return store.PasswordReset{}, store.ErrNotFound
}

func (f *fakeStore) ResetPassword(_ context.Context, _ tenancy.TenantID, resetID, newHash string, at time.Time) (store.ResetResult, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.call("ResetPassword")
	r, ok := f.resets[resetID]
	if !ok || r.UsedAt != nil || !r.ExpiresAt.After(at) {
		return store.ResetResult{}, false, nil
	}
	if o := f.byID[r.UserID]; o == nil || (o.Status != store.UserActive && o.Status != store.UserLocked) {
		return store.ResetResult{}, false, nil // seperti store sungguhan: pemilik disabled = tautan tidak dapat dipakai
	}
	a := at
	r.UsedAt = &a
	u := f.byID[r.UserID]
	u.PasswordHash = newHash
	if u.Status == store.UserLocked {
		u.Status = store.UserActive
	}
	for _, o := range f.resets {
		if o.UserID == r.UserID && o.UsedAt == nil {
			b := at
			o.UsedAt = &b
		}
	}
	ids := f.sessions[u.ID]
	f.sessions[u.ID] = nil
	return store.ResetResult{UserID: u.ID, RevokedSessionIDs: ids}, true, nil
}

func (f *fakeStore) user(email string) store.User {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.users[email]
}

// sentMail adalah pesan yang ditangkap fakeSender beserta jejak waktu.
type fakeSender struct {
	mu    sync.Mutex
	delay time.Duration
	err   error
	block chan struct{} // bila non-nil, Send menunggu sampai ditutup
	sent  []sdkmail.Message
	start atomic.Int32
}

func (s *fakeSender) Send(_ context.Context, m sdkmail.Message) error {
	s.start.Add(1)
	if s.block != nil {
		<-s.block
	}
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if err := sdkmail.Validate(m); err != nil { // kontrak Sender: driver memvalidasi
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, m)
	return nil
}

func (s *fakeSender) messages() []sdkmail.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sdkmail.Message(nil), s.sent...)
}

// fakeSessions mencatat MarkRevoked.
type fakeSessions struct {
	mu  sync.Mutex
	ids []string
}

func (s *fakeSessions) MarkRevoked(_ context.Context, ids ...string) {
	s.mu.Lock()
	s.ids = append(s.ids, ids...)
	s.mu.Unlock()
}

func (s *fakeSessions) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ids...)
}

// countingGate memenuhi reset.Hasher dengan hash argon2id nyata (parameter uji) dan menghitung panggilan.
type countingGate struct {
	n    atomic.Int32
	busy bool
}

func (g *countingGate) Hash(_ context.Context, h *password.Hasher, plain string) (string, error) {
	g.n.Add(1)
	if g.busy {
		return "", &guard.RateLimitedError{RetryAfter: time.Second, Cause: guard.ErrHashBusy}
	}
	return h.Hash(plain)
}

// errNotConfigured meniru settings.ErrNotConfigured.
var errNotConfigured = errors.New("settings: belum dikonfigurasi")

// fakeURL memenuhi reset.PublicURLSource (di produksi: settings.Service).
type fakeURL struct {
	mu    sync.Mutex
	url   string
	err   error
	calls int
}

func (f *fakeURL) PublicURL(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.url, f.err
}

func (f *fakeURL) set(url string, err error) {
	f.mu.Lock()
	f.url, f.err = url, err
	f.mu.Unlock()
}

type harness struct {
	t      *testing.T
	st     *fakeStore
	sender *fakeSender
	sess   *fakeSessions
	gate   *countingGate
	clk    *clock
	svc    *reset.Service
	logs   *bytes.Buffer
	url    *fakeURL
}

type hopts struct {
	mod    func(*reset.Config)
	noWait bool
}

func newHarness(t *testing.T, o hopts) *harness {
	t.Helper()
	h := &harness{t: t, st: newFakeStore(), sender: &fakeSender{}, sess: &fakeSessions{}, gate: &countingGate{}, clk: newClock(), logs: &bytes.Buffer{}, url: &fakeURL{url: baseURL}}
	cfg := reset.Config{
		Store: h.st, Sessions: h.sess, Gate: h.gate, Sender: h.sender, PublicURL: h.url, Argon2: testParams,
		Now:    h.clk.Now,
		Sleep:  func(context.Context, time.Duration) error { return nil },
		Logger: slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	if o.mod != nil {
		o.mod(&cfg)
	}
	svc, err := reset.New(cfg)
	if err != nil {
		t.Fatalf("reset.New: %v", err)
	}
	h.svc = svc
	return h
}

func (h *harness) request(ctx context.Context, email, locale string) error {
	h.t.Helper()
	err := h.svc.Request(ctx, email, locale)
	h.svc.Wait()
	return err
}

// tokenFrom mengambil token mentah dari tautan di pesan (fragmen #token=...).
func tokenFrom(t testing.TB, m sdkmail.Message) string {
	t.Helper()
	i := strings.Index(m.Text, "#token=")
	if i < 0 {
		t.Fatalf("tautan reset tidak ditemukan di teks:\n%s", m.Text)
	}
	rest := m.Text[i+len("#token="):]
	end := strings.IndexAny(rest, " \r\n\"<")
	if end < 0 {
		end = len(rest)
	}
	return rest[:end]
}

var _ = io.Discard
