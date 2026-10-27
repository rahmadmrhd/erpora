package devtools

import (
	"github.com/spf13/cobra"

	"erpora/internal/devtools/commands"
	"erpora/internal/platform/buildinfo"
)

// Banner ditulis ke stderr sebelum setiap command binary flavor dev.
const Banner = "PERINGATAN: binary erpora flavor dev (developer tools aktif); jangan dipakai di production."

// Options adalah dependensi dari cmd/erpora.
type Options struct {
	EnvFile func() string // nilai flag --env-file saat command berjalan
}

// Register menandai flavor "dev", menambah grup command dev, memasang banner ke stderr (bukan stdout, agar
// keluaran --json dan completion tetap bersih), lalu mendaftarkan semua command developer tools.
func Register(root *cobra.Command, opts Options) {
	buildinfo.SetFlavor("dev")
	root.AddGroup(
		&cobra.Group{ID: commands.GroupMake, Title: commands.GroupTitle(commands.GroupMake)},
		&cobra.Group{ID: commands.GroupDB, Title: commands.GroupTitle(commands.GroupDB)},
		&cobra.Group{ID: commands.GroupContracts, Title: commands.GroupTitle(commands.GroupContracts)},
		&cobra.Group{ID: commands.GroupDevtools, Title: commands.GroupTitle(commands.GroupDevtools)},
	)
	prev := root.PersistentPreRunE
	prevPlain := root.PersistentPreRun
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if _, err := cmd.ErrOrStderr().Write([]byte(Banner + "\n")); err != nil {
			return err
		}
		if prev != nil {
			return prev(cmd, args)
		}
		if prevPlain != nil {
			prevPlain(cmd, args)
		}
		return nil
	}
	commands.Register(root, commands.Options{EnvFile: opts.EnvFile})
}
