package sqlgen

import (
	"fmt"
	"strings"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

// Jenis pernyataan keluaran.
const (
	KindCreateTable    = "create_table"
	KindCreateIndex    = "create_index"
	KindAddForeignKey  = "add_foreign_key"
	KindAddColumn      = "add_column"
	KindCreateFunction = "create_function"
	KindCreateTrigger  = "create_trigger"
	KindEnableTrigger  = "enable_trigger"
)

// Options mengatur keluaran. Schema, bila tidak kosong, mengualifikasi nama tabel dan fungsi (dipakai mesin
// selisih untuk membangun skema "diharapkan" pada skema sementara); kosong = tanpa kualifikasi.
type Options struct {
	Schema string
	// IfNotExists membuat AddColumn memakai ADD COLUMN IF NOT EXISTS (idempoten).
	IfNotExists bool
}

// Statement adalah satu pernyataan SQL utuh (tanpa titik koma penutup).
type Statement struct {
	Kind   string
	Table  string
	Object string
	SQL    string
}

func (o Options) qualify(name string) (string, error) {
	q, err := QuoteIdent(name)
	if err != nil {
		return "", err
	}
	if o.Schema == "" {
		return q, nil
	}
	s, err := QuoteIdent(o.Schema)
	if err != nil {
		return "", err
	}
	return s + "." + q, nil
}

// CreateSchema menghasilkan DDL seluruh model: semua tabel (kolom, kunci utama, unique, CHECK) lebih dulu,
// lalu indeks, foreign key, dan pemicu. Foreign key ditambahkan terpisah sehingga urutan tabel tidak penting.
func CreateSchema(s *dbmodel.Schema, o Options) ([]Statement, error) {
	var out []Statement
	for _, t := range s.Tables {
		sql, err := CreateTable(t, o)
		if err != nil {
			return nil, fmt.Errorf("tabel %s: %w", t.Name, err)
		}
		out = append(out, Statement{Kind: KindCreateTable, Table: t.Name, Object: t.Name, SQL: sql})
	}
	for _, t := range s.Tables {
		for _, ix := range t.Indexes {
			sql, err := CreateIndex(t, ix, o)
			if err != nil {
				return nil, fmt.Errorf("indeks %s: %w", ix.Name, err)
			}
			out = append(out, Statement{Kind: KindCreateIndex, Table: t.Name, Object: ix.Name, SQL: sql})
		}
	}
	for _, t := range s.Tables {
		for _, fk := range t.ForeignKeys {
			sql, err := AddForeignKey(t, fk, o)
			if err != nil {
				return nil, fmt.Errorf("foreign key %s: %w", fk.Name, err)
			}
			out = append(out, Statement{Kind: KindAddForeignKey, Table: t.Name, Object: fk.Name, SQL: sql})
		}
	}
	for _, t := range s.Tables {
		for _, tr := range t.Triggers {
			sts, err := TriggerStatements(t, tr, o)
			if err != nil {
				return nil, fmt.Errorf("pemicu %s: %w", tr.Name, err)
			}
			out = append(out, sts...)
		}
	}
	return out, nil
}

func findColumn(t *dbmodel.Table, name string) *dbmodel.Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func quoteList(names []string) (string, error) {
	parts := make([]string, len(names))
	for i, n := range names {
		q, err := QuoteIdent(n)
		if err != nil {
			return "", err
		}
		parts[i] = q
	}
	return strings.Join(parts, ", "), nil
}

// ColumnDefinition menghasilkan definisi satu kolom (tanpa awalan ALTER): nama, tipe, nilai bawaan atau
// ekspresi turunan, lalu NOT NULL.
func ColumnDefinition(t *dbmodel.Table, c *dbmodel.Column) (string, error) {
	name, err := QuoteIdent(c.Name)
	if err != nil {
		return "", err
	}
	if c.Type.Name == "" {
		return "", fmt.Errorf("kolom %s tanpa tipe", c.Name)
	}
	def := name + " " + c.Type.String()
	switch {
	case c.Generated != nil:
		env := EnvForTable(t)
		sql, err := StoredExpr(c.Generated, env)
		if err != nil {
			return "", fmt.Errorf("kolom %s: %w", c.Name, err)
		}
		typ, err := StoredExprType(c.Generated, env)
		if err != nil {
			return "", fmt.Errorf("kolom %s: %w", c.Name, err)
		}
		if f, g := familyOf(c.FieldType), familyOf(typ); typ != metadata.TypeNull && f != g && (!numeric(f) || !numeric(g)) {
			return "", fmt.Errorf("kolom %s: ekspresi bertipe %s tidak cocok dengan kolom %s", c.Name, typ, c.FieldType)
		}
		def += " GENERATED ALWAYS AS (" + sql + ") STORED"
	case c.Default != nil:
		lit, err := DefaultLiteral(c.Default)
		if err != nil {
			return "", fmt.Errorf("kolom %s: %w", c.Name, err)
		}
		def += " DEFAULT " + lit
	}
	if c.NotNull {
		def += " NOT NULL"
	}
	return def, nil
}

// CheckExpression menghasilkan ekspresi CHECK satu constraint.
func CheckExpression(t *dbmodel.Table, ck *dbmodel.Check) (string, error) {
	if findColumn(t, ck.Column) == nil {
		return "", fmt.Errorf("CHECK %s merujuk kolom %q yang tidak ada", ck.Name, ck.Column)
	}
	col, err := QuoteIdent(ck.Column)
	if err != nil {
		return "", err
	}
	var parts []string
	switch ck.Kind {
	case dbmodel.CheckLength:
		for _, b := range []struct{ v, op string }{{ck.Min, ">="}, {ck.Max, "<="}} {
			if b.v == "" {
				continue
			}
			if !isDigitsN(b.v, 1, 9) {
				return "", fmt.Errorf("CHECK %s: batas panjang %q tidak sah", ck.Name, truncate(b.v))
			}
			parts = append(parts, "length("+col+") "+b.op+" "+b.v)
		}
	case dbmodel.CheckRange:
		for _, b := range []struct{ v, op string }{{ck.Min, ">="}, {ck.Max, "<="}} {
			if b.v == "" {
				continue
			}
			lit, err := NumericLiteral(b.v)
			if err != nil {
				return "", fmt.Errorf("CHECK %s: %w", ck.Name, err)
			}
			parts = append(parts, col+" "+b.op+" "+wrapNeg(lit))
		}
	case dbmodel.CheckEnum:
		if len(ck.Values) == 0 {
			return "", fmt.Errorf("CHECK %s: daftar nilai kosong", ck.Name)
		}
		lits := make([]string, len(ck.Values))
		for i, v := range ck.Values {
			l, err := Literal(v)
			if err != nil {
				return "", fmt.Errorf("CHECK %s: %w", ck.Name, err)
			}
			lits[i] = l
		}
		parts = append(parts, col+" IN ("+strings.Join(lits, ", ")+")")
	default:
		return "", fmt.Errorf("CHECK %s: jenis %q tidak dikenal", ck.Name, ck.Kind)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("CHECK %s: tanpa batas", ck.Name)
	}
	return strings.Join(parts, " AND "), nil
}

// CreateTable menghasilkan CREATE TABLE: kolom, kunci utama, unique, dan CHECK. Foreign key dan indeks
// terpisah (AddForeignKey, CreateIndex).
func CreateTable(t *dbmodel.Table, o Options) (string, error) {
	name, err := o.qualify(t.Name)
	if err != nil {
		return "", err
	}
	if len(t.Columns) == 0 {
		return "", fmt.Errorf("tabel %s tanpa kolom", t.Name)
	}
	var lines []string
	for _, c := range t.Columns {
		def, err := ColumnDefinition(t, c)
		if err != nil {
			return "", err
		}
		lines = append(lines, def)
	}
	if len(t.PrimaryKey) > 0 {
		pk, err := QuoteIdent(t.Name + "_pkey")
		if err != nil {
			return "", err
		}
		cols, err := quoteList(t.PrimaryKey)
		if err != nil {
			return "", err
		}
		for _, c := range t.PrimaryKey {
			if findColumn(t, c) == nil {
				return "", fmt.Errorf("kunci utama merujuk kolom %q yang tidak ada", c)
			}
		}
		lines = append(lines, "CONSTRAINT "+pk+" PRIMARY KEY ("+cols+")")
	}
	for _, u := range t.Uniques {
		n, err := QuoteIdent(u.Name)
		if err != nil {
			return "", err
		}
		cols, err := quoteList(u.Columns)
		if err != nil {
			return "", err
		}
		lines = append(lines, "CONSTRAINT "+n+" UNIQUE ("+cols+")")
	}
	for _, ck := range t.Checks {
		n, err := QuoteIdent(ck.Name)
		if err != nil {
			return "", err
		}
		e, err := CheckExpression(t, ck)
		if err != nil {
			return "", err
		}
		lines = append(lines, "CONSTRAINT "+n+" CHECK ("+e+")")
	}
	return "CREATE TABLE " + name + " (\n    " + strings.Join(lines, ",\n    ") + "\n)", nil
}

// AddColumn menghasilkan ALTER TABLE ... ADD COLUMN untuk satu kolom model.
func AddColumn(t *dbmodel.Table, c *dbmodel.Column, o Options) (string, error) {
	name, err := o.qualify(t.Name)
	if err != nil {
		return "", err
	}
	def, err := ColumnDefinition(t, c)
	if err != nil {
		return "", err
	}
	ine := ""
	if o.IfNotExists {
		ine = "IF NOT EXISTS "
	}
	return "ALTER TABLE " + name + " ADD COLUMN " + ine + def, nil
}

// CreateIndex menghasilkan CREATE INDEX btree biasa.
func CreateIndex(t *dbmodel.Table, ix *dbmodel.Index, o Options) (string, error) {
	name, err := QuoteIdent(ix.Name)
	if err != nil {
		return "", err
	}
	table, err := o.qualify(t.Name)
	if err != nil {
		return "", err
	}
	cols, err := quoteList(ix.Columns)
	if err != nil {
		return "", err
	}
	return "CREATE INDEX " + name + " ON " + table + " (" + cols + ")", nil
}

// AddForeignKey menghasilkan ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY.
func AddForeignKey(t *dbmodel.Table, fk *dbmodel.ForeignKey, o Options) (string, error) {
	table, err := o.qualify(t.Name)
	if err != nil {
		return "", err
	}
	name, err := QuoteIdent(fk.Name)
	if err != nil {
		return "", err
	}
	cols, err := quoteList(fk.Columns)
	if err != nil {
		return "", err
	}
	ref, err := o.qualify(fk.RefTable)
	if err != nil {
		return "", err
	}
	refCols, err := quoteList(fk.RefColumns)
	if err != nil {
		return "", err
	}
	var action string
	switch fk.OnDelete {
	case dbmodel.OnDeleteRestrict, "":
		action = "RESTRICT"
	case dbmodel.OnDeleteCascade:
		action = "CASCADE"
	case dbmodel.OnDeleteSetNull:
		action = "SET NULL"
	default:
		return "", fmt.Errorf("foreign key %s: onDelete %q tidak dikenal", fk.Name, fk.OnDelete)
	}
	return "ALTER TABLE " + table + " ADD CONSTRAINT " + name + " FOREIGN KEY (" + cols + ") REFERENCES " + ref + " (" + refCols + ") ON DELETE " + action, nil
}
