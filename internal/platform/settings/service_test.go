package settings

import (
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/settings/store"
	"erpora/internal/platform/tenancy"
)

var (
	tenA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

func ctxOf(t tenancy.TenantID) context.Context { return tenancy.WithTenant(context.Background(), t) }

// fakeStore meniru store per tenant (database terpisah) dan menghitung pembacaan.
type fakeStore struct {
	mu     sync.Mutex
	data   map[tenancy.TenantID]map[string]json.RawMessage
	gets   int
	getErr error
	onGet  func() // dijalankan sekali di dalam Get, sebelum hasil dikembalikan (simulasi Set bersamaan)
}

func newFake() *fakeStore { return &fakeStore{data: map[tenancy.TenantID]map[string]json.RawMessage{}} }

func (f *fakeStore) put(t tenancy.TenantID, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data[t] == nil {
		f.data[t] = map[string]json.RawMessage{}
	}
	f.data[t][PublicURLKey] = json.RawMessage(v)
}

func (f *fakeStore) Get(ctx context.Context, scope, moduleKey, key string) (store.Setting, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return store.Setting{}, store.ErrNoTenant
	}
	f.mu.Lock()
	f.gets++
	if f.getErr != nil {
		err := f.getErr
		f.mu.Unlock()
		return store.Setting{}, err
	}
	v, found := f.data[t][key]
	cb := f.onGet
	f.onGet = nil
	f.mu.Unlock()
	if scope != store.ScopePlatform || moduleKey != "" {
		return store.Setting{}, errors.New("scope/modul tidak terduga")
	}
	if cb != nil {
		cb()
	}
	if !found {
		return store.Setting{}, store.ErrNotFound
	}
	return store.Setting{Scope: scope, Key: key, Value: v}, nil
}

func (f *fakeStore) Upsert(ctx context.Context, s store.Setting, updatedBy string) error {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return store.ErrNoTenant
	}
	if s.Scope != store.ScopePlatform || s.ModuleKey != "" || s.Encrypted || updatedBy == "" {
		return errors.New("upsert tak terduga")
	}
	f.put(t, string(s.Value))
	return nil
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newSvc(f *fakeStore, production bool) (*Service, *clock) {
	c := &clock{t: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)}
	return NewService(f, Options{Production: production, CacheTTL: time.Minute, Now: c.now}), c
}

