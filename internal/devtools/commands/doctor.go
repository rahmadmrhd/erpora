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
			cfg, err := config.Load(envFile)
			in := doctorInputs(cfg, err)
			in.Root, in.EnvFile, in.Ping = root, envFile, pingPostgres
			rs := doctor.Run(c.Context(), in, doctor.ExecProbe{})
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

// doctorInputs memetakan hasil config.Load ke masukan doctor. Galat ERP_SECRET_KEY dan ERP_SECRET_KEY_PREVIOUS adalah temuan tersendiri: DSN dari
// Config parsial tetap dipakai agar pemeriksaan postgres tidak hilang.
func doctorInputs(cfg config.Config, err error) doctor.Inputs {
	var ve config.ValidationError
	switch {
	case err == nil:
		in := doctor.Inputs{DSN: cfg.DatabaseURL, SecretKeyChecked: true}
		if kerr := cfg.RequireSecretKey(); kerr != nil { // kunci kosong: serve akan gagal
			in.SecretKeyErr = kerr.Error()
		}
		return in
	case errors.As(err, &ve) && (ve.Var == "ERP_SECRET_KEY" || ve.Var == "ERP_SECRET_KEY_PREVIOUS"):
		// Keduanya membuat serve gagal: FAIL pada temuan kunci (pesan menyebut variabel yang bermasalah), bukan ConfigErr.
		return doctor.Inputs{DSN: cfg.DatabaseURL, SecretKeyChecked: true, SecretKeyErr: err.Error()}
	case errors.As(err, &ve) && ve.Var == "ERP_DATABASE_URL":
		return doctor.Inputs{} // DSN belum diatur: dilaporkan oleh pemeriksaan postgres sebagai warn
	default:
		return doctor.Inputs{ConfigErr: err.Error()} // galat konfigurasi lain (mis. ERP_ENV tidak valid): tampilkan apa adanya
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
