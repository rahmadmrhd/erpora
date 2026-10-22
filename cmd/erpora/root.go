package main

import (
	"errors"
	"io"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/config"
)

// Grup command untuk `erpora list` dan help.
const (
	groupRuntime = "runtime"
	groupInspect = "inspect"
)

// envFile menampung nilai flag --env-file (persistent).
type rootOpts struct{ envFile string }

// newRootCmd merakit root command. Subcommand ditambahkan di blok "daftar command" di bawah:
// Task 8 menambah newMigrateCmd(opts) (grup runtime), Task 11 menambah newServeCmd(opts).
func newRootCmd(out, errOut io.Writer) *cobra.Command {
	opts := &rootOpts{}
	root := &cobra.Command{
		Use:           "erpora",
		Short:         "ERPora platform CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentFlags().StringVar(&opts.envFile, "env-file", "", "path file .env (environment proses diutamakan)")
	root.AddGroup(
		&cobra.Group{ID: groupRuntime, Title: "Runtime"},
		&cobra.Group{ID: groupInspect, Title: "Inspect"},
	)

	// --- daftar command ---
	root.AddCommand(
		newVersionCmd(),
		newInspectCmd(opts),
		newListCmd(),
		newInspectRoutesCmd("route:list", true),
		newInspectPermissionsCmd("permission:list", true),
		newMigrateCmd(opts),
		newServeCmd(opts),
	)
	registerDevtools(root)
	return root
}

// exitCode: ValidationError -> 2, error lain -> 1, nil -> 0.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var xe *app.ExitError
	if errors.As(err, &xe) {
		return xe.Code
	}
	var ve config.ValidationError
	if errors.As(err, &ve) {
		return 2
	}
	return 1
}
