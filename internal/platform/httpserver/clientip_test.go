package httpserver

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"erpora/sdk/httpx"
)

func prefixes(t *testing.T, ss ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, s := range ss {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func reqFrom(peer string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = peer
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestResolveClientIP(t *testing.T) {
	proxies := prefixes(t, "10.0.0.0/8", "192.0.2.5/32", "2001:db8:aaaa::/48")
	long := strings.Repeat("198.51.100.1, ", 40) // > 32 hop
	cases := []struct {
		name    string
		trusted []netip.Prefix
		peer    string
		xff     []string
		want    string
	}{
		{"tanpa proxy tepercaya: XFF diabaikan", nil, "198.51.100.9:4000", []string{"203.0.113.1"}, "198.51.100.9"},
		{"peer tak tepercaya + XFF palsu diabaikan", proxies, "198.51.100.9:4000", []string{"203.0.113.1"}, "198.51.100.9"},
		{"peer tepercaya tanpa XFF memakai peer", proxies, "10.1.2.3:80", nil, "10.1.2.3"},
		{"peer tepercaya + XFF benar", proxies, "10.1.2.3:80", []string{"203.0.113.7"}, "203.0.113.7"},
		{"rantai berlapis: hop pertama bukan proxy dari kanan", proxies, "10.1.2.3:80", []string{"203.0.113.7, 10.9.9.9, 192.0.2.5"}, "203.0.113.7"},
		{"spoofing hop paling kiri tidak berpengaruh", proxies, "10.1.2.3:80", []string{"6.6.6.6, 203.0.113.7, 10.9.9.9"}, "203.0.113.7"},
		{"header ganda digabung berurutan", proxies, "10.1.2.3:80", []string{"6.6.6.6", "203.0.113.7, 10.9.9.9"}, "203.0.113.7"},
		{"semua hop tepercaya: pakai peer", proxies, "10.1.2.3:80", []string{"10.9.9.9, 192.0.2.5"}, "10.1.2.3"},
		{"entri cacat: pakai peer", proxies, "10.1.2.3:80", []string{"bukan-ip, 203.0.113.7"}, "10.1.2.3"},
		{"entri cacat di kiri hop sah tetap peer (header tak dipercaya utuh)", proxies, "10.1.2.3:80", []string{"203.0.113.7, abc"}, "10.1.2.3"},
		{"hop kosong: pakai peer", proxies, "10.1.2.3:80", []string{"203.0.113.7,, 10.9.9.9"}, "10.1.2.3"},
		{"hop berport: pakai peer", proxies, "10.1.2.3:80", []string{"203.0.113.7:5555"}, "10.1.2.3"},
		{"IPv6 dengan zona: pakai peer", proxies, "10.1.2.3:80", []string{"fe80::1%eth0"}, "10.1.2.3"},
		{"IPv6 klien", proxies, "10.1.2.3:80", []string{"2001:db8:1::7"}, "2001:db8:1::7"},
		{"IPv6 proxy berlapis", proxies, "[2001:db8:aaaa::1]:443", []string{"2001:db8:1::7, 2001:db8:aaaa::2"}, "2001:db8:1::7"},
		{"IPv4-mapped di-unmap", proxies, "10.1.2.3:80", []string{"::ffff:203.0.113.7"}, "203.0.113.7"},
		{"peer IPv4-mapped dikenali sebagai proxy IPv4", proxies, "[::ffff:10.1.2.3]:80", []string{"203.0.113.7"}, "203.0.113.7"},
		{"ribuan hop: dibatasi 32 dari kanan lalu peer bila semua tepercaya", proxies, "10.1.2.3:80", []string{strings.Repeat("10.0.0.1,", 5000) + "10.0.0.2"}, "10.1.2.3"},
		{"lebih dari 32 hop: hanya 32 terkanan dinilai", proxies, "10.1.2.3:80", []string{long + "10.0.0.1"}, "198.51.100.1"},
		{"header raksasa: pakai peer", proxies, "10.1.2.3:80", []string{strings.Repeat("1", 9000)}, "10.1.2.3"},
		{"peer tak terurai", proxies, "bukan-alamat", []string{"203.0.113.7"}, ""},
		{"X-Real-IP tidak dipakai", proxies, "10.1.2.3:80", nil, "10.1.2.3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := reqFrom(c.peer, c.xff...)
			r.Header.Set("X-Real-IP", "203.0.113.99")
			if got := resolveClientIP(r, c.trusted); got != c.want {
				t.Fatalf("resolveClientIP = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRouter_ClientIPHonoursTrustedProxies(t *testing.T) {
	serve := func(trusted []netip.Prefix, peer, xff string) string {
		r, _ := newTestRouter(t, nil)
		r.deps.TrustedProxies = trusted
		var got string
		r.Handle("GET /api/ip", http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			got = httpx.ClientIP(req.Context())
		}), Public())
		req := httptest.NewRequest(http.MethodGet, "/api/ip", nil)
		req.RemoteAddr = peer
		req.Header.Set("X-Forwarded-For", xff)
		r.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}
	trusted := prefixes(t, "10.0.0.0/8")
	if got := serve(trusted, "10.0.0.1:1", "203.0.113.5"); got != "203.0.113.5" {
		t.Errorf("proxy tepercaya: %q", got)
	}
	if got := serve(trusted, "198.51.100.9:1", "203.0.113.5"); got != "198.51.100.9" {
		t.Errorf("peer tak tepercaya: %q", got)
	}
	if got := serve(nil, "10.0.0.1:1", "203.0.113.5"); got != "10.0.0.1" {
		t.Errorf("tanpa konfigurasi: %q", got)
	}
}
