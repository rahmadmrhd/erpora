package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/i18ntools"
)

// Exit code perintah i18n (Requirement 8.6, design 7): 0 sukses; 2 galat validasi; 3 prasyarat atau
// konfigurasi gagal; 4 operasi gagal.
const (
	exitI18nValidation = 2
	exitI18nConfig     = 3
	exitI18nOperation  = 4
)

func registerI18n(root *cobra.Command) {
	root.AddCommand(newI18nCheck(), newI18nStatus(), newI18nAdd(), newI18nGenerate())
}

// i18nExit memetakan galat i18ntools ke exit code.
func i18nExit(err error) error {
	if err == nil {
		return nil
	}
	var ce *i18ntools.ConfigError
	var ve *i18ntools.ValidationError
	switch {
	case errors.As(err, &ce):
		return &app.ExitError{Code: exitI18nConfig, Err: err}
	case errors.As(err, &ve):
		return &app.ExitError{Code: exitI18nValidation, Err: err}
	default:
		return &app.ExitError{Code: exitI18nOperation, Err: err}
	}
}

func newI18nCheck() *cobra.Command {
	var strict, asJSON bool
	cmd := &cobra.Command{
		Use:   "i18n:check",
		Short: "Periksa katalog terjemahan (struktur, ICU, registri, kesesuaian antar bahasa)",
		Long: "Memeriksa katalog platform, ui, dan modul (modules/*/i18n) dari filesystem dengan parser yang sama dengan runtime.\n" +
			"Galat (struktural: JSON/ICU tidak valid, kunci ganda, tabrakan perataan, segmen terlarang, HTML, namespace, registri,\n" +
			"deklarasi manifest) menggagalkan dengan exit 2. Peringatan (kunci hilang/berlebih dan argumen tak cocok terhadap bahasa default;\n" +
			"bahasa partial tidak melaporkan kunci hilang) hanya dicetak, kecuali --strict mengubahnya menjadi galat.\n" +
			"Exit code: 0 sukses; 2 galat validasi; 3 prasyarat/konfigurasi gagal (registri tidak ada, manifest tidak terbaca); 4 operasi gagal.",
		GroupID: GroupDevtools,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return &app.ExitError{Code: exitI18nConfig, Err: err}
			}
			rep, err := i18ntools.Check(root, i18ntools.CheckOptions{Strict: strict})
			if err != nil {
				return i18nExit(err)
			}
			if asJSON {
				if err := rep.WriteJSON(c.OutOrStdout()); err != nil {
					return i18nExit(err)
				}
			} else {
				rep.WriteText(c.OutOrStdout())
			}
			if rep.Failed() {
				return &app.ExitError{Code: exitI18nValidation, Err: fmt.Errorf("i18n:check menemukan %d galat", rep.Errors)}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "ubah peringatan (kunci hilang/berlebih, argumen tak cocok) menjadi galat")
	cmd.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON (file, bahasa, kunci lengkap per temuan)")
	return cmd
}

func newI18nStatus() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "i18n:status",
		Short:   "Tampilkan kelengkapan terjemahan per bahasa per namespace",
		Long:    "Menghitung persentase kunci bahasa default yang sudah diterjemahkan (bukan penanda TODO) per bahasa per namespace. Exit 2 bila katalog tidak valid, 3 bila prasyarat gagal.",
		GroupID: GroupDevtools,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return &app.ExitError{Code: exitI18nConfig, Err: err}
			}
			rows, err := i18ntools.Status(root)
			if err != nil {
				return i18nExit(err)
			}
			if asJSON {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return i18nExit(enc.Encode(rows))
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAMESPACE\tBAHASA\tTERJEMAH\tTOTAL\tPERSEN\tKET")
			for _, r := range rows {
				note := ""
				switch {
				case r.Default:
					note = "default"
				case r.Partial:
					note = "partial"
				}
				fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%.1f%%\t%s\n", r.Namespace, r.Locale, r.Translated, r.Total, r.Percent, note)
			}
			return i18nExit(tw.Flush())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON")
	return cmd
}

