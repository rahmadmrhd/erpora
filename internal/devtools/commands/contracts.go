package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/contracttools"
)

// usageErr adalah kesalahan argumen (exit 2).
func usageErr(format string, a ...any) error {
	return &app.ExitError{Code: 2, Err: fmt.Errorf(format, a...)}
}

func registerContracts(root *cobra.Command) {
	root.AddCommand(newContractsLint())
}

func newContractsLint() *cobra.Command {
	return &cobra.Command{
		Use:     "contracts:lint [module]",
		Short:   "Lint kontrak OpenAPI dengan Spectral (api-guidelines dan klasifikasi data)",
		Long:    "Menjalankan ruleset contracts/spectral.yaml (Spectral terpin, dijalankan Bun) atas common, platform, dan kontrak modul.\nTanpa argumen semua kontrak; dengan argumen satu modul (dir, slug, atau key). Exit 1 bila ada temuan, 2 bila modul tidak dikenal.",
		GroupID: GroupContracts,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			repo, err := contracttools.LoadRepo(root)
			if err != nil {
				return err
			}
			only := ""
			if len(args) == 1 {
				m, ok := repo.Module(args[0])
				if !ok {
					return usageErr("modul %q tidak dikenal; modul: %s", args[0], repo.ModuleNames())
				}
				only = m.Dir
			}
			bun, err := contracttools.FindBun()
			if err != nil {
				return err
			}
			res, err := contracttools.Lint(c.Context(), repo, only, bun)
			if err != nil {
				return err
			}
			for _, r := range res {
				fmt.Fprintf(c.OutOrStdout(), "%s:%d %s %s\n", r.File, r.Line, r.Rule, r.Message)
			}
			if len(res) > 0 {
				return &app.ExitError{Code: 1, Err: fmt.Errorf("%d temuan kontrak", len(res))}
			}
			fmt.Fprintln(c.OutOrStdout(), "kontrak OK")
			return nil
		},
	}
}
