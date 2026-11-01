package check

import (
	"fmt"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

// condEnv menjelaskan lingkungan pemeriksaan satu kondisi: dari mana field pada daun dan ref diselesaikan dan
// kode galat yang dipakai (kondisi pada filter relasi merujuk field resource seberang).
type condEnv struct {
	x *rctx
	// fieldOf menyelesaikan nama field pada daun; refOf menyelesaikan ref (field resource sendiri).
	fieldOf func(name string) *resolve.Field
	refOf   func(name string) (metadata.FieldType, bool)
	// permissive: nama yang tidak ditemukan diabaikan (abstrak dapat merujuk field resource penyertaan).
	permissive        bool
	codeUnknownField  string
	codeUnknownRef    string
	requireFilterable bool
	// loc menerbitkan temuan di lokasi sumber kondisi.
	loc func(sev Severity, code, msg string)
}

func familyOf(t metadata.FieldType) string {
	switch t {
	case metadata.TypeString, metadata.TypeText:
		return "text"
	case metadata.TypeInteger, metadata.TypeDecimal:
		return "num"
	}
	return string(t)
}

func orderable(t metadata.FieldType) bool {
	switch t {
	case metadata.TypeString, metadata.TypeText, metadata.TypeInteger, metadata.TypeDecimal, metadata.TypeDate, metadata.TypeDateTime:
		return true
	}
	return false
}

func textual(t metadata.FieldType) bool { return t == metadata.TypeString || t == metadata.TypeText }

func opSupported(op metadata.Op, t metadata.FieldType) bool {
	switch op {
	case metadata.OpGt, metadata.OpGte, metadata.OpLt, metadata.OpLte:
		return orderable(t)
	case metadata.OpContains:
		return textual(t)
	}
	return true
}

// literalOnly adalah batasan minimal untuk literal pada kondisi: tipe dan anggota enum (tanpa panjang, skala,
// rentang, atau pola field).
func literalOnly(f *resolve.Field, x *rctx) constraints {
	c := x.constraintsOf(f)
	return constraints{typ: c.typ, nullable: true, enumValues: c.enumValues, hasEnum: c.hasEnum}
}

func reasonText(err error) string {
	var me *metadata.Error
	if asMetaError(err, &me) {
		code := map[string]string{
			metadata.ReasonInvalidCondition:  "bentuk kondisi tidak sah",
			metadata.ReasonInvalidExpression: "bentuk ekspresi tidak sah (tata bahasa tertutup: lit, dec, field, concat, coalesce, add, sub, mul, div dengan scale dan round, if)",
			metadata.ReasonInvalidTemplate:   "templat label tidak sah",
			metadata.ReasonTooDeep:           "kedalaman melebihi batas",
			metadata.ReasonTooManyNodes:      "jumlah simpul melebihi batas",
			metadata.ReasonListTooLong:       "daftar nilai melebihi batas",
		}[me.Reason.Code]
		if code == "" {
			code = me.Reason.Code
		}
		return fmt.Sprintf("%s (di %s)", code, me.Reason.Path)
	}
	return err.Error()
}

func asMetaError(err error, target **metadata.Error) bool {
	e, ok := err.(*metadata.Error) //nolint:errorlint // ParseCondition/ParseExpr mengembalikan *Error langsung
	if ok {
		*target = e
	}
	return ok
}

// check memeriksa pohon kondisi terhadap field yang dirujuk.
func (e *condEnv) check(c *metadata.Condition, path string) {
	if c == nil {
		return
	}
	switch c.Kind {
	case metadata.CondAll, metadata.CondAny:
		key := "all"
		if c.Kind == metadata.CondAny {
			key = "any"
		}
		for i, ch := range c.Children {
			e.check(ch, fmt.Sprintf("%s.%s[%d]", path, key, i))
		}
		return
	case metadata.CondNot:
		e.check(c.Children[0], path+".not")
		return
	}
	f := e.fieldOf(c.Field)
	if f == nil {
		if !e.permissive {
			e.loc(SeverityError, e.codeUnknownField, fmt.Sprintf("kondisi %s: field %q tidak ada", path, c.Field))
		}
		return
	}
	typ := metadata.FieldType(f.Type())
	if e.requireFilterable && !f.Bool("filterable") {
		e.loc(SeverityError, CodeRelationFilterNotFilterable, fmt.Sprintf("kondisi %s: field seberang %q tidak filterable (Requirement 6.7)", path, c.Field))
	}
	if !opSupported(c.Op, typ) {
		e.loc(SeverityError, CodeConditionOpType, fmt.Sprintf("kondisi %s: operator %s tidak berlaku untuk field %s bertipe %s", path, c.Op, c.Field, typ))
		return
	}
	lc := literalOnly(f, e.x)
	switch {
	case c.Op == metadata.OpIsNull:
	case c.Ref != "":
		rt, ok := e.refOf(c.Ref)
		if !ok {
			if !e.permissive {
				e.loc(SeverityError, e.codeUnknownRef, fmt.Sprintf("kondisi %s: ref %q tidak merujuk field yang ada", path, c.Ref))
			}
			return
		}
		if !opSupported(c.Op, rt) {
			e.loc(SeverityError, CodeConditionOpType, fmt.Sprintf("kondisi %s: operator %s tidak berlaku untuk ref %s bertipe %s", path, c.Op, c.Ref, rt))
			return
		}
		if familyOf(typ) != familyOf(rt) {
			e.loc(SeverityError, CodeConditionRefType, fmt.Sprintf("kondisi %s: field %s (%s) dan ref %s (%s) berbeda keluarga tipe", path, c.Field, typ, c.Ref, rt))
		}
	case c.Op == metadata.OpIn:
		list, _ := c.Value.([]any)
		for i, el := range list {
			if msg := lc.checkLiteral(el); msg != "" {
				e.loc(SeverityError, CodeConditionValueType, fmt.Sprintf("kondisi %s.value[%d] untuk field %s (%s): %s", path, i, c.Field, typ, msg))
			}
		}
	case c.Value != nil:
		if msg := lc.checkLiteral(c.Value); msg != "" {
			e.loc(SeverityError, CodeConditionValueType, fmt.Sprintf("kondisi %s: nilai untuk field %s (%s) tidak sah: %s", path, c.Field, typ, msg))
		}
	}
}

// condRefs mengumpulkan nama field dan ref pada kondisi (urutan kemunculan, tanpa duplikat).
func condRefs(c *metadata.Condition, into *[]string, seen map[string]bool) {
	if c == nil {
		return
	}
	if c.Kind == metadata.CondLeaf {
		for _, n := range []string{c.Field, c.Ref} {
			if n != "" && !seen[n] {
				seen[n] = true
				*into = append(*into, n)
			}
		}
		return
	}
	for _, ch := range c.Children {
		condRefs(ch, into, seen)
	}
}

// ownEnv membuat lingkungan untuk kondisi yang merujuk field resource sendiri.
func (x *rctx) ownEnv(loc func(sev Severity, code, msg string)) *condEnv {
	return &condEnv{
		x:       x,
		fieldOf: func(n string) *resolve.Field { return x.fields[n] },
		refOf: func(n string) (metadata.FieldType, bool) {
			if f := x.fields[n]; f != nil {
				return metadata.FieldType(f.Type()), true
			}
			return "", false
		},
		permissive:       x.isAbs,
		codeUnknownField: CodeConditionUnknownField,
		codeUnknownRef:   CodeConditionUnknownRef,
		loc:              loc,
	}
}

// needsServer menyatakan kondisi merujuk field berklasifikasi confidential ke atas atau berada di luar grup
// default (Requirement 4.6); nonDefault memuat nama field di luar grup default.
func (x *rctx) needsServer(refs []string) (server bool, nonDefault []string) {
	for _, n := range refs {
		if x.fields[n] == nil {
			continue
		}
		if classRank(x.eff.class[n]) >= classRank("confidential") {
			server = true
		}
		if !x.eff.groups.isDefault(x.eff.group[n]) {
			server = true
			nonDefault = append(nonDefault, n)
		}
	}
	return server, nonDefault
}

// checkFieldConditions memeriksa visibleWhen, requiredWhen, dan readonlyWhen semua field serta siklus
// ketergantungan antarkondisi (Requirement 4.2, 4.3).
func (x *rctx) checkFieldConditions() {
	graph := map[string][]string{}
	for _, f := range x.r.Fields {
		for _, attr := range []string{"visibleWhen", "requiredWhen", "readonlyWhen"} {
			raw, ok := f.Attrs[attr]
			if !ok {
				continue
			}
			ptr := "/fields/" + f.Name + "/" + attr
			origin := f.AttrOrigin[attr]
			cond, err := metadata.ParseCondition(raw)
			if err != nil {
				x.atOrigin(SeverityError, origin, ptr, CodeConditionInvalid, "%s pada field %s tidak sah: %s", attr, f.Name, reasonText(err))
				continue
			}
			env := x.ownEnv(func(sev Severity, code, msg string) { x.atOrigin(sev, origin, ptr, code, "%s", msg) })
			env.check(cond, "$")
			var refs []string
			condRefs(cond, &refs, map[string]bool{})
			graph[f.Name] = append(graph[f.Name], refs...)
			if x.info != nil {
				server, _ := x.needsServer(refs)
				x.info.ConditionScope[ptr] = scopeOf(server)
			}
		}
	}
	x.reportCycles(graph, CodeConditionCycle, "ketergantungan kondisi melingkar antar field: %s (visibleWhen, requiredWhen, dan readonlyWhen tidak boleh saling bergantung)")
}

func scopeOf(server bool) string {
	if server {
		return ScopeServer
	}
	return ScopeBoth
}

// reportCycles melaporkan komponen terhubung kuat berukuran > 1 atau berlingkar diri pada graf field.
func (x *rctx) reportCycles(graph map[string][]string, code, format string) {
	for _, comp := range sccs(x.fieldNames(), graph) {
		first := x.fields[comp[0]]
		if first == nil {
			continue
		}
		x.err(first, "", code, format, strings.Join(comp, " -> ")+" -> "+comp[0])
	}
}

// sccs mengembalikan komponen siklik (ukuran > 1 atau berlingkar diri) dengan urutan simpul deterministik.
func sccs(order []string, graph map[string][]string) [][]string {
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var out [][]string
	next := 0
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	var strong func(v string)
	strong = func(v string) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range graph[v] {
			if _, known := pos[w]; !known {
				continue
			}
			if _, seen := index[w]; !seen {
				strong(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			selfLoop := false
			for _, w := range graph[v] {
				if w == v {
					selfLoop = true
				}
			}
			if len(comp) > 1 || selfLoop {
				sort.Slice(comp, func(i, j int) bool { return pos[comp[i]] < pos[comp[j]] })
				out = append(out, comp)
			}
		}
	}
	for _, n := range order {
		if _, seen := index[n]; !seen {
			strong(n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return pos[out[i][0]] < pos[out[j][0]] })
	return out
}

// checkRules memeriksa aturan lintas field, menentukan scope efektif, dan memperingatkan aturan yang merujuk
// field di luar grup default (Requirement 4.3, 4.6, 9.8).
func (x *rctx) checkRules() {
	for i, rule := range x.r.Rules {
		ptr := fmt.Sprintf("/rules/%d", i)
		raw, ok := rule.Attrs["assert"]
		if !ok {
			continue
		}
		cond, err := metadata.ParseCondition(raw)
		if err != nil {
			x.atOrigin(SeverityError, rule.Origin, ptr+"/assert", CodeConditionInvalid, "assert aturan %s tidak sah: %s", rule.ID, reasonText(err))
			continue
		}
		env := x.ownEnv(func(sev Severity, code, msg string) { x.atOrigin(sev, rule.Origin, ptr+"/assert", code, "%s", msg) })
		env.check(cond, "$")
		if tf, ok := rule.Attrs["field"].(string); ok && x.fields[tf] == nil && !x.isAbs {
			x.atOrigin(SeverityError, rule.Origin, ptr+"/field", CodeConditionUnknownField, "aturan %s: field tujuan %q tidak ada", rule.ID, tf)
		}
		var refs []string
		condRefs(cond, &refs, map[string]bool{})
		server, nonDefault := x.needsServer(refs)
		declared, _ := rule.Attrs["scope"].(string)
		if declared == ScopeBoth && server {
			x.atOrigin(SeverityError, rule.Origin, ptr, CodeRuleScopeConflict,
				"aturan %s dinyatakan scope: both tetapi merujuk field berklasifikasi confidential ke atas atau di luar grup default sehingga wajib scope: server (Requirement 4.6)", rule.ID)
		}
		if x.info != nil {
			x.info.RuleScope[rule.ID] = scopeOf(server || declared == ScopeServer)
		}
		if len(nonDefault) > 0 {
			x.atOrigin(SeverityWarning, rule.Origin, ptr, CodeRuleRefsNonDefaultGroup,
				"aturan %s merujuk field di luar grup default (%s): galat validasi dapat menjadi oracle nilai field itu lewat percobaan berulang; risiko diterima dengan rate limit pada endpoint tulis (Requirement 9.8)",
				rule.ID, joinNames(nonDefault))
		}
	}
}

// checkLockWhen memeriksa kondisi penguncian hasil gabungan (Requirement 4.3, 13.1).
func (x *rctx) checkLockWhen() {
	if x.r.LockWhen == nil {
		return
	}
	cond, err := metadata.ParseCondition(x.r.LockWhen)
	origin := resolve.Origin{File: x.r.Source.File, Line: 1, Column: 1}
	if len(x.r.LockWhenOrigins) > 0 {
		origin = x.r.LockWhenOrigins[len(x.r.LockWhenOrigins)-1]
	}
	if err != nil {
		x.atOrigin(SeverityError, origin, "/lockWhen", CodeConditionInvalid, "lockWhen tidak sah: %s", reasonText(err))
		return
	}
	env := x.ownEnv(func(sev Severity, code, msg string) { x.atOrigin(sev, origin, "/lockWhen", code, "%s", msg) })
	env.check(cond, "$")
	var refs []string
	condRefs(cond, &refs, map[string]bool{})
	if x.info != nil {
		server, _ := x.needsServer(refs)
		x.info.ConditionScope["/lockWhen"] = scopeOf(server)
	}
}
