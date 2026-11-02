package resolve

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/naming"
)

// Result adalah hasil menyelesaikan semua dokumen sebuah katalog. Resources memuat hanya kind resource (satu-
// satunya yang menghasilkan tabel, kontrak, dan kode); Abstracts untuk pelaporan. Dokumen yang gagal tidak
// disertakan; sebabnya ada di Problems.
type Result struct {
	Resources []*Resolved
	Abstracts []*Resolved
	Problems  []load.Problem
}

// Resource mengembalikan resource hasil resolusi menurut kunci, nil bila tidak ada atau bukan resource.
func (r *Result) Resource(key string) *Resolved {
	for _, x := range r.Resources {
		if x.Key == key {
			return x
		}
	}
	return nil
}

type resolver struct {
	c     *Catalog
	cache map[string]*Resolved // nilai nil = gagal
	stack []string
	probs []load.Problem
	seen  map[string]bool
}

func newResolver(c *Catalog) *resolver {
	return &resolver{c: c, cache: map[string]*Resolved{}, seen: map[string]bool{}}
}

func (r *resolver) report(p load.Problem) {
	k := fmt.Sprintf("%s|%d|%d|%s|%s|%s", p.File, p.Line, p.Column, p.Pointer, p.Code, p.Message)
	if r.seen[k] {
		return
	}
	r.seen[k] = true
	r.probs = append(r.probs, p)
}

func sortedProblems(ps []load.Problem) []load.Problem {
	out := append([]load.Problem(nil), ps...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return a.Message < b.Message
	})
	return out
}

// Resolve menyelesaikan satu dokumen resource atau abstrak (beserta dokumen yang disertakannya). Hasil nil
// bila ada temuan; semua temuan dikembalikan (dokumen yang disertakan ikut diperiksa).
func (c *Catalog) Resolve(key string) (*Resolved, []load.Problem) {
	e := c.docs[key]
	if e == nil {
		return nil, []load.Problem{{Code: CodeIncludeNotFound, Message: fmt.Sprintf("metadata %q tidak ada di katalog", key)}}
	}
	if e.doc.Kind == "fieldType" {
		return nil, []load.Problem{problemAtDoc(e.doc, "/kind", CodeIncludeKind,
			fmt.Sprintf("%q adalah tipe domain (fieldType), bukan resource atau abstrak", key))}
	}
	r := newResolver(c)
	res := r.resolveDoc(key)
	return res, sortedProblems(r.probs)
}

// ResolveAll menyelesaikan semua resource dan abstrak di katalog (urutan kunci).
func (c *Catalog) ResolveAll() *Result {
	r := newResolver(c)
	out := &Result{}
	for _, key := range c.Keys() {
		e := c.docs[key]
		if e.doc.Kind == "fieldType" {
			continue
		}
		res := r.resolveDoc(key)
		if res == nil {
			continue
		}
		if res.Kind == "resource" {
			out.Resources = append(out.Resources, res)
		} else {
			out.Abstracts = append(out.Abstracts, res)
		}
	}
	out.Problems = sortedProblems(r.probs)
	return out
}

func (r *resolver) resolveDoc(key string) *Resolved {
	if res, ok := r.cache[key]; ok {
		return res
	}
	e := r.c.docs[key]
	r.stack = append(r.stack, key)
	res := newBuilder(r, e).run()
	r.stack = r.stack[:len(r.stack)-1]
	r.cache[key] = res
	return res
}

type lockOperand struct {
	cond    map[string]any
	origins []Origin
}

type builder struct {
	r      *resolver
	e      *entry
	res    *Resolved
	fields []*Field
	index  map[string]*Field
	failed bool

	ownerTargets map[string]string // nama field akhir -> alasan dilindungi (ownerField/scopeFields)
	referenced   map[string]bool   // field yang dirujuk relasi, indeks, owner, atau scope
	rules        map[string]bool
	lock         []lockOperand
	uses         map[string]Use
}

func newBuilder(r *resolver, e *entry) *builder {
	return &builder{r: r, e: e, index: map[string]*Field{}, ownerTargets: map[string]string{}, referenced: map[string]bool{},
		rules: map[string]bool{}, uses: map[string]Use{}}
}

