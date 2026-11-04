package load

import (
	"strings"
	"testing"
)

func TestStatusAxesSchema(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.order\n"
	mustParse(t, head+`statusAxes:
  deliveryStatus:
    values: [to_deliver, partially_delivered, delivered]
    initial: to_deliver
    transitions:
      - {from: delivered, to: to_deliver}
  billingStatus: {values: [to_bill, billed], initial: to_bill, transitions: []}
`)
	tests := []struct {
		name, src, pointer, substr string
	}{
		{"nama bukan camelCase", "statusAxes:\n  delivery_status: {values: [a], initial: a, transitions: []}\n", "/statusAxes/delivery_status", ""},
		{"initial hilang", "statusAxes:\n  flow: {values: [a], transitions: []}\n", "/statusAxes/flow", "initial"},
		{"transitions hilang", "statusAxes:\n  flow: {values: [a], initial: a}\n", "/statusAxes/flow", "transitions"},
		{"values kosong", "statusAxes:\n  flow: {values: [], initial: a, transitions: []}\n", "/statusAxes/flow/values", ""},
		{"nilai bukan snake_case", "statusAxes:\n  flow: {values: [ToBill], initial: a, transitions: []}\n", "/statusAxes/flow/values/0", ""},
		{"nilai ganda", "statusAxes:\n  flow: {values: [a, a], initial: a, transitions: []}\n", "/statusAxes/flow/values", ""},
		{"initial bukan snake_case", "statusAxes:\n  flow: {values: [a], initial: A, transitions: []}\n", "/statusAxes/flow/initial", ""},
		{"kunci tak dikenal pada sumbu", "statusAxes:\n  flow: {values: [a], initial: a, transitions: [], extra: 1}\n", "/statusAxes/flow/extra", "tidak dikenal"},
		{"transisi tanpa to", "statusAxes:\n  flow: {values: [a], initial: a, transitions: [{from: a}]}\n", "/statusAxes/flow/transitions/0", "to"},
		{"transisi kunci tak dikenal", "statusAxes:\n  flow: {values: [a], initial: a, transitions: [{from: a, to: a, via: x}]}\n", "/statusAxes/flow/transitions/0/via", "tidak dikenal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ps := Parse("v.resource.yaml", []byte(head+tc.src))
			for _, p := range ps {
				if p.Code == CodeSchema && strings.Contains(p.Pointer, tc.pointer) && strings.Contains(p.Message, tc.substr) {
					return
				}
			}
			t.Fatalf("tidak ada temuan schema di %q memuat %q: %v", tc.pointer, tc.substr, ps)
		})
	}
	t.Run("abstrak tidak boleh punya statusAxes", func(t *testing.T) {
		_, ps := Parse("v.abstract.yaml", []byte("schemaVersion: \"1\"\nkind: abstract\nkey: sales.blk\nstatusAxes:\n  flow: {values: [a], initial: a, transitions: []}\n"))
		if find(ps, CodeSchema) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("batas panjang nama sumbu 63 byte", func(t *testing.T) {
		long := strings.Repeat("a", 64)
		_, ps := Parse("v.resource.yaml", []byte(head+"statusAxes:\n  "+long+": {values: [a], initial: a, transitions: []}\n"))
		if find(ps, CodeSchema) == nil {
			t.Fatalf("nama 64 byte harus ditolak: %v", ps)
		}
	})
}
