//go:build integration

package reset_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
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

type intEnv struct {
	st     *store.Store
	sender *fakeSender
	sess   *fakeSessions
	clk    *clock
	svc    *reset.Service
}

func newIntEnv(t *testing.T) *intEnv {
	t.Helper()
	st := setupDB(t)
	e := &intEnv{st: st, sender: &fakeSender{}, sess: &fakeSessions{}, clk: newClock()}
	gate, err := guard.NewHashGate(2, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	e.svc, err = reset.New(reset.Config{
		Store: st, Sessions: e.sess, Gate: gate, Sender: e.sender, PublicURL: &fakeURL{url: baseURL}, Argon2: testParams,
		Now: e.clk.Now, Sleep: func(context.Context, time.Duration) error { return nil }, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *intEnv) addUser(t *testing.T, email string) store.User {
	t.Helper()
	phc, err := (&password.Hasher{Params: testParams}).Hash(oldPass)
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.st.CreateUser(tctx(), testTenant, store.NewUser{Email: email, Name: "Budi Santoso", PasswordHash: phc})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *intEnv) newSession(t *testing.T, userID string) string {
	t.Helper()
	c := e.clk.Now()
	sess := store.Session{ID: uuid.NewString(), UserID: userID, ClientKind: "web", AuthMethod: "password", AMR: []string{"pwd"},
		UserAgent: "uji", IP: "203.0.113.7", CreatedAt: c, LastSeenAt: c, AbsoluteExpiresAt: c.Add(90 * 24 * time.Hour)}
	rt := store.RefreshToken{ID: uuid.NewString(), SessionID: sess.ID, FamilyID: uuid.NewString(), CreatedAt: c, ExpiresAt: c.Add(14 * 24 * time.Hour)}
	rt.TokenHash = make([]byte, 32)
	copy(rt.TokenHash, uuid.NewString())
	if err := e.st.CreateSessionWithRefresh(tctx(), testTenant, sess, rt); err != nil {
		t.Fatal(err)
	}
	return sess.ID
}

func (e *intEnv) requestToken(t *testing.T, email string) string {
	t.Helper()
	before := len(e.sender.messages())
	if err := e.svc.Request(ipCtx("198.51.100.7"), email, ""); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	msgs := e.sender.messages()
	if len(msgs) != before+1 {
		t.Fatalf("surat = %d, want %d", len(msgs), before+1)
	}
	return tokenFrom(t, msgs[len(msgs)-1])
}

func TestIntegrationResetFlow(t *testing.T) {
	e := newIntEnv(t)
	u := e.addUser(t, "budi@example.org")
	s1, s2 := e.newSession(t, u.ID), e.newSession(t, u.ID)
	if ok, err := e.st.LockUser(tctx(), testTenant, u.ID, e.clk.Now()); err != nil || !ok {
		t.Fatalf("LockUser: %v %v", ok, err)
	}
	raw := e.requestToken(t, "Budi@Example.org")

	// Hanya hash yang tersimpan: ditemukan lewat SHA-256, tidak lewat token mentah.
	tok, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(tok) != 32 {
		t.Fatalf("token: %v len=%d", err, len(tok))
	}
	sum := sha256.Sum256(tok)
	rec, err := e.st.FindPasswordResetByHash(tctx(), testTenant, sum[:])
	if err != nil {
		t.Fatalf("hash tidak ditemukan: %v", err)
	}
	if rec.UserID != u.ID || rec.ExpiresAt.Sub(rec.CreatedAt) != 30*time.Minute {
		t.Errorf("rec = %+v", rec)
	}
	if _, err := e.st.FindPasswordResetByHash(tctx(), testTenant, tok); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("token mentah ditemukan di kolom token_hash: %v", err)
	}
	// Tenant lain tidak dapat memakainya.
	otherCtx := tenancy.WithTenant(context.Background(), tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c")))
	if err := e.svc.Complete(otherCtx, raw, goodPass); !errors.Is(err, reset.ErrInvalidToken) {
		t.Errorf("lintas tenant: err = %v, want ErrInvalidToken", err)
	}

	if err := e.svc.Complete(tctx(), raw, goodPass); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, err := e.st.GetUser(tctx(), testTenant, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := &password.Hasher{Params: testParams}
	if ok, _, _ := h.Verify(goodPass, got.PasswordHash); !ok {
		t.Error("sandi baru tidak berlaku")
	}
	if ok, _, _ := h.Verify(oldPass, got.PasswordHash); ok {
		t.Error("sandi lama masih berlaku")
	}
	if got.Status != store.UserActive || got.LockedAt != nil {
		t.Errorf("akun locked harus terbuka: %+v", got)
	}
	for _, id := range []string{s1, s2} {
		s, err := e.st.GetSession(tctx(), testTenant, id)
		if err != nil || s.RevokedAt == nil {
			t.Errorf("sesi %s tidak tercabut: %+v %v", id, s, err)
		}
	}
	revoked := e.sess.got()
	if len(revoked) != 2 {
		t.Errorf("MarkRevoked = %v, want 2 sesi", revoked)
	}
	if err := e.svc.Complete(tctx(), raw, "kata sandi lain yang kuat 777"); !errors.Is(err, reset.ErrInvalidToken) {
		t.Errorf("pemakaian kedua: %v", err)
	}
}

func TestIntegrationCompleteConcurrentExactlyOneWins(t *testing.T) {
	e := newIntEnv(t)
	u := e.addUser(t, "budi@example.org")
	_ = e.newSession(t, u.ID)
	raw := e.requestToken(t, "budi@example.org")
	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = e.svc.Complete(tctx(), raw, goodPass)
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, reset.ErrInvalidToken):
		default:
			if _, limited := guard.AsRateLimited(err); !limited {
				t.Errorf("galat tak terduga: %v", err)
			}
		}
	}
	if wins != 1 {
		t.Errorf("sukses = %d, want tepat 1", wins)
	}
}

func TestIntegrationExpiredAndWeakPassword(t *testing.T) {
	e := newIntEnv(t)
	e.addUser(t, "budi@example.org")
	raw := e.requestToken(t, "budi@example.org")
	// Sandi lemah: ditolak, tautan tidak terpakai.
	var pe *reset.PolicyError
	if err := e.svc.Complete(tctx(), raw, "password1234"); !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *PolicyError", err)
	}
	// Kedaluwarsa menurut jam: ditolak.
	e.clk.Advance(30*time.Minute + time.Second)
	if err := e.svc.Complete(tctx(), raw, goodPass); !errors.Is(err, reset.ErrInvalidToken) {
		t.Fatalf("kedaluwarsa: err = %v", err)
	}
	// Permintaan baru menerbitkan tautan yang berlaku; sandi lemah sebelumnya tidak mengonsumsinya.
	raw2 := e.requestToken(t, "budi@example.org")
	if err := e.svc.Complete(tctx(), "pendek", "x"); !errors.Is(err, reset.ErrInvalidToken) && !errors.As(err, &pe) {
		t.Fatalf("token cacat: %v", err)
	}
	if err := e.svc.Complete(tctx(), raw2, goodPass); err != nil {
		t.Fatalf("tautan baru: %v", err)
	}
}

func TestIntegrationNewRequestInvalidatesOldLinkAndUnknownEmailLeavesNoTrace(t *testing.T) {
	e := newIntEnv(t)
	e.addUser(t, "budi@example.org")
	first := e.requestToken(t, "budi@example.org")
	e.clk.Advance(time.Minute)
	second := e.requestToken(t, "budi@example.org")
	if err := e.svc.Complete(tctx(), first, goodPass); !errors.Is(err, reset.ErrInvalidToken) {
		t.Errorf("tautan lama: %v", err)
	}
	if err := e.svc.Complete(tctx(), second, goodPass); err != nil {
		t.Errorf("tautan baru: %v", err)
	}
	if err := e.svc.Request(ipCtx("198.51.100.8"), "tidak-ada@example.org", ""); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	if n := len(e.sender.messages()); n != 2 {
		t.Errorf("surat = %d, want 2 (tak ada surat untuk email tak dikenal)", n)
	}
}
