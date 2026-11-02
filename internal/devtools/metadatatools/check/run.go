package check

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

type modCtx struct {
	in   Module
	slug string
	deps map[string]bool // slug modul dependensi (hanya yang dikenal di Input)
}

type checker struct {
	cat   *resolve.Catalog
	res   *resolve.Result
	rep   *Report
	mods  map[string]*modCtx
	probs []Problem
	// pendingMF adalah migrateFrom yang merujuk field yang tidak ada di metadata saat ini; diputuskan setelah baseline
	// rilis dibaca (resolvePendingMigrateFrom). baselines menyimpan hasil resolusi metadata pada tag rilis terakhir
	// per slug modul (tidak ada bila modul tanpa tag, tanpa git, atau baseline tidak terbaca).
	pendingMF []pendingMigrateFrom
	baselines map[string]*resolve.Result
	eff       map[string]*effInfo
	tables    *naming.Namespace
}

// Run menjalankan semua pemeriksaan semantik pada Input dan mengembalikan laporan. Run tidak menulis apa pun.
func Run(in Input) *Report {
	rep := &Report{Resources: map[string]*ResourceInfo{}, ModuleDirs: map[string]string{}}
	c := &checker{rep: rep, mods: map[string]*modCtx{}, eff: map[string]*effInfo{}, tables: naming.NewNamespace("skema database"), baselines: map[string]*resolve.Result{}}
	cat := resolve.NewCatalog()
	c.cat = cat
	for _, p := range cat.AddBuiltin() {
		c.add(Problem{Severity: SeverityError, File: p.File, Line: p.Line, Column: p.Column, Pointer: p.Pointer, Code: CodeBuiltinInvalid, Message: "metadata bawaan SDK tidak valid: " + p.Message})
	}

	in = c.checkModuleIdentities(in)
	addModules(cat, in, func(m Module) []*load.Document { return m.Docs }, func(m Module, mc *modCtx, ps []load.Problem) {
		c.mods[m.Manifest.Slug] = mc
		for _, p := range ps {
			c.add(fromLoad(p, m.Manifest.Slug, ""))
		}
	})
	c.res = cat.ResolveAll()
	rep.Resolved = c.res
	for _, p := range c.res.Problems {
		c.add(fromLoad(p, "", ""))
	}

	all := append(append([]*resolve.Resolved(nil), c.res.Resources...), c.res.Abstracts...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	for _, r := range all {
		if r.Module == resolve.SDKSlug {
			continue
		}
		mc := c.mods[r.Module]
		if mc == nil {
			continue
		}
		c.checkResolved(r, mc)
	}
	c.checkMigrationTodos(in.Modules)
	c.checkReleaseBaselines(in)
	c.resolvePendingMigrateFrom()
	rep.Problems = sortProblems(c.probs)
	return rep
}

// moduleDefaultGroup mengembalikan kunci grup default bila semua resource manifest sepakat (dipakai resolver
// untuk pemeriksaan pelonggaran grup); kosong berarti grup default implisit.
func moduleDefaultGroup(m Manifest) string {
	if !m.HasResources {
		return ""
	}
	def := ""
	for _, r := range m.Resources {
		for _, g := range r.FieldGroups {
			if g.Default {
				if def != "" && def != g.Key {
					return ""
				}
				def = g.Key
			}
		}
	}
	return def
}

func (c *checker) add(p Problem) { c.probs = append(c.probs, p) }

// groupDefs menjelaskan grup field sebuah resource (kunci snake_case manifest).
type groupDefs struct {
	def      string            // kunci grup default
	known    map[string]string // kunci grup -> klasifikasi ("" bila tidak dinyatakan)
	implicit bool              // true bila tidak ada definisi grup: hanya default implisit
}

func (g groupDefs) isDefault(key string) bool { return key == g.def }

// rank mengukur keketatan grup: default = 0; selainnya 1 + peringkat klasifikasi grup (bawaan internal).
func (g groupDefs) rank(key string) int {
	if g.isDefault(key) {
		return 0
	}
	cl := g.known[key]
	if cl == "" {
		cl = "internal"
	}
	return 1 + classRank(cl)
}

func classRank(c string) int {
	switch c {
	case "public":
		return 0
	case "internal":
		return 1
	case "confidential":
		return 2
	case "highly_confidential":
		return 3
	}
	return 1
}

var classNames = []string{"public", "internal", "confidential", "highly_confidential"}

func maxClass(a, b string) string {
	if classRank(b) > classRank(a) {
		return b
	}
	return a
}

func snake(s string) string {
	if v, err := naming.ToSnake(s); err == nil {
		return v
	}
	return s
}

func camel(s string) string {
	if v, err := naming.FromSnake(s); err == nil {
		return v
	}
	return s
}

// manifestResource mencari entri resources[] untuk resource metadata (kunci multi-kata dikonversi ke snake_case).
func (mc *modCtx) manifestResource(key string) *ManifestResource {
	mk, err := naming.ManifestResourceKey(key)
	if err != nil {
		return nil
	}
	for i := range mc.in.Manifest.Resources {
		if mc.in.Manifest.Resources[i].Key == mk {
			return &mc.in.Manifest.Resources[i]
		}
	}
	return nil
}

func groupsOf(mc *modCtx, key string) groupDefs {
	g := groupDefs{def: resolve.DefaultGroupName, known: map[string]string{}, implicit: true}
	if !mc.in.Manifest.HasResources {
		return g
	}
	mres := mc.manifestResource(key)
	if mres == nil || len(mres.FieldGroups) == 0 {
		return g
	}
	g.implicit = false
	for _, fg := range mres.FieldGroups {
		g.known[fg.Key] = fg.Classification
		if fg.Default && g.def == resolve.DefaultGroupName {
			g.def = fg.Key
		}
	}
	return g
}

// effInfo adalah klasifikasi dan grup efektif tiap field (accessor mewarisi yang tertinggi/paling ketat).
type effInfo struct {
	class  map[string]string
	group  map[string]string
	groups groupDefs
}

// effective menghitung klasifikasi dan grup efektif; hasilnya di-cache per resource. Siklus accessor tidak
// menyebabkan rekursi tak hingga (dilaporkan terpisah).
func (c *checker) effective(r *resolve.Resolved) *effInfo {
	if e, ok := c.eff[r.Key]; ok {
		return e
	}
	mc := c.mods[r.Module]
	e := &effInfo{class: map[string]string{}, group: map[string]string{}}
	if mc != nil {
		e.groups = groupsOf(mc, r.Key)
	} else {
		e.groups = groupDefs{def: resolve.DefaultGroupName, known: map[string]string{}, implicit: true}
	}
	c.eff[r.Key] = e
	visiting := map[string]bool{}
	var calc func(f *resolve.Field)
	calc = func(f *resolve.Field) {
		if _, done := e.class[f.Name]; done {
			return
		}
		if visiting[f.Name] {
			return
		}
		visiting[f.Name] = true
		cl := f.Classification()
		gr := snake(f.Str("fieldGroup"))
		if gr == "" {
			gr = e.groups.def
		}
		if acc, ok := f.Attrs["accessor"].(map[string]any); ok {
			if kind, _ := acc["kind"].(string); kind == "expr" || kind == "stored" {
				declaredGroup := f.Str("fieldGroup") != ""
				best := ""
				for _, in := range accessorInputs(acc) {
					dep := r.Field(in)
					if dep == nil {
						continue
					}
					calc(dep)
					cl = maxClass(cl, e.class[in])
					if g, ok := e.group[in]; ok && (best == "" || e.groups.rank(g) > e.groups.rank(best)) {
						best = g
					}
				}
				if !declaredGroup && best != "" {
					gr = best
				} else if declaredGroup && best != "" && e.groups.rank(best) > e.groups.rank(gr) {
					gr = best
				}
			}
		}
		delete(visiting, f.Name)
		e.class[f.Name] = cl
		e.group[f.Name] = gr
	}
	for _, f := range r.Fields {
		calc(f)
	}
	return e
}

// accessorInputs mengembalikan nama field masukan accessor expr/stored (terurut kemunculan, tanpa duplikat):
// field pada ekspresi serta field dan ref pada kondisi di dalam if (Requirement 5.8). Ekspresi yang tidak
// dapat di-parse dilaporkan terpisah (accessor_expr_invalid) dan tidak memberi masukan.
func accessorInputs(acc map[string]any) []string {
	expr, err := metadata.ParseExpr(acc["expr"])
	if err != nil {
		return nil
	}
	var out []string
	exprRefs(expr, &out, map[string]bool{})
	return out
}

// rctx adalah konteks pemeriksaan satu resource atau abstrak hasil resolusi.
type rctx struct {
	c      *checker
	r      *resolve.Resolved
	doc    *load.Document
	mod    *modCtx
	mres   *ManifestResource
	eff    *effInfo
	info   *ResourceInfo
	kinds  map[string]metadata.FieldType
	isDoc  bool
	isAbs  bool
	fields map[string]*resolve.Field
	// pivotRel dan pivotPtr (opsional) mengalihkan lokasi temuan field ke kolom pivot sebuah relasi
	// (/relations/<r>/pivotFields/<n>) saat checkField dipakai untuk pivotFields.
	pivotRel *resolve.Relation
	pivotPtr string
}

func (c *checker) checkResolved(r *resolve.Resolved, mc *modCtx) {
	x := &rctx{c: c, r: r, doc: c.cat.Doc(r.Key), mod: mc, isAbs: r.Kind != "resource", kinds: map[string]metadata.FieldType{}, fields: map[string]*resolve.Field{}}
	x.mres = mc.manifestResource(r.Key)
	x.eff = c.effective(r)
	for _, f := range r.Fields {
		x.fields[f.Name] = f
		x.kinds[f.Name] = metadata.FieldType(f.Type())
	}
	for _, u := range r.Uses {
		if u.Key == resolve.KeyDocument {
			x.isDoc = true
		}
	}
	if !x.isAbs {
		x.info = &ResourceInfo{Key: r.Key, Module: r.Module, Document: x.isDoc, RuleScope: map[string]string{}, ConditionScope: map[string]string{},
			Class: x.eff.class, Group: x.eff.group, Dir: mc.in.Dir}
		c.rep.Resources[r.Key] = x.info
	}

	x.checkNames()
	for _, f := range r.Fields {
		x.checkField(f)
	}
	x.checkGroupLoosening()
	x.checkCrossModuleRequires()
	x.checkAccessors()
	x.checkFieldConditions()
	if !x.isAbs {
		x.checkRules()
		x.checkLockWhen()
		x.checkLockWhenSQL()
		x.checkLabelTemplate()
		x.checkRelations()
		x.checkIndexes()
		x.checkManifest()
		x.checkOwnerScope()
		x.checkSnapshots()
		x.checkStatusAxes()
		x.checkUnmanaged()
	}
	x.checkMigrateFrom()
}

// ---- penerbitan temuan ----

func (x *rctx) emit(sev Severity, file string, line, col int, ptr, code, format string, args ...any) {
	x.c.add(Problem{Severity: sev, File: file, Line: line, Column: col, Pointer: ptr, Code: code, Message: fmt.Sprintf(format, args...),
		Module: x.r.Module, Resource: x.r.Key})
}

func (x *rctx) atOrigin(sev Severity, o resolve.Origin, ptr, code, format string, args ...any) {
	x.emit(sev, o.File, o.Line, o.Column, ptr, code, format, args...)
}

// atDoc menunjuk posisi pointer pada dokumen resource itu sendiri.
func (x *rctx) atDoc(sev Severity, ptr, code, format string, args ...any) {
	file, line, col := "", 0, 0
	if x.doc != nil {
		p := x.doc.Pos(ptr)
		file, line, col = x.doc.File, p.Line, p.Column
	}
	x.emit(sev, file, line, col, ptr, code, format, args...)
}

// atField menunjuk atribut field (asal nilai terakhir), atau field itu bila atribut kosong/tidak ada.
func (x *rctx) atField(sev Severity, f *resolve.Field, attr, code, format string, args ...any) {
	if x.pivotRel != nil {
		ptr := x.pivotPtr
		if attr != "" {
			ptr += "/" + attr
		}
		x.atRelation(sev, x.pivotRel, ptr, code, format, args...)
		return
	}
	ptr := "/fields/" + f.Name
	o := f.Origin
	if attr != "" {
		ptr += "/" + attr
		if ao, ok := f.AttrOrigin[attr]; ok {
			o = ao
		}
	}
	x.atOrigin(sev, o, ptr, code, format, args...)
}

func (x *rctx) err(f *resolve.Field, attr, code, format string, args ...any) {
	x.atField(SeverityError, f, attr, code, format, args...)
}

func (x *rctx) fieldNames() []string {
	out := make([]string, len(x.r.Fields))
	for i, f := range x.r.Fields {
		out[i] = f.Name
	}
	return out
}

func containsStr(list []string, s string) bool { return slices.Contains(list, s) }

func joinNames(ns []string) string { return strings.Join(ns, ", ") }

// addModules mendaftarkan semua modul Input ke katalog; docsOf memilih dokumen yang didaftarkan per modul
// (dokumen saat ini, atau baseline rilis untuk satu modul). onAdded (opsional) menerima konteks modul dan temuan.
func addModules(cat *resolve.Catalog, in Input, docsOf func(Module) []*load.Document, onAdded func(Module, *modCtx, []load.Problem)) {
	keyToSlug := map[string]string{}
	for _, m := range in.Modules {
		keyToSlug[m.Manifest.Key] = m.Manifest.Slug
	}
	for _, m := range in.Modules {
		mc := &modCtx{in: m, slug: m.Manifest.Slug, deps: map[string]bool{}}
		var depSlugs []string
		for _, k := range m.Manifest.Dependencies {
			if s, ok := keyToSlug[k]; ok {
				mc.deps[s] = true
				depSlugs = append(depSlugs, s)
			}
		}
		rm := resolve.Module{Slug: m.Manifest.Slug, Dependencies: depSlugs, DefaultGroup: moduleDefaultGroup(m.Manifest)}
		ps := cat.AddModule(rm, docsOf(m)...)
		if onAdded != nil {
			onAdded(m, mc, ps)
		}
	}
}
