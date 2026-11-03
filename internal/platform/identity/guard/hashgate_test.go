package guard

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"erpora/internal/platform/identity/password"
)

// TestConcurrentVerifyBoundedBySemaphore: jumlah Verify yang berjalan bersamaan tidak pernah melebihi kapasitas gerbang,
// betapa pun banyak permintaan yang datang (19 MiB RAM per Verify; login serentak tak boleh menghabiskan memori).
func TestConcurrentVerifyBoundedBySemaphore(t *testing.T) {
	const capacity = 3
	g, err := NewHashGate(capacity, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	params := password.Params{MemoryKiB: 8192, Time: 1, Parallelism: 1}
	h := &password.Hasher{Params: params}
	phc, err := h.Hash("kata sandi yang benar")
	if err != nil {
		t.Fatal(err)
	}

	var running, peak atomic.Int32
	observe := func() {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond) // tahan slot agar tumpang-tindih terbukti
		running.Add(-1)
	}

	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Verify sungguhan melewati gerbang, dibungkus pengamat yang sama dengan Do.
			if err := g.Do(context.Background(), func() error {
				observe()
				_, _, err := h.Verify("salah", phc)
				return err
			}); err != nil {
				t.Errorf("Do: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := peak.Load(); got > capacity {
		t.Fatalf("Verify bersamaan mencapai %d, melebihi kapasitas %d", got, capacity)
	}
	if got := peak.Load(); got < 2 {
		t.Fatalf("puncak konkurensi %d: gerbang tidak boleh menyerialkan semuanya (kapasitas %d)", got, capacity)
	}
}

func TestHashGateVerifyAndHashGoThroughHasher(t *testing.T) {
	g, _ := NewHashGate(2, time.Second)
	h := &password.Hasher{Params: password.Params{MemoryKiB: 8192, Time: 1, Parallelism: 1}}
	phc, err := g.Hash(context.Background(), h, "sandi uji panjang")
	if err != nil {
		t.Fatal(err)
	}
	ok, rehash, err := g.Verify(context.Background(), h, "sandi uji panjang", phc)
	if err != nil || !ok || rehash {
		t.Fatalf("Verify benar: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, _ := g.Verify(context.Background(), h, "salah", phc); ok {
		t.Fatal("Verify sandi salah harus false")
	}
	// Galat Hasher (ErrInvalidHash) dirambatkan apa adanya, bukan dianggap 429.
	if _, _, err := g.Verify(context.Background(), h, "x", "bukan-phc"); !errors.Is(err, password.ErrInvalidHash) {
		t.Fatalf("err = %v, want ErrInvalidHash", err)
	}
	if _, ok := AsRateLimited(err); ok {
		t.Fatal("ErrInvalidHash bukan rate limited")
	}
}

func TestHashGateFullReturnsRateLimitedAfterWait(t *testing.T) {
	g, _ := NewHashGate(1, 30*time.Millisecond)
	hold, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = g.Do(context.Background(), func() error {
			close(hold)
			<-release
			return nil
		})
	}()
	<-hold
	defer close(release)

	start := time.Now()
	err := g.Do(context.Background(), func() error {
		t.Error("fn tidak boleh berjalan saat gerbang penuh")
		return nil
	})
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("menolak setelah %v, harus menunggu batas waktu 30ms", elapsed)
	}
	if !errors.Is(err, ErrHashBusy) {
		t.Fatalf("err = %v, want ErrHashBusy", err)
	}
	ra, ok := AsRateLimited(err)
	if !ok || ra < time.Second {
		t.Fatalf("AsRateLimited = (%v, %v), want Retry-After >= 1s", ra, ok)
	}
}

func TestHashGateWaitsForFreedSlot(t *testing.T) {
	g, _ := NewHashGate(1, 2*time.Second)
	hold, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = g.Do(context.Background(), func() error {
			close(hold)
			<-release
			return nil
		})
	}()
	<-hold
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(release)
	}()
	ran := false
	if err := g.Do(context.Background(), func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("penunggu harus berjalan setelah slot bebas: err=%v ran=%v", err, ran)
	}
}

func TestHashGateContextCancelIsNotRateLimited(t *testing.T) {
	g, _ := NewHashGate(1, 5*time.Second)
	hold, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = g.Do(context.Background(), func() error { close(hold); <-release; return nil })
	}()
	<-hold
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := g.Do(ctx, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if _, ok := AsRateLimited(err); ok {
		t.Fatal("pembatalan context bukan 429")
	}
	if err := g.Do(func() context.Context { c, cn := context.WithCancel(context.Background()); cn(); return c }(), func() error {
		t.Error("tidak boleh berjalan pada context yang sudah batal")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("context batal: err = %v", err)
	}
}

func TestHashGateReleasesSlotOnPanicAndError(t *testing.T) {
	g, _ := NewHashGate(1, 50*time.Millisecond)
	func() {
		defer func() { _ = recover() }()
		_ = g.Do(context.Background(), func() error { panic("boom") })
	}()
	if err := g.Do(context.Background(), func() error { return errBoom }); !errors.Is(err, errBoom) {
		t.Fatalf("galat fn harus dirambatkan: %v", err)
	}
	if err := g.Do(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("slot harus dilepas setelah panic dan galat: %v", err)
	}
}

func TestHashGateConfigValidation(t *testing.T) {
	for _, c := range []int{0, -1} {
		if _, err := NewHashGate(c, time.Second); err == nil {
			t.Errorf("kapasitas %d diterima", c)
		}
	}
	if _, err := NewHashGate(1, 0); err == nil {
		t.Error("batas waktu 0 diterima")
	}
	g, err := NewHashGate(2, DefaultHashGateWait)
	if err != nil || g.Capacity() != 2 {
		t.Fatalf("g=%v err=%v", g, err)
	}
}

func TestRateLimitedErrorAs(t *testing.T) {
	var err error = &RateLimitedError{RetryAfter: 7 * time.Second, Cause: ErrHashBusy}
	wrapped := errors.Join(errors.New("lapisan"), err)
	if ra, ok := AsRateLimited(wrapped); !ok || ra != 7*time.Second {
		t.Fatalf("AsRateLimited(wrapped) = (%v, %v)", ra, ok)
	}
	if _, ok := AsRateLimited(errBoom); ok {
		t.Fatal("galat biasa bukan rate limited")
	}
	if _, ok := AsRateLimited(nil); ok {
		t.Fatal("nil bukan rate limited")
	}
}