func esc(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

func (b *builder) doc() *load.Document { return b.e.doc }

func (b *builder) fail(ptr, code, msg string) {
	b.failed = true
	b.r.report(problemAtDoc(b.doc(), ptr, code, msg))
}

func (b *builder) failOrigin(o Origin, ptr, code, msg string) {
	b.failed = true
	b.r.report(load.Problem{File: o.File, Line: o.Line, Column: o.Column, Pointer: ptr, Code: code, Message: msg})
}

func (b *builder) origin(ptr, op string) Origin {
	p := b.doc().Pos(ptr)
	return Origin{File: b.doc().File, Line: p.Line, Column: p.Column, Doc: b.doc().Key, Op: op}
}

// orderedKeys mengurutkan kunci peta menurut posisi di berkas sumber (urutan deklarasi).
func (b *builder) orderedKeys(m map[string]any, base string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	d := b.doc()
	sort.Slice(keys, func(i, j int) bool {
		pi, pj := d.Pos(base+"/"+esc(keys[i])), d.Pos(base+"/"+esc(keys[j]))
		if pi.Line != pj.Line {
			return pi.Line < pj.Line
		}
		if pi.Column != pj.Column {
			return pi.Column < pj.Column
		}
		return keys[i] < keys[j]
	})
	return keys
}

func (b *builder) prepare() {
	d := b.doc().Value
	if s, _ := d["ownerField"].(string); s != "" {
		b.ownerTargets[s] = "ownerField"
		b.referenced[s] = true
	}
	if sf, ok := d["scopeFields"].(map[string]any); ok {
		for _, v := range sf {
			if s, _ := v.(string); s != "" {
				b.ownerTargets[s] = "scopeFields"
				b.referenced[s] = true
			}
		}
	}
	if rels, ok := d["relations"].(map[string]any); ok {
		for _, rv := range rels {
			rel, _ := rv.(map[string]any)
			if snap, ok := rel["snapshot"].([]any); ok {
				for _, s := range snap {
					if m, _ := s.(map[string]any); m != nil {
						if to, _ := m["to"].(string); to != "" {
							b.referenced[to] = true
						}
					}
				}
			}
			if flt, ok := rel["filter"].(map[string]any); ok {
				collectRefs(flt, b.referenced)
			}
		}
	}
	if idx, ok := d["indexes"].([]any); ok {
		for _, iv := range idx {
			if m, _ := iv.(map[string]any); m != nil {
				if fs, ok := m["fields"].([]any); ok {
					for _, f := range fs {
						if s, _ := f.(string); s != "" {
							b.referenced[s] = true
						}
					}
				}
			}
		}
	}
}

// collectRefs mengumpulkan nilai ref (rujukan ke field resource sendiri) pada pohon kondisi.
func collectRefs(v any, into map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t["ref"].(string); ok {
			into[s] = true
		}
		for _, x := range t {
			collectRefs(x, into)
		}
	case []any:
		for _, x := range t {
			collectRefs(x, into)
		}
	}
}

func (b *builder) run() *Resolved {
	d := b.doc()
	b.res = &Resolved{
		Key: d.Key, Module: b.e.mod.Slug, Kind: d.Kind,
		Source:       Origin{File: d.File, Line: 1, Column: 1, Doc: d.Key, Op: OpDeclared},
		DefaultGroup: b.e.mod.defaultGroup(),
	}
	b.res.Export, _ = d.Value["export"].(bool)
	b.res.Description, _ = d.Value["description"].(string)
	b.prepare()

	// K22: sdk:audit selalu disertakan otomatis ke setiap resource (bukan abstrak), lebih dulu daripada include
	// lain sehingga field bawaan tampil pertama. Dilewati hanya bila katalog tidak memuatnya (katalog tanpa
	// metadata bawaan; AddBuiltin selalu memuatnya pada pemakaian nyata).
	if d.Kind == "resource" && b.r.c.docs[KeyAudit] != nil {
		b.include(-1, map[string]any{"source": "sdk:audit"})
	}
	if incs, ok := d.Value["include"].([]any); ok {
		for i, it := range incs {
			if m, ok := it.(map[string]any); ok {
				b.include(i, m)
			}
		}
	}
	b.ownFields()
	b.statusAxes()
	b.ownDeclarations()
	b.lifecycle()
	b.mergeLockWhen()
	if len(b.fields) > load.MaxFields {
		b.fail("", CodeLimit, fmt.Sprintf("hasil gabungan memuat %d field, melebihi batas %d (Requirement 1.3)", len(b.fields), load.MaxFields))
	}
	if b.failed {
		return nil
	}
	b.res.Fields = b.fields
	b.res.index()
	keys := make([]string, 0, len(b.uses))
	for k := range b.uses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.res.Uses = append(b.res.Uses, b.uses[k])
	}
	return b.res
}

