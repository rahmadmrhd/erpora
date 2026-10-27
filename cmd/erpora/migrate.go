package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

// openDB memuat config, menyiapkan observability, dan membuka database. cleanup wajib dipanggil.
func openDB(ctx context.Context, opts *rootOpts, errOut io.Writer) (config.Config, *database.DB, func(), error) {
	cfg, err := config.Load(opts.envFile)
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	log := observability.NewLogger(errOut, cfg.LogFormat, slog.LevelInfo)
	prov, shutdownOTel, err := observability.Setup(ctx, observability.Options{
		ServiceVersion: buildinfo.Version, Environment: string(cfg.Env),
	})
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	db, err := database.Open(ctx, cfg, prov.Tracer, log)
	if err != nil {
		_ = shutdownOTel(ctx)
		return config.Config{}, nil, nil, err
	}
	cleanup := func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
		_ = shutdownOTel(context.Background())
	}
	return cfg, db, cleanup, nil
}

func newMigrateCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "migrate",
		Short:   "Kelola migrasi database platform",
		GroupID: groupRuntime,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "up",
			Short: "Terapkan semua migrasi (control lalu tenant) dan inisialisasi platform_state",
			Args:  cobra.NoArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				cfg, db, cleanup, err := openDB(c.Context(), opts, c.ErrOrStderr())
				if err != nil {
					return err
				}
				defer cleanup()
				// Binary dev menolak menulis ke DB berpenanda production, atau DB tanpa penanda dengan ERP_ENV=production.
				if err := app.GuardDevWrite(c.Context(), db, cfg); err != nil {
					return err
				}
				if err := migrate.UpWithTenant(c.Context(), db, cfg.Env, cfg.DefaultTenantID); err != nil {
					return err
				}
				log := observability.NewLogger(c.ErrOrStderr(), cfg.LogFormat, slog.LevelInfo)
				if err := app.EnsureRequiredModules(c.Context(), cfg, log); err != nil {
					return err
				}
				st, err := collectStatus(c.Context(), db)
				if err != nil {
					return err
				}
				printStatus(c.OutOrStdout(), st)
				return nil
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "Tampilkan status migrasi per target",
			Args:  cobra.NoArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				_, db, cleanup, err := openDB(c.Context(), opts, c.ErrOrStderr())
				if err != nil {
					return err
				}
				defer cleanup()
				st, err := collectStatus(c.Context(), db)
				if err != nil {
					return err
				}
				printStatus(c.OutOrStdout(), st)
				return nil
			},
		},
	)
	return cmd
}

// collectStatus menggabungkan status control dan tenant dengan satu baris `module:<key>` untuk setiap
// modul yang punya baris installation (bukan available) dan membawa migrasi (FR-07). Read-only; baris modul
// dilewati bila migrasi platform belum diterapkan (tabel lifecycle belum ada).
func collectStatus(ctx context.Context, db *database.DB) ([]migrate.TargetStatus, error) {
	st, err := migrate.Status(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, s := range st {
		if s.Pending > 0 {
			return st, nil
		}
	}
	reg, err := app.BuildRegistry()
	if err != nil {
		return nil, &app.ExitError{Code: 2, Err: fmt.Errorf("katalog modul tidak valid: %w", err)}
	}
	tid, err := migrate.DefaultTenantID(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(database.ControlDB{DB: db}, nil).Installations(ctx, tenancy.TenantID(tid))
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, r := range rows {
		have[r.ModuleKey] = true
	}
	for _, e := range reg.Entries() {
		src, ok := e.Module.(module.MigrationSource)
		if !ok || !have[e.Manifest.Key] {
			continue
		}
		ms, err := migrate.ModuleStatus(ctx, db, e.Manifest.Slug, src.Migrations())
		if err != nil {
			return nil, err
		}
		ms.Target = migrate.Target("module:" + e.Manifest.Key)
		st = append(st, ms)
	}
	return st, nil
}

// printStatus mencetak tabel TARGET CURRENT LATEST PENDING.
func printStatus(w io.Writer, st []migrate.TargetStatus) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TARGET\tCURRENT\tLATEST\tPENDING")
	for _, s := range st {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n", s.Target, s.Current, s.Latest, s.Pending)
	}
	_ = tw.Flush()
}
