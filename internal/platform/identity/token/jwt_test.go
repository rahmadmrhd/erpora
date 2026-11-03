package token

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Vektor uji RFC 8032 §7.1 (TEST 1 s.d. TEST 3), diambil dari teks RFC.
var rfc8032Vectors = []struct{ seed, pub, msg, sig string }{
	{
		"9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60",
		"d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a",
		"",
		"e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b",
	},
	{
		"4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb",
		"3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c",
		"72",
		"92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00",
	},
	{
		"c5aa8df43f9f837bedb7442f31dcb7b166d38535076f094b85ce3a2e0b4458f7",
		"fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025",
		"af82",
		"6291d657deec24024827e69c3abe01a30ce548a284743a445e3680d7db5ac3ac18ff9b538d16f290ae67f760984dc6594a7c15e9716ed28dc027beceea1ec40a",
	},
}

func TestEd25519RFC8032Vectors(t *testing.T) {
	for i, v := range rfc8032Vectors {
		priv := ed25519.NewKeyFromSeed(mustHex(t, v.seed))
		if got := priv.Public().(ed25519.PublicKey); string(got) != string(mustHex(t, v.pub)) {
			t.Fatalf("vektor %d: kunci publik salah", i+1)
		}
		msg := mustHex(t, v.msg)
		if sig := ed25519.Sign(priv, msg); string(sig) != string(mustHex(t, v.sig)) {
			t.Fatalf("vektor %d: tanda tangan salah", i+1)
		}
		if !ed25519.Verify(mustHex(t, v.pub), msg, mustHex(t, v.sig)) {
			t.Fatalf("vektor %d: verifikasi gagal", i+1)
		}
	}
}