// sameAuditField menyatakan kedua field adalah field bawaan sdk:audit yang sama (definisi di berkas dan baris
// yang sama): penyertaan ganda yang tidak mengubah apa pun dan karenanya bukan duplicate_field (K22).
func sameAuditField(a, b *Field) bool {
	return a.ProtectedBy == KeyAudit && b.ProtectedBy == KeyAudit &&
		a.Origin.File == b.Origin.File && a.Origin.Line == b.Origin.Line && a.Origin.Column == b.Origin.Column
}

func (b *builder) addUse(u Use) {
	if prev, ok := b.uses[u.Key]; ok && (prev.Direct || !u.Direct) && len(prev.Via) <= len(u.Via) {
		return
	}
	b.uses[u.Key] = u
}

func (b *builder) newField(name string, attrs map[string]any, ptr, op string) *Field {
	f := &Field{
		Name: name, SourceName: name,
		Attrs:      copyMap(attrs),
		Origin:     b.origin(ptr, op),
		AttrOrigin: map[string]Origin{},
	}
	for k := range attrs {
		f.AttrOrigin[k] = b.origin(ptr+"/"+esc(k), op)
	}
	if op == OpDeclared && (b.doc().Key == KeyAudit || b.doc().Key == KeyDocument) {
		f.ProtectedBy = b.doc().Key
	}
	if op == OpDeclared && b.doc().Key == KeyAudit && (name == "createdAt" || name == "updatedAt") {
		f.DefaultNow = true // K22: DEFAULT now() implisit pada model skema
	}
	b.expand(f)
	return f
}

func (b *builder) ownFields() {
	fm, _ := b.doc().Value["fields"].(map[string]any)
	for _, name := range b.orderedKeys(fm, "/fields") {
		ptr := "/fields/" + esc(name)
		if prev := b.index[name]; prev != nil {
			b.fail(ptr, CodeDuplicateField, fmt.Sprintf(
				"field %q sudah ada (dari %s, baris %d); untuk mengubahnya pakai set pada item include, bukan mendeklarasikan ulang",
				name, prev.Origin.Doc, prev.Origin.Line))
			continue
		}
		attrs, _ := fm[name].(map[string]any)
		f := b.newField(name, attrs, ptr, OpDeclared)
		b.fields = append(b.fields, f)
		b.index[name] = f
	}
}

var classRank = map[string]int{"public": 0, "internal": 1, "confidential": 2, "highly_confidential": 3}

func rank(c string) int { return classRank[c] }

func isDomainRef(t string) bool { return strings.ContainsAny(t, ":.") }

