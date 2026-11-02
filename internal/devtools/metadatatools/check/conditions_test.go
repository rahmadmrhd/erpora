package check

import (
	"strings"
	"testing"
)

// base adalah resource uji dengan beragam tipe dan grup; body tambahan disisipkan di akhir.
func baseResource(extraFields, tail string) file {
	return f("m/r.resource.yaml", resource("m.r", `fields:
  status: {type: enum, values: [draft, open, done], default: draft}
  qty: {type: integer, minimum: 0}
  total: {type: decimal, precision: 18, scale: 2, fieldGroup: pricing, classification: confidential}
  price: {type: decimal, precision: 18, scale: 2}
  note: {type: text, nullable: true}
  flag: {type: boolean, default: false}
  issuedOn: {type: date}
  dueOn: {type: date}
  number: {type: string, maxLength: 32}
  customerName: {type: string, maxLength: 100}
`+extraFields+tail))
}

func TestConditionsOnAllSitesPass(t *testing.T) {
	rep := single(t, baseResource(`  approval:
    type: text
    nullable: true
    visibleWhen: {any: [{field: status, op: eq, value: open}, {not: {field: qty, op: isnull, value: true}}]}
    requiredWhen: {field: price, op: gte, value: "100.00"}
    readonlyWhen: {field: status, op: in, value: [done]}
`, `rules:
  - id: dueAfterIssue
    assert: {field: dueOn, op: gte, ref: issuedOn}
    field: dueOn
    message: m.r.error.due
lockWhen: {field: status, op: eq, value: done}
`))
	noErrorsExceptNonDocLock(t, rep)
}

func TestConditionUnknownFieldsAndRefs(t *testing.T) {
	tests := []struct {
		name, extra, tail, code, ptr string
	}{
		{"visibleWhen field", "  a: {type: text, nullable: true, visibleWhen: {field: nope, op: eq, value: x}}\n", "", CodeConditionUnknownField, "/fields/a/visibleWhen"},
		{"requiredWhen field", "  a: {type: text, nullable: true, requiredWhen: {field: nope, op: eq, value: x}}\n", "", CodeConditionUnknownField, "/fields/a/requiredWhen"},
		{"readonlyWhen ref", "  a: {type: text, nullable: true, readonlyWhen: {field: status, op: eq, ref: nope}}\n", "", CodeConditionUnknownRef, "/fields/a/readonlyWhen"},
		{"lockWhen", "", "lockWhen: {field: nope, op: eq, value: x}\n", CodeConditionUnknownField, "/lockWhen"},
		{"rule assert", "", "rules:\n  - {id: r1, assert: {field: nope, op: isnull, value: false}, message: m.r.err}\n", CodeConditionUnknownField, "/rules/0/assert"},
		{"rule ref", "", "rules:\n  - {id: r1, assert: {field: dueOn, op: gte, ref: nope}, message: m.r.err}\n", CodeConditionUnknownRef, "/rules/0/assert"},
		{"rule field target", "", "rules:\n  - {id: r1, assert: {field: qty, op: gte, value: 0}, field: nope, message: m.r.err}\n", CodeConditionUnknownField, "/rules/0/field"},
		{"nested not", "  a: {type: text, nullable: true, visibleWhen: {all: [{not: {field: nope, op: isnull, value: true}}]}}\n", "", CodeConditionUnknownField, "/fields/a/visibleWhen"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantErr(t, single(t, baseResource(tc.extra, tc.tail)), tc.code, tc.ptr)
		})
	}
}