// Kunci yang dimuat keyring dari seed vektor RFC 8032 harus berkunci publik sama dan menghasilkan token yang
// tanda tangannya diverifikasi kunci publik vektor.
func TestKeyringLoadsRFC8032VectorKey(t *testing.T) {
	e := newBareEnv(t)
	v := rfc8032Vectors[0]
	seed := mustHex(t, v.seed)
	kid := "rfc8032-test-vector-1"
	sealed, err := e.sealer.Seal(seed, sealAAD(kid))
	if err != nil {
		t.Fatal(err)
	}
	now := t0
	if err := e.store.InsertSigningKey(context.Background(), KeyRecord{ID: kid, Status: StatusActive, PublicKey: mustHex(t, v.pub), PrivateKeySealed: sealed, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := e.ring.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.ring.Active(); got.ID != kid || string(got.Public) != string(mustHex(t, v.pub)) {
		t.Fatalf("kunci aktif salah: %v", got)
	}
	tok := e.issue()
	p := strings.Split(tok, ".")
	sig, _ := b64.DecodeString(p[2])
	if !ed25519.Verify(mustHex(t, v.pub), []byte(p[0]+"."+p[1]), sig) {
		t.Fatal("tanda tangan token tidak diverifikasi kunci publik vektor")
	}
	e.mustVerify(tok)
}

func TestIssueVerifyRoundTrip(t *testing.T) {
	e := newEnv(t)
	tok := e.issue()
	c := e.mustVerify(tok)
	if c.Issuer != testIssuer || c.Audience != testAudience || c.Subject != "user-1" || c.SessionID != "sess-1" ||
		c.TenantID != "tenant-1" || c.PermRev != 7 || c.ID == "" {
		t.Fatalf("klaim tidak sesuai: %+v", c)
	}
	if strings.Join(c.AMR, ",") != "pwd,otp" || strings.Join(c.Scope, ",") != "read,write" {
		t.Fatalf("amr/scope salah: %+v", c)
	}
	if !c.IssuedAt.Equal(t0) || c.ExpiresAt.Sub(c.IssuedAt) != AccessTokenTTL {
		t.Fatalf("umur token harus 10 menit: iat=%v exp=%v", c.IssuedAt, c.ExpiresAt)
	}
	// header dan daftar klaim persis; daftar permission tidak masuk token
	p := strings.Split(tok, ".")
	hb, _ := b64.DecodeString(p[0])
	if string(hb) != `{"alg":"EdDSA","kid":"`+e.activeKid()+`"}` {
		t.Fatalf("header: %s", hb)
	}
	pb, _ := b64.DecodeString(p[1])
	var m map[string]json.RawMessage
	if err := json.Unmarshal(pb, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"iss", "sub", "sid", "tid", "aud", "exp", "iat", "jti", "perm_rev", "amr", "scope"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("klaim %s hilang", k)
		}
		delete(m, k)
	}
	if len(m) != 0 {
		t.Fatalf("klaim tak terduga: %v", m)
	}
	if tok2 := e.issue(); tok2 == tok {
		t.Fatal("jti harus unik")
	}
}

func TestIssueWithoutScopeOmitsClaimAndAMREmptyIsArray(t *testing.T) {
	e := newEnv(t)
	tok, err := e.issuer.Issue(Claims{Subject: "u", SessionID: "s", TenantID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	pb, _ := b64.DecodeString(strings.Split(tok, ".")[1])
	if strings.Contains(string(pb), `"scope"`) || !strings.Contains(string(pb), `"amr":[]`) {
		t.Fatalf("payload: %s", pb)
	}
	if c := e.mustVerify(tok); len(c.Scope) != 0 {
		t.Fatalf("%+v", c)
	}
}

func TestIssueRejectsBadInput(t *testing.T) {
	e := newEnv(t)
	for name, c := range map[string]Claims{
		"tanpa sub":      {SessionID: "s", TenantID: "t"},
		"tanpa sid":      {Subject: "u", TenantID: "t"},
		"tanpa tid":      {Subject: "u", SessionID: "s"},
		"scope berspasi": {Subject: "u", SessionID: "s", TenantID: "t", Scope: []string{"a b"}},
		"perm_rev minus": {Subject: "u", SessionID: "s", TenantID: "t", PermRev: -1},
		"lebih dari 4KB": {Subject: strings.Repeat("x", 4096), SessionID: "s", TenantID: "t"},
	} {
		if _, err := e.issuer.Issue(c); err == nil {
			t.Fatalf("%s: harus ditolak", name)
		}
	}
}

func TestIssueWithoutActiveKeyFails(t *testing.T) {
	e := newBareEnv(t)
	if _, err := e.issuer.Issue(Claims{Subject: "u", SessionID: "s", TenantID: "t"}); !errors.Is(err, ErrNoActiveKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyRejectsNoneAlg(t *testing.T) {
	e := newEnv(t)
	kid := e.activeKid()
	claims := e.goodClaims(nil)
	cb, _ := json.Marshal(claims)
	for _, alg := range []string{"none", "None", "NONE", "", "HS256", "RS256", "ES256", "eddsa", "EdDSA "} {
		hb, _ := json.Marshal(map[string]any{"alg": alg, "kid": kid})
		in := b64.EncodeToString(hb) + "." + b64.EncodeToString(cb)
		// dengan tanda tangan acak 64 byte, dan dengan tanda tangan kosong
		e.wantReject(in+"."+b64.EncodeToString(make([]byte, 64)), ReasonAlg)
		e.wantReject(in+".", ReasonMalformed)
		e.wantReject(in, ReasonMalformed)
	}
	// header tanpa alg sama sekali
	hb := `{"kid":"` + kid + `"}`
	e.wantReject(forge(t, e.ring.Active().priv, hb, claims), ReasonAlg)
}

func TestVerifyRejectsHS256WithPublicKeyAsSecret(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	hb, _ := json.Marshal(map[string]any{"alg": "HS256", "kid": k.ID})
	cb, _ := json.Marshal(e.goodClaims(nil))
	in := b64.EncodeToString(hb) + "." + b64.EncodeToString(cb)
	for _, secret := range map[string][]byte{
		"bytes mentah": k.Public,
		"x base64url":  []byte(b64.EncodeToString(k.Public)),
	} {
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(in))
		e.wantReject(in+"."+b64.EncodeToString(mac.Sum(nil)), ReasonAlg)
	}
	// juga: alg EdDSA tetapi tanda tangan 32 byte HMAC
	hb2, _ := json.Marshal(map[string]any{"alg": "EdDSA", "kid": k.ID})
	in2 := b64.EncodeToString(hb2) + "." + b64.EncodeToString(cb)
	mac := hmac.New(sha256.New, k.Public)
	mac.Write([]byte(in2))
	e.wantReject(in2+"."+b64.EncodeToString(mac.Sum(nil)), ReasonSignature)
}

func TestVerifyRejectsUnknownKID(t *testing.T) {
	e := newEnv(t)
	_, otherPriv, _ := ed25519.GenerateKey(nil)
	tok := forge(t, otherPriv, map[string]any{"alg": "EdDSA", "kid": "kid-yang-tidak-ada"}, e.goodClaims(nil))
	e.wantReject(tok, ReasonUnknownKID)
	// kid sah tetapi tanda tangan dari kunci lain
	tok = forge(t, otherPriv, map[string]any{"alg": "EdDSA", "kid": e.activeKid()}, e.goodClaims(nil))
	e.wantReject(tok, ReasonSignature)
	// kid wajib
	tok = forge(t, e.ring.Active().priv, map[string]any{"alg": "EdDSA"}, e.goodClaims(nil))
	e.wantReject(tok, ReasonKID)
}

func TestVerifyRejectsKIDWithPathOrSQL(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	before := e.store.listCalls.Load()
	for _, kid := range []string{
		"../../etc/passwd", "..%2f..%2fetc%2fpasswd", "x' OR '1'='1", "kid'; DROP TABLE signing_keys;--", "a/b/c/d/e/f/g/h",
		"kid.with.dots.xx", "kid:colon:xx", "kid=pad==xx", "short", "", strings.Repeat("a", 65), "kid\x00nullbyte1", "kid with space",
	} {
		tok := forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": kid}, e.goodClaims(nil))
		e.wantReject(tok, ReasonKID)
	}
	if e.store.listCalls.Load() != before {
		t.Fatal("kid berbentuk tak sah tidak boleh memicu muat ulang dari store")
	}
	// kid bukan string
	for _, hdr := range []string{`{"alg":"EdDSA","kid":123}`, `{"alg":"EdDSA","kid":["a"]}`, `{"alg":"EdDSA","kid":null}`} {
		e.wantReject(forge(t, k.priv, hdr, e.goodClaims(nil)), ReasonKID)
	}
}

func TestVerifyRejectsWrongIssuerAudience(t *testing.T) {
	e := newEnv(t)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["iss"] = "https://evil.test" }), ReasonIssuer)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["iss"] = testIssuer + "/" }), ReasonIssuer)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { delete(m, "iss") }), ReasonClaims)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["aud"] = "api-lain" }), ReasonAudience)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["aud"] = []string{testAudience} }), ReasonClaims)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { delete(m, "aud") }), ReasonClaims)
	// bentuk benar tetap lolos (kontrol positif)
	e.mustVerify(e.forgeActive(nil, nil))
}

