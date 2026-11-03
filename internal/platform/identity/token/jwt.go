package token

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxTokenSize adalah batas ukuran token mentah (Issue menolak menerbitkan, Verify menolak menerima, lebih besar).
	MaxTokenSize = 4096
	algEdDSA     = "EdDSA"
)

// Alasan kegagalan verifikasi. HANYA untuk log/metrik server; tidak boleh sampai ke klien.
const (
	ReasonMalformed   = "malformed"
	ReasonSize        = "size"
	ReasonAlg         = "alg"
	ReasonKID         = "kid"
	ReasonUnknownKID  = "unknown_kid"
	ReasonKeyStatus   = "key_status"
	ReasonSignature   = "signature"
	ReasonClaims      = "claims"
	ReasonIssuer      = "issuer"
	ReasonAudience    = "audience"
	ReasonExpired     = "expired"
	ReasonNotYetValid = "not_yet_valid"
)

var (
	// ErrInvalidToken adalah galat generik semua kegagalan verifikasi; klien tidak boleh dibedakan sebabnya
	// (Review Focus 3). Gunakan errors.Is(err, ErrInvalidToken).
	ErrInvalidToken = errors.New("token: token tidak valid")
	// ErrTokenExpired berlaku (errors.Is, selain ErrInvalidToken) hanya untuk token yang tanda tangan, kid, alg,
	// iss, dan aud-nya semuanya sah tetapi sudah lewat exp+leeway, sehingga bukan oracle bagi token palsu.
	// Apakah middleware menampilkannya sebagai 401 token_expired adalah keputusan C1.10.
	ErrTokenExpired = errors.New("token: token kedaluwarsa")
)

// VerifyError adalah galat verifikasi. Error() sengaja generik; Reason hanya untuk log server.
type VerifyError struct{ Reason string }

func (e *VerifyError) Error() string { return ErrInvalidToken.Error() }

// Is mencocokkan ErrInvalidToken (semua) dan ErrTokenExpired (hanya ReasonExpired).
func (e *VerifyError) Is(target error) bool {
	return target == ErrInvalidToken || (target == ErrTokenExpired && e.Reason == ReasonExpired)
}

// Reason mengembalikan alasan kategori internal untuk log, atau "" bila err bukan *VerifyError.
func Reason(err error) string {
	var ve *VerifyError
	if errors.As(err, &ve) {
		return ve.Reason
	}
	return ""
}

func fail(reason string) error { return &VerifyError{Reason: reason} }

// Claims adalah klaim access token: iss sub sid tid aud exp iat jti perm_rev amr scope. Daftar permission tidak
// masuk token. Pada Issue, Issuer mengisi Issuer, Audience, IssuedAt, ExpiresAt, dan ID (jti); nilai pemanggil diabaikan.
type Claims struct {
	Issuer    string
	Subject   string // user id
	SessionID string // sid
	TenantID  string // tid
	Audience  string // aud (satu string)
	ID        string // jti
	IssuedAt  time.Time
	ExpiresAt time.Time
	PermRev   int64    // perm_rev: revisi permission saat token diterbitkan
	AMR       []string // amr (RFC 8176): faktor terverifikasi, mis. "pwd", "otp"
	Scope     []string // scope: dikodekan sebagai string dipisah spasi (RFC 9068); kosong = klaim dihilangkan
}

type wireClaims struct {
	Iss     string   `json:"iss"`
	Sub     string   `json:"sub"`
	Sid     string   `json:"sid"`
	Tid     string   `json:"tid"`
	Aud     string   `json:"aud"`
	Exp     int64    `json:"exp"`
	Iat     int64    `json:"iat"`
	Jti     string   `json:"jti"`
	PermRev int64    `json:"perm_rev"`
	AMR     []string `json:"amr"`
	Scope   string   `json:"scope,omitempty"`
}

// IssuerConfig mengonfigurasi Issuer. Issuer dan Audience wajib (belum ada kunci konfigurasi; dipasang perakit).
type IssuerConfig struct {
	Keyring  *Keyring
	Issuer   string
	Audience string
	Now      func() time.Time
}

// Issuer menerbitkan access token dengan kunci active.
type Issuer struct {
	ring     *Keyring
	iss, aud string
	now      func() time.Time
}

// NewIssuer membuat Issuer.
func NewIssuer(cfg IssuerConfig) (*Issuer, error) {
	if cfg.Keyring == nil || cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("token: Keyring, Issuer, dan Audience wajib")
	}
	i := &Issuer{ring: cfg.Keyring, iss: cfg.Issuer, aud: cfg.Audience, now: cfg.Now}
	if i.now == nil {
		i.now = time.Now
	}
	return i, nil
}

