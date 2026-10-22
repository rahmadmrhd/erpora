package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rahmadmrhd/erp-go/internal/app"
	"github.com/rahmadmrhd/erp-go/internal/platform/config"
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
			return app.Run(ctx, cfg, cmd.OutOrStdout())
		},
	}
}
