package check

import (
	"strings"
	"testing"
)

const docHead = "include:\n  - source: sdk:document\nfields:\n  total: {type: integer}\n"

func axesResource(axes string) file {
	return f("m/so.resource.yaml", resource("m.so", docHead+"statusAxes:\n"+axes))
}

const validAxes = `  billingStatus:
    values: [to_bill, billed]
    initial: to_bill
    transitions:
      - {from: to_bill, to: billed}
      - {from: billed, to: to_bill}
  deliveryStatus:
    values: [to_deliver, partially_delivered, delivered]
    initial: to_deliver
    transitions:
      - {from: to_deliver, to: partially_delivered}
      - {from: partially_delivered, to: delivered}
      - {from: delivered, to: to_deliver}
      - {from: partially_delivered, to: to_deliver}
`

func TestStatusAxesValidPass(t *testing.T) {
	rep := single(t, axesResource(validAxes))
	noErrors(t, rep)
	info := rep.Resource("m.so")
	if info.Class["deliveryStatus"] != "internal" || info.Group["billingStatus"] != "default" {
		t.Errorf("sumbu = %v %v", info.Class["deliveryStatus"], info.Group["billingStatus"])
	}
}

func TestStatusAxesSemanticViolations(t *testing.T) {
	tests := []struct {
		name, axes, code, ptr string
	}{
		{"initial di luar values", "  flow: {values: [a, b], initial: c, transitions: []}\n", CodeStatusAxisInitial, "/statusAxes/flow"},
		{"from di luar values", "  flow: {values: [a, b], initial: a, transitions: [{from: x, to: b}]}\n", CodeStatusAxisTransition, "/statusAxes/flow"},
		{"to di luar values", "  flow: {values: [a, b], initial: a, transitions: [{from: a, to: x}]}\n", CodeStatusAxisTransition, "/statusAxes/flow"},
		{"pasangan ganda", "  flow: {values: [a, b], initial: a, transitions: [{from: a, to: b}, {from: a, to: b}]}\n", CodeStatusAxisTransitionDuplicate, "/statusAxes/flow"},
		{"bernama status", "  status: {values: [a], initial: a, transitions: []}\n", "duplicate_field", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t, axesResource(tc.axes))
			p := wantErr(t, rep, tc.code, tc.ptr)
			if tc.ptr != "" && (p.File != "m/so.resource.yaml" || p.Line == 0) {
				t.Errorf("lokasi = %s:%d", p.File, p.Line)
			}
		})
	}
	t.Run("pasangan mundur dan siklik sah", func(t *testing.T) {
		noErrors(t, single(t, axesResource("  flow: {values: [a, b, c], initial: a, transitions: [{from: a, to: b}, {from: b, to: c}, {from: c, to: a}, {from: b, to: a}]}\n")))
	})
	t.Run("tanpa transisi sah", func(t *testing.T) {
		noErrors(t, single(t, axesResource("  flow: {values: [a], initial: a, transitions: []}\n")))
	})
}

func TestStatusAxesOnNonDocumentRejected(t *testing.T) {
	rep := single(t, f("m/p.resource.yaml", resource("m.p", "statusAxes:\n  flow: {values: [a, b], initial: a, transitions: [{from: a, to: b}]}\n")))
	p := wantErr(t, rep, CodeStatusAxisNotDocument, "/statusAxes/flow")
	if !strings.Contains(p.Message, "sdk:document") {
		t.Errorf("pesan harus menyebut sdk:document: %q", p.Message)
	}
}

func TestStatusAxisNamedStatusOnNonDocumentRejected(t *testing.T) {
	rep := single(t, f("m/p.resource.yaml", resource("m.p", "statusAxes:\n  status: {values: [a], initial: a, transitions: []}\n")))
	wantErr(t, rep, CodeStatusAxisName, "/statusAxes/status")
}

func TestStatusAxisNameRules(t *testing.T) {
	t.Run("bentrokan nama dan bolak-balik mengikuti aturan penamaan", func(t *testing.T) {
		rep := single(t, axesResource("  xmin: {values: [a], initial: a, transitions: []}\n"))
		wantErr(t, rep, CodeNameReservedSQL, "/fields/xmin")
	})
	t.Run("sumbu bentrok dengan field milik resource", func(t *testing.T) {
		rep := single(t, f("m/so.resource.yaml", resource("m.so", docHead+"  deliveryStatus: {type: text}\nstatusAxes:\n  deliveryStatus: {values: [a], initial: a, transitions: []}\n")))
		wantErr(t, rep, "duplicate_field", "")
	})
}

func TestStatusAxisFieldsParticipateInConditionsAndRules(t *testing.T) {
	rep := single(t, f("m/so.resource.yaml", resource("m.so", docHead+`  note: {type: text, nullable: true, visibleWhen: {field: deliveryStatus, op: eq, value: delivered}}
statusAxes:
  deliveryStatus: {values: [to_deliver, delivered], initial: to_deliver, transitions: [{from: to_deliver, to: delivered}]}
rules:
  - {id: deliveredNeedsTotal, assert: {field: deliveryStatus, op: ne, value: ghost}, message: m.so.error.x}
`)))
	// nilai di luar values sumbu ditolak seperti enum biasa.
	wantErr(t, rep, CodeConditionValueType, "/rules/0")
	for _, p := range rep.Problems {
		if p.Code == CodeConditionValueType && strings.Contains(p.Pointer, "/fields/note") {
			t.Errorf("nilai delivered sah: %+v", p)
		}
	}
}
