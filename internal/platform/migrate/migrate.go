// Package migrate menjalankan migrasi goose platform (target control dan tenant).
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

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
