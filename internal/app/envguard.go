package app

import (
	"context"
	"errors"
	"fmt"

	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
)

// ErrProductionDatabase ditolaknya command tulis binary dev (PRD-000 FR-22.5).
var ErrProductionDatabase = errors.New("database berpenanda production (atau belum berpenanda dengan ERP_ENV=production): binary dev menolak command yang menulis; pakai binary release")

// ErrNotDevelopment ditolaknya command destruktif (db:fresh/db:seed) di luar development.
var ErrNotDevelopment = errors.New("db:fresh/db:seed hanya untuk ERP_ENV=development dan database berpenanda development")

// decideDevWrite murni (tanpa I/O): flavor != "dev" -> nil; marker production -> ErrProductionDatabase;
// tanpa marker dengan env production -> ErrProductionDatabase (agar migrate up binary dev tidak menginisialisasi
// DB production); selain itu nil.
func decideDevWrite(flavor string, marker config.Env, hasMarker bool, env config.Env) error {
	if flavor != "dev" {
		return nil
	}
	if hasMarker && marker == config.EnvProduction {
		return ErrProductionDatabase
	}
	if !hasMarker && env == config.EnvProduction {
		return ErrProductionDatabase
	}
	return nil
}

// GuardDevWrite menolak command tulis binary dev terhadap DB production (exit 3). Hanya membaca penanda;
// binary release tidak terpengaruh.
func GuardDevWrite(ctx context.Context, db *database.DB, cfg config.Config) error {
	if buildinfo.Flavor() != "dev" {
		return nil
	}
	marker, has, err := migrate.Environment(ctx, db)
	if err != nil {
		return fmt.Errorf("baca penanda lingkungan: %w", err)
	}
	if err := decideDevWrite(buildinfo.Flavor(), marker, has, cfg.Env); err != nil {
		return &ExitError{Code: 3, Err: err}
	}
	return nil
}

// DestructiveEnvAllowed: cfg.Env != development -> ExitError{3, ErrNotDevelopment}. Dipanggil SEBELUM DB dibuka.
func DestructiveEnvAllowed(cfg config.Config) error {
	if cfg.Env != config.EnvDevelopment {
		return &ExitError{Code: 3, Err: ErrNotDevelopment}
	}
	return nil
}

// GuardDestructive: tanpa marker atau marker bukan development -> ExitError{3, ErrNotDevelopment}.
func GuardDestructive(ctx context.Context, db *database.DB, _ config.Config) error {
	marker, has, err := migrate.Environment(ctx, db)
	if err != nil {
		return fmt.Errorf("baca penanda lingkungan: %w", err)
	}
	if !has {
		return &ExitError{Code: 3, Err: fmt.Errorf("%w: database belum berpenanda; jalankan `erpora migrate up` dulu", ErrNotDevelopment)}
	}
	if marker != config.EnvDevelopment {
		return &ExitError{Code: 3, Err: ErrNotDevelopment}
	}
	return nil
}