// expand mengganti tipe domain pada f dengan dasarnya, menggabungkan klasifikasi (menular ke atas saja), dan
// mencatat widget, format tampilan, dan operator (Requirement 3.8, 3.11).
func (b *builder) expand(f *Field) {
	t := f.Str("type")
	if !isDomainRef(t) {
		return
	}
	site := f.AttrOrigin["type"]
	key := sourceKey(t)
	te := b.r.c.docs[key]
	if te == nil {
		b.failOrigin(site, "/type", CodeTypeNotFound, fmt.Sprintf("tipe domain %q tidak ditemukan", t))
		return
	}
	if te.doc.Kind != "fieldType" {
		b.failOrigin(site, "/type", CodeTypeKind, fmt.Sprintf("%q bukan tipe domain (kind %s)", key, te.doc.Kind))
		return
	}
	if ok, why := accessible(b.e.mod, te); !ok {
		b.failOrigin(site, "/type", CodeTypeAccess, "tipe domain tidak dapat dipakai: "+why)
		return
	}
	b.addUse(Use{Key: key, Module: te.mod.Slug, Kind: "fieldType", Direct: true, Via: []string{key}})

	td := te.doc.Value
	to := func(ptr string) Origin {
		p := te.doc.Pos(ptr)
		return Origin{File: te.doc.File, Line: p.Line, Column: p.Column, Doc: key, Op: OpDomainType}
	}
	base, _ := td["base"].(map[string]any)
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "type" {
			f.History = append(f.History, Change{Attr: "type", Old: t, New: base[k], Origin: to("/base/type")})
			f.Attrs["type"] = base[k]
			f.AttrOrigin["type"] = to("/base/type")
			continue
		}
		if _, has := f.Attrs[k]; !has {
			f.Attrs[k] = deepCopy(base[k])
			f.AttrOrigin[k] = to("/base/" + k)
		}
	}
	if tc, ok := td["classification"].(string); ok {
		cur, has := f.Attrs["classification"].(string)
		o := f.AttrOrigin["classification"]
		switch {
		case has && o.Op != OpDomainType && rank(cur) < rank(tc):
			b.failOrigin(o, "/classification", CodeTypeLowersClassification, fmt.Sprintf(
				"klasifikasi %q lebih rendah daripada klasifikasi tipe domain %s (%q); klasifikasi tipe domain menular dan tidak boleh diturunkan",
				cur, key, tc))
		case !has || rank(tc) > rank(cur):
			f.Attrs["classification"] = tc
			f.AttrOrigin["classification"] = to("/classification")
		}
	}
	if lbl, ok := td["label"].(string); ok {
		if _, has := f.Attrs["label"]; !has {
			f.Attrs["label"] = lbl
			f.AttrOrigin["label"] = to("/label")
		}
	}
	f.DomainType = key
	f.Widget, _ = td["widget"].(string)
	f.Display, _ = td["display"].(string)
	f.Operators = nil
	if ops, ok := td["operators"].([]any); ok {
		for _, o := range ops {
			if s, ok := o.(string); ok {
				f.Operators = append(f.Operators, s)
			}
		}
	}
}

func (b *builder) protectedReason(f *Field, final string) string {
	if f.ProtectedBy != "" {
		return fmt.Sprintf("field bawaan dari %s tidak dapat di-omit atau di-set (Requirement 3.6, 13.1)", f.ProtectedBy)
	}
	if why, ok := b.ownerTargets[final]; ok {
		return fmt.Sprintf("field dipakai sebagai %s dan tidak dapat di-omit atau di-set (Requirement 3.6)", why)
	}
	return ""
}

func findField(fs []*Field, name string) int {
	for i, f := range fs {
		if f.Name == name {
			return i
		}
	}
	return -1
}