func TestVerifyExpiryEdgeWithLeeway(t *testing.T) {
	e := newEnv(t)
	tok := e.issue()
	exp := t0.Add(AccessTokenTTL)
	cases := []struct {
		at time.Time
		ok bool
	}{
		{t0, true},
		{exp, true},
		{exp.Add(Leeway - time.Second), true},
		{exp.Add(Leeway - time.Nanosecond), true},
		{exp.Add(Leeway), false},
		{exp.Add(Leeway + time.Second), false},
	}
	for _, c := range cases {
		e.clk.Set(c.at)
		_, err := e.verifier.Verify(tok)
		if c.ok != (err == nil) {
			t.Fatalf("pada %v: err=%v, ingin ok=%v", c.at.Sub(exp), err, c.ok)
		}
		if !c.ok && (!errors.Is(err, ErrTokenExpired) || !errors.Is(err, ErrInvalidToken)) {
			t.Fatalf("harus ErrTokenExpired dan ErrInvalidToken: %v", err)
		}
	}
	// ErrTokenExpired bukan oracle: token kedaluwarsa dengan tanda tangan salah tetap generik tanpa ErrTokenExpired
	e.clk.Set(exp.Add(time.Hour))
	_, otherPriv, _ := ed25519.GenerateKey(nil)
	bad := forge(t, otherPriv, map[string]any{"alg": "EdDSA", "kid": e.activeKid()}, e.goodClaims(func(m map[string]any) {
		m["iat"], m["exp"] = t0.Unix(), t0.Add(AccessTokenTTL).Unix()
	}))
	if _, err := e.verifier.Verify(bad); errors.Is(err, ErrTokenExpired) {
		t.Fatal("token tanda tangan salah tidak boleh dilaporkan kedaluwarsa")
	}
}

