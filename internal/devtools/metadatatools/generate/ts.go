package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

// Keluaran TypeScript (task 6.4, Requirement 3.13, 4.6, 18.3, 18.8, 19.9): satu ekspor `resourceMeta` bertipe
// ResourceMeta (packages/platform-web/src/metadata/meta.ts). Semua literal berasal dari pengodean JSON
// (jsonLit, setara JSON.stringify); kunci objek adalah kunci tetap skema (tidak pernah teks masukan). Tata
// letak mengikuti formatter Biome proyek (objek selalu diperluas, larik skalar dilipat bila muat 100 kolom)
// sehingga `biome check` tidak mengubah berkas bangkitan.

const tsLineWidth = 100

type tsKV struct {
	k string
	v any
}

type tsObj []tsKV

type tsArr []any

// tsLit adalah skalar yang sudah berupa literal TypeScript sah.
type tsLit string

var (
	tsKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
	tsNumRe = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

// jsonLit mengodekan string sebagai literal string JavaScript yang setara JSON.stringify (tanpa escape HTML).
// Seperti formatter Biome, tanda kutip ganda dipakai kecuali teks memuat lebih banyak kutip ganda daripada
// kutip tunggal (kutip tunggal, tanda kutip ganda tanpa escape); tanpa ini `biome check` mengubah berkas.
func jsonLit(s string) tsLit {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // string selalu dapat dikodekan
	lit := strings.TrimRight(buf.String(), "\n")
	if strings.Count(s, `"`) > strings.Count(s, "'") {
		inner := lit[1 : len(lit)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, "'", `\'`)
		return tsLit("'" + inner + "'")
	}
	return tsLit(lit)
}

func tsBool(b bool) tsLit { return tsLit(strconv.FormatBool(b)) }

// tsNum menormalkan teks bilangan seperti formatter (tanpa nol di belakang pecahan); eksponen ditolak.
func tsNum(text string) (tsLit, error) {
	if !tsNumRe.MatchString(text) {
		return "", inputErr("angka %q tidak dapat menjadi literal TypeScript", text)
	}
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "-0" {
		text = "0"
	}
	return tsLit(text), nil
}

func tsStrings(ss []string) tsArr {
	out := make(tsArr, len(ss))
	for i, s := range ss {
		out[i] = jsonLit(s)
	}
	return out
}

// genericTS mengubah nilai generik YAML (peta, daftar, skalar) menjadi pohon keluaran dengan urutan kunci
// terurut abjad (deterministik).
func genericTS(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return tsLit("null"), nil
	case bool:
		return tsBool(t), nil
	case string:
		return jsonLit(t), nil
	case json.Number:
		return tsNum(t.String())
	case []any:
		out := make(tsArr, len(t))
		for i, x := range t {
			g, err := genericTS(x)
			if err != nil {
				return nil, err
			}
			out[i] = g
		}
		return out, nil
	case map[string]any:
		var out tsObj
		for _, k := range sortedKeys(t) {
			if !tsKeyRe.MatchString(k) {
				return nil, inputErr("kunci %q tidak aman untuk keluaran TypeScript", k)
			}
			g, err := genericTS(t[k])
			if err != nil {
				return nil, err
			}
			out = append(out, tsKV{k, g})
		}
		return out, nil
	}
	return nil, inputErr("nilai bertipe %T tidak dapat menjadi literal TypeScript", v)
}

func tsValue(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return tsLit("null"), nil
	case bool:
		return tsBool(t), nil
	case string:
		return jsonLit(t), nil
	case json.Number:
		return tsNum(t.String())
	case []any:
		out := make(tsArr, len(t))
		for i, x := range t {
			g, err := tsValue(x)
			if err != nil {
				return nil, err
			}
			out[i] = g
		}
		return out, nil
	}
	return nil, inputErr("nilai kondisi bertipe %T tidak didukung", v)
}