var b64 = base64.RawURLEncoding

// Issue menerbitkan token berumur AccessTokenTTL, ditandatangani kunci active (tidak pernah pending).
// Subject, SessionID, TenantID wajib; scope tidak boleh berisi spasi.
func (i *Issuer) Issue(c Claims) (string, error) {
	if c.Subject == "" || c.SessionID == "" || c.TenantID == "" {
		return "", errors.New("token: sub, sid, dan tid wajib")
	}
	for _, s := range c.Scope {
		if s == "" || strings.ContainsAny(s, " \t\r\n") {
			return "", errors.New("token: scope tidak boleh kosong atau berisi spasi")
		}
	}
	if c.PermRev < 0 {
		return "", errors.New("token: perm_rev tidak boleh negatif")
	}
	key := i.ring.Active()
	if key.IsZero() || len(key.priv) != ed25519.PrivateKeySize {
		return "", ErrNoActiveKey
	}
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", errors.New("token: jti acak gagal")
	}
	now := i.now().UTC()
	amr := c.AMR
	if amr == nil {
		amr = []string{}
	}
	payload, err := json.Marshal(wireClaims{
		Iss: i.iss, Sub: c.Subject, Sid: c.SessionID, Tid: c.TenantID, Aud: i.aud,
		Exp: now.Add(AccessTokenTTL).Unix(), Iat: now.Unix(), Jti: b64.EncodeToString(jti),
		PermRev: c.PermRev, AMR: amr, Scope: strings.Join(c.Scope, " "),
	})
	if err != nil {
		return "", errors.New("token: serialisasi klaim gagal")
	}
	hdr, err := json.Marshal(struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}{algEdDSA, key.ID})
	if err != nil {
		return "", errors.New("token: serialisasi header gagal")
	}
	signingInput := b64.EncodeToString(hdr) + "." + b64.EncodeToString(payload)
	sig := ed25519.Sign(key.priv, []byte(signingInput))
	tok := signingInput + "." + b64.EncodeToString(sig)
	if len(tok) > MaxTokenSize {
		return "", errors.New("token: token melebihi batas ukuran")
	}
	return tok, nil
}

// VerifierConfig mengonfigurasi Verifier; Issuer dan Audience yang diharapkan wajib.
type VerifierConfig struct {
	Keyring  *Keyring
	Issuer   string
	Audience string
	Now      func() time.Time
}

// Verifier memverifikasi access token.
type Verifier struct {
	ring     *Keyring
	iss, aud string
	now      func() time.Time
}

// NewVerifier membuat Verifier.
func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	if cfg.Keyring == nil || cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("token: Keyring, Issuer, dan Audience wajib")
	}
	v := &Verifier{ring: cfg.Keyring, iss: cfg.Issuer, aud: cfg.Audience, now: cfg.Now}
	if v.now == nil {
		v.now = time.Now
	}
	return v, nil
}

// Verify memverifikasi raw. Semua kegagalan mengembalikan *VerifyError generik (errors.Is ErrInvalidToken);
// ErrTokenExpired hanya untuk token sah yang kedaluwarsa. Urutan: ukuran, tiga segmen, base64url ketat, header
// ketat, alg EdDSA, bentuk kid, pencarian kunci (muat ulang terbatas), status kunci, tanda tangan, baru klaim.
func (v *Verifier) Verify(raw string) (Claims, error) {
	if len(raw) > MaxTokenSize {
		return Claims{}, fail(ReasonSize)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Claims{}, fail(ReasonMalformed)
	}
	strict := b64.Strict()
	hdrB, err1 := strict.DecodeString(parts[0])
	payB, err2 := strict.DecodeString(parts[1])
	sig, err3 := strict.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return Claims{}, fail(ReasonMalformed)
	}
	hdr, err := parseObject(hdrB)
	if err != nil {
		return Claims{}, fail(ReasonMalformed)
	}
	for k := range hdr { // hanya alg, kid, typ: menolak crit, jku, jwk, x5u, x5c, dan sisanya
		if k != "alg" && k != "kid" && k != "typ" {
			return Claims{}, fail(ReasonMalformed)
		}
	}
	alg, ok := rawString(hdr["alg"])
	if !ok || alg != algEdDSA {
		return Claims{}, fail(ReasonAlg)
	}
	if rt, present := hdr["typ"]; present {
		if typ, ok := rawString(rt); !ok || typ != "JWT" {
			return Claims{}, fail(ReasonMalformed)
		}
	}
	kid, ok := rawString(hdr["kid"])
	if !ok || !ValidKID(kid) {
		return Claims{}, fail(ReasonKID)
	}
	key, found := v.ring.ByKID(kid)
	if !found {
		return Claims{}, fail(ReasonUnknownKID)
	}
	if key.Status != StatusActive && key.Status != StatusRetiring {
		return Claims{}, fail(ReasonKeyStatus)
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(key.Public, []byte(parts[0]+"."+parts[1]), sig) {
		return Claims{}, fail(ReasonSignature)
	}
	return v.checkClaims(payB)
}

