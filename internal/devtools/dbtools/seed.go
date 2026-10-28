package dbtools

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"erpora/internal/app"
	"erpora/internal/platform/config"
)

// Seed menjalankan DevSeeder modul enabled (atau satu modul bila only != ""). Pemeriksaan ERP_ENV terjadi sebelum
// DB dibuka; DB harus berpenanda development (selain itu exit 3, termasuk DB yang belum dimigrasi).
func Seed(ctx context.Context, cfg config.Config, log *slog.Logger, only string, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if err := app.DestructiveEnvAllowed(cfg); err != nil {
		return err
	}
	db, cleanup, err := openDB(ctx, cfg, log)
	if err != nil {
		return err
	}
	err = app.GuardDestructive(ctx, db, cfg)
	cleanup()
	if err != nil {
		return err
	}
	return runSeed(ctx, cfg, log, only, out)
}

func runSeed(ctx context.Context, cfg config.Config, log *slog.Logger, only string, out io.Writer) error {
	keys, err := app.RunDevSeeders(ctx, cfg, log, only)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		fmt.Fprintln(out, "tidak ada modul yang di-seed (tidak ada DevSeeder pada modul enabled)")
		return nil
	}
	for _, k := range keys {
		fmt.Fprintf(out, "seed %s selesai\n", k)
	}
	return nil
}