func (b *builder) include(i int, item map[string]any) {
	base := fmt.Sprintf("/include/%d", i)
	srcRef, _ := item["source"].(string)
	key := sourceKey(srcRef)
	te := b.r.c.docs[key]
	if te == nil {
		b.fail(base+"/source", CodeIncludeNotFound, fmt.Sprintf("metadata %q tidak ditemukan; source adalah kunci metadata (sdk:<nama> atau <slug>.<nama>), bukan jalur berkas", srcRef))
		return
	}
	if te.doc.Kind == "fieldType" {
		b.fail(base+"/source", CodeIncludeKind, fmt.Sprintf("%q adalah tipe domain; pakai sebagai type pada field, bukan include", key))
		return
	}
	if ok, why := accessible(b.e.mod, te); !ok {
		b.fail(base+"/source", CodeIncludeAccess, "metadata tidak dapat disertakan: "+why)
		return
	}
	if at := slices.Index(b.r.stack, key); at >= 0 {
		chain := append(append([]string(nil), b.r.stack[at:]...), key)
		b.fail(base+"/source", CodeIncludeCycle, "penyertaan melingkar: "+strings.Join(chain, " -> "))
		return
	}
	R := b.r.resolveDoc(key)
	if R == nil {
		b.failed = true
		return
	}

	prefix, _ := item["prefix"].(string)
	prefixOK := true
	fin := func(name string) string {
		if prefix == "" {
			return name
		}
		n, err := naming.Prefixed(prefix, name)
		if err != nil {
			prefixOK = false
			b.fail(base+"/prefix", CodePrefixInvalid, fmt.Sprintf("prefix %q pada %q tidak menghasilkan nama sah: %v", prefix, name, err))
			return name
		}
		return n
	}

	contrib := make([]*Field, 0, len(R.Fields))
	for _, f := range R.Fields {
		if f.IsStatusAxis {
			continue // sumbu status tidak diwarisi lewat include (K24)
		}
		contrib = append(contrib, f.clone(R.Key))
	}

	if om, ok := item["omit"].([]any); ok {
		for j, v := range om {
			name, _ := v.(string)
			ptr := fmt.Sprintf("%s/omit/%d", base, j)
			idx := findField(contrib, name)
			if idx < 0 {
				b.fail(ptr, CodeOverrideMissing, fmt.Sprintf("omit: field %q tidak ada di %s", name, key))
				continue
			}
			if why := b.protectedReason(contrib[idx], fin(name)); why != "" {
				b.fail(ptr, CodeOverrideProtected, fmt.Sprintf("omit %q: %s", name, why))
				continue
			}
			contrib = slices.Delete(contrib, idx, idx+1)
		}
	}
	if set, ok := item["set"].(map[string]any); ok {
		for _, name := range b.orderedKeys(set, base+"/set") {
			ptr := base + "/set/" + esc(name)
			idx := findField(contrib, name)
			if idx < 0 {
				b.fail(ptr, CodeOverrideMissing, fmt.Sprintf("set: field %q tidak ada di %s", name, key))
				continue
			}
			if why := b.protectedReason(contrib[idx], fin(name)); why != "" {
				b.fail(ptr, CodeOverrideProtected, fmt.Sprintf("set %q: %s", name, why))
				continue
			}
			patch, _ := set[name].(map[string]any)
			b.applyPatch(contrib[idx], patch, ptr, fin(name))
		}
	}
	if add, ok := item["add"].(map[string]any); ok {
		for _, name := range b.orderedKeys(add, base+"/add") {
			ptr := base + "/add/" + esc(name)
			if findField(contrib, name) >= 0 {
				b.fail(ptr, CodeDuplicateField, fmt.Sprintf("add: field %q sudah ada di %s; pakai set untuk mengubahnya", name, key))
				continue
			}
			attrs, _ := add[name].(map[string]any)
			contrib = append(contrib, b.newField(name, attrs, ptr, OpAdd))
		}
	}

	// Nama akhir (prefix) dan penulisan ulang rujukan di dalam sumbangan item ini.
	rename := map[string]string{}
	for _, f := range contrib {
		rename[f.Name] = fin(f.Name)
	}
	if !prefixOK {
		return
	}
	for _, f := range contrib {
		renameRefs(f.Attrs, rename)
		f.SourceName = f.Name
		f.Name = rename[f.Name]
		if prev := b.index[f.Name]; prev != nil {
			if sameAuditField(prev, f) {
				continue // K22: field sdk:audit yang sama (implisit, eksplisit, atau lewat include lain) hanya satu kali
			}
			b.fail(base, CodeDuplicateField, fmt.Sprintf(
				"field %q dari %s bertabrakan dengan field dari %s (baris %d); pakai prefix, omit, atau set",
				f.Name, key, prev.Origin.Doc, prev.Origin.Line))
			continue
		}
		b.fields = append(b.fields, f)
		b.index[f.Name] = f
	}

	for _, rule := range R.Rules {
		id := fin(rule.ID)
		if b.rules[id] {
			b.fail(base, CodeDuplicateRule, fmt.Sprintf("aturan %q dari %s bertabrakan dengan aturan lain; pakai prefix", id, key))
			continue
		}
		b.rules[id] = true
		attrs := copyMap(rule.Attrs)
		renameRefs(attrs, rename)
		b.res.Rules = append(b.res.Rules, &Rule{ID: id, Attrs: attrs, Origin: rule.Origin.withChain(R.Key)})
	}
	for _, s := range R.Sections {
		order := make([]string, 0, len(s.Order))
		for _, n := range s.Order {
			if nn, ok := rename[n]; ok {
				order = append(order, nn)
			}
		}
		b.res.Sections = append(b.res.Sections, Section{Key: fin(s.Key), Columns: s.Columns, Order: order, Origin: s.Origin.withChain(R.Key)})
	}
	if R.LockWhen != nil {
		cond := copyMap(R.LockWhen)
		renameRefs(cond, rename)
		origins := make([]Origin, len(R.LockWhenOrigins))
		for i, o := range R.LockWhenOrigins {
			origins[i] = o.withChain(R.Key)
		}
		b.lock = append(b.lock, lockOperand{cond: cond, origins: origins})
	}

	b.addUse(Use{Key: R.Key, Module: R.Module, Kind: "include", Direct: true, Via: []string{R.Key}})
	for _, u := range R.Uses {
		b.addUse(Use{Key: u.Key, Module: u.Module, Kind: u.Kind, Direct: false, Via: append([]string{R.Key}, u.Via...)})
	}
}