func TestConditionStructureAndTypes(t *testing.T) {
	tests := []struct {
		name, cond, code string
	}{
		{"isnull dengan ref", "{field: note, op: isnull, ref: flag}", CodeConditionInvalid},
		{"gt pada boolean", "{field: flag, op: gt, value: true}", CodeConditionOpType},
		{"contains pada integer", "{field: qty, op: contains, value: \"1\"}", CodeConditionOpType},
		{"lt pada enum", "{field: status, op: lt, value: open}", CodeConditionOpType},
		{"integer dibandingkan teks", "{field: qty, op: eq, value: abc}", CodeConditionValueType},
		{"boolean dibandingkan teks", "{field: flag, op: eq, value: \"yes\"}", CodeConditionValueType},
		{"desimal dibandingkan bilangan", "{field: price, op: gte, value: 1.5}", CodeConditionValueType},
		{"desimal bukan desimal", "{field: price, op: gte, value: \"abc\"}", CodeConditionValueType},
		{"tanggal tidak sah", "{field: dueOn, op: eq, value: \"2026-02-30\"}", CodeConditionValueType},
		{"enum di luar values", "{field: status, op: eq, value: unknown}", CodeConditionValueType},
		{"in enum salah satu di luar values", "{field: status, op: in, value: [draft, ghost]}", CodeConditionValueType},
		{"ref tipe berbeda", "{field: qty, op: eq, ref: dueOn}", CodeConditionRefType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t, baseResource("  a: {type: text, nullable: true, visibleWhen: "+tc.cond+"}\n", ""))
			wantErr(t, rep, tc.code, "/fields/a/visibleWhen")
		})
	}
	t.Run("kombinasi sah", func(t *testing.T) {
		rep := single(t, baseResource(`  a:
    type: text
    nullable: true
    visibleWhen:
      all:
        - {field: qty, op: gte, value: 1}
        - {field: price, op: lt, value: "10.5"}
        - {field: dueOn, op: gt, value: "2026-01-01"}
        - {field: number, op: contains, value: A}
        - {field: status, op: in, value: [draft, open]}
        - {field: qty, op: gt, ref: qty}
`, ""))
		noCode(t, rep, CodeConditionOpType)
		noCode(t, rep, CodeConditionValueType)
		noCode(t, rep, CodeConditionRefType)
	})
}

func TestConditionCycles(t *testing.T) {
	t.Run("siklus dua field", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: string, maxLength: 5, nullable: true, visibleWhen: {field: b, op: isnull, value: false}}
  b: {type: string, maxLength: 5, nullable: true, requiredWhen: {field: a, op: isnull, value: false}}
`)))
		p := wantErr(t, rep, CodeConditionCycle, "/fields/")
		if !strings.Contains(p.Message, "a") || !strings.Contains(p.Message, "b") {
			t.Errorf("pesan harus menyebut anggota siklus: %q", p.Message)
		}
	})
	t.Run("rujukan diri sendiri", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: string, maxLength: 5, nullable: true, visibleWhen: {field: a, op: isnull, value: false}}\n")))
		wantErr(t, rep, CodeConditionCycle, "/fields/a")
	})
	t.Run("siklus tiga field", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: boolean, readonlyWhen: {field: b, op: eq, value: true}}
  b: {type: boolean, readonlyWhen: {field: c, op: eq, value: true}}
  c: {type: boolean, readonlyWhen: {field: a, op: eq, value: true}}
`)))
		wantErr(t, rep, CodeConditionCycle, "/fields/")
	})
	t.Run("tanpa siklus (rantai)", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: boolean, readonlyWhen: {field: b, op: eq, value: true}}
  b: {type: boolean, readonlyWhen: {field: c, op: eq, value: true}}
  c: {type: boolean}
`)))
		noErrors(t, rep)
	})
}

func TestSingleConditionModelEverywhere(t *testing.T) {
	// field, rules, lockWhen memakai bentuk yang sama; bentuk lain ditolak sejak skema (tanpa mekanisme logika lain).
	rep := single(t, baseResource("", "lockWhen: {all: [{field: status, op: ne, value: draft}, {field: flag, op: eq, value: true}]}\n"))
	noErrorsExceptNonDocLock(t, rep)
}

func TestRuleScopeIsAutomaticForSensitiveFieldsAndGroups(t *testing.T) {
	rep := single(t, baseResource("", `rules:
  - {id: open, assert: {field: dueOn, op: gte, ref: issuedOn}, field: dueOn, message: m.r.err1}
  - {id: grouped, assert: {field: total, op: gte, value: "0"}, field: total, message: m.r.err2, scope: server}
  - {id: classified, assert: {field: creditLimit, op: gte, value: "0"}, field: creditLimit, message: m.r.err3}
  - {id: explicitBoth, assert: {field: total, op: lte, value: "999"}, message: m.r.err4, scope: both}
