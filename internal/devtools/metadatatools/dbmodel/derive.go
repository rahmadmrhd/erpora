package dbmodel

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

var (
	enumValueRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	uuidRe      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	intRe       = regexp.MustCompile(`^-?[0-9]{1,18}$`)
)

type deriver struct {
	probs    []load.Problem
	byKey    map[string]*resolve.Resolved
	tables   map[string]*Table // menurut kunci resource
	relNS    *naming.Namespace // tabel, indeks, dan constraint unique berbagi ruang nama relasi
	conNS    map[string]*naming.Namespace
	schema   *Schema
	resource *resolve.Resolved
}

func (d *deriver) problem(code, msg string) {
	p := load.Problem{Code: code, Message: msg}
	if d.resource != nil {
		p.File, p.Line, p.Column = d.resource.Source.File, d.resource.Source.Line, d.resource.Source.Column
		p.Message = d.resource.Key + ": " + msg
	}
	d.probs = append(d.probs, p)
}

func (d *deriver) problemAt(f *resolve.Field, code, msg string) {
	if f == nil || d.resource == nil {
		d.problem(code, msg)
		return
	}
	d.probs = append(d.probs, load.Problem{File: f.Origin.File, Line: f.Origin.Line, Column: f.Origin.Column,
		Code: code, Message: d.resource.Key + "." + f.Name + ": " + msg})
}

// Derive menurunkan model skema dari semua resource (kind resource) hasil resolusi. Resource abstrak tidak
// menghasilkan tabel. Semua temuan dikumpulkan; model dikembalikan hanya berisi tabel yang berhasil diturunkan
// dan sebaiknya tidak dipakai bila ada temuan.
func Derive(resources []*resolve.Resolved) (*Schema, []load.Problem) {
	d := &deriver{
		byKey:  map[string]*resolve.Resolved{},
		tables: map[string]*Table{},
		relNS:  naming.NewNamespace("skema"),
		conNS:  map[string]*naming.Namespace{},
		schema: &Schema{},
	}
	var rs []*resolve.Resolved
	for _, r := range resources {
		if r.Kind == "resource" {
			rs = append(rs, r)
			d.byKey[r.Key] = r
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].Key < rs[j].Key })

	for _, r := range rs {
		d.resource = r
		d.deriveTable(r)
	}
	for _, r := range rs {
		d.resource = r
		if t := d.tables[r.Key]; t != nil {
			d.deriveRelations(r, t)
			d.deriveIndexes(r, t)
		}
	}
	d.resource = nil
	d.schema.index()
	sort.SliceStable(d.probs, func(i, j int) bool {
		a, b := d.probs[i], d.probs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Message < b.Message
	})
	return d.schema, d.probs
}

func (d *deriver) claimRel(name, origin string) {
	if err := d.relNS.Claim(name, origin); err != nil {
		d.problem(CodeCollision, err.Error())
	}
}

func (d *deriver) claimCon(table, name, origin string) {
	ns := d.conNS[table]
	if ns == nil {
		ns = naming.NewNamespace("tabel " + table)
		d.conNS[table] = ns
	}
	if err := ns.Claim(name, origin); err != nil {
		d.problem(CodeCollision, err.Error())
	}
}

func (d *deriver) nameErr(err error) {
	if err != nil {
		d.problem(CodeName, err.Error())
	}
}

func snake(name string) (string, bool) { s, err := naming.ToSnake(name); return s, err == nil }

