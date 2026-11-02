package diff

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"erpora/internal/platform/database"
)

// tableKinds membatasi introspeksi ke tabel biasa dan tabel berpartisi. Indeks pada materialized view, pemicu
// pada view atau foreign table, dan constraint relasi lain di skema yang sama tidak boleh membuat Introspect
// gagal atau panik; objek itu bukan bagian model.
const tableKinds = `('r', 'p')`

var indexDefRe = regexp.MustCompile(`^CREATE (UNIQUE )?INDEX (?:"(?:[^"]|"")*"|\S+) ON (.*)$`)

// Introspect membaca katalog PostgreSQL untuk satu skema lewat satu koneksi khusus dari db. search_path koneksi
// diarahkan ke skema itu sehingga pg_get_expr, pg_get_constraintdef, pg_get_indexdef, pg_get_triggerdef, dan
// format_type menulis nama tanpa kualifikasi (aktual dan diharapkan dibandingkan dengan normalisasi yang
// sama). Hanya baca.
func Introspect(ctx context.Context, db *database.DB, schema string) (*Catalog, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("koneksi: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), `RESET search_path`)
		_ = conn.Close()
	}()
	if _, err := conn.ExecContext(ctx, `SELECT set_config('search_path', $1, false)`, schema); err != nil {
		return nil, fmt.Errorf("search_path: %w", err)
	}
	cat := &Catalog{Schema: schema, Tables: map[string]*Table{}}
	// table mengembalikan tabel hasil introspeksi menurut nama; nil bila bukan tabel (aman dari panik).
	table := func(name string) *Table { return cat.Tables[name] }

	rows, err := conn.QueryContext(ctx, `
SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relkind IN `+tableKinds+` ORDER BY c.relname`, schema)
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

	// Kolom. attgenerated 's' berarti stored; ekspresinya disimpan di pg_attrdef. Kolasi hanya dicatat bila
	// bukan kolasi bawaan; identity a (ALWAYS) atau d (BY DEFAULT).
	rows, err = conn.QueryContext(ctx, `
SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
       COALESCE(pg_get_expr(d.adbin, d.adrelid), ''), a.attgenerated::text,
       CASE WHEN a.attcollation = 0 OR co.collname = 'default' THEN '' ELSE COALESCE(co.collname, '') END,
       a.attidentity::text
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
LEFT JOIN pg_collation co ON co.oid = a.attcollation
WHERE n.nspname = $1 AND c.relkind IN `+tableKinds+` AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY c.relname, a.attnum`, schema)
	if err != nil {
		return nil, fmt.Errorf("kolom: %w", err)
	}
	for rows.Next() {
		var tname, name, typ, expr, gen, collation, identity string
		var notNull bool
		if err := rows.Scan(&tname, &name, &typ, &notNull, &expr, &gen, &collation, &identity); err != nil {
			_ = rows.Close()
			return nil, err
		}
		tbl := table(tname)
		if tbl == nil {
			continue
		}
		col := Column{Name: name, Type: typ, NotNull: notNull, Collation: collation, Identity: identity}
		if gen == "s" {
			col.Generated = expr
		} else {
			col.Default = expr
		}
		tbl.Columns = append(tbl.Columns, col)
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
WHERE n.nspname = $1 AND c.relkind IN `+tableKinds+` AND con.contype IN ('p', 'u', 'f', 'c')
ORDER BY c.relname, con.conname`, schema)
	if err != nil {
		return nil, fmt.Errorf("constraint: %w", err)
	}
	for rows.Next() {
		var tname, name, kind, def, cols string
		if err := rows.Scan(&tname, &name, &kind, &def, &cols); err != nil {
			_ = rows.Close()
			return nil, err
		}
		tbl := table(tname)
		if tbl == nil {
			continue
		}
		con := Constraint{Name: name, Kind: kind, Def: def}
		if cols != "" {
			con.Columns = strings.Split(cols, ",")
		}
		tbl.Constraints = append(tbl.Constraints, con)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	// Indeks yang tidak mendukung constraint, hanya pada tabel. Kunci: definisi lengkap tanpa nama dan skema
	// (metode, keunikan, kolom beserta opclass, DESC, NULLS, INCLUDE, NULLS NOT DISTINCT, predikat).
	rows, err = conn.QueryContext(ctx, `
SELECT t.relname, i.relname, pg_get_indexdef(ix.indexrelid), ix.indisvalid,
       COALESCE((SELECT string_agg(a.attname, ',' ORDER BY k.ord)
                 FROM unnest(ix.indkey::int2[]) WITH ORDINALITY AS k(attnum, ord)
                 JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum), '')
FROM pg_index ix
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = $1 AND t.relkind IN `+tableKinds+`
  AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conindid = ix.indexrelid)
ORDER BY t.relname, i.relname`, schema)
	if err != nil {
		return nil, fmt.Errorf("indeks: %w", err)
	}
	for rows.Next() {
		var tname, name, def, attrs string
		var valid bool
		if err := rows.Scan(&tname, &name, &def, &valid, &attrs); err != nil {
			_ = rows.Close()
			return nil, err
		}
		tbl := table(tname)
		if tbl == nil {
			continue
		}
		ix := Index{Name: name, Key: normalizeIndexDef(def, schema), Invalid: !valid}
		if attrs != "" {
			ix.Columns = strings.Split(attrs, ",")
		}
		tbl.Indexes = append(tbl.Indexes, ix)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	// Pemicu non-internal pada tabel. tgenabled 'O' = aktif untuk sesi biasa (origin), 'A' = selalu aktif (juga
	// pada session_replication_role = replica); 'D' dan 'R' tidak aktif untuk sesi biasa. Definisi pemicu,
	// pengaturan fungsi, dan bahasa ikut dibaca: WHEN (false), UPDATE OF, SET search_path, SECURITY DEFINER, atau
	// fungsi di skema lain membuat pemicu tidak efektif walau namanya sama.
	rows, err = conn.QueryContext(ctx, `
SELECT c.relname, t.tgname, t.tgenabled::text, t.tgtype::int, p.proname, p.prosrc,
       pg_get_triggerdef(t.oid, true), p.prosecdef, COALESCE(array_to_string(p.proconfig, ','), ''), l.lanname
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_proc p ON p.oid = t.tgfoid
JOIN pg_language l ON l.oid = p.prolang
WHERE n.nspname = $1 AND c.relkind IN `+tableKinds+` AND NOT t.tgisinternal
ORDER BY c.relname, t.tgname`, schema)
	if err != nil {
		return nil, fmt.Errorf("pemicu: %w", err)
	}
	for rows.Next() {
		var tname, name, enabled, fn, body, def, config, lang string
		var typ int
		var secdef bool
		if err := rows.Scan(&tname, &name, &enabled, &typ, &fn, &body, &def, &secdef, &config, &lang); err != nil {
			_ = rows.Close()
			return nil, err
		}
		tbl := table(tname)
		if tbl == nil {
			continue
		}
		tbl.Triggers = append(tbl.Triggers, Trigger{Name: name, Enabled: enabled == "O" || enabled == "A", Always: enabled == "A", Type: typ,
			Function: fn, Body: body, Def: def, SecDef: secdef, Config: config, Lang: lang})
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	return cat, nil
}

// normalizeIndexDef membuang "CREATE [UNIQUE] INDEX <nama> ON " dan kualifikasi skema dari pg_get_indexdef sehingga nama indeks tidak
// ikut dibandingkan (migrasi tulis tangan sering bernama lain).
func normalizeIndexDef(def, schema string) string {
	m := indexDefRe.FindStringSubmatch(def)
	if m == nil {
		return def
	}
	// pg_get_indexdef mengualifikasi tabel dengan skema walau ada di search_path; dibuang agar skema aktual dan
	// skema diharapkan sebanding.
	rest := strings.TrimPrefix(strings.TrimPrefix(m[2], schema+"."), `"`+schema+`".`)
	return "INDEX " + m[1] + "ON " + rest
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
