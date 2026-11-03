package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Batas bawaan peta pembatas dan panjang kunci.
const (
	// DefaultMaxLimiterEntries membatasi jumlah bucket yang dilacak satu pembatas (sekitar 150 byte per entri).
	DefaultMaxLimiterEntries = 50_000
	// maxPrincipalKeyLen: kunci yang lebih panjang di-hash agar memori peta tidak bergantung pada masukan.
	maxPrincipalKeyLen = 128
)

// LimiterConfig menyatakan satu bucket token per kunci: kapasitas Burst dan isi ulang satu token tiap Every
// (kecepatan tetap 1/Every per detik). Now (opsional, default time.Now) adalah jam yang dapat diinjeksi untuk tes.
type LimiterConfig struct {
	Burst      int
	Every      time.Duration
	MaxEntries int // 0 = DefaultMaxLimiterEntries
	Now        func() time.Time
}

func (c LimiterConfig) validate() error {
	switch {
	case c.Burst < 1:
		return errors.New("guard: LimiterConfig.Burst harus >= 1")
	case c.Every <= 0:
		return errors.New("guard: LimiterConfig.Every harus > 0")
	case c.MaxEntries < 0:
		return errors.New("guard: LimiterConfig.MaxEntries tidak boleh negatif")
	}
	return nil
}

// DefaultIPLimiterConfig: 30 permintaan sekaligus, lalu 1 per 2 detik (30 per menit) per IP, untuk seluruh endpoint
// auth/token. Cukup longgar untuk kantor di balik satu NAT, cukup ketat untuk menahan penebakan massal dari satu sumber.
func DefaultIPLimiterConfig() LimiterConfig { return LimiterConfig{Burst: 30, Every: 2 * time.Second} }

// DefaultTargetLimiterConfig: 10 permintaan sekaligus, lalu 1 per 6 detik (10 per menit) per identitas sasaran (email
// pada login). Pembatas per email juga bisa dikuras penyerang berIP banyak sehingga login sah memperoleh 429 sesaat;
// risiko residual itu melekat pada batas per sasaran yang diwajibkan ADR-0013 §4 dan sembuh sendiri (token pulih).
// Endpoint `forgot` memakai instans terpisah yang jauh lebih ketat (banjir email, C1.8).
func DefaultTargetLimiterConfig() LimiterConfig {
	return LimiterConfig{Burst: 10, Every: 6 * time.Second}
}

// bucket adalah satu entri peta pembatas.
type bucket struct {
	lim  *rate.Limiter
	last time.Time // akses terakhir (urutan peta = urutan waktu ini)
}

// keyedLimiter adalah peta bucket token per kunci yang berbatas: entri yang menganggur selama waktu isi penuh
// (Burst x Every) setara bucket baru sehingga dibuang tanpa kehilangan keadaan, dan bila jumlah entri mencapai
// MaxEntries yang paling lama tak diakses digusur (gagal ke arah mengizinkan kunci tergusur, bukan memblokir kunci baru).
type keyedLimiter struct {
	mu      sync.Mutex
	entries *lruMap[bucket]
	burst   int
	every   time.Duration
	ttl     time.Duration
	now     func() time.Time
}

func newKeyedLimiter(cfg LimiterConfig) (*keyedLimiter, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	maxEntries := cfg.MaxEntries
	if maxEntries == 0 {
		maxEntries = DefaultMaxLimiterEntries
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &keyedLimiter{
		entries: newLRU[bucket](maxEntries),
		burst:   cfg.Burst, every: cfg.Every,
		ttl: time.Duration(cfg.Burst) * cfg.Every,
		now: now,
	}, nil
}

// allow mengambil satu token untuk key. Ditolak: (false, lama sampai token berikutnya tersedia); permintaan ditolak
// tidak menghabiskan token (Retry-After tidak membesar pada penolakan berulang).
func (k *keyedLimiter) allow(key string) (bool, time.Duration) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	for {
		old, ok := k.entries.oldest()
		if !ok || now.Sub(old.last) < k.ttl {
			break
		}
		k.entries.removeOldest()
	}
	b, ok := k.entries.get(key)
	if !ok {
		b = k.entries.add(key, bucket{lim: rate.NewLimiter(rate.Every(k.every), k.burst)})
	}
	b.last = now
	r := b.lim.ReserveN(now, 1)
	if !r.OK() { // tidak terjadi: Burst >= 1
		return false, k.every
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

func (k *keyedLimiter) size() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.entries.len()
}

// IPLimiter membatasi laju per IP klien (middleware ADR-0013 §4, setelah tenantContext dan sebelum Authenticate).
// Aman untuk dipakai serentak. Dibuat dengan NewIPLimiter.
type IPLimiter struct{ k *keyedLimiter }

// NewIPLimiter membuat IPLimiter; galat bila konfigurasi tidak valid.
func NewIPLimiter(cfg LimiterConfig) (*IPLimiter, error) {
	k, err := newKeyedLimiter(cfg)
	if err != nil {
		return nil, err
	}
	return &IPLimiter{k: k}, nil
}

// Allow mengambil satu token untuk ip (dinormalisasi lewat NormalizeIP: IPv6 menjadi /64, IPv4-mapped menjadi IPv4;
// alamat tak terurai berbagi satu bucket). ok=false: tolak dengan 429 dan Retry-After retryAfter.
func (l *IPLimiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	return l.k.allow(NormalizeIP(ip))
}

func (l *IPLimiter) size() int { return l.k.size() }

// PrincipalLimiter membatasi laju per principal (sub, pada endpoint terautentikasi) atau per identitas sasaran (email
// pada login/forgot, family pada refresh, lewat TargetKey/EmailTargetKey). Aman untuk dipakai serentak. Dibuat dengan
// NewPrincipalLimiter.
type PrincipalLimiter struct{ k *keyedLimiter }

// NewPrincipalLimiter membuat PrincipalLimiter; galat bila konfigurasi tidak valid.
func NewPrincipalLimiter(cfg LimiterConfig) (*PrincipalLimiter, error) {
	k, err := newKeyedLimiter(cfg)
	if err != nil {
		return nil, err
	}
	return &PrincipalLimiter{k: k}, nil
}

// Allow mengambil satu token untuk key. Kunci lebih panjang dari 128 byte di-hash agar memori peta terbatas.
func (l *PrincipalLimiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	if len(key) > maxPrincipalKeyLen {
		h := sha256.Sum256([]byte(key))
		key = "h:" + hex.EncodeToString(h[:16])
	}
	return l.k.allow(key)
}
