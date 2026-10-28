package egress

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubResolver struct {
	mu    sync.Mutex
	m     map[string][]netip.Addr
	calls map[string]int
	fn    func(host string, call int) []netip.Addr
}

func (s *stubResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls == nil {
		s.calls = map[string]int{}
	}
	s.calls[host]++
	if s.fn != nil {
		return s.fn(host, s.calls[host]), nil
	}
	if a, ok := s.m[host]; ok {
		return append([]netip.Addr(nil), a...), nil
	}
	return nil, fmt.Errorf("no such host %s", host)
}

func addrs(ss ...string) []netip.Addr {
	var out []netip.Addr
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

var errStop = errors.New("stop dial")

func cleanHooks(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { setDialHook(nil); setAllowAddrForTest(nil) })
}

func TestClient_RejectsFR24Table(t *testing.T) {
	cleanHooks(t)
	dials := 0
	setDialHook(func(context.Context, string, string) (net.Conn, error) { dials++; return nil, errStop })
	c, err := New(Config{
		AllowHTTPHosts: []string{"127.0.0.1", "169.254.169.254", "::1", "10.0.0.1", "internal.example.test",
			"mixed.example.test", "::ffff:127.0.0.1", "2130706433", "0x7f000001", "0177.0.0.1", "127.1"},
		Resolver: &stubResolver{m: map[string][]netip.Addr{
			"internal.example.test": addrs("10.1.2.3"),
			"mixed.example.test":    addrs("93.184.216.34", "10.0.0.1"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{
		"http://127.0.0.1/", "http://169.254.169.254/latest/meta-data/", "http://[::1]/", "http://10.0.0.1/",
		"http://internal.example.test/", "http://mixed.example.test/", "http://[::ffff:127.0.0.1]/",
		"http://2130706433/", "http://0x7f000001/", "http://0177.0.0.1/", "http://127.1/",
	} {
		_, err := c.Get(context.Background(), u)
		if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s: err = %v, want ErrBlockedAddress", u, err)
		}
	}
	if dials != 0 {
		t.Errorf("dial dipanggil %d kali, want 0", dials)
	}
}

func TestClient_DialsValidatedIP(t *testing.T) {
	cleanHooks(t)
	var recorded []string
	setDialHook(func(_ context.Context, _, addr string) (net.Conn, error) {
		recorded = append(recorded, addr)
		return nil, errStop
	})
	res := &stubResolver{fn: func(_ string, call int) []netip.Addr {
		if call == 1 {
			return addrs("93.184.216.34")
		}
		return addrs("127.0.0.1")
	}}
	c, err := New(Config{Resolver: res})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "https://rebind.example.test/")
	if !errors.Is(err, errStop) {
		t.Fatalf("err = %v", err)
	}
	if len(recorded) != 1 || recorded[0] != "93.184.216.34:443" {
		t.Errorf("dial = %v", recorded)
	}
	if res.calls["rebind.example.test"] != 1 {
		t.Errorf("resolver dipanggil %d kali, want 1", res.calls["rebind.example.test"])
	}
}

func TestClient_SchemeAndHostAllowlist(t *testing.T) {
	cleanHooks(t)
	dials := 0
	setDialHook(func(context.Context, string, string) (net.Conn, error) { dials++; return nil, errStop })
	res := &stubResolver{fn: func(string, int) []netip.Addr { return addrs("93.184.216.34") }}

	c, _ := New(Config{Resolver: res})
	for _, u := range []string{"ftp://a.example.test/", "http://public.example.test/"} {
		if _, err := c.Get(context.Background(), u); !errors.Is(err, ErrSchemeNotAllowed) {
			t.Errorf("%s: err = %v", u, err)
		}
	}

	c, _ = New(Config{AllowHosts: []string{"api.example.test", "*.svc.example.test"}, Resolver: res})
	for _, u := range []string{"https://evil.example.test/", "https://svc.example.test/"} {
		if _, err := c.Get(context.Background(), u); !errors.Is(err, ErrHostNotAllowed) {
			t.Errorf("%s: err = %v", u, err)
		}
	}
	if dials != 0 {
		t.Fatalf("dial = %d sebelum host yang diizinkan", dials)
	}
	if _, err := c.Get(context.Background(), "https://a.svc.example.test/"); !errors.Is(err, errStop) {
		t.Errorf("a.svc.example.test harus mencapai dial: %v", err)
	}
	if _, err := c.Get(context.Background(), "https://api.example.test/"); !errors.Is(err, errStop) {
		t.Errorf("api.example.test harus mencapai dial: %v", err)
	}
}

// tlsServer menjalankan server TLS uji dan mengizinkan alamatnya (loopback) lewat hook uji.
func tlsServer(t *testing.T, h http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ap := netip.MustParseAddrPort(srv.Listener.Addr().String())
	setAllowAddrForTest(func(a netip.AddrPort) bool { return a == ap })
	t.Cleanup(func() { setAllowAddrForTest(nil) })
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return srv, pool
}

func TestClient_Redirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/chain/{n}", func(w http.ResponseWriter, r *http.Request) {
		n := r.PathValue("n")
		if n == "0" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		k := int(n[0] - '0')
		http.Redirect(w, r, fmt.Sprintf("/chain/%d", k-1), http.StatusFound)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/to-private", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://10.0.0.1/x", http.StatusFound)
	})
	mux.HandleFunc("/to-metadata", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	})
	mux.HandleFunc("/to-http", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.test/", http.StatusFound)
	})
	srv, pool := tlsServer(t, mux)
	c, err := New(Config{RootCAs: pool, AllowHTTPHosts: []string{"169.254.169.254"}})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.Get(context.Background(), srv.URL+"/chain/3")
	if err != nil {
		t.Fatalf("hop3: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "ok" {
		t.Errorf("hop3 = %d %q", resp.StatusCode, body)
	}

	for path, want := range map[string]error{
		"/chain/4":     ErrTooManyRedirects,
		"/to-private":  ErrBlockedAddress,
		"/to-metadata": ErrBlockedAddress,
		"/to-http":     ErrSchemeNotAllowed,
	} {
		if _, err := c.Get(context.Background(), srv.URL+path); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", path, err, want)
		}
	}
}

