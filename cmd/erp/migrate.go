package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/observability"
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
				if err := migrate.UpWithTenant(c.Context(), db, cfg.Env, cfg.DefaultTenantID); err != nil {
					return err
				}
				st, err := migrate.Status(c.Context(), db)
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
				st, err := migrate.Status(c.Context(), db)
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

// printStatus mencetak tabel TARGET CURRENT LATEST PENDING.
func printStatus(w io.Writer, st []migrate.TargetStatus) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TARGET\tCURRENT\tLATEST\tPENDING")
	for _, s := range st {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n", s.Target, s.Current, s.Latest, s.Pending)
	}
	_ = tw.Flush()
}
