//go:build integration

package token_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

// storeAdapter memetakan *store.Store ke token.KeyStore (adaptor sebenarnya dipasang perakit aplikasi).
type storeAdapter struct{ s *store.Store }

func (a storeAdapter) ListSigningKeys(ctx context.Context) ([]token.KeyRecord, error) {
	ks, err := a.s.ListSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]token.KeyRecord, 0, len(ks))
	for _, k := range ks {
		out = append(out, token.KeyRecord{ID: k.ID, Status: k.Status, PublicKey: k.PublicKey, PrivateKeySealed: k.PrivateKeySealed,
			CreatedAt: k.CreatedAt, ActivatedAt: k.ActivatedAt, RetiringAt: k.RetiringAt, RetiredAt: k.RetiredAt})
	}
	return out, nil
}

func (a storeAdapter) InsertSigningKey(ctx context.Context, k token.KeyRecord) error {
	return a.s.InsertSigningKey(ctx, store.SigningKey{ID: k.ID, Status: k.Status, PublicKey: k.PublicKey, PrivateKeySealed: k.PrivateKeySealed,
		CreatedAt: k.CreatedAt, ActivatedAt: k.ActivatedAt, RetiringAt: k.RetiringAt, RetiredAt: k.RetiredAt})
}

func (a storeAdapter) TransitionSigningKey(ctx context.Context, id, from, to string, at time.Time) (bool, error) {
	return a.s.TransitionSigningKey(ctx, id, from, to, at)
}

func TestKeyringWithRealStore(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.Start(t)
	db, err := database.Open(ctx, config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	ks := storeAdapter{s: store.New(database.ControlDB{DB: db}, nil)}

	m := make([]byte, crypto.KeySize)
	_, _ = rand.Read(m)
	sealer, err := crypto.NewSealerFromMaterial(m)
	if err != nil {
		t.Fatal(err)
	}
	var clkMu sync.Mutex
	now := time.Now().UTC().Truncate(time.Microsecond)
	clock := func() time.Time { clkMu.Lock(); defer clkMu.Unlock(); return now }
	advance := func(d time.Duration) { clkMu.Lock(); now = now.Add(d); clkMu.Unlock() }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mkRing := func() *token.Keyring {
		r, err := token.NewKeyring(token.KeyringConfig{Store: ks, Sealer: sealer, Now: clock, Log: log})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	ringA, ringB := mkRing(), mkRing()
	if created, err := ringA.EnsureActive(ctx); err != nil || !created {
		t.Fatalf("EnsureActive: %v %v", created, err)
	}
	if err := ringB.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	issuer, _ := token.NewIssuer(token.IssuerConfig{Keyring: ringA, Issuer: "iss", Audience: "aud", Now: clock})
	verifierB, _ := token.NewVerifier(token.VerifierConfig{Keyring: ringB, Issuer: "iss", Audience: "aud", Now: clock})
	oldTok, err := issuer.Issue(token.Claims{Subject: "u", SessionID: "s", TenantID: "t", AMR: []string{"pwd"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifierB.Verify(oldTok); err != nil {
		t.Fatalf("instance B: %v", err)
	}
	oldKid := ringA.Active().ID

	pend, err := ringA.Rotate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pend.Status != token.StatusPending {
		t.Fatalf("status = %s", pend.Status)
	}
	// kolom privat tersegel di database sungguhan
	var row struct{ PrivateKeySealed []byte }
	if err := db.Raw(`SELECT private_key_sealed FROM signing_keys WHERE kid = ?`, pend.ID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	sealed := row.PrivateKeySealed
	seed, err := sealer.Open(sealed, []byte("signing_keys|private|"+pend.ID))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, seed) || bytes.Contains(sealed, ed25519.NewKeyFromSeed(seed)) {
		t.Fatal("kolom privat memuat kunci mentah")
	}

	// dua instance men-tick serentak: tepat satu pemenang promosi; hasil akhir 1 active + 1 retiring
	advance(token.JWKSMaxAge)
	pa, _ := token.NewPromoter(token.PromoterConfig{Keyring: ringA, Now: clock, Log: log})
	pb, _ := token.NewPromoter(token.PromoterConfig{Keyring: ringB, Now: clock, Log: log})
	var wg sync.WaitGroup
	for _, p := range []*token.Promoter{pa, pb, pa, pb} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Tick(ctx, clock()); err != nil {
				t.Errorf("Tick: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := ringB.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if ringA.Active().ID != pend.ID || ringB.Active().ID != pend.ID {
		t.Fatalf("kunci aktif: A=%s B=%s ingin %s", ringA.Active().ID, ringB.Active().ID, pend.ID)
	}
	statuses := map[string]string{}
	for _, k := range ringB.Keys() {
		statuses[k.ID] = k.Status
	}
	if statuses[pend.ID] != token.StatusActive || statuses[oldKid] != token.StatusRetiring || len(statuses) != 2 {
		t.Fatalf("status: %v", statuses)
	}
	if _, err := verifierB.Verify(oldTok); err != nil {
		t.Fatalf("token lama harus valid saat retiring: %v", err)
	}
	// pensiun setelah umur token + leeway
	advance(token.RetireAfter)
	if err := pa.Tick(ctx, clock()); err != nil {
		t.Fatal(err)
	}
	if err := ringB.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	for _, k := range ringB.Keys() {
		if k.ID == oldKid && k.Status != token.StatusRetired {
			t.Fatalf("status kunci lama = %s", k.Status)
		}
	}
}
