package session

import (
	"fmt"
	"net/url"
	"strings"
)

// OriginChecker memutuskan apakah nilai header Origin sebuah permintaan refresh diizinkan. Nilai kosong (header
// hilang) dan "null" tidak pernah diizinkan oleh pemeriksa bawaan (NewOriginChecker). nil berarti tolak semua.
type OriginChecker func(origin string) bool

// NewOriginChecker membuat OriginChecker dari daftar origin yang diizinkan (cocok persis setelah normalisasi
// skema dan host ke huruf kecil dan pembuangan port bawaan). Setiap entri harus origin murni
// (skema://host[:port], tanpa path, query, fragmen, atau userinfo); entri tidak sah menghasilkan galat agar
// konfigurasi salah menggagalkan startup, bukan membuka CSRF. Daftar kosong menolak semua origin.
func NewOriginChecker(allowed []string) (OriginChecker, error) {
	set := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		n, ok := normalizeOrigin(a)
		if !ok {
			return nil, fmt.Errorf("session: origin yang diizinkan tidak sah: %q", a)
		}
		set[n] = struct{}{}
	}
	return func(origin string) bool {
		n, ok := normalizeOrigin(origin)
		if !ok {
			return false
		}
		_, found := set[n]
		return found
	}, nil
}

// normalizeOrigin mengembalikan bentuk kanonik skema://host[:port] dari origin murni, atau false.
func normalizeOrigin(s string) (string, bool) {
	if s == "" || len(s) > 512 || s == "null" {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Opaque != "" || u.User != nil || u.Path != "" ||
		u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Hostname() == "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	switch scheme {
	case "https":
		host = strings.TrimSuffix(host, ":443")
	case "http":
		host = strings.TrimSuffix(host, ":80")
	}
	return scheme + "://" + host, true
}
