package diff

import (
	"context"
	"fmt"
	"strings"

	"erpora/internal/platform/database"
)

// Introspect membaca katalog PostgreSQL untuk satu skema lewat satu koneksi khusus dari db. search_path koneksi
// diarahkan ke skema itu sehingga pg_get_expr, pg_get_constraintdef, dan format_type menulis nama tanpa
// kualifikasi (aktual dan diharapkan dibandingkan dengan normalisasi yang sama). Hanya baca.
func Introspect(ctx context.Context, db *database.DB, schema string) (*Catalog, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("koneksi: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `SELECT set_config('search_path', $1, false)`, schema); err != nil {
		return nil, fmt.Errorf("search_path: %w", err)
	}
	cat := &Catalog{Schema: schema, Tables: map[string]*Table{}}

	rows, err := conn.QueryContext(ctx, `
SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relkind IN ('r', 'p') ORDER BY c.relname`, schema)
	if err != nil {
		return nil, fmt.Errorf("tabel: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		cat.Tables[name] = &Table{Name: name}
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	// Kolom. attgenerated 's' berarti stored; ekspresinya disimpan di pg_attrdef.
	rows, err = conn.QueryContext(ctx, `
SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
       COALESCE(pg_get_expr(d.adbin, d.adrelid), ''), a.attgenerated::text
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE n.nspname = $1 AND c.relkind IN ('r', 'p') AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY c.relname, a.attnum`, schema)
	if err != nil {
		return nil, fmt.Errorf("kolom: %w", err)
	}
	for rows.Next() {
		var table, name, typ, expr, gen string
		var notNull bool
		if err := rows.Scan(&table, &name, &typ, &notNull, &expr, &gen); err != nil {
			_ = rows.Close()
			return nil, err
		}
		col := Column{Name: name, Type: typ, NotNull: notNull}
		if gen == "s" {
			col.Generated = expr
		} else {
			col.Default = expr
		}
		cat.Tables[table].Columns = append(cat.Tables[table].Columns, col)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	// Constraint: kunci utama, unique, foreign key, dan CHECK (NOT NULL dibaca dari attnotnull).
	rows, err = conn.QueryContext(ctx, `
SELECT c.relname, con.conname, con.contype::text, pg_get_constraintdef(con.oid),
       COALESCE((SELECT string_agg(a.attname, ',' ORDER BY k.ord)
                 FROM unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord)
                 JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum), '')
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND con.contype IN ('p', 'u', 'f', 'c')
ORDER BY c.relname, con.conname`, schema)
	if err != nil {
		return nil, fmt.Errorf("constraint: %w", err)
	}
	for rows.Next() {
		var table, name, kind, def, cols string
		if err := rows.Scan(&table, &name, &kind, &def, &cols); err != nil {
			_ = rows.Close()
			return nil, err
		}
		con := Constraint{Name: name, Kind: kind, Def: def}
		if cols != "" {
			con.Columns = strings.Split(cols, ",")
		}
		cat.Tables[table].Constraints = append(cat.Tables[table].Constraints, con)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	// Indeks yang tidak mendukung constraint. Kunci: metode, keunikan, kolom atau ekspresi, predikat.
	rows, err = conn.QueryContext(ctx, `
SELECT t.relname, i.relname, am.amname, ix.indisunique,
       COALESCE((SELECT string_agg(pg_get_indexdef(ix.indexrelid, k, true), ',' ORDER BY k)
                 FROM generate_series(1, ix.indnkeyatts::int) AS k), ''),
       COALESCE(pg_get_expr(ix.indpred, ix.indrelid), ''),
       ix.indnatts::int - ix.indnkeyatts::int,
       COALESCE((SELECT string_agg(a.attname, ',' ORDER BY k.ord)
                 FROM unnest(ix.indkey::int2[]) WITH ORDINALITY AS k(attnum, ord)
                 JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum), '')
FROM pg_index ix
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_am am ON am.oid = i.relam
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = $1
  AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conindid = ix.indexrelid)
ORDER BY t.relname, i.relname`, schema)
	if err != nil {
		return nil, fmt.Errorf("indeks: %w", err)
	}
	for rows.Next() {
		var table, name, method, cols, pred, attrs string
		var unique bool
		var included int
		if err := rows.Scan(&table, &name, &method, &unique, &cols, &pred, &included, &attrs); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ix := Index{Name: name, Key: fmt.Sprintf("%s:%t:%s:%s:include=%d", method, unique, cols, pred, included)}
		if attrs != "" {
			ix.Columns = strings.Split(attrs, ",")
		}
		cat.Tables[table].Indexes = append(cat.Tables[table].Indexes, ix)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	// Pemicu non-internal. tgenabled 'O' = aktif untuk sesi biasa (origin); 'D', 'R', 'A' bukan.
	rows, err = conn.QueryContext(ctx, `
SELECT c.relname, t.tgname, t.tgenabled::text, t.tgtype::int, p.proname, p.prosrc
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_proc p ON p.oid = t.tgfoid
WHERE n.nspname = $1 AND NOT t.tgisinternal
ORDER BY c.relname, t.tgname`, schema)
	if err != nil {
		return nil, fmt.Errorf("pemicu: %w", err)
	}
	for rows.Next() {
		var table, name, enabled, fn, body string
		var typ int
		if err := rows.Scan(&table, &name, &enabled, &typ, &fn, &body); err != nil {
			_ = rows.Close()
			return nil, err
		}
		cat.Tables[table].Triggers = append(cat.Tables[table].Triggers, Trigger{Name: name, Enabled: enabled == "O" || enabled == "A", Always: enabled == "A", Type: typ, Function: fn, Body: body})
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	return cat, nil
}

func closeRows(rows interface {
	Err() error
	Close() error
}) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}
