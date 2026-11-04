package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"erpora/internal/platform/settings/store"
	"erpora/internal/platform/tenancy"
)

// PublicURLKey adalah kunci setting platform untuk URL publik aplikasi.
const PublicURLKey = "platform.public_url"

// MaxPublicURLLen adalah panjang maksimum URL publik (setelah trim).
const MaxPublicURLLen = 512

// DefaultCacheTTL adalah umur cache bawaan; perubahan dari instance lain terlihat paling lama selama ini (perubahan
// dari instance yang sama langsung terlihat).
const DefaultCacheTTL = 30 * time.Second

var (
	// ErrNotConfigured berarti URL publik belum diisi pada tenant itu. Di production, perakit wajib menolak start
	// (serve gagal) bila ini dikembalikan.
	ErrNotConfigured = errors.New("settings: platform.public_url belum diisi")
	// ErrInvalidURL berarti URL tidak memenuhi aturan URL publik. Pesan tidak pernah mengulang masukan mentah.
	ErrInvalidURL = errors.New("settings: URL tidak valid")
	// ErrNoTenant berarti context tidak membawa tenant.
	ErrNoTenant = store.ErrNoTenant
)

// Store adalah bagian store yang dipakai layanan (dipenuhi *store.Store).
type Store interface {
	Get(ctx context.Context, scope, moduleKey, key string) (store.Setting, error)
	Upsert(ctx context.Context, s store.Setting, updatedBy string) error
}

// Options mengatur layanan. Production datang dari konfigurasi (ERP_ENV) dan disuntik sebagai bool agar paket ini
// tidak mengimpor config.
type Options struct {
	// Production mewajibkan https pada URL publik.
	Production bool
	// CacheTTL adalah umur cache; nol berarti DefaultCacheTTL.
	CacheTTL time.Duration
	// Now menyuntik jam (tes); nil berarti time.Now.
	Now func() time.Time
}

type cacheEntry struct {
	url        string // kosong bila belum diisi
	configured bool
	expires    time.Time
}

// Service adalah API tertutup untuk setting platform platform.public_url di atas Store. Ini subset minimal C1;
// Settings penuh (skema, validasi per tipe, audit, UI) adalah C3 (FR-120, PRD-001 Q20). URL tidak pernah diturunkan
// dari permintaan HTTP: nilainya hanya berasal dari tabel settings (atau default development yang diminta pemanggil
// secara eksplisit).
type Service struct {
	st   Store
	opts Options

	mu    sync.Mutex
	gen   uint64 // naik pada setiap Set/Invalidate; pemuatan yang tertinggal tidak boleh menulis cache
	cache map[tenancy.TenantID]cacheEntry
}

// NewService membuat layanan.
func NewService(st Store, opts Options) *Service {
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = DefaultCacheTTL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{st: st, opts: opts, cache: map[tenancy.TenantID]cacheEntry{}}
}

// PublicURL mengembalikan URL publik tenant di context (ternormalisasi: skema://host[:port][/path], tanpa slash
// akhir). ErrNotConfigured bila belum diisi; ErrNoTenant tanpa tenant. Nilai tersimpan yang rusak atau tidak lolos
// validasi lagi (mis. http di production) menghasilkan galat (gagal tertutup), bukan ErrNotConfigured.
func (s *Service) PublicURL(ctx context.Context) (string, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return "", ErrNoTenant
	}
	now := s.opts.Now()
	s.mu.Lock()
	if e, hit := s.cache[t]; hit && now.Before(e.expires) {
		s.mu.Unlock()
		if !e.configured {
			return "", ErrNotConfigured
		}
		return e.url, nil
	}
	gen := s.gen
	s.mu.Unlock()

	u, configured, err := s.load(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if s.gen == gen {
		s.cache[t] = cacheEntry{url: u, configured: configured, expires: now.Add(s.opts.CacheTTL)}
	}
	s.mu.Unlock()
	if !configured {
		return "", ErrNotConfigured
	}
	return u, nil
}

