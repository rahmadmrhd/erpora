package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/modcheck"
)

func registerModule(root *cobra.Command) {
	root.AddCommand(newModuleCheck())
}

func newModuleCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "module:check [name]",
		Short: "Periksa manifest, dependensi (D8), versi Go, dan awalan capability modul",
		Long: "Memeriksa setiap modul (atau satu modul: direktori, slug, atau key) tanpa menjalankan seluruh gate: manifest terhadap JSON Schema\n" +
			"dan aturan tambahan (SCHEMA), dependensi manifest terhadap go.mod dan impor (D8), direktif go/toolchain (GOVERSION), serta\n" +
			"awalan capability = slug modul (CAPABILITY_PREFIX). Exit 1 bila ada temuan.",
		GroupID: GroupModule,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			only := ""
			if len(args) == 1 {
				only = args[0]
			}
			findings, err := modcheck.Check(root, only)
			if err != nil {
				return usageErr("%v", err)
			}
			w := c.OutOrStdout()
			if len(findings) == 0 {
				fmt.Fprintln(w, "OK")
				return nil
			}
			for _, f := range findings {
				fmt.Fprintf(w, "%s %s %s\n", f.Module, f.Rule, f.Detail)
			}
			return &app.ExitError{Code: 1, Err: errors.New("module:check menemukan masalah")}
		},
	}
}