// condTS mengubah kondisi ke bentuk JSON yang sama dengan berkas metadata dan evaluator klien.
func condTS(c *metadata.Condition) (any, error) {
	switch c.Kind {
	case metadata.CondAll, metadata.CondAny:
		key := "all"
		if c.Kind == metadata.CondAny {
			key = "any"
		}
		arr := make(tsArr, len(c.Children))
		for i, ch := range c.Children {
			v, err := condTS(ch)
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return tsObj{{key, arr}}, nil
	case metadata.CondNot:
		v, err := condTS(c.Children[0])
		if err != nil {
			return nil, err
		}
		return tsObj{{"not", v}}, nil
	}
	if !naming.IdentifierRegexp().MatchString(c.Field) {
		return nil, inputErr("nama field kondisi %q tidak sah", c.Field)
	}
	o := tsObj{{"field", jsonLit(c.Field)}, {"op", jsonLit(string(c.Op))}}
	switch {
	case c.Op == metadata.OpIsNull:
		null := true
		if b, ok := c.Value.(bool); ok {
			null = b
		}
		o = append(o, tsKV{"value", tsBool(null)})
	case c.Ref != "":
		if !naming.IdentifierRegexp().MatchString(c.Ref) {
			return nil, inputErr("rujukan kondisi %q tidak sah", c.Ref)
		}
		o = append(o, tsKV{"ref", jsonLit(c.Ref)})
	default:
		v, err := tsValue(c.Value)
		if err != nil {
			return nil, err
		}
		o = append(o, tsKV{"value", v})
	}
	return o, nil
}

// clientCtx menilai apakah kondisi aman untuk chunk publik (D-8, D-21): hanya merujuk field yang ikut dipancarkan,
// berklasifikasi paling tinggi internal, dan berada di grup default.
type clientCtx struct {
	fields       map[string]*fld
	defaultGroup string
	// remote mencari klasifikasi dan grup field resource seberang (untuk filter relasi); relTarget adalah kunci
	// resource seberang relasi yang sedang diperiksa. Tanpa remote (atau target) filter relasi tidak dipancarkan.
	remote    remoteLookup
	relTarget string
}

// remoteField adalah informasi field pada resource seberang yang dibutuhkan untuk menilai kebocoran ke klien.
type remoteField struct {
	Class, Group, DefaultGroup string
	ClientVisible              bool
}

// remoteLookup mengembalikan informasi field resource seberang (klasifikasi dan grup efektif); ok false bila resource
// atau field tidak dikenal.
type remoteLookup func(resourceKey, field string) (remoteField, bool)

// remoteSafe menilai rujukan field pada resource seberang (kunci field pada filter relasi): gagal tertutup bila tidak
// dapat diperiksa, dan menolak field confidential ke atas, di luar grup default, atau yang tidak ikut ke klien.
func (c *clientCtx) remoteSafe(field string) (bool, string) {
	if c.remote == nil || c.relTarget == "" {
		return false, "resource seberang tidak dapat diperiksa (tanpa informasi klasifikasi)"
	}
	rf, ok := c.remote(c.relTarget, field)
	switch {
	case !ok:
		return false, fmt.Sprintf("merujuk field %q pada %s yang tidak dikenal", field, c.relTarget)
	case !rf.ClientVisible:
		return false, fmt.Sprintf("merujuk field %q pada %s yang tidak ikut ke klien", field, c.relTarget)
	case rf.Class != "public" && rf.Class != "internal":
		return false, fmt.Sprintf("merujuk field %q pada %s berklasifikasi %s", field, c.relTarget, rf.Class)
	case rf.Group != rf.DefaultGroup:
		return false, fmt.Sprintf("merujuk field %q pada %s di luar grup default", field, c.relTarget)
	}
	return true, ""
}

func (c *clientCtx) refsSafe(cond *metadata.Condition, relFilter bool) (bool, string) {
	switch cond.Kind {
	case metadata.CondAll, metadata.CondAny, metadata.CondNot:
		for _, ch := range cond.Children {
			if ok, why := c.refsSafe(ch, relFilter); !ok {
				return false, why
			}
		}
		return true, ""
	}
	names := []string{}
	if relFilter { // pada filter relasi, field menunjuk resource seberang
		if ok, why := c.remoteSafe(cond.Field); !ok {
			return false, why
		}
	} else {
		names = append(names, cond.Field)
	}
	if cond.Ref != "" {
		names = append(names, cond.Ref)
	}
	for _, n := range names {
		f, ok := c.fields[n]
		switch {
		case !ok:
			return false, fmt.Sprintf("merujuk field %q yang tidak ikut ke klien", n)
		case f.Class != "public" && f.Class != "internal":
			return false, fmt.Sprintf("merujuk field %q berklasifikasi %s", n, f.Class)
		case f.Group != c.defaultGroup:
			return false, fmt.Sprintf("merujuk field %q di luar grup default", n)
		}
	}
	return true, ""
}

// tsBuilder membangun pohon keluaran satu resource.
type tsBuilder struct {
	m       *resModel
	ctx     *clientCtx
	omitted []string
	docPos  func(pointer string) int
}

func (b *tsBuilder) omit(format string, a ...any) {
	b.omitted = append(b.omitted, b.m.Res.Key+": "+fmt.Sprintf(format, a...))
}

func (b *tsBuilder) cond(where string, c *metadata.Condition, relFilter bool) (any, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	if ok, why := b.ctx.refsSafe(c, relFilter); !ok {
		b.omit("%s tidak dipancarkan ke klien (%s)", where, why)
		return nil, false, nil
	}
	v, err := condTS(c)
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

func (b *tsBuilder) defaultValue(f *fld) (any, error) {
	if f.Default == nil {
		return tsLit("null"), nil
	}
	switch f.Type {
	case "boolean":
		bv, _ := f.Default.(bool)
		return tsBool(bv), nil
	case "integer":
		return tsNum(defaultText(f.Default))
	}
	return jsonLit(defaultText(f.Default)), nil
}

func (b *tsBuilder) fieldMeta(f *fld) (any, error) {
	o := tsObj{{"name", jsonLit(f.Name)}, {"type", jsonLit(f.Type)}}
	if f.DomainType != "" {
		o = append(o, tsKV{"domainType", jsonLit(f.DomainType)})
	}
	o = append(o,
		tsKV{"label", jsonLit(f.Label)},
		tsKV{"nullable", tsBool(f.Nullable)},
		tsKV{"readOnly", tsBool(f.ReadOnly)},
	)
	if f.HasDefault {
		d, err := b.defaultValue(f)
		if err != nil {
			return nil, err
		}
		o = append(o, tsKV{"default", d})
	}
	o = append(o,
		tsKV{"fieldGroup", jsonLit(f.Group)},
		tsKV{"classification", jsonLit(f.Class)},
		tsKV{"filterable", tsBool(f.Filterable)},
		tsKV{"sortable", tsBool(f.Sortable)},
		tsKV{"groupable", tsBool(f.Groupable)},
		tsKV{"aggregatable", tsBool(f.Aggregatable)},
	)
	numAttr := func(key string, n int) {
		if n > 0 {
			o = append(o, tsKV{key, tsLit(strconv.Itoa(n))})
		}
	}
	numAttr("minLength", f.MinLength)
	numAttr("maxLength", f.MaxLength)
	if f.Pattern != "" {
		o = append(o, tsKV{"pattern", jsonLit(f.Pattern)})
	}
	if f.Format != "" {
		o = append(o, tsKV{"format", jsonLit(f.Format)})
	}
	numAttr("precision", f.Precision)
	numAttr("scale", f.Scale)
	if f.Currency != "" {
		o = append(o, tsKV{"currency", jsonLit(f.Currency)})
	}
	for _, bd := range []struct{ key, val string }{{"minimum", f.Minimum}, {"maximum", f.Maximum}} {
		if bd.val == "" {
			continue
		}
		if f.Type == "integer" {
			t, ok := intText(bd.val)
			if !ok {
				return nil, inputErr("field %s: %s integer bukan bilangan bulat", f.Name, bd.key)
			}
			n, err := tsNum(t)
			if err != nil {
				return nil, err
			}
			o = append(o, tsKV{bd.key, n})
			continue
		}
		o = append(o, tsKV{bd.key, jsonLit(bd.val)})
	}
	if len(f.Enum) > 0 && f.Type == "enum" {
		o = append(o, tsKV{"values", tsStrings(f.Enum)})
	}
	if f.Extensible {
		o = append(o, tsKV{"extensible", tsBool(true)})
	}
	if f.Widget != "" {
		o = append(o, tsKV{"widget", jsonLit(f.Widget)})
	}
	if f.Display != "" {
		o = append(o, tsKV{"display", jsonLit(f.Display)})
	}
	if len(f.Operators) > 0 {
		o = append(o, tsKV{"operators", tsStrings(f.Operators)})
	}
	for _, c := range []struct {
		key string
		c   *metadata.Condition
	}{{"visibleWhen", f.VisibleWhen}, {"requiredWhen", f.RequiredWhen}, {"readonlyWhen", f.ReadonlyWhen}} {
		if f.ServerCond[c.key] && c.c != nil {
			b.omit("field %s.%s tidak dipancarkan ke klien (scope server menurut check)", f.Name, c.key)
			continue
		}
		v, ok, err := b.cond("field "+f.Name+"."+c.key, c.c, false)
		if err != nil {
			return nil, err
		}
		if ok {
			o = append(o, tsKV{c.key, v})
		}
	}
	if f.Deprecated {
		d := tsObj{{"since", jsonLit(f.DeprecatedMap["since"])}}
		if r := f.DeprecatedMap["replacedBy"]; r != "" {
			if _, visible := b.ctx.fields[r]; visible {
				d = append(d, tsKV{"replacedBy", jsonLit(r)})
			} else {
				b.omit("field %s: deprecated.replacedBy tidak dipancarkan (field tujuan tidak ikut ke klien)", f.Name)
			}
		}
		o = append(o, tsKV{"deprecated", d})
	}
	if f.Computed {
		acc, ok, err := b.accessor(f)
		if err != nil {
			return nil, err
		}
		if ok {
			o = append(o, tsKV{"accessor", acc})
		}
	}
	return o, nil
}

// exprRefs mengumpulkan semua rujukan field (kunci field dan ref) di dalam ekspresi atau kondisi mentah.
func exprRefs(v any, into map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if s, ok := x.(string); ok && (k == "field" || k == "ref") {
				into[s] = true
				continue
			}
			exprRefs(x, into)
		}
	case []any:
		for _, x := range t {
			exprRefs(x, into)
		}
	}
}