func (d *deriver) deriveTable(r *resolve.Resolved) {
	slug, name, err := naming.SplitResourceKey(r.Key)
	if err != nil {
		d.nameErr(err)
		return
	}
	tn, err := naming.TableName(slug, name)
	if err != nil {
		d.nameErr(err)
		return
	}
	t := &Table{Name: tn, Resource: r.Key, Module: slug, Kind: KindResource, Large: r.Large}
	d.claimRel(tn, "tabel "+r.Key)
	d.claimRel(tn+"_pkey", "kunci utama "+tn)

	seen := map[string]string{}
	addCol := func(c *Column, origin string) bool {
		if prev, ok := seen[c.Name]; ok {
			d.problem(CodeCollision, fmt.Sprintf("kolom %q pada %s bentrok: dipakai oleh %s dan %s", c.Name, tn, prev, origin))
			return false
		}
		seen[c.Name] = origin
		t.Columns = append(t.Columns, c)
		return true
	}

	for _, f := range r.Fields {
		acc, _ := f.Attrs["accessor"].(map[string]any)
		kind, _ := acc["kind"].(string)
		if kind == "expr" || kind == "func" {
			continue // dihitung di Go; tanpa kolom (Requirement 5.1)
		}
		c := d.fieldColumn(tn, f, kind == "stored", acc)
		if c == nil {
			continue
		}
		if !addCol(c, "field "+f.Name) {
			continue
		}
		if ck := d.fieldCheck(tn, f, c); ck != nil {
			t.Checks = append(t.Checks, ck)
			d.claimCon(tn, ck.Name, "CHECK "+c.Name)
		}
		if f.Bool("unique") {
			if n, err := naming.UniqueName(tn, f.Name); err != nil {
				d.nameErr(err)
			} else {
				t.Uniques = append(t.Uniques, &Unique{Name: n, Columns: []string{c.Name}})
				d.claimRel(n, "unique "+c.Name)
			}
		}
		if f.Bool("index") {
			if n, err := naming.IndexName(tn, f.Name); err != nil {
				d.nameErr(err)
			} else {
				t.Indexes = append(t.Indexes, &Index{Name: n, Columns: []string{c.Name}})
				d.claimRel(n, "indeks "+c.Name)
			}
		}
	}

	id := t.colByName("id")
	if id == nil || id.Type != TypeUUID {
		d.problem(CodeNoID, "resource wajib punya field id bertipe uuid (kunci utama); sertakan sdk:audit")
	} else {
		id.NotNull = true
		t.PrimaryKey = []string{"id"}
	}

	for _, u := range r.UnmanagedColumns {
		if n, _ := u["name"].(string); n != "" {
			if s, ok := snake(n); ok {
				t.Unmanaged = append(t.Unmanaged, s)
			} else {
				d.problem(CodeName, fmt.Sprintf("unmanagedColumns: nama %q tidak sah", n))
			}
		}
	}
	sort.Strings(t.Unmanaged)

	if r.LockWhen != nil {
		cond, err := metadata.ParseCondition(r.LockWhen)
		if err != nil {
			d.problem(CodeLock, fmt.Sprintf("lockWhen tidak sah: %v", err))
		} else {
			t.Lock = cond
		}
	}

	d.tables[r.Key] = t
	d.schema.Tables = append(d.schema.Tables, t)
	// Kolom diindeks sementara agar derivasi tahap berikut dapat memeriksa tabrakan.
	t.index()
	for _, c := range t.Columns {
		if c.Generated != nil {
			d.checkGeneratedRefs(r, t, c)
		}
	}
}

func (t *Table) colByName(n string) *Column {
	for _, c := range t.Columns {
		if c.Name == n {
			return c
		}
	}
	return nil
}

var builtinTypes = map[string]metadata.FieldType{
	"string": metadata.TypeString, "text": metadata.TypeText, "integer": metadata.TypeInteger,
	"decimal": metadata.TypeDecimal, "boolean": metadata.TypeBoolean, "date": metadata.TypeDate,
	"datetime": metadata.TypeDateTime, "enum": metadata.TypeEnum, "uuid": metadata.TypeUUID,
}

