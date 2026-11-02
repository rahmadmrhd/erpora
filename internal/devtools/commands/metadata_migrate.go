package commands

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/devtools/gen"
	"erpora/internal/devtools/metadatatools/diff"
	"erpora/internal/devtools/metadatatools/migrate"
)

func newMetadataMigrate() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "metadata:migrate <modul> <nama> [--dry-run]",
		Short: "Tulis migrasi goose dari selisih antara metadata dan database (tidak pernah diterapkan; hanya dev dan CI)",
		Long: "Menjalankan alur metadata:diff untuk satu modul (database sementara di server " + diff.EnvAdminDSN + ", role non-superuser), lalu\n" +
			"menulis modules/<dir>/migrations/NNNN_<nama>.sql bernomor berikutnya (konvensi make:migration) berisi Up dan Down. Berkas TIDAK pernah\n" +
			"diterapkan oleh perintah ini.\n" +
			"Perubahan aman (tabel baru, kolom nullable atau berdefault, indeks baru, pelonggaran batasan, nilai enum extensible baru) dihasilkan\n" +
			"otomatis. Perubahan yang perlu keputusan (ganti nama, ubah tipe, perkecil batas, NOT NULL pada kolom ada, unique atau foreign key pada\n" +
			"data ada, hapus apa pun) hanya menjadi kerangka komentar berpenanda " + migrate.TodoMarker + " yang menggagalkan metadata:check sampai\n" +
			"dilengkapi dan ditinjau. Tabel large: true memakai penanda NO TRANSACTION dan templat backfill per batch. Down yang tidak dapat\n" +
			"dibalik ditandai eksplisit dan menolak dijalankan. Tanpa selisih, tidak ada berkas yang dibuat.\n" +
			"Exit code: 0 sukses; 2 metadata tidak sah atau masukan salah; 3 prasyarat gagal (root repo atau " + diff.EnvAdminDSN + ").",
		GroupID: GroupDevtools,
		Args:    cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			root, err := metadataRoot()
			if err != nil {
				return err
			}
			adminDSN := strings.TrimSpace(os.Getenv(diff.EnvAdminDSN))
			if adminDSN == "" {
				return &app.ExitError{Code: exitMetadataConfig, Err: fmt.Errorf("%s belum diset: metadata:migrate memakai database sementara di server pengembangan atau CI (jangan arahkan ke database produksi)", diff.EnvAdminDSN)}
			}
			ref, err := gen.ResolveModule(root, args[0])
			if err != nil {
				return &app.ExitError{Code: exitMetadataValidation, Err: err}
			}
			if ref.Strategy != "versioned" {
				return &app.ExitError{Code: exitMetadataValidation, Err: fmt.Errorf("modul %s memakai migrations.strategy %q: ubah menjadi \"versioned\" di modules/%s/module.json sebelum membuat migrasi", ref.Key, ref.Strategy, ref.Dir)}
			}
			dir := "modules/" + ref.Dir + "/migrations"
			var existing fs.FS = fstest.MapFS{}
			if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err == nil && st.IsDir() {
				existing = os.DirFS(filepath.Join(root, filepath.FromSlash(dir)))
			}
			file, err := migrate.FileName(existing, args[1])
			if err != nil {
				return &app.ExitError{Code: exitMetadataValidation, Err: err}
			}
			plan, err := planMetadataDiff(root, args[0], c.ErrOrStderr())
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
			res, err := migrate.Generate(rep, plan.model, migrate.Options{})
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			if res.Empty {
				fmt.Fprintln(w, "tidak ada selisih antara metadata dan database; tidak ada berkas migrasi yang dibuat")
				return nil
			}
			p := gen.Plan{NoForce: true, Ops: []gen.FileOp{{Path: dir + "/" + file, Kind: gen.OpCreate, Content: []byte(res.SQL)}}}
			if err := gen.Apply(c.Context(), p, gen.Options{Root: root, DryRun: dryRun, Out: w}); err != nil {
				return err
			}
			for _, ch := range res.Changes {
				fmt.Fprintln(w, "  otomatis:", ch)
			}
			for _, td := range res.Todos {
				fmt.Fprintf(w, "  perlu keputusan: %s %s %s: %s\n", td.Kind, td.Table, td.Object, td.Reason)
			}
			switch {
			case len(res.Todos) > 0:
				fmt.Fprintf(w, "%d butir perlu keputusan (penanda %s); lengkapi sebelum metadata:check dijalankan. Berkas tidak diterapkan.\n", len(res.Todos), migrate.TodoMarker)
			case res.Irreversible:
				fmt.Fprintln(w, "Down ditandai tidak dapat dibalik. Berkas tidak diterapkan; tinjau lalu terapkan lewat migrate modul.")
			default:
				fmt.Fprintln(w, "Berkas tidak diterapkan; tinjau lalu terapkan lewat migrate modul.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "cetak isi migrasi tanpa menulis berkas")
	return cmd
}
