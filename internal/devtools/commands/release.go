package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/release"
)

func registerRelease(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:   "release:verify-tag <tag>",
		Short: "Periksa tag rilis (modules/<dir>/vX.Y.Z atau sdk/vX.Y.Z) terhadap versi di sumber",
		Long: "Memastikan versi pada tag sama dengan version di modules/<dir>/module.json (dan go.mod ada dengan path yang benar) atau dengan\n" +
			"SDKVersion di sdk/module/version.go. Dipakai workflow rilis (.github/workflows/release.yml). Exit 1 bila tidak cocok.",
		GroupID: GroupDevtools,
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			if err := release.VerifyTag(root, args[0]); err != nil {
				return &app.ExitError{Code: 1, Err: err}
			}
			fmt.Fprintf(c.OutOrStdout(), "OK %s\n", args[0])
			return nil
		},
	})
}
