package database

import (
	"context"
	"sync"
)

type counterKey struct{}

// queryCounter menghitung query per bentuk SQL (string SQL tanpa nilai parameter) dalam satu request.
type queryCounter struct {
	mu        sync.Mutex
	threshold int
	counts    map[string]int
}

// WithQueryCounter memasang penghitung N+1 pada ctx (satu per request). threshold <= 0 mengembalikan ctx
// apa adanya (deteksi mati). Peringatan dicatat oleh logger GORM saat hitungan satu bentuk SQL mencapai
// threshold+1, sekali per bentuk per request (FR-07 guardrail 5).
func WithQueryCounter(ctx context.Context, threshold int) context.Context {
	if threshold <= 0 {
		return ctx
	}
	return context.WithValue(ctx, counterKey{}, &queryCounter{threshold: threshold, counts: map[string]int{}})
}

// observe mencatat satu query; mengembalikan (hitungan, threshold, true) tepat saat hitungan == threshold+1.
func observe(ctx context.Context, sql string) (count, threshold int, warn bool) {
	c, ok := ctx.Value(counterKey{}).(*queryCounter)
	if !ok {
		return 0, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[sql]++
	return c.counts[sql], c.threshold, c.counts[sql] == c.threshold+1
}
