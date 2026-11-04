package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"erpora/internal/platform/tenancy"
)

// Retensi login_attempts (PRD-001 Q30 butir 3, ADR-0013 section 5): baris memuat user_id dan IP (data pribadi; UU PDP),
// jadi dihapus berkala dan tidak disimpan selamanya. 30 hari aman terhadap semua jendela lockout: hitungan pasangan
// memakai jendela 24 jam (Q19) dan ambang `locked` memakai jendela 1 jam (Q16).
const (
	// DefaultAttemptRetention adalah umur maksimum baris login_attempts sebelum dipangkas.
	DefaultAttemptRetention = 30 * 24 * time.Hour
	// DefaultRetentionInterval adalah jarak antar pemangkasan.
	DefaultRetentionInterval = time.Hour
	// retentionJobTimeout membatasi satu putaran pemangkasan agar koneksi DB tidak tertahan.
	retentionJobTimeout = time.Minute
)

// AttemptPruner adalah bagian *store.Store yang dipakai RetentionWorker.
type AttemptPruner interface {
	PruneLoginAttempts(ctx context.Context, t tenancy.TenantID, before time.Time) (int64, error)
}

// RetentionConfig merakit RetentionWorker.
type RetentionConfig struct {
	Store  AttemptPruner // wajib
	Tenant tenancy.TenantID
	// Retention adalah umur maksimum baris (bawaan DefaultAttemptRetention); harus >= PairWindow bawaan (24 jam) agar
	// jendela jeda pasangan tidak terpotong.
	Retention time.Duration
	Interval  time.Duration // bawaan DefaultRetentionInterval
	Now       func() time.Time
	Log       *slog.Logger
}

// RetentionWorker memangkas login_attempts yang lebih tua dari Retention. Aman dijalankan di beberapa instance: DELETE
// bersyarat waktu bersifat idempoten.
type RetentionWorker struct {
	st        AttemptPruner
	tenant    tenancy.TenantID
	retention time.Duration
	interval  time.Duration
	now       func() time.Time
	log       *slog.Logger
}

// NewRetentionWorker membuat RetentionWorker; Retention di bawah PairWindow bawaan (24 jam) ditolak.
func NewRetentionWorker(cfg RetentionConfig) (*RetentionWorker, error) {
	if cfg.Store == nil {
		return nil, errors.New("guard: RetentionConfig.Store wajib")
	}
	w := &RetentionWorker{st: cfg.Store, tenant: cfg.Tenant, retention: cfg.Retention, interval: cfg.Interval, now: cfg.Now, log: cfg.Log}
	if w.retention == 0 {
		w.retention = DefaultAttemptRetention
	}
	if min := DefaultLockoutConfig().PairWindow; w.retention < min {
		return nil, fmt.Errorf("guard: RetentionConfig.Retention harus >= %s (jendela hitungan pasangan)", min)
	}
	if w.interval <= 0 {
		w.interval = DefaultRetentionInterval
	}
	if w.now == nil {
		w.now = time.Now
	}
	if w.log == nil {
		w.log = slog.Default()
	}
	return w, nil
}

// Tick memangkas baris yang lebih tua dari now-Retention dan mengembalikan jumlah yang dihapus.
func (w *RetentionWorker) Tick(ctx context.Context, now time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(tenancy.WithTenant(ctx, w.tenant), retentionJobTimeout)
	defer cancel()
	n, err := w.st.PruneLoginAttempts(ctx, w.tenant, now.UTC().Add(-w.retention))
	if err != nil {
		return 0, fmt.Errorf("guard: pangkas login_attempts: %w", err)
	}
	return n, nil
}

// Run memangkas segera lalu tiap Interval sampai ctx selesai (berhenti bersih saat shutdown, seperti token.Promoter).
// Galat dicatat dan tidak menghentikan pekerja; log tidak memuat data pengguna (hanya jumlah baris).
func (w *RetentionWorker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		switch n, err := w.Tick(ctx, w.now()); {
		case err != nil:
			if ctx.Err() == nil {
				w.log.ErrorContext(ctx, "identity: pemangkasan login_attempts gagal", "error", err.Error())
			}
		case n > 0:
			w.log.InfoContext(ctx, "identity: login_attempts lama dipangkas", "rows", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
