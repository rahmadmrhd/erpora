package runtime

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"erpora/internal/platform/tenancy"
)

// StateNotifier menyebarkan perubahan revision status modul. Implementasi in-process (LocalNotifier)
// dipakai sekarang; kelak diganti LISTEN/NOTIFY tanpa mengubah pemanggil (PRD-000 FR-19.7).
type StateNotifier interface {
	Publish(ctx context.Context, t tenancy.TenantID, revision int64)
	Subscribe(fn func(ctx context.Context, t tenancy.TenantID, revision int64)) (cancel func())
}

// LocalNotifier adalah StateNotifier in-process.
type LocalNotifier struct {
	mu   sync.Mutex
	next int
	subs map[int]func(context.Context, tenancy.TenantID, int64)
}

// NewLocalNotifier membuat notifier in-process.
func NewLocalNotifier() *LocalNotifier {
	return &LocalNotifier{subs: map[int]func(context.Context, tenancy.TenantID, int64){}}
}

// Subscribe mendaftarkan fn; hasilnya membatalkan langganan.
func (n *LocalNotifier) Subscribe(fn func(ctx context.Context, t tenancy.TenantID, revision int64)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	id := n.next
	n.next++
	n.subs[id] = fn
	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(n.subs, id)
	}
}

// Publish memanggil semua subscriber secara sinkron (urutan langganan). Panic subscriber di-recover
// dan dicatat; subscriber lain tetap dipanggil.
func (n *LocalNotifier) Publish(ctx context.Context, t tenancy.TenantID, revision int64) {
	n.mu.Lock()
	ids := make([]int, 0, len(n.subs))
	for id := range n.subs {
		ids = append(ids, id)
	}
	n.mu.Unlock()
	slices.Sort(ids)
	for _, id := range ids {
		n.mu.Lock()
		fn, ok := n.subs[id]
		n.mu.Unlock()
		if ok {
			call(ctx, fn, t, revision)
		}
	}
}

func call(ctx context.Context, fn func(context.Context, tenancy.TenantID, int64), t tenancy.TenantID, revision int64) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "subscriber StateNotifier panic", "panic", r, "tenant", t.String(), "revision", revision)
		}
	}()
	fn(ctx, t, revision)
}
