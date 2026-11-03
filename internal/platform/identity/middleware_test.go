package identity

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
)

const (
	testIss = "erpora-test"
	testAud = "erpora-api"
)

var testTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

// memKeys adalah KeyStore memori minimal (cukup untuk EnsureActive dan Reload).
type memKeys struct {
	mu   sync.Mutex
	recs []token.KeyRecord
}

func (m *memKeys) ListSigningKeys(context.Context) ([]token.KeyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]token.KeyRecord(nil), m.recs...), nil
}

func (m *memKeys) InsertSigningKey(_ context.Context, k token.KeyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append(m.recs, k)
	return nil
}

func (m *memKeys) TransitionSigningKey(context.Context, string, string, string, time.Time) (bool, error) {
	return false, nil
}

type revoked map[string]bool

func (r revoked) IsRevoked(sid string) bool { return r[sid] }

type authFixture struct {
	keyring  *token.Keyring
	issuer   *token.Issuer
	verifier *token.Verifier
	revoked  revoked
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 1)
	}
	sealer, err := crypto.NewSealerFromMaterial(mat)
	if err != nil {
		t.Fatal(err)
	}
	kr, err := token.NewKeyring(token.KeyringConfig{Store: &memKeys{}, Sealer: sealer})
	if err != nil {
		t.Fatal(err)
	}
	if err := kr.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.EnsureActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	iss, err := token.NewIssuer(token.IssuerConfig{Keyring: kr, Issuer: testIss, Audience: testAud})
	if err != nil {
		t.Fatal(err)
	}
	ver, err := token.NewVerifier(token.VerifierConfig{Keyring: kr, Issuer: testIss, Audience: testAud})
	if err != nil {
		t.Fatal(err)
	}
	return &authFixture{keyring: kr, issuer: iss, verifier: ver, revoked: revoked{}}
}

func (f *authFixture) claims() token.Claims {
	return token.Claims{
		Subject: "0190a1b2-0000-7e5f-8a9b-0c1d2e3f4a01", SessionID: "0190a1b2-0000-7e5f-8a9b-0c1d2e3f4a02",
		TenantID: testTenant.String(), AMR: []string{"pwd"}, PermRev: 3,
	}
}

func (f *authFixture) issue(t *testing.T, c token.Claims) string {
	t.Helper()
	raw, err := f.issuer.Issue(c)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type resolverFn func(*http.Request) (tenancy.TenantID, error)

func (f resolverFn) Resolve(r *http.Request) (tenancy.TenantID, error) { return f(r) }

// serve membangun Router sungguhan (tenantContext sebelum Authenticate) dengan satu rute terproteksi dan satu publik.
func (f *authFixture) serve(t *testing.T, accepted ...string) (http.Handler, *capture) {
	t.Helper()
	c := &capture{}
	r := httpserver.NewRouter(httpserver.Deps{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:  resolverFn(func(*http.Request) (tenancy.TenantID, error) { return testTenant, nil }),
		Connector: database.SingleTenantConnector{Default: testTenant},
		Authenticate: Authenticate(AuthenticateDeps{
			Verifier: f.verifier, Revoked: f.revoked, AcceptedScopes: accepted,
			Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}),
	})
	h := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c.set(req)
		w.WriteHeader(http.StatusNoContent)
	})
	r.Handle("GET /api/private", h)
	r.Handle("GET /api/open", h, httpserver.Public())
	return r, c
}

type capture struct {
	mu   sync.Mutex
	p    sdkidentity.Principal
	has  bool
	ip   string
	hits int
}

func (c *capture) set(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.p, c.has = sdkidentity.FromContext(r.Context())
	c.ip = httpx.ClientIP(r.Context())
	c.hits++
}

func call(h http.Handler, path string, hdr http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "198.51.100.9:4000"
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func bearer(raw string) http.Header { return http.Header{"Authorization": {"Bearer " + raw}} }

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Error struct{ Code string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("body bukan amplop galat: %q", rec.Body.String())
	}
	return b.Error.Code
}

