package commands

import (
	"log/slog"

	"github.com/spf13/cobra"

	"erpora/internal/devtools/dbtools"
	"erpora/internal/platform/config"
	"erpora/internal/platform/observability"
)

func registerDB(root *cobra.Command, opts Options) {
	root.AddCommand(newDBFresh(opts), newDBSeed(opts))
}

func loadConfig(c *cobra.Command, opts Options) (config.Config, *slog.Logger, error) {
	envFile := ""
	if opts.EnvFile != nil {
		envFile = opts.EnvFile()
	}
	cfg, err := config.Load(envFile)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, observability.NewLogger(c.ErrOrStderr(), cfg.LogFormat, slog.LevelInfo), nil
}

func newDBFresh(opts Options) *cobra.Command {
	var seed, yes bool
	cmd := &cobra.Command{
		Use:   "db:fresh [--seed] [--yes]",
		Short: "HAPUS semua data lalu bangun ulang database development (migrate up, install semua modul, seed opsional)",
		Long: "Menghapus semua objek di schema aktif database, menjalankan migrate up, meng-install dan meng-enable semua modul, lalu (--seed)\n" +
			"menjalankan DevSeeder. Hanya untuk ERP_ENV=development dan database berpenanda development; selain itu ditolak dengan exit 3\n" +
			"SEBELUM database disentuh. Tanpa --yes, pengguna harus mengetik nama database untuk konfirmasi.",
		GroupID: GroupDB,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(c, opts)
			if err != nil {
				return err
			}
			return dbtools.Fresh(c.Context(), cfg, log, dbtools.FreshOptions{Seed: seed, Yes: yes, In: c.InOrStdin(), Out: c.OutOrStdout()})
		},
	}
	cmd.Flags().BoolVar(&seed, "seed", false, "jalankan DevSeeder setelah database dibangun ulang")
	cmd.Flags().BoolVar(&yes, "yes", false, "lewati konfirmasi mengetik nama database")
	return cmd
}

func newDBSeed(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "db:seed [module]",
		Short: "Jalankan DevSeeder modul enabled (atau satu modul) dalam urutan dependensi",
		Long: "Memanggil SeedDev setiap modul berstatus enabled yang mengimplementasi DevSeeder (idempoten), dependensi lebih dulu.\n" +
			"Argumen module (key, slug, atau nama direktori) membatasi ke satu modul; modul itu harus enabled (exit 3). Hanya untuk\n" +
			"ERP_ENV=development dan database berpenanda development (exit 3 bila tidak).",
		GroupID: GroupDB,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, log, err := loadConfig(c, opts)
			if err != nil {
				return err
			}
			only := ""
			if len(args) == 1 {
				only = args[0]
			}
			return dbtools.Seed(c.Context(), cfg, log, only, c.OutOrStdout())
		},
	}
}