func (v *Verifier) checkClaims(payload []byte) (Claims, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return Claims{}, fail(ReasonClaims)
	}
	str := func(name string) (string, bool) {
		s, ok := rawString(obj[name])
		return s, ok && s != ""
	}
	num := func(name string) (int64, bool) { return rawInt(obj[name]) }
	iss, ok1 := str("iss")
	sub, ok2 := str("sub")
	sid, ok3 := str("sid")
	tid, ok4 := str("tid")
	aud, ok5 := str("aud")
	jti, ok6 := str("jti")
	exp, ok7 := num("exp")
	iat, ok8 := num("iat")
	rev, ok9 := num("perm_rev")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 || !ok9 || rev < 0 {
		return Claims{}, fail(ReasonClaims)
	}
	var amr []string
	if r, ok := obj["amr"]; !ok || len(r) == 0 || r[0] != '[' || json.Unmarshal(r, &amr) != nil {
		return Claims{}, fail(ReasonClaims)
	}
	for _, a := range amr {
		if a == "" {
			return Claims{}, fail(ReasonClaims)
		}
	}
	var scope []string
	if r, present := obj["scope"]; present {
		s, ok := rawString(r)
		if !ok {
			return Claims{}, fail(ReasonClaims)
		}
		scope = strings.Fields(s)
	}
	if iss != v.iss {
		return Claims{}, fail(ReasonIssuer)
	}
	if aud != v.aud {
		return Claims{}, fail(ReasonAudience)
	}
	if exp <= iat || exp-iat > int64(AccessTokenTTL/time.Second) {
		return Claims{}, fail(ReasonClaims)
	}
	now := v.now().UTC()
	expT, iatT := time.Unix(exp, 0).UTC(), time.Unix(iat, 0).UTC()
	if iatT.After(now.Add(Leeway)) {
		return Claims{}, fail(ReasonNotYetValid)
	}
	if !now.Before(expT.Add(Leeway)) {
		return Claims{}, fail(ReasonExpired)
	}
	return Claims{
		Issuer: iss, Subject: sub, SessionID: sid, TenantID: tid, Audience: aud, ID: jti,
		IssuedAt: iatT, ExpiresAt: expT, PermRev: rev, AMR: amr, Scope: scope,
	}, nil
}

// parseObject mem-parse satu objek JSON dengan ketat: tepat satu objek tanpa sisa data, kunci tanpa duplikat,
// dan tanpa dua kunci yang sama bila huruf diabaikan (encoding/json mencocokkan nama tanpa memandang huruf dan
// memakai nilai terakhir; keduanya dapat dimanfaatkan untuk menyelundupkan klaim). Nilai dikembalikan mentah.
func parseObject(b []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("bukan objek")
	}
	out := map[string]json.RawMessage{}
	folded := map[string]struct{}{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := t.(string)
		if !ok {
			return nil, errors.New("kunci bukan string")
		}
		lk := strings.ToLower(key)
		if _, dup := folded[lk]; dup {
			return nil, errors.New("kunci duplikat")
		}
		folded[lk] = struct{}{}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		out[key] = raw
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return nil, errors.New("objek tidak tertutup")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("data berlebih")
	}
	return out, nil
}

// rawString membaca nilai JSON yang HARUS berupa string.
func rawString(r json.RawMessage) (string, bool) {
	if len(r) == 0 || r[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(r, &s) != nil {
		return "", false
	}
	return s, true
}

// rawInt membaca nilai JSON yang HARUS berupa bilangan bulat (tanpa pecahan atau eksponen).
func rawInt(r json.RawMessage) (int64, bool) {
	if len(r) == 0 || (r[0] != '-' && (r[0] < '0' || r[0] > '9')) {
		return 0, false
	}
	n, err := strconv.ParseInt(string(r), 10, 64)
	return n, err == nil
}
