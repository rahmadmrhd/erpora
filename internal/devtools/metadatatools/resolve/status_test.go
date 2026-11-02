package resolve

import (
	"reflect"
	"strings"
	"testing"
)

var lifecycleValues = []string{"draft", "deleted", "need_approval", "accepted", "rejected", "cancelled", "done"}

var lifecyclePairs = []Transition{
	{"draft", "deleted"}, {"draft", "accepted"}, {"draft", "need_approval"},
	{"need_approval", "accepted"}, {"need_approval", "rejected"},
	{"accepted", "cancelled"}, {"accepted", "done"},
}

func TestBuiltinDocumentLifecycle(t *testing.T) {
	c := builtinCatalog(t)
	st := mustResolve(t, c, "sdk.document").Field("status")
	vals, _ := st.Attrs["values"].([]any)
	var got []string
	for _, v := range vals {
		got = append(got, v.(string))
	}
	if !reflect.DeepEqual(got, lifecycleValues) || st.Str("default") != "draft" || !st.Bool("readOnly") {
		t.Fatalf("status = %v default=%q readOnly=%v", got, st.Str("default"), st.Bool("readOnly"))
	}
	if st.Bool("nullable") {
		t.Error("status tidak boleh nullable")
	}
	// K23: submitted bukan status persisten.
	for _, v := range got {
		if v == "submitted" {
			t.Error("submitted tidak boleh menjadi nilai status")
		}
	}
}

func TestLifecycleOnDocumentResources(t *testing.T) {
	c := builtinCatalog(t)
	addModule(t, c, Module{Slug: "m"},
		f("m/doc.resource.yaml", resource("m.doc", "include:\n  - source: sdk:document\nfields:\n  total: {type: integer}\n")),
		f("m/plain.resource.yaml", resource("m.plain", "fields:\n  total: {type: integer}\n")),
		f("m/mid.abstract.yaml", abstract("m.mid", "include:\n  - source: sdk:document\n")),
		f("m/viaabs.resource.yaml", resource("m.viaAbs", "include:\n  - source: m.mid\n")))
	doc := mustResolve(t, c, "m.doc")
	if !doc.IsDocument() || doc.Lifecycle == nil {
		t.Fatalf("dokumen harus punya Lifecycle: %+v", doc.Lifecycle)
	}
	l := doc.Lifecycle
	if !reflect.DeepEqual(l.Values, lifecycleValues) || l.Initial != "draft" || !reflect.DeepEqual(l.Transitions, lifecyclePairs) {
		t.Fatalf("lifecycle = %+v", l)
	}
	if l.Origin.Doc != KeyDocument || l.Origin.File == "" || l.Origin.Line == 0 {
		t.Errorf("jejak asal lifecycle = %+v", l.Origin)
	}
	for _, p := range lifecyclePairs {
		if !l.Allows(p.From, p.To) {
			t.Errorf("%s -> %s harus sah", p.From, p.To)
		}
	}
	for _, p := range []Transition{{"draft", "done"}, {"accepted", "draft"}, {"deleted", "draft"}, {"rejected", "draft"}, {"cancelled", "accepted"},
		{"done", "cancelled"}, {"draft", "draft"}, {"draft", "submitted"}, {"need_approval", "draft"}} {
		if l.Allows(p.From, p.To) {
			t.Errorf("%s -> %s tidak boleh sah", p.From, p.To)
		}
	}
	if plain := mustResolve(t, c, "m.plain"); plain.IsDocument() || plain.Lifecycle != nil {
		t.Errorf("resource bukan dokumen tidak punya lifecycle: %+v", plain.Lifecycle)
	}
	if via := mustResolve(t, c, "m.viaAbs"); !via.IsDocument() || via.Lifecycle == nil {
		t.Error("dokumen lewat abstrak lain tetap dokumen")
	}
	// salinan: mengubah hasil tidak boleh mengubah tabel bawaan.
	l.Transitions[0] = Transition{"x", "y"}
	if again := mustResolve(t, c, "m.doc"); !reflect.DeepEqual(again.Lifecycle.Transitions, lifecyclePairs) {
		t.Error("tabel transisi bawaan harus tidak berubah")
	}
	if got := LifecycleTransitions(); !reflect.DeepEqual(got, lifecyclePairs) {
		t.Errorf("LifecycleTransitions() = %v", got)
	}
}

const axesDoc = `include:
  - source: sdk:document
fields:
  total: {type: integer}
statusAxes:
  billingStatus:
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
      - {from: to_deliver, to: delivered}
      - {from: partially_delivered, to: delivered}
      - {from: partially_delivered, to: to_deliver}
      - {from: delivered, to: partially_delivered}
      - {from: delivered, to: to_deliver}
`

