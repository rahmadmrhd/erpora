package check

import (
	"strings"

	"erpora/internal/devtools/metadatatools/migrate"
)

// checkMigrationTodos menggagalkan pemeriksaan bila modules/<dir>/migrations masih memuat kerangka migrasi bertanda
// TODO(metadata:migrate): perubahan yang perlu keputusan (rename, ubah tipe, hapus kolom, dst.) hanya dibangkitkan
// sebagai kerangka dan harus dilengkapi dan ditinjau manusia (Requirement 11.3, design D-7).
func (c *checker) checkMigrationTodos(mods []Module) {
	for _, m := range mods {
		if m.MigrationsFS == nil {
			continue
		}
		refs, err := migrate.FindTodos(m.MigrationsFS)
		if err != nil {
			c.add(Problem{Severity: SeverityError, File: m.Dir + "/migrations", Code: CodeMigrationTodo, Module: m.Manifest.Slug,
				Message: "direktori migrations tidak dapat dipindai untuk penanda TODO: " + err.Error()})
			continue
		}
		for _, r := range refs {
			c.add(Problem{Severity: SeverityError, File: strings.TrimSuffix(m.Dir, "/") + "/migrations/" + r.File, Line: r.Line, Column: 1,
				Code: CodeMigrationTodo, Module: m.Manifest.Slug,
				Message: "kerangka migrasi masih bertanda " + migrate.TodoMarker + ": lengkapi dan tinjau migrasi (data, backfill, penjaga), lalu hapus penanda (Requirement 11.3)"})
		}
	}
}