// renameRefs menulis ulang rujukan field (kunci field, ref, replacedBy, migrateFrom) yang cocok dengan peta.
func renameRefs(v any, rename map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			switch k {
			case "field", "ref", "replacedBy", "migrateFrom":
				if s, ok := x.(string); ok {
					if n, ok := rename[s]; ok {
						t[k] = n
					}
					continue
				}
			}
			renameRefs(x, rename)
		}
	case []any:
		for _, x := range t {
			renameRefs(x, rename)
		}
	}
}

func (b *builder) applyPatch(f *Field, patch map[string]any, ptr, final string) {
	for _, attr := range b.orderedKeys(patch, ptr) {
		apos := ptr + "/" + esc(attr)
		nv := deepCopy(patch[attr])
		switch attr {
		case "classification":
			ns, _ := nv.(string)
			if rank(ns) < rank(f.Classification()) {
				b.fail(apos, CodeOverrideLowersClassification, fmt.Sprintf(
					"set %s.classification: %q menurunkan klasifikasi %q (Requirement 3.6); alasan tertulis untuk pelonggaran belum didefinisikan",
					f.Name, ns, f.Classification()))
				continue
			}
		case "fieldGroup":
			dg := b.e.mod.defaultGroup()
			cur := f.Str("fieldGroup")
			if cur == "" {
				cur = dg
			}
			if ns, _ := nv.(string); ns == dg && cur != dg {
				b.fail(apos, CodeOverrideLoosensGroup, fmt.Sprintf(
					"set %s.fieldGroup: memindahkan dari grup %q ke grup default %q melonggarkan akses (Requirement 3.6)", f.Name, cur, dg))
				continue
			}
		case "readOnly":
			if nb, ok := nv.(bool); ok && !nb && f.Bool("readOnly") {
				b.fail(apos, CodeOverrideRevokesReadOnly, fmt.Sprintf("set %s.readOnly: mencabut readOnly tidak diizinkan (Requirement 3.6)", f.Name))
				continue
			}
		case "type":
			if b.referenced[final] {
				b.fail(apos, CodeOverrideTypeReferenced, fmt.Sprintf(
					"set %s.type: field dirujuk relasi, indeks, ownerField, atau scopeFields sehingga tipenya tidak dapat diubah (Requirement 3.3)", f.Name))
				continue
			}
		}
		o := b.origin(apos, OpSet)
		f.History = append(f.History, Change{Attr: attr, Old: deepCopy(f.Attrs[attr]), New: deepCopy(nv), Origin: o})
		f.Attrs[attr] = nv
		f.AttrOrigin[attr] = o
		if attr == "type" {
			if ts, _ := nv.(string); isDomainRef(ts) {
				b.expand(f)
			} else {
				f.DomainType, f.Widget, f.Display, f.Operators = "", "", "", nil
			}
		}
	}
}

