package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/config"
)

func newServeCmd(opts *rootOpts) *cobra.Command {
	return &cobra.Command{
		Use:     "serve",
		Short:   "Jalankan server HTTP (API + frontend ter-embed)",
		GroupID: groupRuntime,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(opts.envFile)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			// Begitu sinyal pertama diterima (shutdown anggun dimulai), kembalikan penanganan sinyal ke default:
			// SIGINT/SIGTERM kedua mengakhiri proses langsung, tidak menunggu shutdown yang macet.
			go func() {
				<-ctx.Done()
				stop()
			}()
			log := app.NewLogger(cfg, cmd.OutOrStdout())
			slog.SetDefault(log) // slog.Default() = logger platform agar log handler modul (slog.ErrorContext) seragam
			return app.Run(ctx, cfg, log)
		},
	}
}