func TestNormalizePublicURL(t *testing.T) {
	good := []struct {
		in, prod, dev string
	}{
		{"https://erp.example.com", "https://erp.example.com", "https://erp.example.com"},
		{"HTTPS://ERP.Example.COM/", "https://erp.example.com", "https://erp.example.com"},
		{"  https://erp.example.com/  ", "https://erp.example.com", "https://erp.example.com"},
		{"https://erp.example.com/app/", "https://erp.example.com/app", "https://erp.example.com/app"},
		{"https://erp.example.com/App/Sub", "https://erp.example.com/App/Sub", "https://erp.example.com/App/Sub"},
		{"https://erp.example.com:443", "https://erp.example.com", "https://erp.example.com"},
		{"https://erp.example.com:8443/x", "https://erp.example.com:8443/x", "https://erp.example.com:8443/x"},
		{"https://[::1]:8443", "https://[::1]:8443", "https://[::1]:8443"},
		{"https://203.0.113.10", "https://203.0.113.10", "https://203.0.113.10"},
		{"https://xn--erp-abc.example.com", "https://xn--erp-abc.example.com", "https://xn--erp-abc.example.com"},
	}
	for _, g := range good {
		for _, mode := range []struct {
			prod bool
			want string
		}{{true, g.prod}, {false, g.dev}} {
			got, err := NormalizePublicURL(g.in, mode.prod)
			if err != nil || got != mode.want {
				t.Errorf("NormalizePublicURL(%q, prod=%v) = %q, %v; want %q", g.in, mode.prod, got, err, mode.want)
			}
		}
	}
	// hanya di development
	for in, want := range map[string]string{
		"http://localhost:8080":  "http://localhost:8080",
		"http://127.0.0.1:8080/": "http://127.0.0.1:8080",
		"http://erp.local:80":    "http://erp.local",
	} {
		got, err := NormalizePublicURL(in, false)
		if err != nil || got != want {
			t.Errorf("dev %q = %q, %v; want %q", in, got, err, want)
		}
		if _, err := NormalizePublicURL(in, true); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("production harus menolak %q (https wajib): err = %v", in, err)
		}
	}
	bad := map[string]string{
		"kosong":                 "",
		"spasi saja":             "   ",
		"tanpa skema":            "erp.example.com",
		"javascript":             "javascript:alert(1)",
		"ftp":                    "ftp://erp.example.com",
		"file":                   "file:///etc/passwd",
		"data":                   "data:text/html,<script>1</script>",
		"opaque":                 "https:erp.example.com",
		"host kosong":            "https:///path",
		"hanya skema":            "https://",
		"userinfo":               "https://user:pw@erp.example.com",
		"userinfo tanpa sandi":   "https://user@erp.example.com",
		"query":                  "https://erp.example.com?x=1",
		"query kosong":           "https://erp.example.com?",
		"query pada path":        "https://erp.example.com/a?x=1",
		"fragment":               "https://erp.example.com#frag",
		"fragment kosong":        "https://erp.example.com#",
		"path dot-dot":           "https://erp.example.com/../x",
		"path dot-dot terkode":   "https://erp.example.com/%2e%2e/x",
		"path ganda":             "https://erp.example.com//x",
		"path garis miring kode": "https://erp.example.com/a%2fb",
		"path backslash":         "https://erp.example.com/a\\b",
		"path berspasi":          "https://erp.example.com/a b",
		"kontrol":                "https://erp.example.com/a\nb",
		"port nol":               "https://erp.example.com:0",
		"port terlalu besar":     "https://erp.example.com:99999",
		"port bukan angka":       "https://erp.example.com:abc",
		"host dengan underscore": "https://er_p.example.com",
		"host diakhiri titik":    "https://erp.example.com.",
		"host unicode":           "https://erp.exämple.com",
		"host dengan persen":     "https://erp%2eexample.com",
		"terlalu panjang":        "https://erp.example.com/" + strings.Repeat("a", MaxPublicURLLen),
	}
	for name, in := range bad {
		if got, err := NormalizePublicURL(in, false); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%s (%q): dev err = %v got %q, want ErrInvalidURL", name, in, err, got)
		}
		if _, err := NormalizePublicURL(in, true); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%s (%q): production err = %v, want ErrInvalidURL", name, in, err)
		}
	}
	// batas panjang: tepat MaxPublicURLLen lolos, +1 ditolak
	base := "https://erp.example.com/"
	atLimit := base + strings.Repeat("a", MaxPublicURLLen-len(base))
	if _, err := NormalizePublicURL(atLimit, true); err != nil {
		t.Errorf("tepat batas: %v", err)
	}
	if _, err := NormalizePublicURL(atLimit+"a", true); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("melebihi batas: %v", err)
	}
	// pesan galat tidak menggemakan masukan mentah (bisa berisi rahasia userinfo)
	_, err := NormalizePublicURL("https://admin:sandi-rahasia@erp.example.com", true)
	if err == nil || strings.Contains(err.Error(), "sandi-rahasia") {
		t.Errorf("galat tidak boleh memuat userinfo: %v", err)
	}
}

func TestOriginOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://erp.example.com":              "https://erp.example.com",
		"https://ERP.example.com/app/x":        "https://erp.example.com",
		"https://erp.example.com:443/":         "https://erp.example.com",
		"https://erp.example.com:8443/app":     "https://erp.example.com:8443",
		"http://localhost:8080":                "http://localhost:8080",
		"http://localhost:80":                  "http://localhost",
		"https://[::1]:8443/x":                 "https://[::1]:8443",
		"https://erp.example.com/app?q=1#frag": "https://erp.example.com",
	} {
		got, err := OriginOf(in)
		if err != nil || got != want {
			t.Errorf("OriginOf(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "null", "javascript:alert(1)", "ftp://h", "https://", "erp.example.com", "https://u:p@h", "https://h:0", "chrome-extension://abc"} {
		if got, err := OriginOf(in); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("OriginOf(%q) = %q, %v; want ErrInvalidURL", in, got, err)
		}
	}
}

