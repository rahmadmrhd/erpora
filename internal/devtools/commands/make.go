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
	root.AddCommand(newMakeMigration(), newMakeModule(), newMakeHandler(),
		newMakeLayer("make:entity", "<module> <Name>", "Buat entity domain (internal/domain) beserta test-nya",
			func(root string, a []string, _ bool) (gen.Plan, error) { return gen.MakeEntity(root, a[0], a[1]) }, false),
		newMakeLayer("make:usecase", "<module> <Name> [--query]", "Buat use case (internal/application): command atau query",
			func(root string, a []string, q bool) (gen.Plan, error) { return gen.MakeUsecase(root, a[0], a[1], q) }, true),
		newMakeLayer("make:contract", "<module> <Name>", "Buat kontrak publik modul (contract/) tanpa impor internal/",
			func(root string, a []string, _ bool) (gen.Plan, error) { return gen.MakeContract(root, a[0], a[1]) }, false),
		newMakeLayer("make:command", "<module> <name>", "Buat command CLI modul dan daftarkan di module.go",
			func(root string, a []string, _ bool) (gen.Plan, error) { return gen.MakeCommand(root, a[0], a[1]) }, false),
		newMakeLayer("make:repository", "<module> <Name>", "Buat repository (port, model GORM, repo, mapping) untuk entity yang sudah ada",
			func(root string, a []string, _ bool) (gen.Plan, error) { return gen.MakeRepository(root, a[0], a[1]) }, false),
	)
}

func newMakeHandler() *cobra.Command {
	var f makeFlags
	var route, permission string
	cmd := &cobra.Command{
		Use:   "make:handler <module> <Name> --route \"METHOD /path\" --permission <slug.resource.action>",
		Short: "Buat endpoint: handler + test IDOR, route, capability, dan operasi OpenAPI sekaligus",
		Long: "Membuat handler (internal/presentation/http/<name>.go) beserta test (endpoint ber-ID selalu disertai kerangka test IDOR),\n" +
			"mendaftarkan route di module.go (Classify + MaxBodyBytes), menambah capability di module.json, dan operasi serta skema di openapi.yaml.\n" +
			"Bentuk route: GET|POST /<koleksi>, GET|PATCH|DELETE /<koleksi>/{id}, POST /<koleksi>/{id}/<aksi>. PUT ditolak (api-guidelines section 3).",
		GroupID: GroupMake,
		Args:    cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, a []string) error {
			if route == "" || permission == "" {
				return usageErr("--route dan --permission wajib diisi")
			}
			return f.apply(c, func(root string) (gen.Plan, error) {
				return gen.MakeHandler(root, gen.HandlerOptions{Module: a[0], Name: a[1], Route: route, Permission: permission})
			})
		},
	}
	cmd.Flags().StringVar(&route, "route", "", "\"METHOD /path\" relatif modul, mis. \"GET /items/{id}\"")
	cmd.Flags().StringVar(&permission, "permission", "", "permission <slug>.<resource>.<action> (ditambahkan ke capabilities)")
	f.bind(cmd)
	return cmd
}

// newMakeLayer membuat command make:* dengan argumen <module> <name> dan flag --force/--dry-run (dan --query bila hasQuery).
func newMakeLayer(use, args, short string, build func(root string, args []string, query bool) (gen.Plan, error), hasQuery bool) *cobra.Command {
	var f makeFlags
	var query bool
	cmd := &cobra.Command{
		Use:     use + " " + args,
		Short:   short,
		GroupID: GroupMake,
		Args:    cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, a []string) error {
			return f.apply(c, func(root string) (gen.Plan, error) { return build(root, a, query) })
		},
	}
	if hasQuery {
		cmd.Flags().BoolVar(&query, "query", false, "use case baca saja (Query + Result), bukan command")
	}
	f.bind(cmd)
	return cmd
}

func newMakeModule() *cobra.Command {
	var f makeFlags
	var frontend bool
	var publisher string
	var depends []string
	cmd := &cobra.Command{
		Use:   "make:module <name> [--frontend] [--depends k1,k2] [--publisher id]",
		Short: "Buat modul baru turunan example yang langsung lolos gate",
		Long: "Membuat modules/<dir> (Go module sendiri) lengkap dengan manifest, kontrak OpenAPI, empat lapisan, migrasi, dan test, lalu\n" +
			"mendaftarkannya di go.work, go.mod, dan internal/app/modules.go. <name> adalah slug kebab-case (mis. stock-opname); direktori dan package\n" +
			"memakai slug tanpa tanda hubung (stockopname). --frontend juga membuat modules/<dir>/web (@erpora/module-<dir>) dan mendaftarkannya di web/.",
		GroupID: GroupMake,
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return f.apply(c, func(root string) (gen.Plan, error) {
				return gen.MakeModule(root, gen.ModuleOptions{Name: args[0], Publisher: publisher, Frontend: frontend, Depends: depends})
			})
		},
	}
	cmd.Flags().BoolVar(&frontend, "frontend", false, "buat juga paket frontend modules/<dir>/web")
	cmd.Flags().StringSliceVar(&depends, "depends", nil, "key modul dependensi, dipisah koma (mis. acme.example); platform.core selalu ditambahkan")
	cmd.Flags().StringVar(&publisher, "publisher", "acme", "id publisher untuk key modul (<publisher>.<slug>)")
	f.bind(cmd)
	return cmd
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