// namesSafe menilai apakah semua nama field aman untuk chunk publik (D-8, D-21).
func (c *clientCtx) namesSafe(names map[string]bool) (bool, string) {
	for _, n := range sortedKeys(names) {
		f, ok := c.fields[n]
		switch {
		case !ok:
			return false, fmt.Sprintf("merujuk field %q yang tidak ikut ke klien", n)
		case f.Class != "public" && f.Class != "internal":
			return false, fmt.Sprintf("merujuk field %q berklasifikasi %s", n, f.Class)
		case f.Group != c.defaultGroup:
			return false, fmt.Sprintf("merujuk field %q di luar grup default", n)
		}
	}
	return true, ""
}

// accessor menyusun accessor klien. Nilai accessor stored datang dari server, sehingga ekspresinya tidak pernah
// dipancarkan. Ekspresi expr dipancarkan hanya bila semua rujukan field (termasuk kondisi if) berklasifikasi paling
// tinggi internal dan berada di grup default; selain itu seluruh accessor tidak dipancarkan (field tetap hanya-baca).
func (b *tsBuilder) accessor(f *fld) (any, bool, error) {
	acc := tsObj{{"kind", jsonLit(f.AccessorKind)}}
	switch f.AccessorKind {
	case "stored":
		if f.AccessorExpr != nil {
			b.omit("field %s: ekspresi accessor stored tidak dipancarkan (nilai datang dari server)", f.Name)
		}
	case "func":
	case "expr":
		if f.AccessorExpr == nil {
			return nil, false, inputErr("field %s: accessor expr tanpa ekspresi", f.Name)
		}
		refs := map[string]bool{}
		exprRefs(f.AccessorExpr, refs)
		if ok, why := b.ctx.namesSafe(refs); !ok {
			b.omit("field %s: accessor expr tidak dipancarkan ke klien (%s)", f.Name, why)
			return nil, false, nil
		}
		e, err := genericTS(f.AccessorExpr)
		if err != nil {
			return nil, false, fmt.Errorf("field %s: %w", f.Name, err)
		}
		acc = append(acc, tsKV{"expr", e})
	default:
		return nil, false, inputErr("field %s: jenis accessor %q tidak dikenal", f.Name, f.AccessorKind)
	}
	return acc, true, nil
}

