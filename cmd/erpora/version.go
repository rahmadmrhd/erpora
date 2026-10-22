package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"erpora/internal/platform/buildinfo"
	"erpora/sdk/module"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Tampilkan versi platform, SDK, dan info build",
		GroupID: groupInspect,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "platform: %s\n", buildinfo.Version)
			fmt.Fprintf(w, "sdk: %s\n", module.SDKVersion)
			fmt.Fprintf(w, "commit: %s\n", buildinfo.Commit)
			fmt.Fprintf(w, "built: %s\n", buildinfo.BuildTime)
			fmt.Fprintf(w, "flavor: %s\n", buildinfo.Flavor())
			return nil
		},
	}
}
