package check

import (
	"errors"
	"sort"

	"erpora/internal/devtools/metadatatools/naming"
)

func nameCode(c naming.Code) string { return "name_" + string(c) }

// reportName menerbitkan galat penamaan; emit menentukan lokasi.
func reportName(err error, emit func(code, msg string)) {
	var ne *naming.Error
	if errors.As(err, &ne) {
		emit(nameCode(ne.Code), ne.Error())
		return
	}
	emit(CodeNameInvalid, err.Error())
}

func relationColumns(name string, typ string) []string {
	switch typ {
	case "belongsTo":
		c, err := naming.Prefixed(name, "id")
		if err != nil {
			return nil
		}
		return []string{c}
	case "morphTo":
		var out []string
		for _, suf := range []string{"type", "id"} {
			if c, err := naming.Prefixed(name, suf); err == nil {
				out = append(out, c)
			}
		}
		return out
	}
	return nil
}

// checkNames memeriksa aturan penamaan (Requirement 1.4, 1.7, 1.8): bentuk, kata tercadang, bolak-balik,
// tabrakan setelah konversi, dan nama tabel serta turunannya.
func (x *rctx) checkNames() {
	for _, f := range x.r.Fields {
		for _, v := range naming.Validate(f.Name) {
			x.err(f, "", nameCode(v.Code), "%s", v.Error())
		}
	}
	// tabrakan antar field setelah konversi ke snake_case dan PascalCase
	for _, conv := range []struct {
		target string
		fn     func(string) (string, error)
	}{{"snake_case (SQL)", naming.ToSnake}, {"PascalCase (Go)", naming.ToPascal}} {
		for _, col := range naming.FindCollisions(x.fieldNames(), conv.fn) {
			second := x.fields[col.Sources[len(col.Sources)-1]]
			if second != nil {
				x.err(second, "", CodeNameCollision, "field %s bertabrakan setelah konversi ke %s: %s menjadi %q", joinNames(col.Sources), conv.target, joinNames(col.Sources), col.Target)
			}
		}
	}
	if x.isAbs {
		return
	}
	// kolom relasi (<nama>Id, <nama>Type) tidak boleh bertabrakan dengan field
	fieldSnake := map[string]string{}
	for _, f := range x.r.Fields {
		if s, err := naming.ToSnake(f.Name); err == nil {
			fieldSnake[s] = f.Name
		}
	}
	for _, rel := range x.r.Relations {
		ptr := "/relations/" + rel.Name
		for _, v := range naming.Validate(rel.Name) {
			x.atDoc(SeverityError, ptr, nameCode(v.Code), "%s", v.Error())
		}
		typ, _ := rel.Attrs["type"].(string)
		for _, col := range relationColumns(rel.Name, typ) {
			s, err := naming.ToSnake(col)
			if err != nil {
				continue
			}
			if other, clash := fieldSnake[s]; clash {
				x.atDoc(SeverityError, ptr, CodeRelationColumnCollision,
					"relasi %s (%s) menghasilkan kolom %q yang bertabrakan dengan field %s", rel.Name, typ, s, other)
			}
		}
	}
	for _, rule := range x.r.Rules {
		for _, v := range naming.Validate(rule.ID) {
			x.atOrigin(SeverityError, rule.Origin, "/rules/"+rule.ID, nameCode(v.Code), "id aturan: %s", v.Error())
		}
	}
	// nama tabel resource (<slug>_<nama jamak>) unik dan <= 63 byte
	slug, name, err := naming.SplitResourceKey(x.r.Key)
	if err != nil {
		return
	}
	table, err := naming.TableName(slug, name)
	if err != nil {
		reportName(err, func(code, msg string) { x.atDoc(SeverityError, "/key", code, "%s", msg) })
		return
	}
	if err := x.c.tables.Claim(table, "resource "+x.r.Key); err != nil {
		reportName(err, func(code, msg string) { x.atDoc(SeverityError, "/key", code, "nama tabel %s: %s", table, msg) })
	}
}

// tableName mengembalikan nama tabel resource (kosong bila tidak dapat dihitung).
func (x *rctx) tableName() string {
	slug, name, err := naming.SplitResourceKey(x.r.Key)
	if err != nil {
		return ""
	}
	t, _ := naming.TableName(slug, name)
	return t
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
