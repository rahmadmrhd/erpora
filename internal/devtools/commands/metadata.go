package commands

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/metadatatools/check"
	"erpora/internal/devtools/metadatatools/generate"
	"erpora/internal/devtools/metadatatools/resolve"
)

// Exit code perintah metadata:* (selaras perintah i18n): 0 sukses; 2 galat validasi; 3 prasyarat gagal
// (root repo tidak ditemukan). Subperintah yang menulis menambah kode 4 (operasi gagal).
const (
	exitMetadataValidation = 2
	exitMetadataConfig     = 3
	exitMetadataWrite      = 4
)

// registerMetadata mendaftarkan perintah metadata:* (flavor dev saja; perintah ini tidak ada di binary rilis,
// matriks IDOR design baris 13). Subperintah baru (metadata:generate, metadata:diff, ...) ditambahkan ke
// AddCommand di bawah dan memakai metadataRoot untuk mencari root repo.
func registerMetadata(root *cobra.Command) {
	root.AddCommand(newMetadataCheck(), newMetadataGenerate(), newMetadataDiff(), newMetadataMigrate())
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
			ws.Git = check.ExecGit(root) // aturan deprecate-before-remove terhadap tag rilis terakhir modul (Requirement 12.1)
			rep := check.Run(ws)
			problems := append(append([]check.Problem(nil), loadProblems...), rep.Problems...)
			sortCheckProblems(problems)
			nErr, nWarn := writeCheckProblems(c.OutOrStdout(), problems)
			resources := 0
			if rep.Resolved != nil {
				resources = len(rep.Resolved.Resources)
				writeIncludeRelaxations(c.OutOrStdout(), rep.Resolved.Resources, rep.Resolved.Abstracts)
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

func newMetadataGenerate() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "metadata:generate [modul...]",
		Short: "Bangkitkan skema OpenAPI, kode Go (DTO dan validasi), dan metadata TypeScript dari berkas metadata resource",
		Long: "Menjalankan metadata:check lalu membangkitkan per resource: skema components.schemas pada modules/<dir>/openapi.yaml\n" +
			"(bertanda x-erp-generated), domain/<nama>_meta.gen.go, presentation/http/<nama>_dto.gen.go (beserta runtime per modul), dan\n" +
			"web/src/metadata/<nama>.meta.gen.ts bila modul punya web/. Hasil deterministik dan hanya ditulis di bawah modules/<dir>/.\n" +
			"Tidak menulis apa pun bila metadata:check bergalat atau bila berkas keluaran kehilangan penanda \"Code generated\".\n" +
			"Argumen modul (nama direktori, mis. sales) membatasi pembangkitan; kosong berarti semua modul bermetadata.\n" +
			"--check hanya membandingkan, tidak menulis, dan gagal bila ada selisih (drift atau edit tangan).\n" +
			"Exit code: 0 sukses; 2 galat check, drift, edit tangan, atau masukan tidak sah; 3 root repo tidak ditemukan; 4 penulisan gagal.",
		GroupID: GroupDevtools,
		Args:    cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			root, err := metadataRoot()
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			res, gerr := generate.GenerateWorkspace(root, generate.Options{Check: checkOnly, Modules: args})
			for _, p := range res.Problems {
				fmt.Fprintln(out, p.String())
			}
			for _, o := range res.Omitted {
				fmt.Fprintf(out, "tidak dipancarkan ke klien: %s\n", o)
			}
			for _, p := range res.Stale {
				fmt.Fprintf(out, "basi: %s\n", p)
			}
			for _, p := range res.Written {
				fmt.Fprintf(out, "ditulis: %s\n", p)
			}
			for _, p := range res.Removed {
				fmt.Fprintf(out, "dihapus: %s\n", p)
			}
			if gerr != nil {
				code := exitMetadataWrite
				for _, sentinel := range []error{generate.ErrCheckFailed, generate.ErrDrift, generate.ErrHandEdited,
					generate.ErrUnsafePath, generate.ErrConflict, generate.ErrInput, generate.ErrNoChecker} {
					if errors.Is(gerr, sentinel) {
						code = exitMetadataValidation
						break
					}
				}
				return &app.ExitError{Code: code, Err: fmt.Errorf("metadata:generate: %w", gerr)}
			}
			if checkOnly {
				fmt.Fprintf(out, "metadata:generate --check: %d berkas keluaran sinkron dengan metadata\n", len(res.Files))
			} else {
				fmt.Fprintf(out, "metadata:generate: %d berkas keluaran, %d ditulis, %d dihapus\n", len(res.Files), len(res.Written), len(res.Removed))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "hanya bandingkan dengan disk; tidak menulis, gagal bila ada selisih")
	return cmd
}

// writeIncludeRelaxations mencetak setiap pelonggaran include set (resource, source, field, reason) agar
// pelonggaran atas properti turunan tampil di keluaran metadata:check (keputusan pemilik produk, 2 Nov 2026;
// K27). Berlaku untuk resource dan abstrak modul (IncludeSets tidak diwariskan lewat include, jadi tiap
// dokumen dicetak sendiri); urutan mengikuti kunci dokumen lalu urutan deklarasi. Entri tanpa reason tetap
// tercetak (galat include_set_missing_reason dilaporkan terpisah oleh check).
func writeIncludeRelaxations(w io.Writer, docs ...[]*resolve.Resolved) {
	var all []*resolve.Resolved
	for _, d := range docs {
		all = append(all, d...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	for _, r := range all {
		if r.Module == resolve.SDKSlug {
			continue // dokumen SDK dikelola platform dan tidak diperiksa modul
		}
		for _, is := range r.IncludeSets {
			reason := is.Reason
			if reason == "" {
				reason = "(tanpa reason)"
			}
			fmt.Fprintf(w, "pelonggaran include set: %s <- %s field=[%s] reason=%q (%s:%d)\n",
				r.Key, is.Source, strings.Join(is.Fields, ","), reason, is.Origin.File, is.Origin.Line)
		}
	}
}
