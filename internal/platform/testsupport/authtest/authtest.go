// Package authtest menerbitkan access token uji untuk tes yang melewati rantai HTTP platform dengan autentikasi
// nyata (PRD-001 C1.10): tes in-process (internal/app) dan tes biner (tests/integration). Hanya dipakai dari file
// _test.go; token yang diterbitkan sah di mata Authenticate (tanda tangan, iss, aud, tid) sehingga tidak ada jalur
// pintas autentikasi di kode produksi.
//
// Principal uji adalah pengguna fiktif (UUID acak) tanpa baris di database: tidak punya faktor MFA dan bukan
// administrator, sehingga lolos aturan MFA ADR-0013 §4 dan keputusan permission ditentukan stub (ERP_AUTHZ_STUB).
// Tes yang membutuhkan pengguna, sesi, atau MFA sungguhan membuat datanya lewat store.
package authtest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/secretkey"
)

// Mint menerbitkan access token uji (amr ["pwd"], tanpa scope) untuk pengguna fiktif baru pada tenant tid.
func Mint(t testing.TB, issuer *token.Issuer, tid tenancy.TenantID) string {
	t.Helper()
	raw, err := issuer.Issue(token.Claims{
		Subject: uuid.NewString(), SessionID: uuid.NewString(), TenantID: tid.String(), AMR: []string{"pwd"},
	})
	if err != nil {
		t.Fatalf("authtest: terbitkan token: %v", err)
	}
	return raw
}

// Authed membungkus h: setiap request tanpa header Authorization diberi token baru (token sudah sah per request, jadi
// tes panjang tidak kedaluwarsa). Request yang sudah membawa Authorization dibiarkan, sehingga tes 401 dan token
// buruk tetap dapat dilakukan lewat handler yang sama.
func Authed(t testing.TB, h http.Handler, issuer *token.Issuer, tid tenancy.TenantID) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+Mint(t, issuer, tid))
		}
		h.ServeHTTP(w, r)
	})
}

// BearerFromDB menerbitkan token uji untuk proses `serve` (biner) yang memakai database dsn dan ERP_SECRET_KEY uji
// proses ini (testsupport/secretkey). Kunci penandatangan dibaca dari tabel signing_keys (server sudah membuat kunci
// aktif pertama saat start), jadi panggil setelah server siap. Server mengenali kid-nya lewat keyring-nya sendiri.
func BearerFromDB(t testing.TB, dsn string) string {
	t.Helper()
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(ctx, config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, tnoop.NewTracerProvider(), log)
	if err != nil {
		t.Fatalf("authtest: buka database: %v", err)
	}
	defer func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}()
	id, err := migrate.DefaultTenantID(ctx, db)
	if err != nil {
		t.Fatalf("authtest: tenant default: %v", err)
	}
	tid := tenancy.TenantID(id)
	sealer, err := crypto.NewSealerFromMaterial(secretkey.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	st := identitystore.New(database.ControlDB{DB: db}, database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}})
	kr, err := token.NewKeyring(token.KeyringConfig{Store: keyStore{st}, Sealer: sealer, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	defer kr.Close()
	if err := kr.Reload(ctx); err != nil {
		t.Fatalf("authtest: muat kunci: %v", err)
	}
	if kr.Active().IsZero() {
		t.Fatal("authtest: tidak ada kunci penandatangan aktif; server belum menyiapkannya")
	}
	issuer, err := token.NewIssuer(token.IssuerConfig{Keyring: kr, Issuer: identity.TokenIssuer, Audience: identity.TokenAudience})
	if err != nil {
		t.Fatal(err)
	}
	return Mint(t, issuer, tid)
}

// keyStore memetakan *store.Store ke token.KeyStore (salinan adaptor internal/app; tes tidak mengimpor internal/app
// karena tes internal/app memakai paket ini).
type keyStore struct{ st *identitystore.Store }

func (a keyStore) ListSigningKeys(ctx context.Context) ([]token.KeyRecord, error) {
	keys, err := a.st.ListSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]token.KeyRecord, len(keys))
	for i, k := range keys {
		out[i] = token.KeyRecord{
			ID: k.ID, Status: k.Status, PublicKey: k.PublicKey, PrivateKeySealed: k.PrivateKeySealed,
			CreatedAt: k.CreatedAt, ActivatedAt: k.ActivatedAt, RetiringAt: k.RetiringAt, RetiredAt: k.RetiredAt,
		}
	}
	return out, nil
}

func (a keyStore) InsertSigningKey(ctx context.Context, k token.KeyRecord) error {
	return a.st.InsertSigningKey(ctx, identitystore.SigningKey{
		ID: k.ID, Status: k.Status, PublicKey: k.PublicKey, PrivateKeySealed: k.PrivateKeySealed,
		CreatedAt: k.CreatedAt, ActivatedAt: k.ActivatedAt, RetiringAt: k.RetiringAt, RetiredAt: k.RetiredAt,
	})
}

func (a keyStore) TransitionSigningKey(ctx context.Context, id, from, to string, at time.Time) (bool, error) {
	return a.st.TransitionSigningKey(ctx, id, from, to, at)
}