func TestVerifyIssuedAtInFuture(t *testing.T) {
	e := newEnv(t)
	e.mustVerify(e.forgeActive(nil, func(m map[string]any) {
		m["iat"] = t0.Add(Leeway).Unix()
		m["exp"] = t0.Add(Leeway + AccessTokenTTL).Unix()
	}))
	e.wantReject(e.forgeActive(nil, func(m map[string]any) {
		m["iat"] = t0.Add(Leeway + time.Second).Unix()
		m["exp"] = t0.Add(Leeway + time.Second + AccessTokenTTL).Unix()
	}), ReasonNotYetValid)
}

func TestVerifyRejectsOverlongLifetimeAndInvertedTimes(t *testing.T) {
	e := newEnv(t)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["exp"] = t0.Add(AccessTokenTTL + time.Second).Unix() }), ReasonClaims)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["exp"] = t0.Unix() }), ReasonClaims)
	e.wantReject(e.forgeActive(nil, func(m map[string]any) { m["exp"] = t0.Add(-time.Minute).Unix() }), ReasonClaims)
}

func TestVerifyMalformedInputs(t *testing.T) {
	e := newEnv(t)
	good := e.issue()
	p := strings.Split(good, ".")
	sig, _ := b64.DecodeString(p[2])
	other := b64.EncodeToString(make([]byte, 64))
	enc := func(s string) string { return b64.EncodeToString([]byte(s)) }

	cases := map[string]struct{ tok, reason string }{
		"kosong":                        {"", ReasonMalformed},
		"satu segmen":                   {p[0], ReasonMalformed},
		"dua segmen":                    {p[0] + "." + p[1], ReasonMalformed},
		"empat segmen":                  {good + ".x", ReasonMalformed},
		"segmen kosong":                 {p[0] + ".." + p[2], ReasonMalformed},
		"tanda tangan dipotong":         {p[0] + "." + p[1] + "." + b64.EncodeToString(sig[:63]), ReasonSignature},
		"tanda tangan lebih panjang":    {p[0] + "." + p[1] + "." + b64.EncodeToString(append(append([]byte{}, sig...), 0)), ReasonSignature},
		"tanda tangan diganti":          {p[0] + "." + p[1] + "." + other, ReasonSignature},
		"tanda tangan satu bit berubah": {p[0] + "." + p[1] + "." + b64.EncodeToString(append([]byte{sig[0] ^ 1}, sig[1:]...)), ReasonSignature},
		"payload diganti":               {p[0] + "." + enc(`{"sub":"admin"}`) + "." + p[2], ReasonSignature},
		"base64 cacat header":           {"!!!!." + p[1] + "." + p[2], ReasonMalformed},
		"base64 berpadding":             {p[0] + "=." + p[1] + "." + p[2], ReasonMalformed},
		"base64 standar (+ /)":          {strings.ReplaceAll(p[0], "-", "+") + "+/." + p[1] + "." + p[2], ReasonMalformed},
		"header bukan JSON":             {enc("bukan json") + "." + p[1] + "." + p[2], ReasonMalformed},
		"header array":                  {enc(`["EdDSA"]`) + "." + p[1] + "." + p[2], ReasonMalformed},
		"header sisa data":              {enc(`{"alg":"EdDSA"}{}`) + "." + p[1] + "." + p[2], ReasonMalformed},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { e.wantReject(c.tok, c.reason) })
	}

	// payload yang ditandatangani sah tetapi bukan klaim yang valid
	k := e.ring.Active()
	hdr := map[string]any{"alg": "EdDSA", "kid": k.ID}
	e.wantReject(forge(t, k.priv, hdr, "bukan json"), ReasonClaims)
	e.wantReject(forge(t, k.priv, hdr, `["a"]`), ReasonClaims)
	e.wantReject(forge(t, k.priv, hdr, `{"iss":"x"}{}`), ReasonClaims)
	e.wantReject(forge(t, k.priv, hdr, `null`), ReasonClaims)
}

