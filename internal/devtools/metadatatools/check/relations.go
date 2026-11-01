package check

import (
	"fmt"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

// relTargets menyebut kunci resource sasaran sebuah relasi beserta pointer-nya.
func relTargets(rel *resolve.Relation) []struct{ ptr, key string } {
	var out []struct{ ptr, key string }
	base := "/relations/" + rel.Name
	typ, _ := rel.Attrs["type"].(string)
	switch typ {
	case "belongsTo", "hasOne", "belongsToMany", "morphToMany":
		if k, _ := rel.Attrs["to"].(string); k != "" {
			out = append(out, struct{ ptr, key string }{base + "/to", k})
		}
	case "hasMany", "morphMany":
		if k, _ := rel.Attrs["of"].(string); k != "" {
			out = append(out, struct{ ptr, key string }{base + "/of", k})
		}
	case "morphTo":
		if list, ok := rel.Attrs["types"].([]any); ok {
			for i, v := range list {
				if k, _ := v.(string); k != "" {
					out = append(out, struct{ ptr, key string }{fmt.Sprintf("%s/types/%d", base, i), k})
				}
			}
		}
	}
	return out
}

func relType(rel *resolve.Relation) string { t, _ := rel.Attrs["type"].(string); return t }

// resolveTarget memeriksa keberadaan resource sasaran dan hak merujuknya (modul sendiri, atau modul lain hanya
// lewat dependensi manifest; Requirement 6.2, 6.3). Mengembalikan target hasil resolusi (nil bila tidak
// tersedia) dan apakah sasaran sah.
func (x *rctx) resolveTarget(ptr, key string) (*resolve.Resolved, bool) {
	slug, _, err := naming.SplitResourceKey(key)
	if err != nil {
		x.atDoc(SeverityError, ptr, CodeRelationTargetMissing, "kunci resource sasaran %q tidak sah", key)
		return nil, false
	}
	if slug != x.r.Module {
		if x.c.mods[slug] == nil {
			x.atDoc(SeverityError, ptr, CodeRelationTargetMissing, "resource sasaran %s tidak ada: modul %q tidak dikenal", key, slug)
			return nil, false
		}
		if !x.mod.deps[slug] {
			x.atDoc(SeverityError, ptr, CodeRelationModuleDependency,
				"relasi lintas modul ke %s ditolak: modul %q tidak tercantum di dependencies manifest %s (D1, D8; Requirement 6.2)", key, slug, x.mod.slug)
			return nil, false
		}
	}
	doc := x.c.cat.Doc(key)
	if doc == nil || doc.Kind != "resource" {
		x.atDoc(SeverityError, ptr, CodeRelationTargetMissing, "resource sasaran %s tidak ada (relasi hanya ke metadata kind resource)", key)
		return nil, false
	}
	return x.c.res.Resource(key), true
}

// checkRelations memeriksa sasaran, embedded, filter, siklus filter, dan indeks FK relasi (Requirement 6).
func (x *rctx) checkRelations() {
	own := x.r.Module
	relOrder := make([]string, 0, len(x.r.Relations))
	relSet := map[string]bool{}
	for _, rel := range x.r.Relations {
		relOrder = append(relOrder, rel.Name)
		relSet[rel.Name] = true
	}
	graph := map[string][]string{}
	for _, rel := range x.r.Relations {
		base := "/relations/" + rel.Name
		typ := relType(rel)
		var main *resolve.Resolved
		mainOK := false
		for i, t := range relTargets(rel) {
			target, ok := x.resolveTarget(t.ptr, t.key)
			if i == 0 {
				main, mainOK = target, ok
			}
			if emb, _ := rel.Attrs["embedded"].(bool); emb && ok {
				if slug, _, _ := naming.SplitResourceKey(t.key); slug != own {
					x.atDoc(SeverityError, base, CodeRelationEmbeddedModule,
						"relasi embedded %s harus ke resource satu modul: baris anak tidak boleh berada di modul %q (tanpa FK lintas modul)", rel.Name, slug)
				}
			}
		}
		if raw, ok := rel.Attrs["filter"]; ok {
			x.checkRelationFilter(rel, raw, main, mainOK, relSet, graph)
		}
		if typ == "belongsTo" && mainOK {
			if slug, _, _ := naming.SplitResourceKey(stringAttr(rel.Attrs, "to")); slug == own && !x.fkIndexed(rel.Name) {
				x.atDoc(SeverityWarning, base, CodeRelationFKNoIndex,
					"foreign key relasi %s (kolom %sId) tidak punya indeks: deklarasikan indeks yang diawali %sId di indexes (indeks tidak diturunkan otomatis; Requirement 2.3)", rel.Name, rel.Name, rel.Name)
			}
		}
	}
	for _, comp := range sccs(relOrder, graph) {
		x.atDoc(SeverityError, "/relations/"+comp[0], CodeRelationFilterCycle,
			"rujukan filter relasi melingkar: %s (Requirement 6.7)", joinArrow(comp))
	}
}

func joinArrow(comp []string) string {
	s := ""
	for _, c := range comp {
		s += c + " -> "
	}
	return s + comp[0]
}

func stringAttr(m map[string]any, k string) string { s, _ := m[k].(string); return s }

// fkIndexed menyatakan ada indeks yang diawali kolom FK <relasi>Id.
func (x *rctx) fkIndexed(rel string) bool {
	col, err := naming.Prefixed(rel, "id")
	if err != nil {
		return true
	}
	for _, idx := range x.r.Indexes {
		if fs, ok := idx["fields"].([]any); ok && len(fs) > 0 {
			if s, _ := fs[0].(string); s == col {
				return true
			}
		}
	}
	return false
}

func (x *rctx) checkRelationFilter(rel *resolve.Relation, raw any, target *resolve.Resolved, targetOK bool, relSet map[string]bool, graph map[string][]string) {
	ptr := "/relations/" + rel.Name + "/filter"
	cond, err := metadata.ParseCondition(raw)
	if err != nil {
		x.atDoc(SeverityError, ptr, CodeConditionInvalid, "filter relasi %s tidak sah: %s", rel.Name, reasonText(err))
		return
	}
	env := &condEnv{
		x: x,
		fieldOf: func(n string) *resolve.Field {
			if target == nil {
				return nil
			}
			return target.Field(n)
		},
		refOf: func(n string) (metadata.FieldType, bool) {
			if f := x.fields[n]; f != nil {
				return metadata.FieldType(f.Type()), true
			}
			if relSet[n] {
				return metadata.TypeUUID, true
			}
			return "", false
		},
		permissive:        target == nil, // sasaran tak dapat diperiksa (hilang atau gagal diselesaikan)
		codeUnknownField:  CodeRelationFilterUnknownField,
		codeUnknownRef:    CodeRelationFilterUnknownRef,
		requireFilterable: true,
		loc:               func(sev Severity, code, msg string) { x.atDoc(sev, ptr, code, "%s", msg) },
	}
	_ = targetOK
	env.check(cond, "$")
	var refs []string
	collectLeafRefs(cond, &refs, map[string]bool{})
	for _, n := range refs {
		if x.fields[n] == nil && relSet[n] {
			graph[rel.Name] = append(graph[rel.Name], n)
		}
	}
}

// collectLeafRefs mengumpulkan hanya nilai ref (rujukan ke resource sendiri) pada kondisi.
func collectLeafRefs(c *metadata.Condition, into *[]string, seen map[string]bool) {
	if c == nil {
		return
	}
	if c.Kind == metadata.CondLeaf {
		if c.Ref != "" && !seen[c.Ref] {
			seen[c.Ref] = true
			*into = append(*into, c.Ref)
		}
		return
	}
	for _, ch := range c.Children {
		collectLeafRefs(ch, into, seen)
	}
}

// physicalColumns adalah nama kolom yang dapat diindeks: field fisik (bukan accessor expr atau func) dan kolom
// relasi (<relasi>Id untuk belongsTo; <relasi>Type dan <relasi>Id untuk morphTo).
func (x *rctx) physicalColumns() map[string]bool {
	cols := map[string]bool{}
	for _, f := range x.r.Fields {
		if acc, ok := f.Attrs["accessor"].(map[string]any); ok {
			if k, _ := acc["kind"].(string); k != "stored" {
				continue
			}
		}
		cols[f.Name] = true
	}
	for _, rel := range x.r.Relations {
		for _, c := range relationColumns(rel.Name, relType(rel)) {
			cols[c] = true
		}
	}
	return cols
}

// checkIndexes memeriksa indeks yang dideklarasikan dan panjang nama indeks, unique, dan foreign key.
func (x *rctx) checkIndexes() {
	table := x.tableName()
	cols := x.physicalColumns()
	claim := func(name, origin, ptr string) {
		if err := x.c.tables.Claim(name, origin); err != nil {
			reportName(err, func(code, msg string) { x.atDoc(SeverityError, ptr, code, "%s", msg) })
		}
	}
	for i, idx := range x.r.Indexes {
		ptr := fmt.Sprintf("/indexes/%d", i)
		fs, _ := idx["fields"].([]any)
		var names []string
		for j, v := range fs {
			n, _ := v.(string)
			names = append(names, n)
			if !cols[n] {
				x.atDoc(SeverityError, fmt.Sprintf("%s/fields/%d", ptr, j), CodeIndexUnknownField,
					"indeks merujuk %q yang bukan kolom fisik resource (field, accessor stored, atau kolom relasi <relasi>Id)", n)
			}
		}
		if table == "" {
			continue
		}
		fn := naming.IndexName
		if u, _ := idx["unique"].(bool); u {
			fn = naming.UniqueName
		}
		if name, err := fn(table, names...); err != nil {
			reportName(err, func(code, msg string) { x.atDoc(SeverityError, ptr, code, "nama indeks: %s", msg) })
		} else {
			claim(name, "indeks "+ptr+" pada "+x.r.Key, ptr)
		}
	}
	if table == "" {
		return
	}
	for _, f := range x.r.Fields {
		if f.Bool("index") {
			if name, err := naming.IndexName(table, f.Name); err != nil {
				reportName(err, func(code, msg string) { x.err(f, "index", code, "nama indeks: %s", msg) })
			} else {
				claim(name, "index field "+f.Name+" pada "+x.r.Key, "/fields/"+f.Name+"/index")
			}
		}
		if f.Bool("unique") {
			if name, err := naming.UniqueName(table, f.Name); err != nil {
				reportName(err, func(code, msg string) { x.err(f, "unique", code, "nama unique: %s", msg) })
			} else {
				claim(name, "unique field "+f.Name+" pada "+x.r.Key, "/fields/"+f.Name+"/unique")
			}
		}
	}
	for _, rel := range x.r.Relations {
		if relType(rel) != "belongsTo" {
			continue
		}
		col, err := naming.Prefixed(rel.Name, "id")
		if err != nil {
			continue
		}
		if _, err := naming.ForeignKeyName(table, col); err != nil {
			reportName(err, func(code, msg string) {
				x.atDoc(SeverityError, "/relations/"+rel.Name, code, "nama foreign key: %s", msg)
			})
		}
	}
}
