package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"text/tabwriter"

	"github.com/spf13/cobra"
	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/app"
	"erpora/internal/devtools/doctor"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
)

func newDoctor(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Periksa lingkungan developer: go, bun, task, docker, PostgreSQL, .env",
		Long: "Memeriksa versi toolchain terhadap yang dipin repo (go.mod, packageManager, Taskfile), ketersediaan docker (hanya untuk db:up dan\n" +
			"test integrasi), keterjangkauan PostgreSQL (ERP_DATABASE_URL; password tidak pernah dicetak), dan keberadaan .env. Exit 1 bila ada fail.",
		GroupID: GroupDevtools,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			envFile := ""
			if opts.EnvFile != nil {
				envFile = opts.EnvFile()
			}
			dsn, cfgErr := "", ""
			cfg, err := config.Load(envFile)
			var ve config.ValidationError
			switch {
			case err == nil:
				dsn = cfg.DatabaseURL
			case errors.As(err, &ve) && ve.Var == "ERP_DATABASE_URL":
				// DSN belum diatur: dilaporkan oleh pemeriksaan postgres sebagai warn.
			default:
				cfgErr = err.Error() // galat konfigurasi lain (mis. ERP_ENV tidak valid): tampilkan apa adanya
			}
			rs := doctor.Run(c.Context(), doctor.Inputs{Root: root, EnvFile: envFile, DSN: dsn, ConfigErr: cfgErr, Ping: pingPostgres}, doctor.ExecProbe{})
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAMA\tSTATUS\tDETAIL")
			for _, r := range rs {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Status, r.Detail)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if doctor.Failed(rs) {
				return &app.ExitError{Code: 1, Err: errors.New("doctor menemukan masalah (status fail)")}
			}
			return nil
		},
	}
}

// pingPostgres membuka koneksi (Open sudah melakukan ping 5 detik dan tidak pernah membocorkan password) lalu menutupnya.
func pingPostgres(ctx context.Context, dsn string) error {
	db, err := database.Open(ctx, config.Config{DatabaseURL: dsn}, tnoop.NewTracerProvider(), slog.New(slog.DiscardHandler))
	if err != nil {
		return err
	}
	if s, err := db.DB(); err == nil {
		_ = s.Close()
	}
	return nil
}