func TestAuthenticate_ValidTokenBuildsPrincipalFromTokenOnly(t *testing.T) {
	f := newAuthFixture(t)
	h, c := f.serve(t)
	cl := f.claims()
	cl.AMR = []string{"pwd", "otp"}
	hdr := bearer(f.issue(t, cl))
	// Header/identitas lain tidak pernah menjadi sumber Principal (IDOR: Principal selalu dari token).
	hdr.Set("X-User-Id", "attacker")
	hdr.Set("X-Forwarded-For", "203.0.113.50")
	rec := call(h, "/api/private", hdr)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	want := sdkidentity.Principal{
		UserID: cl.Subject, Kind: sdkidentity.KindHuman, SessionID: cl.SessionID,
		AuthMethod: sdkidentity.AuthMethodPassword, AMR: []string{"pwd", "otp"},
	}
	if c.p.UserID != want.UserID || c.p.SessionID != want.SessionID || c.p.Kind != want.Kind ||
		c.p.AuthMethod != want.AuthMethod || strings.Join(c.p.AMR, ",") != "pwd,otp" || len(c.p.TokenScope) != 0 || !c.p.MFAVerified() {
		t.Fatalf("Principal = %+v", c.p)
	}
	if c.ip != "198.51.100.9" {
		t.Errorf("IP klien = %q (harus alamat peer, bukan X-Forwarded-For)", c.ip)
	}
}