func (b *tsBuilder) relation(rel *resolve.Relation) (any, error) {
	a := rel.Attrs
	if !naming.IdentifierRegexp().MatchString(rel.Name) {
		return nil, inputErr("nama relasi %q tidak sah", rel.Name)
	}
	o := tsObj{{"name", jsonLit(rel.Name)}, {"type", jsonLit(attrString(a, "type"))}}
	for _, k := range []string{"to", "of"} {
		if s := attrString(a, k); s != "" {
			if !resourceKeyRe.MatchString(s) {
				return nil, inputErr("relasi %s: kunci %q tidak sah", rel.Name, s)
			}
			o = append(o, tsKV{k, jsonLit(s)})
		}
	}
	if list, ok := a["types"].([]any); ok {
		var ss []string
		for _, e := range list {
			s, _ := e.(string)
			if !resourceKeyRe.MatchString(s) {
				return nil, inputErr("relasi %s: kunci tipe %q tidak sah", rel.Name, s)
			}
			ss = append(ss, s)
		}
		o = append(o, tsKV{"types", tsStrings(ss)})
	}
	if s := attrString(a, "as"); s != "" {
		if !naming.IdentifierRegexp().MatchString(s) {
			return nil, inputErr("relasi %s: as tidak sah", rel.Name)
		}
		o = append(o, tsKV{"as", jsonLit(s)})
	}
	if s := attrString(a, "onDelete"); s != "" {
		o = append(o, tsKV{"onDelete", jsonLit(s)})
	}
	if _, ok := a["required"]; ok {
		o = append(o, tsKV{"required", tsBool(attrBool(a, "required"))})
	}
	if _, ok := a["embedded"]; ok {
		o = append(o, tsKV{"embedded", tsBool(attrBool(a, "embedded"))})
	}
	fc, err := conditionOf(a, "filter")
	if err != nil {
		return nil, err
	}
	target := attrString(a, "to")
	if target == "" {
		target = attrString(a, "of")
	}
	b.ctx.relTarget = target
	v, okFilter, ferr := b.cond("relation "+rel.Name+".filter", fc, true)
	b.ctx.relTarget = ""
	if ferr != nil {
		return nil, ferr
	}
	if okFilter {
		o = append(o, tsKV{"filter", v})
	}
	if list, ok := a["snapshot"].([]any); ok {
		var arr tsArr
		for _, e := range list {
			sm, _ := e.(map[string]any)
			from, to := attrString(sm, "from"), attrString(sm, "to")
			if !naming.IdentifierRegexp().MatchString(from) || !naming.IdentifierRegexp().MatchString(to) {
				return nil, inputErr("relasi %s: snapshot tidak sah", rel.Name)
			}
			if _, visible := b.ctx.fields[to]; !visible {
				b.omit("relation %s: snapshot ke field %q tidak dipancarkan (field tujuan tidak ikut ke klien)", rel.Name, to)
				continue
			}
			arr = append(arr, tsObj{{"from", jsonLit(from)}, {"to", jsonLit(to)}})
		}
		if len(arr) > 0 {
			o = append(o, tsKV{"snapshot", arr})
		}
	}
	if pf, ok := a["pivotFields"].(map[string]any); ok && len(pf) > 0 {
		names := sortedKeys(pf)
		sort.SliceStable(names, func(i, j int) bool {
			return b.docPos("/relations/"+escapePtr(rel.Name)+"/pivotFields/"+escapePtr(names[i])) <
				b.docPos("/relations/"+escapePtr(rel.Name)+"/pivotFields/"+escapePtr(names[j]))
		})
		slug, rname, _ := naming.SplitResourceKey(b.m.Res.Key)
		var kept []*fld
		pctx := &clientCtx{fields: map[string]*fld{}, defaultGroup: b.m.DefaultGroup}
		for _, n := range names {
			attrs, _ := pf[n].(map[string]any)
			cls := attrString(attrs, "classification")
			if cls == "" {
				cls = "internal"
			}
			group := b.m.DefaultGroup
			if g := attrString(attrs, "fieldGroup"); g != "" {
				group = manifestGroup(g)
			}
			f, err := newFld(n, attrs, slug+"."+rname+"."+rel.Name+"."+n, group, cls)
			if err != nil {
				return nil, fmt.Errorf("relasi %s: %w", rel.Name, err)
			}
			switch {
			case !f.ClientVisible:
				b.omit("relasi %s: pivotFields.%s tidak dipancarkan (clientVisible: false)", rel.Name, n)
				continue
			case f.Class != "public" && f.Class != "internal":
				b.omit("relasi %s: pivotFields.%s tidak dipancarkan (berklasifikasi %s)", rel.Name, n, f.Class)
				continue
			case f.Group != b.m.DefaultGroup:
				b.omit("relasi %s: pivotFields.%s tidak dipancarkan (di luar grup default)", rel.Name, n)
				continue
			}
			kept = append(kept, f)
			pctx.fields[f.Name] = f
		}
		// Kondisi dan accessor field pivot hanya boleh merujuk sesama field pivot yang aman.
		saved := b.ctx
		b.ctx = pctx
		var arr tsArr
		for _, f := range kept {
			fm, err := b.fieldMeta(f)
			if err != nil {
				b.ctx = saved
				return nil, err
			}
			arr = append(arr, fm)
		}
		b.ctx = saved
		if len(arr) > 0 {
			o = append(o, tsKV{"pivotFields", arr})
		}
	}
	return o, nil
}

