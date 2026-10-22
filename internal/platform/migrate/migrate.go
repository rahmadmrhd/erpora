// Package migrate menjalankan migrasi goose platform (target control dan tenant).
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/rahmadmrhd/erp-go/internal/platform/config"
	"github.com/rahmadmrhd/erp-go/internal/platform/database"
	"github.com/rahmadmrhd/erp-go/migrations"
)

// Target adalah kelompok migrasi: "control" atau "tenant".
type Target string

const (
	Control Target = "control"
	Tenant  Target = "tenant"
)

// TargetStatus ringkasan keadaan satu target.
type TargetStatus struct {
	Target          Target
	Current, Latest int64
	Pending         int
}

var targets = []Target{Control, Tenant}

func provider(db *sql.DB, t Target) (*goose.Provider, error) {
	sub, err := fs.Sub(migrations.FS, string(t))
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, sub,
		goose.WithTableName("goose_"+string(t)),
		goose.WithSessionLocker(locker),
	)
}

// Up menjalankan migrasi control lalu tenant, kemudian EnsurePlatformState.
func Up(ctx context.Context, db *database.DB, env config.Env) error {
	return UpWithTenant(ctx, db, env, nil)
}

// UpWithTenant seperti Up, tetapi memakai want (ERP_DEFAULT_TENANT_ID) sebagai tenant default
// pada database baru dan menolak bila berbeda dengan yang tercatat.
func UpWithTenant(ctx context.Context, db *database.DB, env config.Env, want *uuid.UUID) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	for _, t := range targets {
		p, err := provider(sqlDB, t)
		if err != nil {
			return fmt.Errorf("migrasi %s: %w", t, err)
		}
		// Jangan p.Close(): itu menutup *sql.DB milik pemanggil.
		_, err = p.Up(ctx)
		if err != nil {
			return fmt.Errorf("migrasi %s: %w", t, err)
		}
	}
	_, err = EnsurePlatformState(ctx, db, env, want)
	return err
}

// Status melaporkan keadaan tiap target. Murni baca (read-only): tidak membuat tabel goose_*;
// tabel versi yang belum ada berarti Current=0 dan seluruh migrasi target itu pending.
func Status(ctx context.Context, db *database.DB) ([]TargetStatus, error) {
	var out []TargetStatus
	for _, t := range targets {
		embedded, err := embeddedVersions(t)
		if err != nil {
			return nil, err
		}
		applied, err := appliedVersions(ctx, db, t)
		if err != nil {
			return nil, err
		}
		st := TargetStatus{Target: t}
		for _, v := range embedded {
			if v > st.Latest {
				st.Latest = v
			}
			if !applied[v] {
				st.Pending++
			}
		}
		for v := range applied {
			if v > st.Current {
				st.Current = v
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// Pending mengembalikan total migrasi yang belum diterapkan di semua target (read-only, lihat Status).
func Pending(ctx context.Context, db *database.DB) (int, error) {
	sts, err := Status(ctx, db)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range sts {
		n += s.Pending
	}
	return n, nil
}

// appliedVersions membaca versi yang sudah diterapkan lewat to_regclass + SELECT (tanpa menulis).
func appliedVersions(ctx context.Context, db *database.DB, t Target) (map[int64]bool, error) {
	table := "goose_" + string(t)
	var reg *string
	if err := db.WithContext(ctx).Raw("SELECT to_regclass(?)::text", table).Scan(&reg).Error; err != nil {
		return nil, fmt.Errorf("status %s: %w", t, err)
	}
	applied := map[int64]bool{}
	if reg == nil {
		return applied, nil
	}
	var rows []int64
	// Nama tabel berasal dari konstanta internal, bukan input pengguna.
	if err := db.WithContext(ctx).Raw("SELECT version_id FROM " + table + " WHERE is_applied").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("status %s: %w", t, err)
	}
	for _, v := range rows {
		if v > 0 { // baris versi 0 adalah penanda awal goose
			applied[v] = true
		}
	}
	return applied, nil
}

// embeddedVersions membaca nomor versi dari nama file migrasi tertanam ("0001_nama.sql").
func embeddedVersions(t Target) ([]int64, error) {
	entries, err := fs.ReadDir(migrations.FS, string(t))
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, _ := strings.Cut(name, "_")
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("nama migrasi %s/%s tidak valid: %w", t, name, err)
		}
		out = append(out, v)
	}
	return out, nil
}