func TestAuthenticate_MissingOrMalformedHeaderIs401Unauthenticated(t *testing.T) {
	f := newAuthFixture(t)
	h, c := f.serve(t)
	good := f.issue(t, f.claims())
	cases := map[string]http.Header{
		"tanpa header":        nil,
		"skema Basic":         {"Authorization": {"Basic " + good}},
		"tanpa skema":         {"Authorization": {good}},
		"Bearer kosong":       {"Authorization": {"Bearer "}},
		"hanya Bearer":        {"Authorization": {"Bearer"}},
		"header ganda":        {"Authorization": {"Bearer " + good, "Bearer " + good}},
		"spasi di token":      {"Authorization": {"Bearer " + good + " extra"}},
		"tab di token":        {"Authorization": {"Bearer " + good + "\tx"}},
		"token raksasa":       {"Authorization": {"Bearer " + strings.Repeat("A", 5000)}},
		"sampah":              bearer("bukan.jwt.sama-sekali"),
		"dua segmen":          bearer("aaaa.bbbb"),
		"kosong antar segmen": bearer(".."),
		"non-ASCII":           bearer("tokén.a.b"),
	}
	for name, hdr := range cases {
		t.Run(name, func(t *testing.T) {
			rec := call(h, "/api/private", hdr)
			if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthenticated" {
				t.Fatalf("respons = %d %s", rec.Code, rec.Body)
			}
			if rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("WWW-Authenticate = %q", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
	if c.hits != 0 {
		t.Fatal("handler tidak boleh dipanggil")
	}
	// Skema Bearer tidak peka huruf besar-kecil (RFC 9110 11.1).
	if rec := call(h, "/api/private", http.Header{"Authorization": {"bearer " + good}}); rec.Code != http.StatusNoContent {
		t.Fatalf("skema bearer huruf kecil ditolak: %d", rec.Code)
	}
}

func TestAuthenticate_ExpiredIsTokenExpiredOnlyForOtherwiseValidToken(t *testing.T) {
	f := newAuthFixture(t)
	h, _ := f.serve(t)
	old, err := token.NewIssuer(token.IssuerConfig{
		Keyring: f.keyring, Issuer: testIss, Audience: testAud, Now: func() time.Time { return time.Now().Add(-time.Hour) },
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := old.Issue(f.claims())
	if err != nil {
		t.Fatal(err)
	}
	rec := call(h, "/api/private", bearer(raw))
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "token_expired" {
		t.Fatalf("respons = %d %s", rec.Code, rec.Body)
	}
	// Token kedaluwarsa dengan tanda tangan RUSAK tidak boleh dijawab token_expired (bukan oracle).
	parts := strings.Split(raw, ".")
	parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	rec = call(h, "/api/private", bearer(strings.Join(parts, ".")))
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthenticated" {
		t.Fatalf("token kedaluwarsa bertanda tangan palsu = %d %s", rec.Code, rec.Body)
	}
}

func TestAuthenticate_RevokedSessionTenantMismatchAndScopeAreIndistinguishable(t *testing.T) {
	f := newAuthFixture(t)
	h, c := f.serve(t)

	cl := f.claims()
	f.revoked[cl.SessionID] = true
	revokedTok := f.issue(t, cl)

	other := f.claims()
	other.TenantID = uuid.NewString()
	wrongTenant := f.issue(t, other)

	upper := f.claims()
	upper.TenantID = strings.ToUpper(testTenant.String())
	upperTenant := f.issue(t, upper)

	scoped := f.claims()
	scoped.Scope = []string{"mfa:pending"}
	scopedTok := f.issue(t, scoped)

	var bodies []string
	for name, raw := range map[string]string{
		"sid tercabut": revokedTok, "tid berbeda": wrongTenant, "tid non-kanonik": upperTenant, "scope terbatas": scopedTok,
	} {
		rec := call(h, "/api/private", bearer(raw))
		if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthenticated" {
			t.Errorf("%s: respons = %d %s", name, rec.Code, rec.Body)
		}
		bodies = append(bodies, rec.Body.String())
	}
	// Tubuh respons identik selain request id: klien tidak bisa membedakan sebab.
	norm := func(s string) string {
		var m map[string]map[string]any
		_ = json.Unmarshal([]byte(s), &m)
		delete(m["error"], "requestId")
		b, _ := json.Marshal(m)
		return string(b)
	}
	for _, b := range bodies[1:] {
		if norm(b) != norm(bodies[0]) {
			t.Errorf("tubuh galat berbeda antar sebab: %s vs %s", b, bodies[0])
		}
	}
	if c.hits != 0 {
		t.Fatal("handler tidak boleh dipanggil")
	}
}

func TestAuthenticate_AcceptedScopesPassThrough(t *testing.T) {
	f := newAuthFixture(t)
	h, c := f.serve(t, "reports:read")
	cl := f.claims()
	cl.Scope = []string{"reports:read"}
	if rec := call(h, "/api/private", bearer(f.issue(t, cl))); rec.Code != http.StatusNoContent {
		t.Fatalf("scope yang diterima ditolak: %d %s", rec.Code, rec.Body)
	}
	if len(c.p.TokenScope) != 1 || c.p.TokenScope[0] != "reports:read" {
		t.Fatalf("TokenScope = %v", c.p.TokenScope)
	}
	cl.Scope = []string{"reports:read", "admin:all"} // satu scope tak dikenal cukup untuk menolak
	if rec := call(h, "/api/private", bearer(f.issue(t, cl))); rec.Code != http.StatusUnauthorized {
		t.Fatalf("scope tak dikenal diterima: %d", rec.Code)
	}
}

func TestAuthenticate_RejectsAlgNoneAndForeignSignature(t *testing.T) {
	f := newAuthFixture(t)
	h, _ := f.serve(t)
	enc := base64.RawURLEncoding.EncodeToString
	kid := f.keyring.Active().ID
	cl := f.claims()
	payload, _ := json.Marshal(map[string]any{
		"iss": testIss, "aud": testAud, "sub": cl.Subject, "sid": cl.SessionID, "tid": cl.TenantID,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "jti": "x", "amr": []string{"pwd"},
	})
	none := enc([]byte(`{"alg":"none","kid":"`+kid+`"}`)) + "." + enc(payload) + "."
	hs := enc([]byte(`{"alg":"HS256","kid":"`+kid+`"}`)) + "." + enc(payload) + "." + enc([]byte("sig"))
	_, foreignPriv, _ := ed25519.GenerateKey(nil)
	hdr := enc([]byte(`{"alg":"EdDSA","kid":"` + kid + `"}`))
	signing := hdr + "." + enc(payload)
	foreign := signing + "." + enc(ed25519.Sign(foreignPriv, []byte(signing)))
	for name, raw := range map[string]string{"alg none": none, "HS256": hs, "kunci asing": foreign} {
		rec := call(h, "/api/private", bearer(raw))
		if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthenticated" {
			t.Errorf("%s: respons = %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestAuthenticate_PublicRouteIgnoresTokens(t *testing.T) {
	f := newAuthFixture(t)
	h, c := f.serve(t)
	for name, hdr := range map[string]http.Header{
		"tanpa token": nil, "token buruk": bearer("sampah"), "token sah": bearer(f.issue(t, f.claims())),
	} {
		c.hits, c.has = 0, false
		if rec := call(h, "/api/open", hdr); rec.Code != http.StatusNoContent {
			t.Errorf("%s: rute publik = %d", name, rec.Code)
		}
		if c.hits != 1 || c.has {
			t.Errorf("%s: rute publik tidak boleh memasang Principal (hits=%d has=%v)", name, c.hits, c.has)
		}
	}
}

func TestAuthenticate_NoTenantInContextFailsClosed(t *testing.T) {
	f := newAuthFixture(t)
	mw := Authenticate(AuthenticateDeps{Verifier: f.verifier, Revoked: f.revoked})
	called := false
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	rec := call(h, "/x", bearer(f.issue(t, f.claims())))
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatalf("tanpa tenant harus 401: %d called=%v", rec.Code, called)
	}
}

func TestAuthenticate_PanicsWithoutDependencies(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Authenticate tanpa Verifier/Revoked harus panic saat perakitan")
		}
	}()
	Authenticate(AuthenticateDeps{})
}
