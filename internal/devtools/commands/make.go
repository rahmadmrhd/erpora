package commands

import (
	"github.com/spf13/cobra"

	"erpora/internal/devtools/gen"
)

// makeFlags adalah flag bersama semua command make:*.
type makeFlags struct{ force, dryRun bool }

func (f *makeFlags) bind(c *cobra.Command) {
	c.Flags().BoolVar(&f.force, "force", false, "timpa file yang sudah ada (hanya file yang dibuat; file yang diubah tidak terpengaruh)")
	c.Flags().BoolVar(&f.dryRun, "dry-run", false, "tampilkan file dan diff tanpa menulis apa pun")
}

// apply menjalankan Plan di root repo; plan yang gagal dibangun dilaporkan apa adanya (exit 1).
func (f *makeFlags) apply(c *cobra.Command, build func(root string) (gen.Plan, error)) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	plan, err := build(root)
	if err != nil {
		return err
	}
	return gen.Apply(c.Context(), plan, gen.Options{Root: root, Force: f.force, DryRun: f.dryRun, Out: c.OutOrStdout()})
}

func registerMake(root *cobra.Command) {
	root.AddCommand(newMakeMigration())
}

func newMakeMigration() *cobra.Command {
	var f makeFlags
	var platform string
	cmd := &cobra.Command{
		Use:   "make:migration <module> <name> | --platform control|tenant <name>",
		Short: "Buat file migrasi goose bernomor berikutnya (modul atau platform)",
		Long: "Membuat <NNNN>_<name>.sql di modules/<dir>/migrations (nomor berikutnya otomatis). Dengan --platform control|tenant\n" +
			"membuat migrasi platform di migrations/<target>/ dan argumen <module> ditiadakan. Modul bermigrations.strategy \"none\" ditolak.",
		GroupID: GroupMake,
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			var module, name string
			if platform != "" {
				if len(args) != 1 {
					return usageErr("dengan --platform argumennya hanya <name>")
				}
				name = args[0]
			} else {
				if len(args) != 2 {
					return usageErr("argumen: make:migration <module> <name>")
				}
				module, name = args[0], args[1]
			}
			return f.apply(c, func(root string) (gen.Plan, error) { return gen.MakeMigration(root, module, name, platform) })
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "", "migrasi platform: control atau tenant")
	f.bind(cmd)
	return cmd
}
