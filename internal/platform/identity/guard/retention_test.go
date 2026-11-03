package guard

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/tenancy"
)

type fakePruner struct {
	mu      sync.Mutex
	befores []time.Time
	tenants []tenancy.TenantID
	ctxT    []bool // apakah ctx membawa tenant
	err     error
	called  chan struct{}
}

func (f *fakePruner) PruneLoginAttempts(ctx context.Context, t tenancy.TenantID, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.befores = append(f.befores, before)
	f.tenants = append(f.tenants, t)
	_, ok := tenancy.FromContext(ctx)
	f.ctxT = append(f.ctxT, ok)
	select {
	case f.called <- struct{}{}:
	default:
	}
	return 3, f.err
}

func (f *fakePruner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.befores)
}

var retTenant = tenancy.TenantID{1}

func TestRetentionTickPrunesOlderThanThirtyDays(t *testing.T) {
	fp := &fakePruner{}
	w, err := NewRetentionWorker(RetentionConfig{Store: fp, Tenant: retTenant, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	n, err := w.Tick(context.Background(), now)
	if err != nil || n != 3 {
		t.Fatalf("Tick = %d %v", n, err)
	}
	if want := now.Add(-30 * 24 * time.Hour); !fp.befores[0].Equal(want) {
		t.Errorf("before = %v, want %v (30 hari)", fp.befores[0], want)
	}
	if fp.tenants[0] != retTenant || !fp.ctxT[0] {
		t.Errorf("tenant argumen=%v ctx=%v", fp.tenants[0], fp.ctxT[0])
	}
	// Retensi bawaan lebih panjang dari semua jendela lockout.
	if c := DefaultLockoutConfig(); DefaultAttemptRetention < c.PairWindow || DefaultAttemptRetention < c.AccountWindow {
		t.Error("retensi bawaan lebih pendek dari jendela lockout")
	}
}

func TestRetentionRejectsRetentionShorterThanPairWindow(t *testing.T) {
	if _, err := NewRetentionWorker(RetentionConfig{Store: &fakePruner{}, Retention: 23 * time.Hour}); err == nil {
		t.Error("retensi 23 jam harus ditolak (jendela pasangan 24 jam)")
	}
	if _, err := NewRetentionWorker(RetentionConfig{}); err == nil {
		t.Error("Store wajib")
	}
}

func TestRetentionRunPrunesImmediatelyPeriodicallyAndStopsOnCancel(t *testing.T) {
	fp := &fakePruner{err: errors.New("db sementara"), called: make(chan struct{}, 16)}
	w, err := NewRetentionWorker(RetentionConfig{Store: fp, Tenant: retTenant, Interval: 5 * time.Millisecond, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	for range 3 { // putaran langsung + berkala; galat tidak menghentikan pekerja
		select {
		case <-fp.called:
		case <-time.After(5 * time.Second):
			t.Fatal("pemangkasan tidak berjalan berkala")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run tidak berhenti setelah ctx dibatalkan")
	}
	n := fp.calls()
	time.Sleep(30 * time.Millisecond)
	if fp.calls() != n {
		t.Error("pemangkasan berlanjut setelah Run berhenti")
	}
}
