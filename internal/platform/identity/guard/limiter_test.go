package guard

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestIPLimiter(t *testing.T, c *clock, burst int, every time.Duration, maxEntries int) *IPLimiter {
	t.Helper()
	l, err := NewIPLimiter(LimiterConfig{Burst: burst, Every: every, MaxEntries: maxEntries, Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestIPLimiterBurstThenDeniedWithRetryAfter(t *testing.T) {
	c := newClock()
	l := newTestIPLimiter(t, c, 3, 2*time.Second, 0)
	for i := range 3 {
		if ok, d := l.Allow("198.51.100.7"); !ok || d != 0 {
			t.Fatalf("permintaan %d: ok=%v retryAfter=%v, want diizinkan", i+1, ok, d)
		}
	}
	ok, d := l.Allow("198.51.100.7")
	if ok || d != 2*time.Second {
		t.Fatalf("setelah burst: ok=%v retryAfter=%v, want ditolak 2s", ok, d)
	}
	// Penolakan tidak menghabiskan token: Retry-After tidak membesar pada penolakan berulang.
	if ok, d := l.Allow("198.51.100.7"); ok || d != 2*time.Second {
		t.Fatalf("penolakan kedua: ok=%v retryAfter=%v, want ditolak 2s", ok, d)
	}
	c.Advance(time.Second)
	if ok, d := l.Allow("198.51.100.7"); ok || d != time.Second {
		t.Fatalf("setelah 1s: ok=%v retryAfter=%v, want ditolak 1s", ok, d)
	}
	c.Advance(time.Second)
	if ok, _ := l.Allow("198.51.100.7"); !ok {
		t.Fatal("setelah 2s token pulih, harus diizinkan")
	}
}

func TestIPLimiterIsolatesIPs(t *testing.T) {
	c := newClock()
	l := newTestIPLimiter(t, c, 1, time.Minute, 0)
	if ok, _ := l.Allow("198.51.100.1"); !ok {
		t.Fatal("IP 1 pertama harus diizinkan")
	}
	if ok, _ := l.Allow("198.51.100.1"); ok {
		t.Fatal("IP 1 kedua harus ditolak")
	}
	if ok, _ := l.Allow("198.51.100.2"); !ok {
		t.Fatal("IP 2 tidak boleh terkena batas IP 1")
	}
}

func TestIPLimiterIPv6Uses64PrefixAndUnmapsV4(t *testing.T) {
	c := newClock()
	l := newTestIPLimiter(t, c, 1, time.Minute, 0)
	if ok, _ := l.Allow("2001:db8:1:2::1"); !ok {
		t.Fatal("alamat v6 pertama harus diizinkan")
	}
	// Alamat lain di /64 yang sama berbagi bucket (penyerang tak dapat berputar di dalam satu /64).
	if ok, _ := l.Allow("2001:db8:1:2:ffff:ffff:ffff:ffff"); ok {
		t.Fatal("alamat lain pada /64 yang sama harus berbagi bucket")
	}
	if ok, _ := l.Allow("2001:db8:1:3::1"); !ok {
		t.Fatal("/64 berbeda harus berbucket terpisah")
	}
	// IPv4-mapped sama dengan IPv4; zona dan port dibuang.
	if ok, _ := l.Allow("198.51.100.9"); !ok {
		t.Fatal("v4 pertama harus diizinkan")
	}
	if ok, _ := l.Allow("::ffff:198.51.100.9"); ok {
		t.Fatal("IPv4-mapped IPv6 harus sama dengan IPv4-nya")
	}
	if ok, _ := l.Allow("[2001:db8:1:2::7]:443"); ok {
		t.Fatal("alamat:port harus dinormalisasi ke /64 yang sama")
	}
}

func TestNormalizeIP(t *testing.T) {
	cases := map[string]string{
		"198.51.100.7":                     "198.51.100.7",
		" 198.51.100.7 ":                   "198.51.100.7",
		"198.51.100.7:8080":                "198.51.100.7",
		"::ffff:198.51.100.7":              "198.51.100.7",
		"2001:db8:1:2:aaaa:bbbb:cccc:dddd": "2001:db8:1:2::/64",
		"fe80::1%eth0":                     "fe80::/64",
		"[2001:db8::1]:443":                "2001:db8::/64",
		"":                                 "unknown",
		"bukan-ip":                         "unknown",
		"999.1.1.1":                        "unknown",
		strings.Repeat("a", 500):           "unknown",
		"198.51.100.7\x00 <script>":        "unknown",
	}
	for in, want := range cases {
		got := NormalizeIP(in)
		if got != want {
			t.Errorf("NormalizeIP(%q) = %q, want %q", in, got, want)
		}
		if len(got) > 45 {
			t.Errorf("NormalizeIP(%q) = %q melebihi 45 karakter (batas kolom login_attempts.ip)", in, got)
		}
	}
}

func TestLimiterEvictsIdleEntries(t *testing.T) {
	c := newClock()
	l := newTestIPLimiter(t, c, 2, time.Second, 0)
	for i := range 50 {
		l.Allow(fmt.Sprintf("198.51.100.%d", i))
	}
	if n := l.size(); n != 50 {
		t.Fatalf("size = %d, want 50", n)
	}
	// Entri yang menganggur selama waktu isi penuh (burst x every) setara bucket baru, jadi dibuang tanpa kehilangan apa pun.
	c.Advance(2*time.Second + time.Millisecond)
	l.Allow("203.0.113.1")
	if n := l.size(); n != 1 {
		t.Fatalf("setelah idle, size = %d, want 1 (hanya entri baru)", n)
	}
}

func TestLimiterBoundedByMaxEntriesEvictsOldest(t *testing.T) {
	c := newClock()
	l := newTestIPLimiter(t, c, 1, time.Hour, 100)
	for i := range 1000 {
		l.Allow(fmt.Sprintf("10.0.%d.%d", i/250, i%250))
		c.Advance(time.Millisecond)
	}
	if n := l.size(); n > 100 {
		t.Fatalf("size = %d melebihi batas 100: map tumbuh tanpa batas", n)
	}
	// Kunci terbaru masih terlacak (tidak diloloskan sebagai kunci baru); yang tertua sudah tergusur.
	if ok, _ := l.Allow("10.0.3.249"); ok {
		t.Fatal("kunci terbaru harus masih terlacak dan ditolak pada permintaan kedua")
	}
	if ok, _ := l.Allow("10.0.0.0"); !ok {
		t.Fatal("kunci tertua sudah tergusur, diperlakukan sebagai bucket baru")
	}
}

func TestPrincipalLimiterPerKeyAndTargetKeyHidesEmail(t *testing.T) {
	c := newClock()
	p, err := NewPrincipalLimiter(LimiterConfig{Burst: 2, Every: 30 * time.Second, Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	alice := EmailTargetKey("login", testTenant, "Alice@Example.COM ")
	if strings.Contains(alice, "alice") || strings.Contains(strings.ToLower(alice), "example") {
		t.Fatalf("kunci sasaran memuat email: %q", alice)
	}
	if alice != EmailTargetKey("login", testTenant, "alice@example.com") {
		t.Fatal("kunci sasaran harus menormalisasi email (trim + huruf kecil)")
	}
	if alice == EmailTargetKey("forgot", testTenant, "alice@example.com") {
		t.Fatal("lingkup berbeda harus berkunci berbeda")
	}
	bob := EmailTargetKey("login", testTenant, "bob@example.com")
	for range 2 {
		if ok, _ := p.Allow(alice); !ok {
			t.Fatal("dua permintaan pertama harus diizinkan")
		}
	}
	ok, d := p.Allow(alice)
	if ok || d != 30*time.Second {
		t.Fatalf("ketiga: ok=%v retryAfter=%v, want ditolak 30s", ok, d)
	}
	if ok, _ := p.Allow(bob); !ok {
		t.Fatal("sasaran lain tidak boleh terkena batas alice")
	}
	// Principal (sub) pada endpoint terautentikasi: kunci apa pun dipakai apa adanya.
	if ok, _ := p.Allow("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"); !ok {
		t.Fatal("principal pertama harus diizinkan")
	}
}

func TestPrincipalLimiterBoundsKeyLength(t *testing.T) {
	c := newClock()
	p, _ := NewPrincipalLimiter(LimiterConfig{Burst: 1, Every: time.Hour, Now: c.Now})
	long := strings.Repeat("x", 1<<20)
	if ok, _ := p.Allow(long); !ok {
		t.Fatal("pertama harus diizinkan")
	}
	if ok, _ := p.Allow(long); ok {
		t.Fatal("kunci panjang yang sama harus tetap dibatasi")
	}
	for k := range p.k.entries.m {
		if len(k) > 128 {
			t.Fatalf("kunci tersimpan %d byte: panjang kunci tak terbatas", len(k))
		}
	}
}

func TestLimiterConfigValidation(t *testing.T) {
	for name, cfg := range map[string]LimiterConfig{
		"burst nol":     {Burst: 0, Every: time.Second},
		"burst negatif": {Burst: -1, Every: time.Second},
		"every nol":     {Burst: 1, Every: 0},
		"every negatif": {Burst: 1, Every: -time.Second},
		"max negatif":   {Burst: 1, Every: time.Second, MaxEntries: -1},
	} {
		if _, err := NewIPLimiter(cfg); err == nil {
			t.Errorf("%s: NewIPLimiter menerima konfigurasi tidak valid", name)
		}
		if _, err := NewPrincipalLimiter(cfg); err == nil {
			t.Errorf("%s: NewPrincipalLimiter menerima konfigurasi tidak valid", name)
		}
	}
	if _, err := NewIPLimiter(DefaultIPLimiterConfig()); err != nil {
		t.Errorf("default IP: %v", err)
	}
	if _, err := NewPrincipalLimiter(DefaultTargetLimiterConfig()); err != nil {
		t.Errorf("default sasaran: %v", err)
	}
}

func TestLimiterConcurrentAllowIsSafeAndBounded(t *testing.T) {
	c := newClock()
	l := newTestIPLimiter(t, c, 5, time.Hour, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow("198.51.100.77"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 5 {
		t.Fatalf("diizinkan = %d, want tepat burst 5", allowed)
	}
}

// NormalizeIP harus idempoten: hasilnya dipakai ulang oleh lapisan lain (Authenticator -> Lockout), dan satu
// pasangan harus selalu memakai kunci yang sama. Hanya prefix /64 ter-mask yang diterima apa adanya.
func TestNormalizeIPIdempotent(t *testing.T) {
	for _, in := range []string{
		"198.51.100.7", "::ffff:198.51.100.7", "2001:db8:1:2:aaaa:bbbb:cccc:dddd", "fe80::1%eth0",
		"[2001:db8::1]:443", "", "bukan-ip", "198.51.100.7:8080",
	} {
		once := NormalizeIP(in)
		if twice := NormalizeIP(once); twice != once {
			t.Errorf("NormalizeIP tidak idempoten untuk %q: %q -> %q", in, once, twice)
		}
	}
	for in, want := range map[string]string{
		"2001:db8:1:2::/64":      "2001:db8:1:2::/64",
		" 2001:db8:1:2::/64 ":    "2001:db8:1:2::/64",
		"2001:db8:1:2::1/64":     "unknown", // bukan prefix ter-mask
		"2001:db8:1:2::/48":      "unknown", // panjang prefix lain
		"2001:db8:1:2::/128":     "unknown",
		"198.51.100.0/24":        "unknown",
		"::ffff:198.51.100.0/64": "unknown",
		"2001:db8::/64/64":       "unknown",
		"2001:db8::/064":         "unknown",
	} {
		if got := NormalizeIP(in); got != want {
			t.Errorf("NormalizeIP(%q) = %q, want %q", in, got, want)
		}
	}
}
