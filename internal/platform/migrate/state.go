package migrate

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
)

const (
	keyDefaultTenant = "default_tenant_id"
	keyEnvironment   = "environment"

	// advisory lock key untuk inisialisasi platform_state (konstan arbitrer, unik per tujuan).
	stateLockKey int64 = 0x6572705f7374 // "erp_st"
)

// EnsurePlatformState memastikan default_tenant_id dan environment tercatat. Keduanya hanya
// ditulis sekali (ON CONFLICT DO NOTHING) dan tidak pernah ditimpa. Bila want != nil dan berbeda
// dengan tenant yang tercatat, mengembalikan error.
func EnsurePlatformState(ctx context.Context, db *database.DB, env config.Env, want *uuid.UUID) (uuid.UUID, error) {
	var tenant uuid.UUID
	err := db.WithContext(ctx).Transaction(func(tx *database.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", stateLockKey).Error; err != nil {
			return err
		}
		var candidate uuid.UUID
		if want != nil {
			candidate = *want
		} else {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			candidate = id
		}
		const ins = "INSERT INTO platform_state (key, value) VALUES (?, ?) ON CONFLICT (key) DO NOTHING"
		if err := tx.Exec(ins, keyDefaultTenant, candidate.String()).Error; err != nil {
			return err
		}
		if err := tx.Exec(ins, keyEnvironment, string(env)).Error; err != nil {
			return err
		}
		var v string
		if err := tx.Raw("SELECT value FROM platform_state WHERE key = ?", keyDefaultTenant).Scan(&v).Error; err != nil {
			return err
		}
		id, err := uuid.Parse(v)
		if err != nil {
			return fmt.Errorf("platform_state.%s bukan UUID valid: %w", keyDefaultTenant, err)
		}
		if want != nil && id != *want {
			return fmt.Errorf("ERP_DEFAULT_TENANT_ID (%s) berbeda dengan tenant default di database (%s)", *want, id)
		}
		tenant = id
		return nil
	})
	return tenant, err
}

// DefaultTenantID membaca tenant default yang tercatat di platform_state.
func DefaultTenantID(ctx context.Context, db *database.DB) (uuid.UUID, error) {
	var v string
	res := db.WithContext(ctx).Raw("SELECT value FROM platform_state WHERE key = ?", keyDefaultTenant).Scan(&v)
	if res.Error != nil {
		return uuid.Nil, res.Error
	}
	if res.RowsAffected == 0 {
		return uuid.Nil, fmt.Errorf("default tenant belum diinisialisasi; jalankan `erpora migrate up`")
	}
	return uuid.Parse(v)
}

// Environment membaca platform_state.environment (read-only). ok=false bila tabel platform_state belum ada
// atau barisnya belum ditulis. Nilai selain development|production adalah error.
func Environment(ctx context.Context, db *database.DB) (env config.Env, ok bool, err error) {
	var exists bool
	if err := db.WithContext(ctx).Raw("SELECT to_regclass('platform_state') IS NOT NULL").Scan(&exists).Error; err != nil {
		return "", false, err
	}
	if !exists {
		return "", false, nil
	}
	var v string
	res := db.WithContext(ctx).Raw("SELECT value FROM platform_state WHERE key = ?", keyEnvironment).Scan(&v)
	if res.Error != nil {
		return "", false, res.Error
	}
	if res.RowsAffected == 0 {
		return "", false, nil
	}
	switch e := config.Env(v); e {
	case config.EnvDevelopment, config.EnvProduction:
		return e, true, nil
	}
	return "", false, fmt.Errorf("platform_state.%s bernilai tidak dikenal %q (harus %s atau %s)", keyEnvironment, v, config.EnvDevelopment, config.EnvProduction)
}
