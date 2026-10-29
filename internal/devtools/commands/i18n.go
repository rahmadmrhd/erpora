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
	root.AddCommand(newI18nCheck(), newI18nStatus())
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
