package lifecycle

import (
	"context"
	"time"

	"erpora/internal/platform/tenancy"
)

// DefaultPollInterval adalah interval polling revision antarproses (keputusan 4); kelak diganti LISTEN/NOTIFY.
const DefaultPollInterval = 2 * time.Second

// republishAfter: bila snapshot masih tertinggal setelah sekian tick sejak publish (mis. Refresh subscriber
// gagal), revision yang sama dipublikasikan ulang.
const republishAfter = 10

// Watch mempublikasikan revision DB yang lebih baru dari snapshot ke Notifier sampai ctx selesai.
// Subscriber (internal/app) memanggil Refresh sehingga snapshot ikut maju.
func (s *Service) Watch(ctx context.Context, t tenancy.TenantID, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.watch(ctx, t, ticker.C)
}

// watch adalah inti Watch dengan sumber tick yang dapat diinjeksi (test deterministik: kirim tick lewat
// channel tak-berbuffer; pengiriman tick berikutnya baru diterima setelah poll sebelumnya selesai).
func (s *Service) watch(ctx context.Context, t tenancy.TenantID, ticks <-chan time.Time) {
	var last int64
	waited := 0
	for {
		s.poll(ctx, t, &last, &waited)
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

func (s *Service) poll(ctx context.Context, t tenancy.TenantID, last *int64, waited *int) {
	if s.d.Notifier == nil || s.d.Snapshot == nil {
		return
	}
	rev, err := s.d.Store.Revision(ctx, t)
	if err != nil {
		if ctx.Err() == nil {
			s.log.WarnContext(ctx, "polling revision modul gagal", "error", err)
		}
		return
	}
	_, snapRev := s.d.Snapshot.Statuses(t)
	if rev <= snapRev {
		*last, *waited = 0, 0
		return
	}
	if rev == *last {
		*waited++
		if *waited < republishAfter {
			return
		}
	}
	*last, *waited = rev, 0
	s.d.Notifier.Publish(ctx, t, rev)
}