func TestClient_ResponseLimits(t *testing.T) {
	const max = 10 << 20
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(max+1))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/big-chunked", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", max+1)))
	})
	mux.HandleFunc("/exact", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", max)))
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>x</p>"))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{}"))
	})
	srv, pool := tlsServer(t, mux)
	c, _ := New(Config{RootCAs: pool})

	if _, err := c.Get(context.Background(), srv.URL+"/big"); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("/big err = %v", err)
	}
	resp, err := c.Get(context.Background(), srv.URL+"/big-chunked")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(resp.Body); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("/big-chunked read err = %v", err)
	}
	_ = resp.Body.Close()

	resp, err = c.Get(context.Background(), srv.URL+"/exact")
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if err != nil || n != max {
		t.Errorf("/exact n=%d err=%v", n, err)
	}

	cj, _ := New(Config{RootCAs: pool, AllowedContentTypes: []string{"application/json"}})
	if _, err := cj.Get(context.Background(), srv.URL+"/html"); !errors.Is(err, ErrUnexpectedContentType) {
		t.Errorf("/html err = %v", err)
	}
	resp, err = cj.Get(context.Background(), srv.URL+"/json")
	if err != nil {
		t.Fatalf("/json: %v", err)
	}
	_ = resp.Body.Close()
}

func TestClient_Timeout(t *testing.T) {
	srv, pool := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second)
	}))
	c, _ := New(Config{RootCAs: pool, Timeout: 200 * time.Millisecond})
	start := time.Now()
	_, err := c.Get(context.Background(), srv.URL+"/")
	var ne net.Error
	if err == nil {
		t.Fatal("harus error timeout")
	}
	isTimeout := errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout()
	if !isTimeout {
		t.Fatalf("err = %v, want timeout", err)
	}
	if time.Since(start) >= time.Second {
		t.Errorf("timeout terlalu lama: %v", time.Since(start))
	}
}

func TestNew_DefaultsAndValidation(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := configOf(c)
	if cfg.ConnectTimeout != 5*time.Second || cfg.Timeout != 30*time.Second || cfg.MaxResponseBytes != 10<<20 || cfg.MaxRedirects != 3 {
		t.Errorf("default = %+v", cfg)
	}
	for name, bad := range map[string]Config{
		"redirect 4":      {MaxRedirects: 4},
		"redirect -1":     {MaxRedirects: -1},
		"max bytes -1":    {MaxResponseBytes: -1},
		"allow host '*.'": {AllowHosts: []string{"*."}},
	} {
		if _, err := New(bad); err == nil {
			t.Errorf("%s: New harus error", name)
		}
	}
}

func TestClient_IgnoresProxyEnv(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	c, _ := New(Config{})
	if proxyOf(c) != nil {
		t.Error("Transport.Proxy harus nil")
	}
}