func TestPublicURLRoundTripAndCache(t *testing.T) {
	f := newFake()
	svc, clk := newSvc(f, true)
	a := ctxOf(tenA)

	if _, err := svc.PublicURL(a); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("belum diisi: err = %v, want ErrNotConfigured", err)
	}
	if err := svc.SetPublicURL(a, "HTTPS://ERP.Example.com/", "admin@example.com"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.PublicURL(a)
	if err != nil || got != "https://erp.example.com" {
		t.Fatalf("PublicURL = %q, %v", got, err)
	}
	if raw := string(f.data[tenA][PublicURLKey]); raw != `"https://erp.example.com"` {
		t.Errorf("tersimpan %s, want string JSON ternormalisasi", raw)
	}

	// cache: pembacaan berulang dalam TTL tidak menyentuh store
	before := f.gets
	for i := 0; i < 5; i++ {
		if _, err := svc.PublicURL(a); err != nil {
			t.Fatal(err)
		}
	}
	if f.gets != before {
		t.Errorf("store dibaca %d kali tambahan dalam TTL, want 0", f.gets-before)
	}
	// perubahan dari instance lain baru terlihat setelah TTL (atau Invalidate)
	f.put(tenA, `"https://changed.example.com"`)
	if got, _ := svc.PublicURL(a); got != "https://erp.example.com" {
		t.Errorf("dalam TTL masih nilai lama, got %q", got)
	}
	clk.advance(2 * time.Minute)
	if got, _ := svc.PublicURL(a); got != "https://changed.example.com" {
		t.Errorf("setelah TTL harus memuat ulang, got %q", got)
	}
	f.put(tenA, `"https://again.example.com"`)
	svc.Invalidate(tenA)
	if got, _ := svc.PublicURL(a); got != "https://again.example.com" {
		t.Errorf("setelah Invalidate harus memuat ulang, got %q", got)
	}
	f.put(tenA, `"https://third.example.com"`)
	svc.InvalidateAll()
	if got, _ := svc.PublicURL(a); got != "https://third.example.com" {
		t.Errorf("setelah InvalidateAll harus memuat ulang, got %q", got)
	}
}

func TestSetInvalidatesCacheImmediately(t *testing.T) {
	f := newFake()
	svc, _ := newSvc(f, true)
	a := ctxOf(tenA)
	if err := svc.SetPublicURL(a, "https://one.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.PublicURL(a); got != "https://one.example.com" {
		t.Fatalf("got %q", got)
	}
	if err := svc.SetPublicURL(a, "https://two.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.PublicURL(a); got != "https://two.example.com" {
		t.Errorf("cache basi setelah Set: %q", got)
	}
	// negatif juga tidak basi: belum diisi lalu diisi
	svc2, _ := newSvc(newFake(), true)
	if _, err := svc2.PublicURL(a); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if err := svc2.SetPublicURL(a, "https://x.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	if got, err := svc2.PublicURL(a); err != nil || got != "https://x.example.com" {
		t.Errorf("setelah Set dari belum-diisi: %q, %v", got, err)
	}
}