// ownDeclarations memuat relasi, aturan, indeks, tata letak, dan atribut tingkat resource milik dokumen sendiri.
func (b *builder) ownDeclarations() {
	d := b.doc()
	v := d.Value
	if rels, ok := v["relations"].(map[string]any); ok {
		for _, name := range b.orderedKeys(rels, "/relations") {
			attrs, _ := rels[name].(map[string]any)
			b.res.Relations = append(b.res.Relations, &Relation{Name: name, Attrs: copyMap(attrs), Origin: b.origin("/relations/"+esc(name), OpDeclared)})
		}
	}
	if rules, ok := v["rules"].([]any); ok {
		for i, rv := range rules {
			m, _ := rv.(map[string]any)
			id, _ := m["id"].(string)
			ptr := fmt.Sprintf("/rules/%d", i)
			if b.rules[id] {
				b.fail(ptr+"/id", CodeDuplicateRule, fmt.Sprintf("aturan %q ganda; pakai id berbeda atau prefix pada include", id))
				continue
			}
			b.rules[id] = true
			b.res.Rules = append(b.res.Rules, &Rule{ID: id, Attrs: copyMap(m), Origin: b.origin(ptr, OpDeclared)})
		}
	}
	if lay, ok := v["layout"].(map[string]any); ok {
		s := Section{Origin: b.origin("/layout", OpDeclared)}
		s.Key, _ = lay["section"].(string)
		if n, ok := lay["columns"].(interface{ Int64() (int64, error) }); ok {
			c, _ := n.Int64()
			s.Columns = int(c)
		}
		if ord, ok := lay["order"].([]any); ok {
			for _, o := range ord {
				if n, ok := o.(string); ok {
					s.Order = append(s.Order, n)
				}
			}
		}
		b.res.Sections = append(b.res.Sections, s)
	}
	if lw, ok := v["lockWhen"].(map[string]any); ok {
		b.lock = append(b.lock, lockOperand{cond: copyMap(lw), origins: []Origin{b.origin("/lockWhen", OpDeclared)}})
	}
	if d.Kind != "resource" {
		return
	}
	b.res.LabelTemplate, _ = v["labelTemplate"].(string)
	b.res.Large, _ = v["large"].(bool)
	b.res.OwnerField, _ = v["ownerField"].(string)
	if g, ok := v["generate"].(map[string]any); ok {
		b.res.Generate = copyMap(g)
	}
	if idx, ok := v["indexes"].([]any); ok {
		for _, iv := range idx {
			if m, ok := iv.(map[string]any); ok {
				b.res.Indexes = append(b.res.Indexes, copyMap(m))
			}
		}
	}
	if sf, ok := v["scopeFields"].(map[string]any); ok {
		b.res.ScopeFields = map[string]string{}
		for k, x := range sf {
			b.res.ScopeFields[k], _ = x.(string)
		}
	}
	if uc, ok := v["unmanagedColumns"].([]any); ok {
		for _, x := range uc {
			if m, ok := x.(map[string]any); ok {
				b.res.UnmanagedColumns = append(b.res.UnmanagedColumns, copyMap(m))
			}
		}
	}
}

// mergeLockWhen menggabungkan kondisi penguncian dengan OR: kondisi turunan memperketat, tidak menggantikan
// kondisi bawaan (Requirement 13.1). Hasil diperiksa ulang terhadap batas node dan kedalaman.
func (b *builder) mergeLockWhen() {
	switch len(b.lock) {
	case 0:
		return
	case 1:
		b.res.LockWhen = b.lock[0].cond
		b.res.LockWhenOrigins = b.lock[0].origins
		return
	}
	var items []any
	for _, op := range b.lock {
		b.res.LockWhenOrigins = append(b.res.LockWhenOrigins, op.origins...)
		if any1, ok := op.cond["any"].([]any); ok && len(op.cond) == 1 {
			items = append(items, any1...)
			continue
		}
		items = append(items, op.cond)
	}
	merged := map[string]any{"any": items}
	nodes, depth := load.ConditionStats(merged)
	if depth > load.MaxConditionDepth || nodes > load.MaxConditionNodes {
		ptr := ""
		if _, ok := b.doc().Value["lockWhen"]; ok {
			ptr = "/lockWhen"
		}
		b.fail(ptr, CodeLockWhenLimit, fmt.Sprintf(
			"gabungan lockWhen (OR dengan kondisi bawaan) memuat %d node dengan kedalaman %d; batas %d node dan kedalaman %d",
			nodes, depth, load.MaxConditionNodes, load.MaxConditionDepth))
		return
	}
	b.res.LockWhen = merged
}
