package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
)

const (
	lockBackoffMin = 50 * time.Millisecond
	lockBackoffMax = 500 * time.Millisecond
)

// ErrLockTimeout dikembalikan WithTenantLock bila lock tidak diperoleh dalam batas waktu.
var ErrLockTimeout = errors.New("store: lock lifecycle tenant tidak diperoleh")

func lockName(t tenancy.TenantID) string {
	return "erp.module.lifecycle:" + t.String()
}

// WithTenantLock menjalankan fn sambil memegang advisory lock lifecycle tenant t. Lock dipegang pada satu
// koneksi khusus (session-level) dan dicoba dengan pg_try_advisory_lock + backoff 50ms -> 500ms sampai
// timeout; kegagalan menghasilkan ErrLockTimeout. fn berjalan pada ctx yang sama dan memakai koneksi lain
// dari pool. Lock selalu dilepas (juga saat fn panic atau ctx dibatalkan) memakai context.WithoutCancel;
// bila unlock gagal, pg_advisory_unlock_all() dijalankan agar koneksi bersih saat kembali ke pool.
func (s *Store) WithTenantLock(ctx context.Context, t tenancy.TenantID, timeout time.Duration, fn func(ctx context.Context) error) error {
	name := lockName(t)
	return s.control.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		deadline := time.Now().Add(timeout)
		backoff := lockBackoffMin
		for {
			var got bool
			if err := conn.WithContext(ctx).Raw("SELECT pg_try_advisory_lock(hashtextextended(?, 0))", name).Scan(&got).Error; err != nil {
				return fmt.Errorf("store: ambil lock lifecycle: %w", err)
			}
			if got {
				break
			}
			wait := min(backoff, time.Until(deadline))
			if wait <= 0 {
				return ErrLockTimeout
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			backoff = min(backoff*2, lockBackoffMax)
		}
		defer func() {
			uctx := context.WithoutCancel(ctx)
			var released bool
			err := conn.WithContext(uctx).Raw("SELECT pg_advisory_unlock(hashtextextended(?, 0))", name).Scan(&released).Error
			if err != nil || !released {
				// Koneksi tidak boleh kembali ke pool dengan lock tersisa.
				_ = conn.WithContext(uctx).Exec("SELECT pg_advisory_unlock_all()").Error
			}
		}()
		return fn(ctx)
	})
}
