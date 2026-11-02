package check

import (
	"errors"
	"sort"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
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

// checkDerivedNames menurunkan model database (dbmodel.Derive) dan melaporkan temuan penamaannya sebagai galat
// metadata:check: nama tabel, kolom, constraint, serta nama pemicu dan fungsi bangkitan (pemicu penguncian,
// penjaga TRUNCATE, pemicu anak, dan pivot; mis. <tabel>_truncate_lock_trg) yang melampaui 63 byte atau
// bertabrakan (Requirement 1.7, 1.8), agar tidak baru gagal di metadata:diff dan metadata:migrate. Hanya
// dijalankan bila pemeriksaan lain bersih karena model dari resource yang gagal diselesaikan tidak berarti.
func (c *checker) checkDerivedNames() {
	for _, p := range c.probs {
		if p.Severity == SeverityError {
			return
		}
	}
	_, ps := dbmodel.Derive(c.res.Resources)
	for _, p := range ps {
		if p.Code == dbmodel.CodeName || p.Code == dbmodel.CodeCollision {
			c.add(fromLoad(p, "", ""))
		}
	}
}

// checkGeneratedGoNames memeriksa identifier Go bangkitan seluruh resource tiap modul dengan aturan yang sama
// dengan metadata:generate (naming.CheckGoPackage): field tulis bernama validate, present, atau fillRenamePairs
// (anggota struct DTO), resource yang menghasilkan <Pascal>Spec yang sama dengan tipe runtime (field, rule, status,
// resource), dan bentrok identifier tingkat paket antar resource. Dilaporkan dengan kode name_reserved_go.
func (c *checker) checkGeneratedGoNames() {
	byModule := map[string][]*resolve.Resolved{}
	for _, r := range c.res.Resources {
		if r.Module == resolve.SDKSlug || r.Kind != "resource" {
			continue
		}
		byModule[r.Module] = append(byModule[r.Module], r)
	}
	for _, mod := range sortedKeys(byModule) {
		var in []naming.GoResource
		byKey := map[string]*resolve.Resolved{}
		for _, r := range byModule[mod] {
			_, name, err := naming.SplitResourceKey(r.Key)
			if err != nil {
				continue
			}
			byKey[r.Key] = r
			gr := naming.GoResource{Key: r.Key, Name: name}
			for _, f := range r.Fields {
				gf := naming.GoField{Name: f.Name, Writable: fieldWritable(f)}
				if vals, ok := f.Attrs["values"].([]any); ok {
					for _, v := range vals {
						if s, ok := v.(string); ok {
							gf.Enum = append(gf.Enum, s)
						}
					}
				}
				gr.Fields = append(gr.Fields, gf)
			}
			for _, rel := range r.Relations {
				typ, _ := rel.Attrs["type"].(string)
				for _, col := range relationColumns(rel.Name, typ) {
					gr.Fields = append(gr.Fields, naming.GoField{Name: col, Writable: true})
				}
			}
			in = append(in, gr)
		}
		for _, cf := range naming.CheckGoPackage(in) {
			r := byKey[cf.Resource]
			ptr := "/key"
			if cf.Field != "" {
				ptr = "/fields/" + cf.Field
			}
			file, line, col := r.Source.File, 1, 1
			if doc := c.cat.Doc(r.Key); doc != nil {
				p := doc.Pos(ptr)
				file, line, col = doc.File, p.Line, p.Column
			}
			c.add(Problem{Severity: SeverityError, File: file, Line: line, Column: col, Pointer: ptr, Code: nameCode(cf.Err.Code),
				Message: cf.Err.Error(), Module: r.Module, Resource: r.Key})
		}
	}
}

// fieldWritable melaporkan apakah field menjadi masukan tulis pada DTO bangkitan: bukan readOnly dan bukan terhitung
// (accessor). Aturan sama dengan generate (fld.Writable); hanya field tulis yang dapat bentrok dengan anggota DTO.
func fieldWritable(f *resolve.Field) bool {
	if ro, _ := f.Attrs["readOnly"].(bool); ro {
		return false
	}
	_, computed := f.Attrs["accessor"].(map[string]any)
	return !computed
}