func TestVerifyRejectsOversizedToken(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	big := e.goodClaims(func(m map[string]any) { m["pad"] = strings.Repeat("a", MaxTokenSize) })
	tok := forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": k.ID}, big)
	if len(tok) <= MaxTokenSize {
		t.Fatal("token uji harus melewati 4 KB")
	}
	e.wantReject(tok, ReasonSize)
	// tepat di batas diterima bila sah
	small := e.goodClaims(func(m map[string]any) { m["pad"] = strings.Repeat("a", 100) })
	e.mustVerify(forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": k.ID}, small))
	e.wantReject(strings.Repeat("a", MaxTokenSize+1), ReasonSize)
}

func TestVerifyRejectsExtraOrDangerousHeaderFields(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	for name, extra := range map[string]map[string]any{
		"crit":    {"crit": []string{"exp"}},
		"jku":     {"jku": "https://evil.test/jwks.json"},
		"jwk":     {"jwk": map[string]any{"kty": "OKP", "crv": "Ed25519", "x": "AAAA"}},
		"x5u":     {"x5u": "https://evil.test/cert"},
		"x5c":     {"x5c": []string{"AAAA"}},
		"cty":     {"cty": "JWT"},
		"b64":     {"b64": false},
		"lainnya": {"foo": "bar"},
	} {
		t.Run(name, func(t *testing.T) {
			h := map[string]any{"alg": "EdDSA", "kid": k.ID}
			for kk, vv := range extra {
				h[kk] = vv
			}
			e.wantReject(forge(t, k.priv, h, e.goodClaims(nil)), ReasonMalformed)
		})
	}
	// typ boleh bila "JWT"
	e.mustVerify(forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": k.ID, "typ": "JWT"}, e.goodClaims(nil)))
	e.wantReject(forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": k.ID, "typ": "evil"}, e.goodClaims(nil)), ReasonMalformed)
}

// encoding/json mencocokkan nama tanpa memandang huruf dan memakai nilai terakhir; parser ketat menolak keduanya.
func TestVerifyRejectsCaseVariantAndDuplicateKeys(t *testing.T) {
	e := newEnv(t)
	k := e.ring.Active()
	kid := k.ID
	for name, hdr := range map[string]string{
		"ALG besar":    `{"alg":"EdDSA","ALG":"none","kid":"` + kid + `"}`,
		"alg duplikat": `{"alg":"none","alg":"EdDSA","kid":"` + kid + `"}`,
		"Kid kapital":  `{"alg":"EdDSA","kid":"` + kid + `","Kid":"../x"}`,
		"kid duplikat": `{"alg":"EdDSA","kid":"../x","kid":"` + kid + `"}`,
		"hanya Alg":    `{"Alg":"EdDSA","kid":"` + kid + `"}`,
	} {
		t.Run(name, func(t *testing.T) { e.wantReject(forge(t, k.priv, hdr, e.goodClaims(nil)), "") })
	}
	good := e.goodClaims(nil)
	gb, _ := json.Marshal(good)
	body := string(gb)
	for name, claims := range map[string]string{
		"sub duplikat": strings.Replace(body, `"sub":"user-1"`, `"sub":"user-1","sub":"admin"`, 1),
		"Sub kapital":  strings.Replace(body, `"sub":"user-1"`, `"sub":"user-1","Sub":"admin"`, 1),
		"EXP kapital":  strings.Replace(body, `"exp":`, `"EXP":99999999999,"exp":`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if claims == body {
				t.Fatal("penggantian gagal")
			}
			e.wantReject(forge(t, k.priv, map[string]any{"alg": "EdDSA", "kid": kid}, claims), ReasonClaims)
		})
	}
}

func TestVerifyRejectsWrongClaimTypes(t *testing.T) {
	e := newEnv(t)
	now := t0
	exp := now.Add(AccessTokenTTL).Unix()
	for name, mut := range map[string]func(map[string]any){
		"exp string":       func(m map[string]any) { m["exp"] = "9999999999" },
		"exp float":        func(m map[string]any) { m["exp"] = float64(exp) + 0.5 },
		"exp null":         func(m map[string]any) { m["exp"] = nil },
		"exp bool":         func(m map[string]any) { m["exp"] = true },
		"exp array":        func(m map[string]any) { m["exp"] = []int64{exp} },
		"iat string":       func(m map[string]any) { m["iat"] = "1" },
		"iat hilang":       func(m map[string]any) { delete(m, "iat") },
		"exp hilang":       func(m map[string]any) { delete(m, "exp") },
		"sub angka":        func(m map[string]any) { m["sub"] = 5 },
		"sub kosong":       func(m map[string]any) { m["sub"] = "" },
		"sid hilang":       func(m map[string]any) { delete(m, "sid") },
		"tid kosong":       func(m map[string]any) { m["tid"] = "" },
		"jti hilang":       func(m map[string]any) { delete(m, "jti") },
		"perm_rev string":  func(m map[string]any) { m["perm_rev"] = "1" },
		"perm_rev negatif": func(m map[string]any) { m["perm_rev"] = -1 },
		"perm_rev hilang":  func(m map[string]any) { delete(m, "perm_rev") },
		"amr string":       func(m map[string]any) { m["amr"] = "pwd" },
		"amr hilang":       func(m map[string]any) { delete(m, "amr") },
		"amr elemen angka": func(m map[string]any) { m["amr"] = []any{1} },
		"amr null":         func(m map[string]any) { m["amr"] = nil },
		"scope array":      func(m map[string]any) { m["scope"] = []string{"a"} },
	} {
		t.Run(name, func(t *testing.T) { e.wantReject(e.forgeActive(nil, mut), ReasonClaims) })
	}
}

func TestVerifyRejectsRetiredKey(t *testing.T) {
	e := newEnv(t)
	tok := e.issue()
	oldKid := e.activeKid()
	if _, err := e.ring.Rotate(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(JWKSMaxAge)
	e.tick() // baru aktif, lama retiring
	e.mustVerify(tok)
	e.clk.Advance(RetireAfter)
	e.tick()
	if got := e.store.statuses()[oldKid]; got != StatusRetired {
		t.Fatalf("status kunci lama = %s", got)
	}
	// token kembali "valid" menurut waktu (jam diputar mundur) tetap ditolak karena kuncinya retired
	e.clk.Set(t0.Add(time.Minute))
	e.wantReject(tok, ReasonKeyStatus)
}

func TestVerifyRejectsPendingKey(t *testing.T) {
	e := newEnv(t)
	pend, err := e.ring.Rotate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	priv := ed25519.NewKeyFromSeed(e.seedOf(pend.ID))
	tok := forge(t, priv, map[string]any{"alg": "EdDSA", "kid": pend.ID}, e.goodClaims(nil))
	e.wantReject(tok, ReasonKeyStatus)
}

func TestPrivateAndPublicKeyNotLeakedByVerifyError(t *testing.T) {
	e := newEnv(t)
	_, err := e.verifier.Verify("a.b.c")
	if err == nil || strings.Contains(err.Error(), e.activeKid()) {
		t.Fatalf("err = %v", err)
	}
}
