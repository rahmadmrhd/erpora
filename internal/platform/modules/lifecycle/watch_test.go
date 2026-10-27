package lifecycle

import (
	"context"
	"testing"
	"time"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
)

// tickBarrier mengirim dua tick: tick kedua baru diterima setelah poll akibat tick pertama selesai.
func tickBarrier(ticks chan time.Time) {
	ticks <- time.Time{}
	ticks <- time.Time{}
}

func TestWatch_PublishesNewerRevisionOnce(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.revision[tenantA] = 3
	e.snap.Apply(tenantA, map[string]runtime.Status{}, 3)

	var got []int64 // hanya ditulis dari goroutine watch (Publish sinkron); dibaca setelah barrier
	e.notifier.Subscribe(func(ctx context.Context, tn tenancy.TenantID, rev int64) {
		got = append(got, rev)
		_, _ = e.svc.Refresh(ctx, tn) // subscriber produksi memanggil Refresh
	})
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() { e.svc.watch(ctx, tenantA, ticks); close(done) }()

	tickBarrier(ticks) // revision tetap (3 == snapshot): tidak ada publish
	if len(got) != 0 {
		t.Fatalf("publish tanpa perubahan: %v", got)
	}

	e.st.mu.Lock()
	e.st.revision[tenantA] = 4
	e.st.mu.Unlock()
	tickBarrier(ticks)
	if len(got) != 1 || got[0] != 4 || e.snap.Revision(tenantA) != 4 {
		t.Fatalf("publish = %v, snapshot revision = %d; want [4], 4", got, e.snap.Revision(tenantA))
	}
	tickBarrier(ticks) // tidak ada perubahan lagi
	tickBarrier(ticks)
	if len(got) != 1 {
		t.Fatalf("publish ulang: %v", got)
	}
	cancel()
	<-done
}

// Bila snapshot tetap tertinggal (Refresh subscriber gagal), revision yang sama dipublikasikan ulang
// setelah republishAfter tick.
func TestWatch_RepublishesWhenSnapshotStaysBehind(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.snap.Apply(tenantA, map[string]runtime.Status{}, 1)
	e.st.revision[tenantA] = 2
	n := 0
	e.notifier.Subscribe(func(context.Context, tenancy.TenantID, int64) { n++ }) // tidak me-Refresh
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() { e.svc.watch(ctx, tenantA, ticks); close(done) }()
	tickBarrier(ticks)
	if n != 1 {
		t.Fatalf("publish pertama = %d", n)
	}
	for range republishAfter {
		ticks <- time.Time{}
	}
	ticks <- time.Time{} // barrier
	if n != 2 {
		t.Fatalf("publish ulang = %d, want 2", n)
	}
	cancel()
	<-done
}
