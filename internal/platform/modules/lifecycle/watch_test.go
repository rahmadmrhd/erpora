package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
)

func TestWatch_PublishesNewerRevisionOnce(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.revision[tenantA] = 3
	e.snap.Apply(tenantA, map[string]runtime.Status{}, 3)

	var mu sync.Mutex
	var got []int64
	e.notifier.Subscribe(func(ctx context.Context, tn tenancy.TenantID, rev int64) {
		mu.Lock()
		got = append(got, rev)
		mu.Unlock()
		_, _ = e.svc.Refresh(ctx, tn) // subscriber produksi memanggil Refresh
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.svc.Watch(ctx, tenantA, 5*time.Millisecond); close(done) }()

	time.Sleep(50 * time.Millisecond) // revision tetap: tidak ada publish
	mu.Lock()
	if len(got) != 0 {
		t.Fatalf("publish tanpa perubahan: %v", got)
	}
	mu.Unlock()

	e.st.mu.Lock()
	e.st.revision[tenantA] = 4
	e.st.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // beberapa tick lagi: tidak boleh publish ulang
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != 4 {
		t.Fatalf("publish = %v, want [4]", got)
	}
	if e.snap.Revision(tenantA) != 4 {
		t.Error("snapshot belum diperbarui oleh subscriber")
	}
}
