package token

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

// JWKSHandler menyajikan JWKS (RFC 7517/8037): hanya kunci pending, active, dan retiring; tidak pernah kunci
// privat maupun retired. Cache-Control: public, max-age=300 (JWKSMaxAge). Hanya GET dan HEAD.
func JWKSHandler(r *Keyring) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		keys := make([]jwk, 0)
		for _, k := range r.Keys() {
			if k.Status == StatusRetired {
				continue
			}
			keys = append(keys, jwk{Kty: "OKP", Crv: "Ed25519", X: b64.EncodeToString(k.Public), Kid: k.ID, Alg: algEdDSA, Use: "sig"})
		}
		body, err := json.Marshal(struct {
			Keys []jwk `json:"keys"`
		}{keys})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(JWKSMaxAge.Seconds())))
		h.Set("X-Content-Type-Options", "nosniff")
		if req.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	})
}
