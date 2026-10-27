package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultConnectTimeout         = 5 * time.Second
	DefaultTimeout                = 30 * time.Second
	DefaultMaxResponseBytes int64 = 10 << 20
	DefaultMaxRedirects           = 3
)

var (
	ErrSchemeNotAllowed      = errors.New("egress: skema URL tidak diizinkan")
	ErrHostNotAllowed        = errors.New("egress: host tidak ada di allowlist")
	ErrBlockedAddress        = errors.New("egress: alamat tujuan diblokir")
	ErrTooManyRedirects      = errors.New("egress: terlalu banyak redirect")
	ErrResponseTooLarge      = errors.New("egress: respons melebihi batas ukuran")
	ErrUnexpectedContentType = errors.New("egress: Content-Type respons tidak diizinkan")
)

// BlockedError adalah penolakan karena alamat atau host tujuan berada di rentang terlarang.
// Reason: loopback|private|link_local|cgnat|unspecified|multicast|reserved|unique_local|nat64|6to4|numeric_host|zone.
type BlockedError struct {
	Host   string
	Addr   netip.Addr
	Reason string
}

func (e *BlockedError) Error() string {
	if e.Addr.IsValid() {
		return fmt.Sprintf("egress: alamat %s untuk host %q diblokir (%s)", e.Addr, e.Host, e.Reason)
	}
	return fmt.Sprintf("egress: host %q diblokir (%s)", e.Host, e.Reason)
}

// Is membuat errors.Is(err, ErrBlockedAddress) berlaku.
func (e *BlockedError) Is(target error) bool { return target == ErrBlockedAddress }

// Resolver diimplementasi *net.Resolver.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Config mengatur Client; nilai nol memakai default yang aman.
type Config struct {
	AllowHosts          []string       // kosong = semua host publik; isi = exact host atau "*.example.com" (subdomain saja)
	AllowHTTPHosts      []string       // host (exact) yang boleh memakai skema http
	AllowedContentTypes []string       // media type tanpa parameter; kosong = semua
	ConnectTimeout      time.Duration  // 0 -> 5s
	Timeout             time.Duration  // 0 -> 30s (total, termasuk redirect dan membaca body)
	MaxResponseBytes    int64          // 0 -> 10<<20
	MaxRedirects        int            // 0 -> 3; > 3 atau < 0 ditolak New
	RootCAs             *x509.CertPool // nil = CA sistem
	Resolver            Resolver       // nil = net.DefaultResolver
}

// Client adalah HTTP client keluar anti-SSRF.
type Client struct {
	hc        *http.Client
	transport *http.Transport
	cfg       Config
	resolver  Resolver
	allowHost []string // lowercase
	allowHTTP []string // lowercase
}

// New memvalidasi cfg dan membangun Client.
func New(cfg Config) (*Client, error) {
	if cfg.ConnectTimeout < 0 || cfg.Timeout < 0 {
		return nil, errors.New("egress: timeout tidak boleh negatif")
	}
	if cfg.MaxResponseBytes < 0 {
		return nil, errors.New("egress: MaxResponseBytes tidak boleh negatif")
	}
	if cfg.MaxRedirects < 0 || cfg.MaxRedirects > DefaultMaxRedirects {
		return nil, fmt.Errorf("egress: MaxRedirects harus 0..%d", DefaultMaxRedirects)
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = DefaultConnectTimeout
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if cfg.MaxRedirects == 0 {
		cfg.MaxRedirects = DefaultMaxRedirects
	}
	c := &Client{cfg: cfg, resolver: cfg.Resolver}
	if c.resolver == nil {
		c.resolver = net.DefaultResolver
	}
	for _, h := range cfg.AllowHosts {
		h = normalizeHost(h)
		if h == "" || h == "*." || strings.HasPrefix(h, "*.") && strings.Contains(h[2:], "*") ||
			!strings.HasPrefix(h, "*.") && strings.Contains(h, "*") {
			return nil, fmt.Errorf("egress: pola AllowHosts %q tidak valid", h)
		}
		c.allowHost = append(c.allowHost, h)
	}
	for _, h := range cfg.AllowHTTPHosts {
		c.allowHTTP = append(c.allowHTTP, normalizeHost(h))
	}
	c.cfg.AllowedContentTypes = nil
	for _, ct := range cfg.AllowedContentTypes {
		c.cfg.AllowedContentTypes = append(c.cfg.AllowedContentTypes, strings.ToLower(strings.TrimSpace(ct)))
	}

	c.transport = &http.Transport{
		Proxy:                 nil, // proxy dari environment sengaja tidak dipakai
		DialContext:           c.dialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: cfg.RootCAs},
		TLSHandshakeTimeout:   cfg.ConnectTimeout,
		ResponseHeaderTimeout: cfg.Timeout,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
	}
	c.hc = &http.Client{
		Transport:     c.transport,
		Timeout:       cfg.Timeout,
		CheckRedirect: c.checkRedirect,
	}
	return c, nil
}

func normalizeHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}

// numericHostRE mengenali host numerik non-kanonik (desimal, heksadesimal, oktal, singkat).
var numericHostRE = regexp.MustCompile(`(?i)^(0x[0-9a-f]+|[0-9]+)(\.(0x[0-9a-f]+|[0-9]+)){0,3}$`)

// validateURL menerapkan aturan skema, host, dan IP literal. Dipakai untuk request awal dan tiap hop redirect.
func (c *Client) validateURL(u *url.URL) error {
	if u == nil {
		return errors.New("egress: URL kosong")
	}
	host := normalizeHost(u.Hostname())
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !slices.Contains(c.allowHTTP, host) {
			return ErrSchemeNotAllowed
		}
	default:
		return ErrSchemeNotAllowed
	}
	if host == "" {
		return fmt.Errorf("%w: host kosong", ErrHostNotAllowed)
	}
	if len(c.allowHost) > 0 && !c.hostAllowed(host) {
		return ErrHostNotAllowed
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" {
			return &BlockedError{Host: host, Reason: "zone"}
		}
		port := u.Port()
		p := uint64(443)
		if strings.EqualFold(u.Scheme, "http") {
			p = 80
		}
		if port != "" {
			if n, perr := strconv.ParseUint(port, 10, 16); perr == nil {
				p = n
			}
		}
		if reason, bad := blockedAddr(netip.AddrPortFrom(ip.Unmap(), uint16(p))); bad {
			return &BlockedError{Host: host, Addr: ip.Unmap(), Reason: reason}
		}
	} else if numericHostRE.MatchString(host) {
		return &BlockedError{Host: host, Reason: "numeric_host"}
	}
	return nil
}

func (c *Client) hostAllowed(host string) bool {
	for _, p := range c.allowHost {
		if suffix, ok := strings.CutPrefix(p, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == p {
			return true
		}
	}
	return false
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.cfg.MaxRedirects {
		return ErrTooManyRedirects
	}
	return c.validateURL(req.URL)
}

// Do mengirim req setelah validasi URL; respons dibatasi ukuran dan (opsional) Content-Type.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if err := c.validateURL(req.URL); err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.ContentLength > c.cfg.MaxResponseBytes {
		_ = resp.Body.Close()
		return nil, ErrResponseTooLarge
	}
	if len(c.cfg.AllowedContentTypes) > 0 {
		mt, _, perr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if perr != nil || !slices.Contains(c.cfg.AllowedContentTypes, strings.ToLower(mt)) {
			_ = resp.Body.Close()
			return nil, ErrUnexpectedContentType
		}
	}
	resp.Body = &limitedBody{rc: resp.Body, remaining: c.cfg.MaxResponseBytes}
	return resp, nil
}

// Get adalah Do untuk GET sederhana.
func (c *Client) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
