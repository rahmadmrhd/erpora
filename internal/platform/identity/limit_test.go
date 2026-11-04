package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func TestIPLimit_429WithRetryAfterAndPerIPBuckets(t *testing.T) {
	l, err := guard.NewIPLimiter(guard.LimiterConfig{Burst: 2, Every: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	h := IPLimit(l)(okHandler())
	do := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req = req.WithContext(httpx.WithClientIP(req.Context(), ip))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for i := 0; i < 2; i++ {
		if rec := do("198.51.100.1"); rec.Code != http.StatusNoContent {
			t.Fatalf("permintaan %d = %d", i, rec.Code)
		}
	}
	rec := do("198.51.100.1")
	if rec.Code != http.StatusTooManyRequests || errCode(t, rec) != "rate_limited" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("melebihi batas = %d %s Retry-After=%q", rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}
	if rec := do("198.51.100.2"); rec.Code != http.StatusNoContent {
		t.Fatalf("IP lain tidak boleh terdampak: %d", rec.Code)
	}
}

func TestIPLimit_KeepsClientIPOnContext(t *testing.T) {
	l, _ := guard.NewIPLimiter(guard.DefaultIPLimiterConfig())
	var seen context.Context
	h := IPLimit(l)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.Context() }))
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	h.ServeHTTP(httptest.NewRecorder(), req.WithContext(httpx.WithClientIP(req.Context(), "198.51.100.1")))
	if seen == nil {
		t.Fatal("handler tidak dipanggil")
	}
	if got := httpx.ClientIP(seen); got != "198.51.100.1" {
		t.Fatalf("ClientIP = %q", got)
	}
}