func escapePtr(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// buildTS membangun isi berkas <nama>.meta.gen.ts dan daftar catatan bagian yang sengaja tidak dipancarkan.
func buildTS(m *resModel, docPos func(string) int, remote remoteLookup) ([]byte, []string, error) {
	ctx := &clientCtx{fields: map[string]*fld{}, defaultGroup: m.DefaultGroup, remote: remote}
	var emitted []*fld
	for _, f := range m.Fields {
		if f.Column {
			continue // kolom relasi sintetis diwakili relasinya (model datar resolver)
		}
		if !f.ClientVisible {
			continue
		}
		ctx.fields[f.Name] = f
		emitted = append(emitted, f)
	}
	b := &tsBuilder{m: m, ctx: ctx, docPos: docPos}
	root := tsObj{{"key", jsonLit(m.Res.Key)}}
	if m.LabelTpl != "" {
		if hidden := hiddenPlaceholders(m.LabelTpl, ctx.fields); len(hidden) > 0 {
			b.omit("labelTemplate tidak dipancarkan (merujuk field yang tidak ikut ke klien: %s)", strings.Join(hidden, ", "))
		} else {
			root = append(root, tsKV{"labelTemplate", jsonLit(m.LabelTpl)})
		}
	}
	root = append(root, tsKV{"defaultGroup", jsonLit(m.DefaultGroup)})
	fields := tsArr{}
	for _, f := range m.Fields {
		if !f.Column && !f.ClientVisible {
			b.omit("field %s tidak dipancarkan (clientVisible: false)", f.Name)
		}
	}
	for _, f := range emitted {
		fm, err := b.fieldMeta(f)
		if err != nil {
			return nil, nil, fmt.Errorf("field %s: %w", f.Name, err)
		}
		fields = append(fields, fm)
	}
	root = append(root, tsKV{"fields", fields})
	rels := tsArr{}
	for _, rel := range m.Res.Relations {
		r, err := b.relation(rel)
		if err != nil {
			return nil, nil, err
		}
		rels = append(rels, r)
	}
	root = append(root, tsKV{"relations", rels})
	rules := tsArr{}
	for _, r := range m.Rules {
		if r.ServerOnly {
			b.omit("aturan %s tidak dipancarkan (scope: server)", r.ID)
			continue
		}
		av, ok, err := b.cond("aturan "+r.ID, r.Assert, false)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		ro := tsObj{{"id", jsonLit(r.ID)}, {"assert", av}}
		if _, vis := ctx.fields[r.Field]; r.Field != "" && vis {
			ro = append(ro, tsKV{"field", jsonLit(r.Field)})
		}
		ro = append(ro, tsKV{"message", jsonLit(r.Message)})
		rules = append(rules, ro)
	}
	root = append(root, tsKV{"rules", rules})
	sections := tsArr{}
	for _, s := range m.Res.Sections {
		var order []string
		for _, n := range s.Order {
			if _, ok := ctx.fields[n]; ok {
				order = append(order, n)
			}
		}
		sections = append(sections, tsObj{{"key", jsonLit(s.Key)}, {"columns", tsLit(strconv.Itoa(s.Columns))}, {"order", tsStrings(order)}})
	}
	root = append(root, tsKV{"sections", sections})
	if m.LockWhen != nil && m.LockServer {
		b.omit("lockWhen tidak dipancarkan ke klien (scope server menurut check)")
	} else if m.LockWhen != nil {
		lv, ok, err := b.cond("lockWhen", m.LockWhen, false)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			root = append(root, tsKV{"lockWhen", lv})
		}
	}
	var sb strings.Builder
	sb.WriteString(GeneratedHeader + "\n\n")
	sb.WriteString("import type { ResourceMeta } from \"@erpora/platform-web/metadata\";\n\n")
	const prefix = "export const resourceMeta: ResourceMeta = "
	body, err := tsRender(root, 0, len(prefix))
	if err != nil {
		return nil, nil, err
	}
	sb.WriteString(prefix + body + ";\n")
	return []byte(sb.String()), b.omitted, nil
}