`+"\n"))
	// creditLimit tidak ada: abaikan temuan itu; fokus pada scope.
	info := rep.Resource("m.r")
	if info == nil {
		t.Fatal("ResourceInfo tidak ada")
	}
	if info.RuleScope["open"] != ScopeBoth {
		t.Errorf("open = %q", info.RuleScope["open"])
	}
	if info.RuleScope["grouped"] != ScopeServer || info.RuleScope["explicitBoth"] != ScopeServer {
		t.Errorf("scope = %v", info.RuleScope)
	}
	wantErr(t, rep, CodeRuleScopeConflict, "/rules/3")
	wantWarning(t, rep, CodeRuleRefsNonDefaultGroup, "/rules/1")
	for _, p := range rep.Problems {
		if p.Code == CodeRuleRefsNonDefaultGroup && strings.Contains(p.Pointer, "/rules/0") {
			t.Errorf("rule default tidak diperingatkan: %+v", p)
		}
	}
}

func TestRuleReferencingConfidentialFieldInDefaultGroupIsServer(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  limit1: {type: decimal, precision: 18, scale: 2, classification: confidential}
rules:
  - {id: capRule, assert: {field: limit1, op: gte, value: "0"}, field: limit1, message: m.r.err}
`)))
	noErrors(t, rep)
	if got := rep.Resource("m.r").RuleScope["capRule"]; got != ScopeServer {
		t.Fatalf("scope = %q", got)
	}
	// Kondisi tingkat field yang merujuk field sensitif juga hanya server.
	rep = single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  confLevel: {type: integer, classification: confidential}
  shown: {type: text, nullable: true, visibleWhen: {field: confLevel, op: gt, value: 5}}
  plain: {type: text, nullable: true, visibleWhen: {field: shown, op: isnull, value: false}}
lockWhen: {field: confLevel, op: eq, value: 9}
`)))
	noErrorsExceptNonDocLock(t, rep)
	info := rep.Resource("m.r")
	if info.ConditionScope["/fields/shown/visibleWhen"] != ScopeServer || info.ConditionScope["/fields/plain/visibleWhen"] != ScopeBoth ||
		info.ConditionScope["/lockWhen"] != ScopeServer {
		t.Errorf("ConditionScope = %v", info.ConditionScope)
	}
}

func TestAccessorsValid(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  price: {type: decimal, precision: 18, scale: 2}
  qty: {type: decimal, precision: 18, scale: 3}
  name: {type: string, maxLength: 50}
  total:
    type: decimal
    precision: 18
    scale: 2
    accessor: {kind: stored, expr: {mul: [{field: price}, {field: qty}], scale: 2, round: half_up}}
    sortable: true
    filterable: true
  summary:
    type: string
    maxLength: 100
    readOnly: true
    accessor: {kind: expr, expr: {concat: [{field: name}, {lit: " / "}, {coalesce: [{field: name}, {lit: "-"}]}]}}
  labelFn:
    type: string
    maxLength: 50
    fieldGroup: general
    classification: internal
    accessor: {kind: func, func: labelOf}
`)))
	noErrors(t, rep)
}

func TestAccessorViolations(t *testing.T) {
	pre := `fields:
  price: {type: decimal, precision: 18, scale: 2, fieldGroup: pricing, classification: confidential}
  name: {type: string, maxLength: 50}
`
	tests := []struct {
		name, field, code string
	}{
		{"readOnly false", "  a: {type: string, maxLength: 5, readOnly: false, accessor: {kind: expr, expr: {field: name}}}\n", CodeAccessorNotReadOnly},
		{"expr tidak sah (dua bentuk)", "  a: {type: string, maxLength: 5, accessor: {kind: expr, expr: {field: name, lit: 1}}}\n", CodeAccessorExprInvalid},
		{"aritmetika tanpa skala dan pembulatan", "  a: {type: decimal, precision: 10, scale: 2, accessor: {kind: expr, expr: {add: [{field: price}, {dec: \"1\"}]}}}\n", CodeAccessorExprInvalid},
		{"field di expr tidak ada", "  a: {type: string, maxLength: 5, accessor: {kind: expr, expr: {field: ghost}}}\n", CodeAccessorUnknownField},
		{"kondisi di if merujuk field tak ada", "  a: {type: string, maxLength: 5, accessor: {kind: expr, expr: {if: {field: ghost, op: eq, value: x}, then: {lit: a}, else: {lit: b}}}}\n", CodeConditionUnknownField},
		{"expr sortable", "  a: {type: string, maxLength: 5, sortable: true, accessor: {kind: expr, expr: {field: name}}}\n", CodeAccessorFlagNotStored},
		{"expr filterable", "  a: {type: string, maxLength: 5, filterable: true, accessor: {kind: expr, expr: {field: name}}}\n", CodeAccessorFlagNotStored},
		{"func groupable", "  a: {type: string, maxLength: 5, groupable: true, fieldGroup: general, classification: internal, accessor: {kind: func, func: fn}}\n", CodeAccessorFlagNotStored},
		{"func tanpa grup", "  a: {type: string, maxLength: 5, classification: internal, accessor: {kind: func, func: fn}}\n", CodeAccessorFuncNeedsGroup},
		{"func tanpa klasifikasi", "  a: {type: string, maxLength: 5, fieldGroup: general, accessor: {kind: func, func: fn}}\n", CodeAccessorFuncNeedsClassification},
		{"klasifikasi lebih longgar dari masukan", "  a: {type: decimal, precision: 10, scale: 2, classification: internal, fieldGroup: pricing, accessor: {kind: expr, expr: {mul: [{field: price}, {dec: \"2\"}], scale: 2, round: half_up}}}\n", CodeAccessorLooserClassification},
		{"grup lebih longgar dari masukan", "  a: {type: decimal, precision: 10, scale: 2, classification: confidential, fieldGroup: default, accessor: {kind: expr, expr: {mul: [{field: price}, {dec: \"2\"}], scale: 2, round: half_up}}}\n", CodeAccessorLooserGroup},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t, f("m/r.resource.yaml", resource("m.r", pre+tc.field)))
			wantErr(t, rep, tc.code, "/fields/a")
		})
	}
}