// Set yang terjadi saat pemuatan lama sedang berjalan tidak boleh ditimpa oleh hasil pemuatan lama itu.
func TestSetDuringInFlightLoadDoesNotLeaveStaleCache(t *testing.T) {
	f := newFake()
	f.put(tenA, `"https://old.example.com"`)
	svc, _ := newSvc(f, true)
	a := ctxOf(tenA)
	f.onGet = func() {
		if err := svc.SetPublicURL(a, "https://new.example.com", "x"); err != nil {
			t.Error(err)
		}
	}
	// pemuatan yang sedang berjalan boleh mengembalikan nilai lama...
	_, _ = svc.PublicURL(a)
	// ...tetapi pembacaan berikutnya wajib melihat nilai baru.
	if got, err := svc.PublicURL(a); err != nil || got != "https://new.example.com" {
		t.Errorf("setelah Set bersamaan: %q, %v; cache basi", got, err)
	}
}

func TestSetPublicURLValidatesAndDoesNotPersistInvalid(t *testing.T) {
	f := newFake()
	svc, _ := newSvc(f, true)
	a := ctxOf(tenA)
	for _, in := range []string{"javascript:alert(1)", "http://erp.example.com", "https://u:p@erp.example.com", "https://erp.example.com?x=1", ""} {
		if err := svc.SetPublicURL(a, in, "x"); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("SetPublicURL(%q) err = %v, want ErrInvalidURL", in, err)
		}
	}
	if len(f.data[tenA]) != 0 {
		t.Errorf("nilai tidak valid tersimpan: %v", f.data[tenA])
	}
	if err := svc.SetPublicURL(a, "https://erp.example.com", ""); err == nil {
		t.Error("updatedBy kosong harus ditolak (jejak siapa yang mengubah)")
	}
	if err := svc.SetPublicURL(context.Background(), "https://erp.example.com", "x"); !errors.Is(err, store.ErrNoTenant) {
		t.Errorf("tanpa tenant: err = %v, want ErrNoTenant", err)
	}
	// development menerima http
	dev, _ := newSvc(newFake(), false)
	if err := dev.SetPublicURL(a, "http://localhost:8080", "x"); err != nil {
		t.Errorf("dev http: %v", err)
	}
}