func (s *Service) load(ctx context.Context) (string, bool, error) {
	set, err := s.st.Get(ctx, store.ScopePlatform, "", PublicURLKey)
	if errors.Is(err, store.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("settings: baca %s: %w", PublicURLKey, err)
	}
	var v any
	if err := json.Unmarshal(set.Value, &v); err != nil {
		return "", false, fmt.Errorf("%w: nilai tersimpan %s rusak", ErrInvalidURL, PublicURLKey)
	}
	str, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("%w: nilai tersimpan %s bukan string", ErrInvalidURL, PublicURLKey)
	}
	norm, err := NormalizePublicURL(str, s.opts.Production)
	if err != nil {
		return "", false, fmt.Errorf("nilai tersimpan %s: %w", PublicURLKey, err)
	}
	return norm, true, nil
}

// SetPublicURL memvalidasi, menormalisasi, dan menyimpan URL publik tenant di context, lalu membatalkan cache
// (pembacaan berikutnya pada instance ini langsung melihat nilai baru). updatedBy (wajib, mis. email admin atau
// "cli:core:settings:set") dicatat sebagai jejak perubahan. Dipanggil oleh command core:settings:set (C1.12).
func (s *Service) SetPublicURL(ctx context.Context, rawURL, updatedBy string) error {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return ErrNoTenant
	}
	if strings.TrimSpace(updatedBy) == "" {
		return errors.New("settings: updatedBy wajib diisi")
	}
	norm, err := NormalizePublicURL(rawURL, s.opts.Production)
	if err != nil {
		return err
	}
	val, err := json.Marshal(norm)
	if err != nil {
		return err
	}
	if err := s.st.Upsert(ctx, store.Setting{Scope: store.ScopePlatform, Key: PublicURLKey, Value: val}, updatedBy); err != nil {
		return fmt.Errorf("settings: simpan %s: %w", PublicURLKey, err)
	}
	s.mu.Lock()
	s.gen++
	delete(s.cache, t)
	s.mu.Unlock()
	return nil
}

// PublicURLOrDevDefault seperti PublicURL, tetapi di development (Options.Production = false) dan hanya bila
// setting belum diisi, memakai devDefault yang diminta pemanggil secara eksplisit (mis. DevURLFromHTTPAddr) agar
// pengembangan lokal tanpa setup tetap jalan. Di production selalu ErrNotConfigured bila belum diisi. Nilai
// tersimpan yang rusak tidak pernah diganti default.
func (s *Service) PublicURLOrDevDefault(ctx context.Context, devDefault string) (string, error) {
	u, err := s.PublicURL(ctx)
	if err == nil || !errors.Is(err, ErrNotConfigured) || s.opts.Production || strings.TrimSpace(devDefault) == "" {
		return u, err
	}
	return NormalizePublicURL(devDefault, false)
}

// Invalidate membuang cache satu tenant (mis. dari notifikasi perubahan lintas instance).
func (s *Service) Invalidate(t tenancy.TenantID) {
	s.mu.Lock()
	s.gen++
	delete(s.cache, t)
	s.mu.Unlock()
}

// InvalidateAll membuang seluruh cache.
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	s.gen++
	s.cache = map[tenancy.TenantID]cacheEntry{}
	s.mu.Unlock()
}

var hostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func invalid(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidURL, reason) }

// parsed adalah URL http(s) yang lolos pemeriksaan dasar.
type parsed struct {
	u      *url.URL
	scheme string
	host   string // huruf kecil, tanpa kurung IPv6
	port   int    // 0 bila tidak ada atau port bawaan skema
}

func (p parsed) origin() string {
	h := p.host
	if strings.Contains(h, ":") {
		h = "[" + h + "]"
	}
	if p.port != 0 {
		h += ":" + strconv.Itoa(p.port)
	}
	return p.scheme + "://" + h
}

