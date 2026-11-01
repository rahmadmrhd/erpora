package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/metadatatools/check"
	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/diff"
)

func newMetadataDiff() *cobra.Command {
	return &cobra.Command{
		Use:   "metadata:diff [modul]",
		Short: "Bandingkan skema database hasil migrasi dengan skema turunan metadata (database sementara; hanya dev dan CI)",
		Long: "Membuat database sementara di server " + diff.EnvAdminDSN + " (DSN admin; bukan database aplikasi), membuat role non-superuser, menjalankan\n" +
			"semua migrasi goose modul dan dependensinya sebagai role itu, membaca katalog PostgreSQL, lalu membandingkannya dengan skema\n" +
			"turunan metadata: tabel, kolom, tipe, nullability, default, CHECK, unique, indeks, foreign key relasi satu modul, kolom turunan,\n" +
			"dan pemicu. Indeks atau unique yang tidak dideklarasikan dihitung selisih; kolom di unmanagedColumns dikecualikan.\n" +
			"Argumen modul (direktori, slug, atau key) membatasi ke satu modul; kosong berarti semua modul bermetadata.\n" +
			"Perintah ini tidak pernah menyentuh database aplikasi dan tidak menulis berkas.\n" +
			"Exit code: 0 selisih kosong; 2 ada selisih atau metadata tidak sah; 3 prasyarat gagal (root repo atau " + diff.EnvAdminDSN + ").",
		GroupID: GroupDevtools,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			root, err := metadataRoot()
			if err != nil {
				return err
			}
			adminDSN := strings.TrimSpace(os.Getenv(diff.EnvAdminDSN))
			if adminDSN == "" {
				return &app.ExitError{Code: exitMetadataConfig, Err: fmt.Errorf("%s belum diset: metadata:diff memakai database sementara di server pengembangan atau CI (jangan arahkan ke database produksi)", diff.EnvAdminDSN)}
			}
			only := ""
			if len(args) == 1 {
				only = args[0]
			}
			plan, err := planMetadataDiff(root, only, c.ErrOrStderr())
			if err != nil {
				return err
			}
			var rep *diff.Report
			err = diff.WithTempDatabase(c.Context(), adminDSN, func(dsn string) error {
				r, e := diff.Run(c.Context(), diff.Options{DSN: dsn, Sources: plan.sources, Model: plan.model, NotePrefixes: plan.prefixes})
				rep = r
				return e
			})
			if err != nil {
				return err
			}
			fmt.Fprint(c.OutOrStdout(), rep.String())
			if !rep.Empty() {
				return &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("metadata:diff menemukan %d selisih", len(rep.Findings))}
			}
			return nil
		},
	}
}

type metadataDiffPlan struct {
	sources  []diff.Source
	model    *dbmodel.Schema
	prefixes []string
}

// planMetadataDiff memuat metadata semua modul, memeriksanya (metadata:check harus bersih), menurunkan model,
// dan menyusun urutan migrasi (dependensi lebih dulu).
func planMetadataDiff(root, only string, warn io.Writer) (*metadataDiffPlan, error) {
	ws, loadProblems := check.LoadWorkspace(root)
	rep := check.Run(ws)
	problems := append(append([]check.Problem(nil), loadProblems...), rep.Problems...)
	sortCheckProblems(problems)
	if nErr, _ := writeCheckProblems(warn, problems); nErr > 0 {
		return nil, &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("metadata:diff dibatalkan: metadata:check menemukan %d galat", nErr)}
	}
	if rep.Resolved == nil {
		return nil, &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("metadata tidak dapat diselesaikan")}
	}
	schema, dps := dbmodel.Derive(rep.Resolved.Resources)
	if len(dps) > 0 {
		for _, p := range dps {
			fmt.Fprintln(warn, p.String())
		}
		return nil, &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("model skema database tidak dapat diturunkan (%d temuan)", len(dps))}
	}

	byKey := map[string]int{}
	for i, m := range ws.Modules {
		byKey[m.Manifest.Key] = i
	}
	hasModel := map[string]bool{}
	for _, t := range schema.Tables {
		hasModel[t.Module] = true
	}

	// Modul sasaran.
	var targets []int
	for i, m := range ws.Modules {
		if only == "" {
			if hasModel[m.Manifest.Slug] {
				targets = append(targets, i)
			}
			continue
		}
		if only == filepath.Base(m.Dir) || only == m.Manifest.Slug || only == m.Manifest.Key {
			if !hasModel[m.Manifest.Slug] {
				return nil, &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("modul %q tidak punya resource bermetadata", only)}
			}
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		if only != "" {
			return nil, &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("modul %q tidak ditemukan", only)}
		}
		return nil, &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("tidak ada modul bermetadata")}
	}

	// Urutan migrasi: penelusuran dalam-dulu atas dependensi manifest (dependensi lebih dulu).
	var order []int
	state := map[int]int{} // 1 sedang dikunjungi, 2 selesai
	var visit func(i int) error
	visit = func(i int) error {
		switch state[i] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("siklus dependensi modul di sekitar %s", ws.Modules[i].Manifest.Slug)
		}
		state[i] = 1
		for _, dk := range ws.Modules[i].Manifest.Dependencies {
			if j, ok := byKey[dk]; ok {
				if err := visit(j); err != nil {
					return err
				}
			}
		}
		state[i] = 2
		order = append(order, i)
		return nil
	}
	for _, i := range targets {
		if err := visit(i); err != nil {
			return nil, &app.ExitError{Code: exitMetadataValidation, Err: err}
		}
	}

	plan := &metadataDiffPlan{}
	model := &dbmodel.Schema{}
	for _, i := range order {
		m := ws.Modules[i]
		src := diff.Source{Slug: m.Manifest.Slug}
		dir := filepath.Join(root, filepath.FromSlash(m.Dir), "migrations")
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			src.Migrations = os.DirFS(dir)
		}
		plan.sources = append(plan.sources, src)
	}
	for _, i := range targets {
		slug := ws.Modules[i].Manifest.Slug
		model.Tables = append(model.Tables, schema.ForModule(slug).Tables...)
		plan.prefixes = append(plan.prefixes, strings.ReplaceAll(slug, "-", "_")+"_")
	}
	plan.model = reindexSchema(model)
	return plan, nil
}

// reindexSchema membangun Schema dari daftar tabel (tabel diurutkan menurut nama).
func reindexSchema(s *dbmodel.Schema) *dbmodel.Schema { return dbmodel.NewSchema(s.Tables) }
