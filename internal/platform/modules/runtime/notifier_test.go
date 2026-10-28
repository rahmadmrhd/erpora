package runtime

import (
	"context"
	"testing"

	"erpora/internal/platform/tenancy"
)

func TestLocalNotifier_PublishToSubscribers(t *testing.T) {
	n := NewLocalNotifier()
	var a, b []int64
	cancelA := n.Subscribe(func(_ context.Context, tn tenancy.TenantID, rev int64) {
		if tn == tenantA {
			a = append(a, rev)
		}
	})
	n.Subscribe(func(_ context.Context, _ tenancy.TenantID, rev int64) { b = append(b, rev) })

	n.Publish(context.Background(), tenantA, 7)
	if len(a) != 1 || a[0] != 7 || len(b) != 1 || b[0] != 7 {
		t.Fatalf("a=%v b=%v", a, b)
	}
	cancelA()
	n.Publish(context.Background(), tenantA, 8)
	if len(a) != 1 {
		t.Errorf("subscriber yang dibatalkan masih menerima: %v", a)
	}
	if len(b) != 2 || b[1] != 8 {
		t.Errorf("b = %v", b)
	}
}

func TestLocalNotifier_PanickingSubscriberDoesNotStopOthers(t *testing.T) {
	n := NewLocalNotifier()
	got := 0
	n.Subscribe(func(context.Context, tenancy.TenantID, int64) { panic("boom") })
	n.Subscribe(func(context.Context, tenancy.TenantID, int64) { got++ })
	n.Publish(context.Background(), tenantA, 1)
	if got != 1 {
		t.Errorf("subscriber kedua dipanggil %d kali, want 1", got)
	}
}
