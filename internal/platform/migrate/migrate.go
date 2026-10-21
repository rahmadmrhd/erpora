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

// Status melaporkan keadaan tiap target. Catatan: goose membuat tabel versi (goose_*) bila belum ada.
func Status(ctx context.Context, db *database.DB) ([]TargetStatus, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	var out []TargetStatus
	for _, t := range targets {
		p, err := provider(sqlDB, t)
		if err != nil {
			return nil, fmt.Errorf("status %s: %w", t, err)
		}
		st, err := statusOf(ctx, p, t)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func statusOf(ctx context.Context, p *goose.Provider, t Target) (TargetStatus, error) {
	cur, latest, err := p.GetVersions(ctx)
	if err != nil {
		return TargetStatus{}, fmt.Errorf("status %s: %w", t, err)
	}
	ms, err := p.Status(ctx)
	if err != nil {
		return TargetStatus{}, fmt.Errorf("status %s: %w", t, err)
	}
	pending := 0
	for _, m := range ms {
		if m.State == goose.StatePending {
			pending++
		}
	}
	return TargetStatus{Target: t, Current: cur, Latest: latest, Pending: pending}, nil
}

// Pending mengembalikan total migrasi yang belum diterapkan di semua target.
// Murni baca (read-only): tidak membuat tabel apa pun, aman untuk /readyz dan startup serve.
// Tabel versi goose yang belum ada berarti seluruh migrasi target itu pending.
func Pending(ctx context.Context, db *database.DB) (int, error) {
	total := 0
	for _, t := range targets {
		embedded, err := embeddedVersions(t)
		if err != nil {
			return 0, err
		}
		table := "goose_" + string(t)
		var reg *string
		if err := db.WithContext(ctx).Raw("SELECT to_regclass(?)::text", table).Scan(&reg).Error; err != nil {
			return 0, fmt.Errorf("pending %s: %w", t, err)
		}
		applied := map[int64]bool{}
		if reg != nil {
			var rows []int64
			// Nama tabel berasal dari konstanta internal, bukan input pengguna.
			if err := db.WithContext(ctx).Raw("SELECT version_id FROM " + table + " WHERE is_applied").Scan(&rows).Error; err != nil {
				return 0, fmt.Errorf("pending %s: %w", t, err)
			}
			for _, v := range rows {
				applied[v] = true
			}
		}
		for _, v := range embedded {
			if !applied[v] {
				total++
			}
		}
	}
	return total, nil
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