func TestPrincipalLimit_PerPrincipalAndRequiresPrincipal(t *testing.T) {
	l, err := guard.NewPrincipalLimiter(guard.LimiterConfig{Burst: 1, Every: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	h := PrincipalLimit(l)(okHandler())
	as := func(user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		ctx := tenancy.WithTenant(req.Context(), testTenant)
		ctx = sdkidentity.WithPrincipal(ctx, sdkidentity.Principal{UserID: user, Kind: sdkidentity.KindHuman})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req.WithContext(ctx))
		return rec
	}
	if rec := as("u1"); rec.Code != http.StatusNoContent {
		t.Fatalf("pertama = %d", rec.Code)
	}
	if rec := as("u1"); rec.Code != http.StatusTooManyRequests || errCode(t, rec) != "rate_limited" {
		t.Fatalf("kedua = %d %s", rec.Code, rec.Body)
	}
	if rec := as("u2"); rec.Code != http.StatusNoContent {
		t.Fatalf("principal lain tidak boleh terdampak: %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tanpa Principal = %d, want 401", rec.Code)
	}
}

type fakeMFAStore struct {
	factor    store.MFAFactor
	factorErr error
	admin     bool
	adminErr  error
	reads     int
}

func (f *fakeMFAStore) GetMFAFactor(context.Context, tenancy.TenantID, string) (store.MFAFactor, error) {
	f.reads++
	return f.factor, f.factorErr
}

func (f *fakeMFAStore) HasRole(context.Context, tenancy.TenantID, string, string) (bool, error) {
	return f.admin, f.adminErr
}

func TestMFAStateReader(t *testing.T) {
	ctx := tenancy.WithTenant(context.Background(), testTenant)
	now := time.Now()
	confirmed := now
	t.Run("faktor terkonfirmasi dan admin", func(t *testing.T) {
		fs := &fakeMFAStore{factor: store.MFAFactor{ConfirmedAt: &confirmed}, admin: true}
		r, _ := NewMFAStateReader(fs, time.Minute, func() time.Time { return now })
		st, err := r.Status(ctx, "u1")
		if err != nil || !st.FactorActive || !st.EnrollmentRequired {
			t.Fatalf("Status = %+v, %v", st, err)
		}
	})
	t.Run("pendaftaran belum dikonfirmasi bukan faktor aktif", func(t *testing.T) {
		fs := &fakeMFAStore{factor: store.MFAFactor{}}
		r, _ := NewMFAStateReader(fs, time.Minute, nil)
		if st, _ := r.Status(ctx, "u1"); st.FactorActive {
			t.Fatal("faktor pending tidak boleh dianggap aktif")
		}
	})
	t.Run("tidak ada faktor atau pengguna", func(t *testing.T) {
		fs := &fakeMFAStore{factorErr: store.ErrNotFound}
		r, _ := NewMFAStateReader(fs, time.Minute, nil)
		st, err := r.Status(ctx, "tak-dikenal")
		if err != nil || st != (MFAStatus{}) {
			t.Fatalf("Status = %+v, %v", st, err)
		}
	})
	t.Run("galat store dikembalikan", func(t *testing.T) {
		boom := errors.New("db")
		r, _ := NewMFAStateReader(&fakeMFAStore{factorErr: boom}, time.Minute, nil)
		if _, err := r.Status(ctx, "u1"); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		r, _ = NewMFAStateReader(&fakeMFAStore{adminErr: boom}, time.Minute, nil)
		if _, err := r.Status(ctx, "u1"); !errors.Is(err, boom) {
			t.Fatalf("err peran = %v", err)
		}
	})
	t.Run("tanpa tenant", func(t *testing.T) {
		r, _ := NewMFAStateReader(&fakeMFAStore{}, time.Minute, nil)
		if _, err := r.Status(context.Background(), "u1"); err == nil {
			t.Fatal("tanpa tenant harus galat")
		}
	})
	t.Run("cache TTL dan Invalidate", func(t *testing.T) {
		fs := &fakeMFAStore{}
		clock := now
		r, _ := NewMFAStateReader(fs, 10*time.Second, func() time.Time { return clock })
		_, _ = r.Status(ctx, "u1")
		_, _ = r.Status(ctx, "u1")
		if fs.reads != 1 {
			t.Fatalf("pembacaan store = %d, want 1 (cache)", fs.reads)
		}
		clock = clock.Add(11 * time.Second)
		_, _ = r.Status(ctx, "u1")
		if fs.reads != 2 {
			t.Fatalf("setelah TTL pembacaan = %d, want 2", fs.reads)
		}
		r.Invalidate(testTenant, "u1")
		_, _ = r.Status(ctx, "u1")
		if fs.reads != 3 {
			t.Fatalf("setelah Invalidate pembacaan = %d, want 3", fs.reads)
		}
	})
	t.Run("cache terikat tenant", func(t *testing.T) {
		fs := &fakeMFAStore{}
		r, _ := NewMFAStateReader(fs, time.Minute, nil)
		_, _ = r.Status(ctx, "u1")
		other := tenancy.WithTenant(context.Background(), tenancy.TenantID{1})
		_, _ = r.Status(other, "u1")
		if fs.reads != 2 {
			t.Fatalf("tenant berbeda harus membaca ulang, reads = %d", fs.reads)
		}
	})
}

// gateMFAStore menahan HasRole (setelah GetMFAFactor dibaca) sampai dilepas, untuk meniru Invalidate yang terjadi di
// tengah pembacaan Status.
type gateMFAStore struct {
	mu       sync.Mutex
	factor   store.MFAFactor
	reached  chan struct{}
	release  chan struct{}
	blockOne bool
}

func (g *gateMFAStore) GetMFAFactor(context.Context, tenancy.TenantID, string) (store.MFAFactor, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.factor, nil
}

func (g *gateMFAStore) HasRole(context.Context, tenancy.TenantID, string, string) (bool, error) {
	g.mu.Lock()
	block := g.blockOne
	g.blockOne = false
	g.mu.Unlock()
	if block {
		close(g.reached)
		<-g.release
	}
	return false, nil
}

func (g *gateMFAStore) setFactor(f store.MFAFactor) {
	g.mu.Lock()
	g.factor = f
	g.mu.Unlock()
}

// Invalidate yang terjadi saat pembacaan sedang berjalan tidak boleh ditimpa hasil basi: pembacaan lama (sebelum
// Invalidate) tidak boleh menulis cache; Status berikutnya harus membaca ulang dari store (pola gen seperti settings).
func TestMFAStateReader_InvalidateDuringReadDoesNotCacheStale(t *testing.T) {
	ctx := tenancy.WithTenant(context.Background(), testTenant)
	gs := &gateMFAStore{reached: make(chan struct{}), release: make(chan struct{}), blockOne: true}
	r, _ := NewMFAStateReader(gs, time.Hour, nil)

	done := make(chan MFAStatus, 1)
	go func() {
		st, _ := r.Status(ctx, "u1") // membaca faktor pending/kosong lalu tertahan di HasRole
		done <- st
	}()
	<-gs.reached
	confirmed := time.Now()
	gs.setFactor(store.MFAFactor{ConfirmedAt: &confirmed}) // konfirmasi MFA terjadi di tengah pembacaan
	r.Invalidate(testTenant, "u1")
	close(gs.release)
	if st := <-done; st.FactorActive {
		t.Log("pembacaan yang sedang berjalan boleh mengembalikan hasil lama untuk permintaannya sendiri")
	}
	st, err := r.Status(ctx, "u1")
	if err != nil || !st.FactorActive {
		t.Fatalf("setelah Invalidate Status harus membaca ulang dan melihat faktor aktif, dapat %+v, %v", st, err)
	}
}
