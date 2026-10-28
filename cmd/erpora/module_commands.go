package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/observability"
)

// groupModule adalah grup command yang disumbang modul (`<slug>:<name>`).
const groupModule = "module"

// moduleRunner menjalankan satu command modul; default memanggil app.RunModuleCommand.
type moduleRunner func(ctx context.Context, opts *rootOpts, run app.CommandRun) error

func defaultModuleRunner(ctx context.Context, opts *rootOpts, run app.CommandRun) error {
	cfg, err := config.Load(opts.envFile)
	if err != nil {
		return err
	}
	return app.RunModuleCommand(ctx, cfg, observability.NewLogger(os.Stderr, cfg.LogFormat, slog.LevelInfo), run)
}

// addModuleCommands menambah command modul (FR-22.4) dari katalog terkompilasi. Bila katalog tidak valid
// tidak menambah apa pun: serve yang melaporkan kesalahan itu (exit 2).
func addModuleCommands(root *cobra.Command, opts *rootOpts) {
	reg, err := app.BuildRegistry()
	if err != nil {
		return
	}
	addModuleCommandsFrom(root, opts, reg.Commands(), defaultModuleRunner)
}

// addModuleCommandsFrom adalah inti addModuleCommands dengan sumber command dan runner yang dapat diganti.
func addModuleCommandsFrom(root *cobra.Command, opts *rootOpts, cmds []registry.CommandInfo, runner moduleRunner) {
	for _, info := range cmds {
		cmd := &cobra.Command{
			Use:     info.Name,
			Short:   info.Command.Short,
			GroupID: groupModule,
			Long:    info.Command.Short + "\n\nCommand modul " + info.Module + "; hanya berjalan bila modul berstatus enabled.",
			RunE: func(c *cobra.Command, args []string) error {
				flags := map[string]string{}
				for _, f := range info.Command.Flags {
					v, err := c.Flags().GetString(f.Name)
					if err != nil {
						return err
					}
					flags[f.Name] = v
				}
				return runner(c.Context(), opts, app.CommandRun{
					Command: info, Args: args, Flags: flags, Out: c.OutOrStdout(), Actor: cliActor(),
				})
			},
		}
		for _, f := range info.Command.Flags {
			cmd.Flags().String(f.Name, f.Default, f.Usage)
		}
		root.AddCommand(cmd)
	}
}