func parseBase(raw string) (parsed, error) {
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c <= 0x20 || c >= 0x7f || c == '\\' {
			return parsed{}, invalid("berisi karakter terlarang (spasi, kontrol, backslash, atau non-ASCII)")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return parsed{}, invalid("tidak dapat diurai")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return parsed{}, invalid("skema harus http atau https")
	}
	if u.Opaque != "" || u.Host == "" {
		return parsed{}, invalid("host wajib diisi")
	}
	if u.User != nil {
		return parsed{}, invalid("tidak boleh memuat userinfo")
	}
	host := strings.ToLower(u.Hostname())
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" {
			return parsed{}, invalid("host tidak valid")
		}
	} else if len(host) > 253 || !hostPattern.MatchString(host) {
		return parsed{}, invalid("host tidak valid")
	}
	port := 0
	if ps := u.Port(); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n < 1 || n > 65535 {
			return parsed{}, invalid("port tidak valid")
		}
		if (scheme != "http" || n != 80) && (scheme != "https" || n != 443) {
			port = n
		}
	}
	return parsed{u: u, scheme: scheme, host: host, port: port}, nil
}

// NormalizePublicURL memvalidasi dan menormalisasi URL publik: http(s) absolut dengan host, tanpa userinfo, query,
// atau fragment; path bersih (tanpa "..", "." atau "//" dan tanpa %2f/%5c); https wajib bila production; skema dan
// host huruf kecil, port bawaan dibuang, slash akhir dibuang; panjang <= MaxPublicURLLen. Hanya ASCII (IDN harus
// punycode). Galat selalu ErrInvalidURL tanpa mengulang masukan.
func NormalizePublicURL(raw string, production bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", invalid("kosong")
	}
	if len(raw) > MaxPublicURLLen {
		return "", invalid("terlalu panjang")
	}
	if strings.ContainsAny(raw, "?#") {
		return "", invalid("tidak boleh memuat query atau fragment")
	}
	p, err := parseBase(raw)
	if err != nil {
		return "", err
	}
	if production && p.scheme != "https" {
		return "", invalid("https wajib di production")
	}
	esc := strings.ToLower(p.u.EscapedPath())
	if strings.Contains(esc, "%2f") || strings.Contains(esc, "%5c") {
		return "", invalid("path tidak valid")
	}
	trimmed := strings.TrimRight(p.u.Path, "/")
	if trimmed != "" && path.Clean(trimmed) != trimmed {
		return "", invalid("path tidak valid")
	}
	out := p.origin() + strings.TrimRight(p.u.EscapedPath(), "/")
	if len(out) > MaxPublicURLLen {
		return "", invalid("terlalu panjang")
	}
	return out, nil
}

// OriginOf mengembalikan origin (skema://host[:port], huruf kecil, port bawaan dibuang) dari URL atau nilai header
// Origin/Referer, untuk pemeriksa Origin. Path, query, dan fragment diabaikan; userinfo, skema selain http(s),
// "null", dan host tidak valid menghasilkan ErrInvalidURL.
func OriginOf(rawURL string) (string, error) {
	p, err := parseBase(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	return p.origin(), nil
}

// DevURLFromHTTPAddr menurunkan URL development dari ERP_HTTP_ADDR (mis. "127.0.0.1:8080" menjadi
// "http://127.0.0.1:8080"; host kosong atau tak-terspesifikasi menjadi "localhost"). Mengembalikan "" bila addr
// tidak berbentuk host:port. Hanya untuk PublicURLOrDevDefault pada development; tidak pernah dipakai di production.
func DevURLFromHTTPAddr(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return ""
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return ""
	}
	if ip, err := netip.ParseAddr(host); host == "" || (err == nil && ip.IsUnspecified()) {
		host = "localhost"
	} else if err == nil && ip.Is6() {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + strconv.Itoa(n)
}
