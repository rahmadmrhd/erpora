package check

import (
	"strings"

	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

// exprRefs mengumpulkan nama field pada ekspresi dan kondisi di dalam if.
func exprRefs(e *metadata.Expr, into *[]string, seen map[string]bool) {
	if e == nil {
		return
	}
	if e.Kind == metadata.ExprField && !seen[e.Field] {
		seen[e.Field] = true
		*into = append(*into, e.Field)
	}
	for _, a := range e.Args {
		exprRefs(a, into, seen)
	}
	if e.Kind == metadata.ExprIf {
		condRefs(e.Cond, into, seen)
		exprRefs(e.Then, into, seen)
		exprRefs(e.Else, into, seen)
	}
}

// exprConditions mengumpulkan kondisi pada simpul if (untuk pemeriksaan tipe dan rujukan).
func exprConditions(e *metadata.Expr, into *[]*metadata.Condition) {
	if e == nil {
		return
	}
	for _, a := range e.Args {
		exprConditions(a, into)
	}
	if e.Kind == metadata.ExprIf {
		*into = append(*into, e.Cond)
		exprConditions(e.Then, into)
		exprConditions(e.Else, into)
	}
}

// checkAccessors memeriksa accessor expr, stored, dan func (Requirement 5.1 sampai 5.4, 5.8).
func (x *rctx) checkAccessors() {
	graph := map[string][]string{}
	for _, f := range x.r.Fields {
		acc, ok := f.Attrs["accessor"].(map[string]any)
		if !ok {
			continue
		}
		kind, _ := acc["kind"].(string)
		if ro, has := f.Attrs["readOnly"]; has {
			if b, _ := ro.(bool); !b {
				x.err(f, "readOnly", CodeAccessorNotReadOnly, "field accessor %s harus readOnly dan tidak pernah diikat dari input (Requirement 5.1)", f.Name)
			}
		}
		if kind != "stored" {
			for _, flag := range []string{"sortable", "filterable", "groupable"} {
				if f.Bool(flag) {
					x.err(f, flag, CodeAccessorFlagNotStored, "accessor %s (%s) tidak boleh %s: hanya accessor stored yang boleh sortable, filterable, atau groupable (Requirement 5.3, 5.4)", f.Name, kind, flag)
				}
			}
		}
		switch kind {
		case "func":
			if _, has := f.Attrs["fieldGroup"]; !has {
				x.err(f, "", CodeAccessorFuncNeedsGroup, "accessor func %s wajib menyatakan fieldGroup secara eksplisit (Requirement 5.8)", f.Name)
			}
			if _, has := f.Attrs["classification"]; !has {
				x.err(f, "", CodeAccessorFuncNeedsClassification, "accessor func %s wajib menyatakan classification secara eksplisit (Requirement 5.8)", f.Name)
			}
		case "expr", "stored":
			x.checkExprAccessor(f, acc, graph)
		}
	}
	x.reportCycles(graph, CodeAccessorCycle, "ketergantungan accessor melingkar: %s")
}

func (x *rctx) checkExprAccessor(f *resolve.Field, acc map[string]any, graph map[string][]string) {
	ptr := "/fields/" + f.Name + "/accessor/expr"
	origin := f.AttrOrigin["accessor"]
	expr, err := metadata.ParseExpr(acc["expr"])
	if err != nil {
		x.atOrigin(SeverityError, origin, ptr, CodeAccessorExprInvalid, "ekspresi accessor %s tidak sah: %s", f.Name, reasonText(err))
		return
	}
	var refs []string
	exprRefs(expr, &refs, map[string]bool{})
	var conds []*metadata.Condition
	exprConditions(expr, &conds)
	env := x.ownEnv(func(sev Severity, code, msg string) { x.atOrigin(sev, origin, ptr, code, "%s", msg) })
	for _, c := range conds {
		env.check(c, "$")
	}
	known := refs[:0:0]
	for _, n := range refs {
		in := x.fields[n]
		if in == nil {
			if !x.isAbs {
				x.atOrigin(SeverityError, origin, ptr, CodeAccessorUnknownField, "ekspresi accessor %s merujuk field %q yang tidak ada", f.Name, n)
			}
			continue
		}
		known = append(known, n)
		if a, ok := in.Attrs["accessor"].(map[string]any); ok {
			if k, _ := a["kind"].(string); k == "expr" || k == "stored" {
				graph[f.Name] = append(graph[f.Name], n)
			}
		}
	}
	if len(known) == 0 {
		return
	}
	// accessor mewarisi klasifikasi tertinggi dan grup paling ketat dari field masukan (Requirement 5.8)
	inClass, inGroup := "", ""
	for _, n := range known {
		inClass = maxClass(inClass, x.eff.class[n])
		if g := x.eff.group[n]; inGroup == "" || x.eff.groups.rank(g) > x.eff.groups.rank(inGroup) {
			inGroup = g
		}
	}
	if declared, has := f.Attrs["classification"].(string); has && classRank(declared) < classRank(inClass) {
		x.err(f, "classification", CodeAccessorLooserClassification,
			"accessor %s berklasifikasi %s, lebih longgar daripada field masukannya (%s); accessor mewarisi klasifikasi tertinggi (Requirement 5.8)", f.Name, declared, inClass)
	}
	if declared, has := f.Attrs["fieldGroup"].(string); has {
		if g := snake(declared); x.eff.groups.rank(g) < x.eff.groups.rank(inGroup) {
			x.err(f, "fieldGroup", CodeAccessorLooserGroup,
				"accessor %s berada di grup %s, lebih longgar daripada grup field masukannya (%s); accessor mewarisi grup paling ketat (Requirement 5.8)", f.Name, g, inGroup)
		}
	}
}

// checkLabelTemplate memeriksa templat label (Requirement 5.5, 5.6): placeholder hanya field resource,
// berada di grup default, dan berklasifikasi paling tinggi internal.
func (x *rctx) checkLabelTemplate() {
	tpl := x.r.LabelTemplate
	if tpl == "" {
		return
	}
	if _, err := metadata.ParseLabelTemplate(tpl); err != nil {
		x.atDoc(SeverityError, "/labelTemplate", CodeLabelTemplateInvalid, "labelTemplate %q tidak sah: %s", tpl, reasonText(err))
		return
	}
	for _, name := range placeholders(tpl) {
		f := x.fields[name]
		if f == nil {
			x.atDoc(SeverityError, "/labelTemplate", CodeLabelUnknownField, "labelTemplate merujuk field %q yang tidak ada", name)
			continue
		}
		if !x.eff.groups.isDefault(x.eff.group[name]) {
			x.atDoc(SeverityError, "/labelTemplate", CodeLabelFieldGroup,
				"labelTemplate memakai field %s di grup %s; field pada label wajib di grup default (label dikembalikan kepada siapa pun yang boleh membaca record; Requirement 5.6)", name, x.eff.group[name])
		}
		if classRank(x.eff.class[name]) > classRank("internal") {
			x.atDoc(SeverityError, "/labelTemplate", CodeLabelFieldClassification,
				"labelTemplate memakai field %s berklasifikasi %s; field pada label wajib berklasifikasi paling tinggi internal tanpa pengecualian (Requirement 5.6)", name, x.eff.class[name])
		}
	}
}

// placeholders mengembalikan nama placeholder {nama} pada templat yang sudah lolos ParseLabelTemplate.
func placeholders(tpl string) []string {
	var out []string
	for {
		i := strings.IndexByte(tpl, '{')
		if i < 0 {
			return out
		}
		j := strings.IndexByte(tpl[i:], '}')
		if j < 0 {
			return out
		}
		out = append(out, tpl[i+1:i+j])
		tpl = tpl[i+j+1:]
	}
}
