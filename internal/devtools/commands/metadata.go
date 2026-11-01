package commands

import (
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/metadatatools/check"
)

// Exit code perintah metadata:* (selaras perintah i18n): 0 sukses; 2 galat validasi; 3 prasyarat gagal
// (root repo tidak ditemukan). Subperintah yang menulis menambah kode 4 (operasi gagal).
const (
	exitMetadataValidation = 2
	exitMetadataConfig     = 3
)

// registerMetadata mendaftarkan perintah metadata:* (flavor dev saja; perintah ini tidak ada di binary rilis,
// matriks IDOR design baris 13). Subperintah baru (metadata:generate, metadata:diff, ...) ditambahkan ke
// AddCommand di bawah dan memakai metadataRoot untuk mencari root repo.
func registerMetadata(root *cobra.Command) {
	root.AddCommand(newMetadataCheck())
}

// metadataRoot mencari root repo untuk perintah metadata:*; kegagalan dipetakan ke exit 3.
func metadataRoot() (string, error) {
	r, err := repoRoot()
	if err != nil {
		return "", &app.ExitError{Code: exitMetadataConfig, Err: err}
	}
	return r, nil
}

func newMetadataCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "metadata:check",
		Short: "Periksa berkas metadata resource (skema, semantik, relasi, manifest) tanpa menulis apa pun",
		Long: "Memuat semua modules/*/metadata/** dan modules/*/module.json, menyelesaikan penyertaan, timpaan, tipe domain, dan blok, lalu\n" +
			"memeriksa skema, tipe dan batasan, kondisi, accessor, templat label, relasi, integrasi manifest, serta aturan deprecated, salinan,\n" +
			"dan penguncian. Semua temuan dicetak dengan berkas dan baris. Perintah ini hanya membaca.\n" +
			"Exit code: 0 sukses (peringatan boleh ada); 2 ada galat; 3 prasyarat gagal (root repo tidak ditemukan).",
		GroupID: GroupDevtools,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := metadataRoot()
			if err != nil {
				return err
			}
			ws, loadProblems := check.LoadWorkspace(root)
			rep := check.Run(ws)
			problems := append(append([]check.Problem(nil), loadProblems...), rep.Problems...)
			sortCheckProblems(problems)
			nErr, nWarn := writeCheckProblems(c.OutOrStdout(), problems)
			resources := 0
			if rep.Resolved != nil {
				resources = len(rep.Resolved.Resources)
			}
			fmt.Fprintf(c.OutOrStdout(), "metadata:check: %d resource diperiksa, %d galat, %d peringatan\n", resources, nErr, nWarn)
			if nErr > 0 {
				return &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("metadata:check menemukan %d galat", nErr)}
			}
			return nil
		},
	}
}

func sortCheckProblems(ps []check.Problem) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return a.Code < b.Code
	})
}

// writeCheckProblems mencetak temuan satu per baris dan mengembalikan jumlah galat dan peringatan.
func writeCheckProblems(w io.Writer, ps []check.Problem) (errs, warns int) {
	for _, p := range ps {
		if p.Severity == check.SeverityError {
			errs++
		} else {
			warns++
		}
		fmt.Fprintln(w, p.String())
	}
	return errs, warns
}
