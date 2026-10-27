package dbtools

import (
	"context"
	"fmt"
	"strings"

	"erpora/internal/platform/database"
)

// Objek milik ekstensi dikecualikan (deptype 'e'); sisanya di current_schema() dihapus.
const (
	// relkind: v view, m materialized view, r tabel, p tabel berpartisi, f foreign table, c tipe komposit mandiri, S sequence.
	listRelationsSQL = `
SELECT c.relkind::text AS kind, format('%I.%I', n.nspname, c.relname) AS name
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema()
  AND c.relkind IN ('v', 'm', 'r', 'p', 'f', 'c', 'S')
  AND NOT c.relispartition
  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = c.oid AND d.deptype = 'e')
ORDER BY c.oid`
	listRoutinesSQL = `
SELECT p.oid::regprocedure::text AS name
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = current_schema()
  AND p.prokind IN ('f', 'p', 'a')
  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')`
	listTypesSQL = `
SELECT format('%I.%I', n.nspname, t.typname) AS name
FROM pg_type t
JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE n.nspname = current_schema()
  AND t.typtype IN ('e', 'd')
  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = t.oid AND d.deptype = 'e')`
)

type relation struct{ Kind, Name string }

// dropStatements: urutan penghapusan relasi; view lebih dulu, lalu tabel, sequence, dan tipe komposit. Nama sudah
// di-quote oleh format('%I.%I') di sisi PostgreSQL.
var dropStatements = []struct{ kinds, format string }{
	{"v", "DROP VIEW IF EXISTS %s CASCADE"},
	{"m", "DROP MATERIALIZED VIEW IF EXISTS %s CASCADE"},
	{"rpf", "DROP TABLE IF EXISTS %s CASCADE"},
	{"S", "DROP SEQUENCE IF EXISTS %s CASCADE"},
	{"c", "DROP TYPE IF EXISTS %s CASCADE"},
}

// DropAll menghapus semua objek (view, tabel, sequence, fungsi, tipe enum/domain) di current_schema() dalam satu
// transaksi. Schema tidak di-DROP agar hak schema milik DBA tetap utuh (keputusan 17); objek milik ekstensi tidak disentuh.
func DropAll(ctx context.Context, db *database.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *database.DB) error {
		var rels []relation
		if err := tx.Raw(listRelationsSQL).Scan(&rels).Error; err != nil {
			return fmt.Errorf("daftar objek: %w", err)
		}
		for _, ds := range dropStatements {
			for _, r := range rels {
				if strings.Contains(ds.kinds, r.Kind) {
					if err := tx.Exec(fmt.Sprintf(ds.format, r.Name)).Error; err != nil {
						return fmt.Errorf("hapus %s: %w", r.Name, err)
					}
				}
			}
		}
		var routines []string
		if err := tx.Raw(listRoutinesSQL).Scan(&routines).Error; err != nil {
			return fmt.Errorf("daftar fungsi: %w", err)
		}
		for _, r := range routines {
			if err := tx.Exec("DROP ROUTINE IF EXISTS " + r + " CASCADE").Error; err != nil {
				return fmt.Errorf("hapus fungsi %s: %w", r, err)
			}
		}
		var types []string
		if err := tx.Raw(listTypesSQL).Scan(&types).Error; err != nil {
			return fmt.Errorf("daftar tipe: %w", err)
		}
		for _, ty := range types {
			if err := tx.Exec("DROP TYPE IF EXISTS " + ty + " CASCADE").Error; err != nil {
				return fmt.Errorf("hapus tipe %s: %w", ty, err)
			}
		}
		return nil
	})
}