func TestAccessorInheritsStrictestClassificationAndGroup(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  price: {type: decimal, precision: 18, scale: 2, fieldGroup: pricing, classification: confidential}
  qty: {type: decimal, precision: 18, scale: 3}
  margin:
    type: decimal
    precision: 18
    scale: 2
    accessor: {kind: expr, expr: {mul: [{field: price}, {field: qty}], scale: 2, round: half_up}}
  plainCalc:
    type: decimal
    precision: 18
    scale: 2
    accessor: {kind: expr, expr: {add: [{field: qty}, {dec: "1"}], scale: 2, round: half_up}}
`)))
	noErrors(t, rep)
	info := rep.Resource("m.r")
	if info.Class["margin"] != "confidential" || info.Group["margin"] != "pricing" {
		t.Errorf("margin = %s/%s", info.Class["margin"], info.Group["margin"])
	}
	if info.Class["plainCalc"] != "internal" || info.Group["plainCalc"] != "default" {
		t.Errorf("both = %s/%s", info.Class["plainCalc"], info.Group["plainCalc"])
	}
	if info.Class["price"] != "confidential" || info.Group["price"] != "pricing" || info.Group["qty"] != "default" {
		t.Errorf("field biasa: %v %v", info.Class, info.Group)
	}
}

func TestAccessorChainInheritsTransitively(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  cost: {type: decimal, precision: 18, scale: 2, classification: confidential}
  step1: {type: decimal, precision: 18, scale: 2, accessor: {kind: expr, expr: {add: [{field: cost}, {dec: "1"}], scale: 2, round: half_up}}}
  step2: {type: decimal, precision: 18, scale: 2, accessor: {kind: expr, expr: {add: [{field: step1}, {dec: "1"}], scale: 2, round: half_up}}}
`)))
	noErrors(t, rep)
	if rep.Resource("m.r").Class["step2"] != "confidential" {
		t.Errorf("Class = %v", rep.Resource("m.r").Class)
	}
}

func TestAccessorCycles(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: integer, accessor: {kind: expr, expr: {add: [{field: b}, {lit: 1}], scale: 0, round: half_up}}}
  b: {type: integer, accessor: {kind: expr, expr: {add: [{field: a}, {lit: 1}], scale: 0, round: half_up}}}
  c: {type: integer, accessor: {kind: expr, expr: {add: [{field: c}, {lit: 1}], scale: 0, round: half_up}}}
`)))
	wantErr(t, rep, CodeAccessorCycle, "/fields/")
}

func TestLabelTemplate(t *testing.T) {
	pre := `fields:
  number: {type: string, maxLength: 32}
  customerName: {type: string, maxLength: 100}
  confNo: {type: string, maxLength: 20, classification: confidential}
  inPricing: {type: string, maxLength: 20, fieldGroup: pricing}
  lbl: {type: string, maxLength: 50, accessor: {kind: expr, expr: {concat: [{field: number}, {lit: "-"}, {field: customerName}]}}}
  lblConf: {type: string, maxLength: 50, accessor: {kind: expr, expr: {concat: [{field: confNo}, {lit: "!"}]}}}
`
	ok := single(t, f("m/r.resource.yaml", resource("m.r", "labelTemplate: \"{number} / {customerName} ({lbl})\"\n"+pre)))
	noErrors(t, ok)
	tests := []struct {
		name, tpl, code string
	}{
		{"field tidak ada", "{number} - {ghost}", CodeLabelUnknownField},
		{"field confidential", "{number} - {confNo}", CodeLabelFieldClassification},
		{"field grup non-default", "{number} - {inPricing}", CodeLabelFieldGroup},
		{"accessor mewarisi confidential", "{lblConf}", CodeLabelFieldClassification},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t, f("m/r.resource.yaml", resource("m.r", "labelTemplate: \""+tc.tpl+"\"\n"+pre)))
			wantErr(t, rep, tc.code, "/labelTemplate")
		})
	}
}