func numText(attrs map[string]any, key string) string {
	switch v := attrs[key].(type) {
	case json.Number:
		return v.String()
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func isIntText(s string) bool { return intRe.MatchString(s) }

func (d *deriver) fieldColumn(tn string, f *resolve.Field, stored bool, acc map[string]any) *Column {
	cn, ok := snake(f.Name)
	if !ok {
		d.problemAt(f, CodeName, fmt.Sprintf("nama field %q tidak dapat dikonversi ke snake_case", f.Name))
		return nil
	}
	ft, ok := builtinTypes[f.Type()]
	if !ok {
		d.problemAt(f, CodeType, fmt.Sprintf("tipe %q tidak dikenal (tipe domain harus sudah diselesaikan ke tipe dasar)", f.Type()))
		return nil
	}
	c := &Column{Name: cn, NotNull: !f.Bool("nullable"), Field: f.Name, FieldType: ft, camel: f.Name}
	switch ft {
	case metadata.TypeString, metadata.TypeText, metadata.TypeEnum:
		c.Type = TypeText
	case metadata.TypeInteger:
		c.Type = TypeBigint
		lo, hi := numText(f.Attrs, "minimum"), numText(f.Attrs, "maximum")
		if lo != "" && hi != "" {
			l, e1 := strconv.ParseInt(lo, 10, 64)
			h, e2 := strconv.ParseInt(hi, 10, 64)
			if e1 == nil && e2 == nil && l >= math.MinInt32 && h <= math.MaxInt32 {
				c.Type = TypeInteger
			}
		}
	case metadata.TypeDecimal:
		p, s := numText(f.Attrs, "precision"), numText(f.Attrs, "scale")
		pi, e1 := strconv.Atoi(p)
		si, e2 := strconv.Atoi(s)
		if p == "" || s == "" || e1 != nil || e2 != nil || si > pi {
			d.problemAt(f, CodeType, "decimal wajib menyatakan precision dan scale (scale <= precision)")
			return nil
		}
		c.Type = Numeric(pi, si)
	case metadata.TypeBoolean:
		c.Type = TypeBoolean
	case metadata.TypeDate:
		c.Type = TypeDate
	case metadata.TypeDateTime:
		c.Type = TypeTimestamp
	case metadata.TypeUUID:
		c.Type = TypeUUID
	}
	if mf := f.Str("migrateFrom"); mf != "" {
		if s, ok := snake(mf); ok {
			c.MigrateFrom = s
		}
	}
	if stored {
		expr, err := metadata.ParseExpr(acc["expr"])
		if err != nil {
			d.problemAt(f, CodeExpr, fmt.Sprintf("ekspresi accessor stored tidak sah: %v", err))
			return nil
		}
		c.Generated = expr
		return c
	}
	if def, ok := d.defaultOf(f, ft); ok {
		c.Default = def
	}
	return c
}

func (d *deriver) defaultOf(f *resolve.Field, ft metadata.FieldType) (*Default, bool) {
	raw, present := f.Attrs["default"]
	if !present || raw == nil {
		return nil, false
	}
	bad := func(msg string) (*Default, bool) {
		d.problemAt(f, CodeDefault, msg)
		return nil, false
	}
	switch ft {
	case metadata.TypeBoolean:
		b, ok := raw.(bool)
		if !ok {
			return bad("default boolean harus true atau false")
		}
		return &Default{Type: ft, Value: strconv.FormatBool(b)}, true
	case metadata.TypeInteger:
		s := ""
		switch v := raw.(type) {
		case json.Number:
			s = v.String()
		case string:
			s = v
		}
		if !isIntText(s) {
			return bad("default integer harus bilangan bulat")
		}
		return &Default{Type: ft, Value: s}, true
	case metadata.TypeDecimal:
		s := ""
		switch v := raw.(type) {
		case json.Number:
			s = v.String()
		case string:
			s = v
		}
		if _, ok := metadata.ParseDecimal(s); !ok {
			return bad("default decimal harus string desimal")
		}
		return &Default{Type: ft, Value: s}, true
	}
	s, ok := raw.(string)
	if !ok {
		return bad("default harus string")
	}
	switch ft {
	case metadata.TypeEnum:
		found := false
		if vals, ok := f.Attrs["values"].([]any); ok {
			for _, v := range vals {
				if v == s {
					found = true
				}
			}
		}
		if !found {
			return bad(fmt.Sprintf("default %q bukan salah satu nilai enum", s))
		}
	case metadata.TypeUUID:
		if !uuidRe.MatchString(s) {
			return bad("default uuid harus huruf kecil kanonik")
		}
	case metadata.TypeDate:
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return bad("default date harus YYYY-MM-DD")
		}
	case metadata.TypeDateTime:
		if tm, err := time.Parse(time.RFC3339Nano, s); err != nil || !strings.HasSuffix(s, "Z") || tm.IsZero() {
			return bad("default datetime harus RFC 3339 UTC (akhiran Z)")
		}
	}
	return &Default{Type: ft, Value: s}, true
}

func (d *deriver) fieldCheck(tn string, f *resolve.Field, c *Column) *Check {
	mk := func(kind CheckKind) *Check {
		n, err := naming.CheckName(tn, f.Name)
		if err != nil {
			d.nameErr(err)
			return nil
		}
		return &Check{Name: n, Column: c.Name, Kind: kind}
	}
	switch c.FieldType {
	case metadata.TypeString, metadata.TypeText:
		lo, hi := numText(f.Attrs, "minLength"), numText(f.Attrs, "maxLength")
		if lo == "0" {
			lo = ""
		}
		for _, v := range []string{lo, hi} {
			if v != "" && !isIntText(v) {
				d.problemAt(f, CodeType, "minLength dan maxLength harus bilangan bulat")
				return nil
			}
		}
		if lo == "" && hi == "" {
			return nil
		}
		ck := mk(CheckLength)
		if ck != nil {
			ck.Min, ck.Max = lo, hi
		}
		return ck
	case metadata.TypeInteger, metadata.TypeDecimal:
		lo, hi := numText(f.Attrs, "minimum"), numText(f.Attrs, "maximum")
		if lo == "" && hi == "" {
			return nil
		}
		for _, v := range []string{lo, hi} {
			if v == "" {
				continue
			}
			if _, ok := metadata.ParseDecimal(v); !ok {
				d.problemAt(f, CodeType, "minimum dan maximum harus angka desimal")
				return nil
			}
			if c.FieldType == metadata.TypeInteger && !isIntText(v) {
				d.problemAt(f, CodeType, "minimum dan maximum integer harus bilangan bulat")
				return nil
			}
		}
		ck := mk(CheckRange)
		if ck != nil {
			ck.Min, ck.Max = lo, hi
		}
		return ck
	case metadata.TypeEnum:
		vals, _ := f.Attrs["values"].([]any)
		if len(vals) == 0 {
			d.problemAt(f, CodeType, "enum wajib punya values")
			return nil
		}
		ck := mk(CheckEnum)
		if ck == nil {
			return nil
		}
		for _, v := range vals {
			s, _ := v.(string)
			if !enumValueRe.MatchString(s) {
				d.problemAt(f, CodeEnumValue, fmt.Sprintf("nilai enum %q harus cocok %s", s, enumValueRe))
				return nil
			}
			ck.Values = append(ck.Values, s)
		}
		return ck
	}
	return nil
}

// checkGeneratedRefs memastikan ekspresi kolom turunan hanya merujuk kolom biasa milik tabel yang sama
// (PostgreSQL menolak rujukan ke kolom turunan lain).
func (d *deriver) checkGeneratedRefs(r *resolve.Resolved, t *Table, c *Column) {
	for _, name := range exprFields(c.Generated) {
		fc := columnOfField(t, name)
		switch {
		case fc == nil:
			d.problem(CodeExpr, fmt.Sprintf("accessor stored %s merujuk field %q yang tidak punya kolom", c.Name, name))
		case fc.Generated != nil:
			d.problem(CodeExpr, fmt.Sprintf("accessor stored %s merujuk kolom turunan %q (tidak didukung PostgreSQL)", c.Name, name))
		}
	}
}

func columnOfField(t *Table, field string) *Column {
	for _, c := range t.Columns {
		if c.Field == field {
			return c
		}
	}
	return nil
}

// exprFields mengumpulkan nama field yang dirujuk ekspresi (termasuk kondisi pada if).
func exprFields(e *metadata.Expr) []string {
	if e == nil {
		return nil
	}
	var out []string
	if e.Kind == metadata.ExprField {
		out = append(out, e.Field)
	}
	for _, a := range e.Args {
		out = append(out, exprFields(a)...)
	}
	out = append(out, condFields(e.Cond)...)
	out = append(out, exprFields(e.Then)...)
	out = append(out, exprFields(e.Else)...)
	return out
}

func condFields(c *metadata.Condition) []string {
	if c == nil {
		return nil
	}
	var out []string
	if c.Kind == metadata.CondLeaf {
		out = append(out, c.Field)
		if c.Ref != "" {
			out = append(out, c.Ref)
		}
	}
	for _, ch := range c.Children {
		out = append(out, condFields(ch)...)
	}
	return out
}

func (d *deriver) deriveIndexes(r *resolve.Resolved, t *Table) {
	for i, ix := range r.Indexes {
		raw, _ := ix["fields"].([]any)
		var cols, camel []string
		ok := true
		for _, x := range raw {
			fname, _ := x.(string)
			sn, snok := snake(fname)
			if !snok || t.Column(sn) == nil {
				d.problem(CodeIndexField, fmt.Sprintf("indexes[%d]: field %q tidak punya kolom pada %s", i, fname, t.Name))
				ok = false
				continue
			}
			cols = append(cols, sn)
			camel = append(camel, fname)
		}
		if !ok || len(cols) == 0 {
			continue
		}
		if u, _ := ix["unique"].(bool); u {
			n, err := naming.UniqueName(t.Name, camel...)
			if err != nil {
				d.nameErr(err)
				continue
			}
			t.Uniques = append(t.Uniques, &Unique{Name: n, Columns: cols})
			d.claimRel(n, fmt.Sprintf("unique indexes[%d] %s", i, t.Name))
			continue
		}
		n, err := naming.IndexName(t.Name, camel...)
		if err != nil {
			d.nameErr(err)
			continue
		}
		t.Indexes = append(t.Indexes, &Index{Name: n, Columns: cols})
		d.claimRel(n, fmt.Sprintf("indeks indexes[%d] %s", i, t.Name))
	}
}

// morphTypeValue adalah nilai yang disimpan pada kolom <relasi>_type untuk satu tipe target: kunci resource
// bentuk manifest (sales.invoice_line), sama dengan kunci permission. Keputusan ini menunggu konfirmasi
// pemilik produk (spek hanya menyebut "kunci resource").
func morphTypeValue(key string) (string, error) { return naming.ManifestResourceKey(key) }

func fkAction(onDelete string) string {
	if onDelete == "" {
		return OnDeleteRestrict
	}
	return onDelete
}

func (d *deriver) deriveRelations(r *resolve.Resolved, t *Table) {
	for _, rel := range r.Relations {
		typ, _ := rel.Attrs["type"].(string)
		switch typ {
		case "belongsTo":
			d.belongsTo(r, t, rel)
		case "hasOne":
			d.hasOne(r, t, rel)
		case "belongsToMany":
			d.belongsToMany(r, t, rel)
		case "morphTo":
			d.morphTo(r, t, rel)
		case "hasMany", "morphMany":
			// Tanpa kolom pada sisi pemilik: FK ada di tabel anak (belongsTo anak) atau pada kolom morph.
		case "morphToMany":
			d.problem(CodeUnsupported, fmt.Sprintf("relasi %s: morphToMany belum didukung pembangkit database (nama dan pemilik tabel pivot bersama belum ditetapkan spek)", rel.Name))
		default:
			d.problem(CodeUnsupported, fmt.Sprintf("relasi %s: jenis %q tidak dikenal", rel.Name, typ))
		}
	}
}

func relBool(rel *resolve.Relation, key string) bool { b, _ := rel.Attrs[key].(bool); return b }

func (d *deriver) belongsTo(r *resolve.Resolved, t *Table, rel *resolve.Relation) {
	to, _ := rel.Attrs["to"].(string)
	cn, ok := snake(rel.Name + "Id")
	if !ok {
		d.problem(CodeName, fmt.Sprintf("relasi %s: nama kolom tidak sah", rel.Name))
		return
	}
	required := relBool(rel, "required")
	onDelete := fkAction(strOf(rel.Attrs["onDelete"]))
	if onDelete == OnDeleteSetNull && required {
		d.problem(CodeRelation, fmt.Sprintf("relasi %s: onDelete setNull tidak dapat dipakai pada relasi required", rel.Name))
		return
	}
	slug, _, _ := strings.Cut(to, ".")
	var target *Table
	if slug == t.Module {
		target = d.tables[to]
		if target == nil {
			d.problem(CodeRelation, fmt.Sprintf("relasi %s: resource tujuan %q tidak ada pada model (resource satu modul harus dideklarasikan)", rel.Name, to))
			return
		}
	}
	c := &Column{Name: cn, Type: TypeUUID, NotNull: required, Relation: rel.Name, FieldType: metadata.TypeUUID, camel: rel.Name + "Id"}
	if prev := t.Column(cn); prev != nil {
		d.problem(CodeCollision, fmt.Sprintf("kolom %q pada %s bentrok: dipakai oleh %s dan relasi %s", cn, t.Name, originOf(prev), rel.Name))
		return
	}
	t.Columns = append(t.Columns, c)
	t.index()
	if target == nil {
		return // lintas modul: hanya kolom ID (Requirement 6.2)
	}
	fkName, err := naming.ForeignKeyName(t.Name, rel.Name+"Id")
	if err != nil {
		d.nameErr(err)
		return
	}
	d.claimCon(t.Name, fkName, "foreign key "+cn)
	t.ForeignKeys = append(t.ForeignKeys, &ForeignKey{Name: fkName, Columns: []string{cn}, RefTable: target.Name, RefColumns: []string{"id"}, OnDelete: onDelete, Relation: rel.Name})
}

func originOf(c *Column) string {
	if c.Relation != "" {
		return "relasi " + c.Relation
	}
	return "field " + c.Field
}

func strOf(v any) string { s, _ := v.(string); return s }

func (d *deriver) hasOne(r *resolve.Resolved, t *Table, rel *resolve.Relation) {
	to, _ := rel.Attrs["to"].(string)
	slug, _, _ := strings.Cut(to, ".")
	if slug != t.Module {
		return // lintas modul: tanpa dampak pada tabel
	}
	target, tr := d.tables[to], d.byKey[to]
	if target == nil || tr == nil {
		d.problem(CodeRelation, fmt.Sprintf("relasi %s: resource tujuan %q tidak ada pada model", rel.Name, to))
		return
	}
	var inverse []string
	for _, tr2 := range tr.Relations {
		if strOf(tr2.Attrs["type"]) == "belongsTo" && strOf(tr2.Attrs["to"]) == r.Key {
			inverse = append(inverse, tr2.Name)
		}
	}
	if len(inverse) != 1 {
		d.problem(CodeRelation, fmt.Sprintf("relasi hasOne %s: resource %q harus mendeklarasikan tepat satu belongsTo balik ke %s (ditemukan %d); nama kolom FK tidak ditebak", rel.Name, to, r.Key, len(inverse)))
		return
	}
	cn, ok := snake(inverse[0] + "Id")
	if !ok || target.Column(cn) == nil {
		d.problem(CodeRelation, fmt.Sprintf("relasi hasOne %s: kolom FK %q tidak ada pada %s", rel.Name, cn, target.Name))
		return
	}
	for _, u := range target.Uniques {
		if len(u.Columns) == 1 && u.Columns[0] == cn {
			return
		}
	}
	n, err := naming.UniqueName(target.Name, inverse[0]+"Id")
	if err != nil {
		d.nameErr(err)
		return
	}
	d.claimRel(n, "unique hasOne "+rel.Name)
	target.Uniques = append(target.Uniques, &Unique{Name: n, Columns: []string{cn}})
}

func (d *deriver) morphTo(r *resolve.Resolved, t *Table, rel *resolve.Relation) {
	typeCol, ok1 := snake(rel.Name + "Type")
	idCol, ok2 := snake(rel.Name + "Id")
	if !ok1 || !ok2 {
		d.problem(CodeName, fmt.Sprintf("relasi %s: nama kolom tidak sah", rel.Name))
		return
	}
	required := relBool(rel, "required")
	for _, n := range []string{typeCol, idCol} {
		if prev := t.Column(n); prev != nil {
			d.problem(CodeCollision, fmt.Sprintf("kolom %q pada %s bentrok: dipakai oleh %s dan relasi %s", n, t.Name, originOf(prev), rel.Name))
			return
		}
	}
	var values []string
	raw, _ := rel.Attrs["types"].([]any)
	for _, x := range raw {
		key, _ := x.(string)
		v, err := morphTypeValue(key)
		if err != nil {
			d.nameErr(err)
			return
		}
		values = append(values, v)
	}
	t.Columns = append(t.Columns,
		&Column{Name: typeCol, Type: TypeText, NotNull: required, Relation: rel.Name, FieldType: metadata.TypeString, camel: rel.Name + "Type"},
		&Column{Name: idCol, Type: TypeUUID, NotNull: required, Relation: rel.Name, FieldType: metadata.TypeUUID, camel: rel.Name + "Id"})
	t.index()
	n, err := naming.CheckName(t.Name, rel.Name+"Type")
	if err != nil {
		d.nameErr(err)
		return
	}
	d.claimCon(t.Name, n, "CHECK "+typeCol)
	t.Checks = append(t.Checks, &Check{Name: n, Column: typeCol, Kind: CheckEnum, Values: values})
}

func (d *deriver) belongsToMany(r *resolve.Resolved, t *Table, rel *resolve.Relation) {
	to, _ := rel.Attrs["to"].(string)
	tslug, tname, err := naming.SplitResourceKey(to)
	if err != nil {
		d.nameErr(err)
		return
	}
	_, oname, _ := naming.SplitResourceKey(r.Key)
	ownerSnake, ok1 := snake(oname)
	relSnake, ok2 := snake(rel.Name)
	targetSnake, ok3 := snake(tname)
	if !ok1 || !ok2 || !ok3 {
		d.problem(CodeName, fmt.Sprintf("relasi %s: nama tabel pivot tidak sah", rel.Name))
		return
	}
	if ownerSnake == targetSnake {
		d.problem(CodeUnsupported, fmt.Sprintf("relasi %s: pivot antar resource yang sama belum didukung (nama kolom kedua sisi bentrok; spek belum menetapkan penamaan)", rel.Name))
		return
	}
	pivotName, err := naming.JoinName(strings.ReplaceAll(t.Module, "-", "_"), ownerSnake, relSnake)
	if err != nil {
		d.nameErr(err)
		return
	}
	leftCol, rightCol := ownerSnake+"_id", targetSnake+"_id"
	p := &Table{Name: pivotName, Resource: r.Key, Module: t.Module, Kind: KindPivot, Relation: rel.Name, PrimaryKey: []string{leftCol, rightCol}}
	d.claimRel(pivotName, "pivot "+rel.Name+" pada "+r.Key)
	d.claimRel(pivotName+"_pkey", "kunci utama "+pivotName)
	p.Columns = append(p.Columns,
		&Column{Name: leftCol, Type: TypeUUID, NotNull: true, Relation: rel.Name, FieldType: metadata.TypeUUID, camel: oname + "Id"},
		&Column{Name: rightCol, Type: TypeUUID, NotNull: true, Relation: rel.Name, FieldType: metadata.TypeUUID, camel: tname + "Id"})
	p.index()

	// Atribut pivot (opsional), terurut menurut nama untuk keluaran deterministik.
	pf, _ := rel.Attrs["pivotFields"].(map[string]any)
	names := make([]string, 0, len(pf))
	for n := range pf {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		attrs, _ := pf[n].(map[string]any)
		fld := &resolve.Field{Name: n, Attrs: attrs, Origin: rel.Origin}
		c := d.fieldColumn(pivotName, fld, false, nil)
		if c == nil {
			continue
		}
		if p.Column(c.Name) != nil {
			d.problem(CodeCollision, fmt.Sprintf("kolom pivot %q pada %s bentrok", c.Name, pivotName))
			continue
		}
		p.Columns = append(p.Columns, c)
		p.index()
		if ck := d.fieldCheck(pivotName, fld, c); ck != nil {
			p.Checks = append(p.Checks, ck)
			d.claimCon(pivotName, ck.Name, "CHECK "+c.Name)
		}
	}

	ownerFK, err := naming.ForeignKeyName(pivotName, oname+"Id")
	if err != nil {
		d.nameErr(err)
		return
	}
	d.claimCon(pivotName, ownerFK, "foreign key "+leftCol)
	p.ForeignKeys = append(p.ForeignKeys, &ForeignKey{Name: ownerFK, Columns: []string{leftCol}, RefTable: t.Name, RefColumns: []string{"id"}, OnDelete: OnDeleteCascade})
	if tslug == t.Module {
		target := d.tables[to]
		if target == nil {
			d.problem(CodeRelation, fmt.Sprintf("relasi %s: resource tujuan %q tidak ada pada model", rel.Name, to))
			return
		}
		fkn, err := naming.ForeignKeyName(pivotName, tname+"Id")
		if err != nil {
			d.nameErr(err)
			return
		}
		d.claimCon(pivotName, fkn, "foreign key "+rightCol)
		p.ForeignKeys = append(p.ForeignKeys, &ForeignKey{Name: fkn, Columns: []string{rightCol}, RefTable: target.Name, RefColumns: []string{"id"}, OnDelete: OnDeleteCascade})
	}
	d.schema.Tables = append(d.schema.Tables, p)
}