func TestPublicURLTenantIsolation(t *testing.T) {
	f := newFake()
	svc, _ := newSvc(f, true)
	if err := svc.SetPublicURL(ctxOf(tenA), "https://a.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublicURL(ctxOf(tenB)); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("tenant B melihat URL tenant A: err = %v", err)
	}
	if err := svc.SetPublicURL(ctxOf(tenB), "https://b.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	ga, _ := svc.PublicURL(ctxOf(tenA))
	gb, _ := svc.PublicURL(ctxOf(tenB))
	if ga != "https://a.example.com" || gb != "https://b.example.com" {
		t.Errorf("A=%q B=%q", ga, gb)
	}
	if _, err := svc.PublicURL(context.Background()); !errors.Is(err, store.ErrNoTenant) {
		t.Errorf("tanpa tenant: err = %v, want ErrNoTenant", err)
	}
}

func TestPublicURLFailsClosedOnBadStoredValue(t *testing.T) {
	for name, stored := range map[string]string{
		"bukan string":          `{"a":1}`,
		"http di production":    `"http://erp.example.com"`,
		"userinfo":              `"https://u:p@erp.example.com"`,
		"javascript":            `"javascript:alert(1)"`,
		"null":                  `null`,
		"string kosong":         `""`,
		"tidak ternormalisasi*": `"https://erp.example.com/"`, // dinormalisasi ulang, bukan galat
	} {
		f := newFake()
		f.put(tenA, stored)
		svc, _ := newSvc(f, true)
		got, err := svc.PublicURL(ctxOf(tenA))
		if strings.HasSuffix(name, "*") {
			if err != nil || got != "https://erp.example.com" {
				t.Errorf("%s: got %q, %v; want dinormalisasi ulang", name, got, err)
			}
			continue
		}
		if err == nil || got != "" {
			t.Errorf("%s: got %q, %v; want galat tertutup", name, got, err)
		}
		if errors.Is(err, ErrNotConfigured) {
			t.Errorf("%s: nilai rusak bukan 'belum diisi'", name)
		}
	}
}

func TestStoreErrorsAreNotCached(t *testing.T) {
	f := newFake()
	f.put(tenA, `"https://erp.example.com"`)
	svc, _ := newSvc(f, true)
	f.getErr = errors.New("db mati")
	if _, err := svc.PublicURL(ctxOf(tenA)); err == nil {
		t.Fatal("galat store harus diteruskan")
	}
	f.getErr = nil
	if got, err := svc.PublicURL(ctxOf(tenA)); err != nil || got != "https://erp.example.com" {
		t.Errorf("setelah pulih: %q, %v", got, err)
	}
}

func TestPublicURLOrDevDefault(t *testing.T) {
	a := ctxOf(tenA)
	// production: tidak pernah memakai default
	prod, _ := newSvc(newFake(), true)
	if _, err := prod.PublicURLOrDevDefault(a, "http://localhost:8080"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("production tanpa isi: err = %v, want ErrNotConfigured", err)
	}
	// development: default dipakai hanya bila belum diisi
	f := newFake()
	dev, _ := newSvc(f, false)
	if got, err := dev.PublicURLOrDevDefault(a, "http://localhost:8080/"); err != nil || got != "http://localhost:8080" {
		t.Errorf("dev default = %q, %v", got, err)
	}
	if _, err := dev.PublicURLOrDevDefault(a, ""); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("dev tanpa default: err = %v, want ErrNotConfigured", err)
	}
	if _, err := dev.PublicURLOrDevDefault(a, "javascript:alert(1)"); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("dev default tidak valid: err = %v, want ErrInvalidURL", err)
	}
	if err := dev.SetPublicURL(a, "https://real.example.com", "x"); err != nil {
		t.Fatal(err)
	}
	if got, err := dev.PublicURLOrDevDefault(a, "http://localhost:8080"); err != nil || got != "https://real.example.com" {
		t.Errorf("nilai terisi harus menang: %q, %v", got, err)
	}
	// nilai rusak di dev bukan alasan jatuh ke default (galat tetap galat)
	f2 := newFake()
	f2.put(tenA, `{"a":1}`)
	dev2, _ := newSvc(f2, false)
	if _, err := dev2.PublicURLOrDevDefault(a, "http://localhost:8080"); err == nil {
		t.Error("nilai tersimpan rusak tidak boleh diganti default diam-diam")
	}
	// galat store diteruskan
	f3 := newFake()
	f3.getErr = errors.New("db mati")
	dev3, _ := newSvc(f3, false)
	if _, err := dev3.PublicURLOrDevDefault(a, "http://localhost:8080"); err == nil || errors.Is(err, ErrNotConfigured) {
		t.Errorf("galat store: err = %v", err)
	}
}

func TestDevURLFromHTTPAddr(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:8080": "http://127.0.0.1:8080",
		"localhost:8080": "http://localhost:8080",
		":8080":          "http://localhost:8080",
		"0.0.0.0:9000":   "http://localhost:9000",
		"[::]:9000":      "http://localhost:9000",
		"[::1]:9000":     "http://[::1]:9000",
		"":               "",
		"tanpa-port":     "",
		"host:abc":       "",
		"host:0":         "",
		"host:70000":     "",
	} {
		if got := DevURLFromHTTPAddr(in); got != want {
			t.Errorf("DevURLFromHTTPAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

// PublicURL tidak pernah diturunkan dari permintaan HTTP: paket tidak mengimpor net/http dan tidak ada parameter
// permintaan pada API-nya (host header palsu tidak dapat memengaruhi tautan email atau pemeriksaan Origin).
func TestPackageDoesNotImportNetHTTP(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		n++
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "net/http" || strings.HasPrefix(p, "net/http/") {
				t.Errorf("%s mengimpor %s", e.Name(), p)
			}
			if strings.HasPrefix(p, "gorm.io/") {
				t.Errorf("%s mengimpor %s (GORM hanya di settings/store, D3b)", e.Name(), p)
			}
		}
	}
	if n == 0 {
		t.Fatal("tidak ada berkas diperiksa")
	}
}