func TestStatusAxesBecomeReadOnlyEnumFields(t *testing.T) {
	c := builtinCatalog(t)
	addModule(t, c, Module{Slug: "m"}, f("m/so.resource.yaml", resource("m.so", axesDoc)))
	r := mustResolve(t, c, "m.so")
	if got := joined(r); got != "id,version,createdAt,updatedAt,createdBy,updatedBy,status,amendedFromId,total,billingStatus,deliveryStatus" {
		t.Fatalf("field = %s", got)
	}
	if len(r.StatusAxes) != 2 || r.StatusAxes[0].Name != "billingStatus" || r.StatusAxes[1].Name != "deliveryStatus" {
		t.Fatalf("StatusAxes = %+v", r.StatusAxes)
	}
	b := r.StatusAxes[0]
	if !reflect.DeepEqual(b.Values, []string{"to_bill", "billed"}) || b.Initial != "to_bill" ||
		!reflect.DeepEqual(b.Transitions, []Transition{{"to_bill", "billed"}, {"billed", "to_bill"}}) {
		t.Errorf("billingStatus = %+v", b)
	}
	if b.Origin.Doc != "m.so" || b.Origin.File != "m/so.resource.yaml" || b.Origin.Line == 0 {
		t.Errorf("jejak asal sumbu = %+v", b.Origin)
	}
	// mundur dan siklik sah; pasangan yang tidak dinyatakan tidak sah.
	d := r.StatusAxes[1]
	if !d.Allows("delivered", "to_deliver") || !d.Allows("partially_delivered", "to_deliver") || d.Allows("to_deliver", "to_deliver") ||
		d.Allows("delivered", "bogus") {
		t.Errorf("transisi sumbu pengiriman salah: %+v", d.Transitions)
	}
	// field sumbu: enum readOnly bukan-nullable, default = initial, bernilai values.
	f2 := r.Field("deliveryStatus")
	vals, _ := f2.Attrs["values"].([]any)
	if f2.Type() != "enum" || !f2.Bool("readOnly") || f2.Bool("nullable") || f2.Str("default") != "to_deliver" || len(vals) != 3 || !f2.IsStatusAxis {
		t.Errorf("deliveryStatus = %+v", f2)
	}
	if o := f2.Origin; o.Doc != "m.so" || o.Line == 0 {
		t.Errorf("Origin field sumbu = %+v", o)
	}
	if r.Field("status").IsStatusAxis || r.Field("total").IsStatusAxis {
		t.Error("hanya field sumbu yang bertanda IsStatusAxis")
	}
	// lifecycle tidak berubah oleh sumbu.
	if r.Lifecycle == nil || !reflect.DeepEqual(r.Lifecycle.Transitions, lifecyclePairs) {
		t.Errorf("lifecycle = %+v", r.Lifecycle)
	}
	// deterministik.
	if r2 := mustResolve(t, c, "m.so"); !reflect.DeepEqual(r, r2) {
		t.Error("hasil resolusi tidak deterministik")
	}
}

func TestStatusAxesAreNotInherited(t *testing.T) {
	c := builtinCatalog(t)
	addModule(t, c, Module{Slug: "m"},
		f("m/so.resource.yaml", resource("m.so", axesDoc)),
		f("m/child.resource.yaml", resource("m.child", "include:\n  - source: m.so\n")))
	child := mustResolve(t, c, "m.child")
	if len(child.StatusAxes) != 0 {
		t.Errorf("sumbu tidak diwarisi: %+v", child.StatusAxes)
	}
	if child.Field("billingStatus") != nil || child.Field("deliveryStatus") != nil {
		t.Errorf("field sumbu tidak boleh ikut include: %s", joined(child))
	}
	if child.Field("total") == nil {
		t.Error("field biasa tetap ikut include")
	}
}

func TestStatusAxisNameCollisions(t *testing.T) {
	c := builtinCatalog(t)
	addModule(t, c, Module{Slug: "m"},
		f("m/a.resource.yaml", resource("m.a", "include:\n  - source: sdk:document\nstatusAxes:\n  status: {values: [a, b], initial: a, transitions: []}\n")),
		f("m/b.resource.yaml", resource("m.b", "fields:\n  deliveryStatus: {type: text}\nstatusAxes:\n  deliveryStatus: {values: [a], initial: a, transitions: []}\n")),
		f("m/c.resource.yaml", resource("m.c", "include:\n  - source: sdk:document\nstatusAxes:\n  id: {values: [a], initial: a, transitions: []}\n")))
	for _, key := range []string{"m.a", "m.b", "m.c"} {
		_, ps := c.Resolve(key)
		p := findProblem(ps, CodeDuplicateField)
		if p == nil || !strings.Contains(p.Message, "sumbu") {
			t.Errorf("%s: temuan = %v", key, ps)
		}
	}
}

func TestStatusAxesOnNonDocumentAreStillResolvedForCheck(t *testing.T) {
	c := builtinCatalog(t)
	addModule(t, c, Module{Slug: "m"}, f("m/p.resource.yaml", resource("m.p", "statusAxes:\n  flow: {values: [a, b], initial: a, transitions: [{from: a, to: b}]}\n")))
	r := mustResolve(t, c, "m.p")
	if r.IsDocument() || len(r.StatusAxes) != 1 || r.Field("flow") == nil {
		t.Fatalf("hasil = %s axes=%v", joined(r), r.StatusAxes)
	}
}