// hiddenPlaceholders mengembalikan nama placeholder {field} templat label yang bukan field yang dipancarkan ke klien.
func hiddenPlaceholders(tpl string, visible map[string]*fld) []string {
	var out []string
	for i := 0; i < len(tpl); i++ {
		if tpl[i] != '{' {
			continue
		}
		end := strings.IndexByte(tpl[i+1:], '}')
		if end < 0 {
			break
		}
		name := tpl[i+1 : i+1+end]
		if _, ok := visible[name]; !ok {
			out = append(out, name)
		}
		i += end + 1
	}
	return out
}

func isScalar(v any) bool {
	_, ok := v.(tsLit)
	return ok
}

// tsRender menata nilai: objek selalu diperluas; larik skalar dilipat bila muat dalam tsLineWidth kolom,
// selain itu satu elemen per baris; larik berisi objek selalu diperluas. col adalah kolom tempat nilai
// dimulai (untuk keputusan lipat).
func tsRender(v any, indent, col int) (string, error) {
	pad := strings.Repeat(" ", indent+2)
	switch t := v.(type) {
	case tsLit:
		return string(t), nil
	case tsObj:
		if len(t) == 0 {
			return "{}", nil
		}
		var sb strings.Builder
		sb.WriteString("{\n")
		for _, kv := range t {
			if !tsKeyRe.MatchString(kv.k) {
				return "", fmt.Errorf("kunci %q tidak aman", kv.k)
			}
			s, err := tsRender(kv.v, indent+2, len(pad)+len(kv.k)+2)
			if err != nil {
				return "", err
			}
			sb.WriteString(pad + kv.k + ": " + s + ",\n")
		}
		sb.WriteString(strings.Repeat(" ", indent) + "}")
		return sb.String(), nil
	case tsArr:
		if len(t) == 0 {
			return "[]", nil
		}
		allScalar := true
		for _, e := range t {
			allScalar = allScalar && isScalar(e)
		}
		if allScalar {
			parts := make([]string, len(t))
			for i, e := range t {
				parts[i] = string(e.(tsLit))
			}
			single := "[" + strings.Join(parts, ", ") + "]"
			if col+len(single)+1 <= tsLineWidth {
				return single, nil
			}
		}
		var sb strings.Builder
		sb.WriteString("[\n")
		for _, e := range t {
			s, err := tsRender(e, indent+2, len(pad))
			if err != nil {
				return "", err
			}
			sb.WriteString(pad + s + ",\n")
		}
		sb.WriteString(strings.Repeat(" ", indent) + "]")
		return sb.String(), nil
	}
	return "", fmt.Errorf("simpul keluaran bertipe %T tidak dikenal", v)
}