func newI18nAdd() *cobra.Command {
	var name, fallback string
	cmd := &cobra.Command{
		Use:   "i18n:add <locale> [module...]",
		Short: "Buat kerangka katalog bahasa baru dari bahasa default (nilai bertanda TODO)",
		Long: "Membuat <locale>.json untuk setiap namespace terpilih (platform, ui, dan modul; tanpa argumen modul = semua) dari katalog bahasa\n" +
			"default: struktur dan urutan kunci sama, nilai diberi awalan \"TODO: \" (dihitung belum diterjemahkan oleh i18n:status). Bahasa ditambahkan\n" +
			"ke registri (internal/platform/i18n/locales.json) bila belum ada (--name, --fallback) dan ke i18n.locales manifest modul. Idempoten: berkas\n" +
			"yang sudah ada tidak ditimpa. Exit 2 galat argumen/katalog, 3 prasyarat gagal, 4 penulisan gagal.",
		GroupID: GroupDevtools,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return &app.ExitError{Code: exitI18nConfig, Err: err}
			}
			res, err := i18ntools.Add(root, i18ntools.AddOptions{Locale: args[0], Name: name, Fallback: fallback, Modules: args[1:]})
			if err != nil {
				return i18nExit(err)
			}
			w := c.OutOrStdout()
			if res.RegistryUpdated {
				fmt.Fprintf(w, "registri: bahasa %s ditambahkan ke %s\n", args[0], i18ntools.RegistryPath)
			}
			for _, f := range res.Created {
				fmt.Fprintf(w, "buat %s\n", f)
			}
			for _, f := range res.ManifestsUpdated {
				fmt.Fprintf(w, "ubah %s (i18n.locales)\n", f)
			}
			for _, f := range res.Skipped {
				fmt.Fprintf(w, "lewati %s (sudah ada)\n", f)
			}
			if !res.RegistryUpdated && len(res.Created) == 0 && len(res.ManifestsUpdated) == 0 {
				fmt.Fprintln(w, "tidak ada perubahan")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "nama asli bahasa untuk registri bila bahasa belum terdaftar (default: kode)")
	cmd.Flags().StringVar(&fallback, "fallback", "", "bahasa fallback untuk registri bila bahasa belum terdaftar (default: bahasa default registri)")
	return cmd
}

func newI18nGenerate() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "i18n:generate",
		Short: "Bangkitkan tipe kunci terketik (keys.gen.ts) dari katalog bahasa default",
		Long: "Menulis packages/platform-web/src/i18n/keys.platform.gen.ts, packages/ui/src/i18n/keys.gen.ts, dan modules/<dir>/web/src/i18n/keys.gen.ts\n" +
			"(modul ber-web/) berisi augmentasi I18nCatalogs: bentuk argumen tiap kunci (string | number, number untuk plural, gabungan literal untuk\n" +
			"select, RichTag untuk tag rich). Dengan --check tidak menulis apa pun: berkas yang tidak sesuai katalog dicetak dan exit 2.\n" +
			"Exit 2 bila katalog tidak valid atau hasil generate usang, 3 prasyarat gagal, 4 penulisan gagal.",
		GroupID: GroupDevtools,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return &app.ExitError{Code: exitI18nConfig, Err: err}
			}
			files, err := i18ntools.Generate(root)
			if err != nil {
				return i18nExit(err)
			}
			w := c.OutOrStdout()
			if !check {
				if err := i18ntools.WriteGenerated(root, files); err != nil {
					return i18nExit(err)
				}
				for _, f := range files {
					fmt.Fprintf(w, "tulis %s\n", f.Path)
				}
				return nil
			}
			stale, err := i18ntools.CheckGenerated(root, files)
			if err != nil {
				return i18nExit(err)
			}
			for _, s := range stale {
				fmt.Fprintf(w, "basi: %s (jalankan erpora i18n:generate)\n", s)
			}
			if len(stale) > 0 {
				return &app.ExitError{Code: exitI18nValidation, Err: fmt.Errorf("%d file hasil generate tidak mutakhir", len(stale))}
			}
			fmt.Fprintln(w, "hasil generate mutakhir")
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "hanya periksa drift; tidak menulis; exit 2 bila ada file basi")
	return cmd
}
