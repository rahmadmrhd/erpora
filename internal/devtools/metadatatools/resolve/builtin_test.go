package resolve

import (
	"reflect"
	"sort"
	"testing"
)

func builtinCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, ps := NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatalf("metadata bawaan tidak valid: %v", ps)
	}
	return c
}

func TestBuiltinInventory(t *testing.T) {
	c := builtinCatalog(t)
	want := []string{"sdk.audit", "sdk.document", "sdk.email", "sdk.money", "sdk.percentage", "sdk.phone"}
	if got := c.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("kunci bawaan = %v, ingin %v", got, want)
	}
	for _, k := range want {
		if exp, _ := c.Doc(k).Value["export"].(bool); !exp {
			t.Errorf("%s harus export: true", k)
		}
	}
	for _, f := range []string{"sdk/metadata/builtin/audit.abstract.yaml"} {
		if c.Doc("sdk.audit").File != f {
			t.Errorf("File = %q", c.Doc("sdk.audit").File)
		}
	}
}

func TestBuiltinDocumentFieldsAreReadOnlyAndProtected(t *testing.T) {
	c := builtinCatalog(t)
	doc := mustResolve(t, c, "sdk.document")
	got := fieldNames(doc)
	want := []string{"id", "version", "createdAt", "updatedAt", "createdBy", "updatedBy", "status", "amendedFromId"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("field = %v, ingin %v", got, want)
	}
	for _, f := range doc.Fields {
		if !f.Bool("readOnly") {
			t.Errorf("field bawaan %s harus readOnly (Requirement 2.6)", f.Name)
		}
		if f.ProtectedBy == "" {
			t.Errorf("field bawaan %s harus dilindungi", f.Name)
		}
	}
	if doc.Field("id").ProtectedBy != KeyAudit || doc.Field("status").ProtectedBy != KeyDocument {
		t.Errorf("ProtectedBy id=%q status=%q", doc.Field("id").ProtectedBy, doc.Field("status").ProtectedBy)
	}
	if doc.LockWhen["field"] != "status" || doc.LockWhen["op"] != "ne" || doc.LockWhen["value"] != "draft" {
		t.Errorf("lockWhen bawaan = %v", doc.LockWhen)
	}
	st := doc.Field("status")
	if vals, _ := st.Attrs["values"].([]any); len(vals) != 3 || vals[0] != "draft" {
		t.Errorf("status = %v", st.Attrs)
	}
	if !doc.Field("amendedFromId").Bool("nullable") {
		t.Error("amendedFromId harus nullable")
	}
	audit := mustResolve(t, c, "sdk.audit")
	if joined(audit) != "id,version,createdAt,updatedAt,createdBy,updatedBy" || audit.LockWhen != nil {
		t.Errorf("audit = %s lockWhen=%v", joined(audit), audit.LockWhen)
	}
}

func TestBuiltinDomainTypes(t *testing.T) {
	c := builtinCatalog(t)
	addModule(t, c, Module{Slug: "hr"}, f("hr/person.resource.yaml", resource("hr.person", `include:
  - source: sdk:document
fields:
  email: {type: sdk:email}
  phone: {type: sdk:phone}
  share: {type: sdk:percentage}
  salary: {type: sdk:money, currency: IDR}
`)))
	r := mustResolve(t, c, "hr.person")
	tests := []struct {
		field, base, class, widget string
	}{
		{"email", "string", "confidential", "email"},
		{"phone", "string", "confidential", "phone"},
		{"share", "decimal", "internal", "percentage"},
		{"salary", "decimal", "internal", "money"},
	}
	for _, tc := range tests {
		fl := r.Field(tc.field)
		if fl.Type() != tc.base || fl.Classification() != tc.class || fl.Widget != tc.widget || len(fl.Operators) == 0 || fl.Display == "" {
			t.Errorf("%s = type %s kelas %s widget %s ops %v display %q", tc.field, fl.Type(), fl.Classification(), fl.Widget, fl.Operators, fl.Display)
		}
	}
	if r.Field("email").Format() != "email" || r.Field("email").Num("maxLength") != "254" {
		t.Errorf("email = %v", r.Field("email").Attrs)
	}
	if p := r.Field("phone"); p.Num("maxLength") != "32" || p.Str("pattern") == "" {
		t.Errorf("phone = %v", p.Attrs)
	}
	if s := r.Field("share"); s.Num("precision") != "5" || s.Num("scale") != "2" || s.Str("minimum") != "0.00" || s.Str("maximum") != "100.00" {
		t.Errorf("percentage = %v", s.Attrs)
	}
	if m := r.Field("salary"); m.Num("precision") != "18" || m.Num("scale") != "2" || m.Str("currency") != "IDR" {
		t.Errorf("money = %v", m.Attrs)
	}
	// Uses mencatat dokumen yang dipakai (include dan tipe domain, transitif).
	var uses []string
	for _, u := range r.Uses {
		uses = append(uses, u.Key+":"+u.Kind)
	}
	sort.Strings(uses)
	want := []string{"sdk.audit:include", "sdk.document:include", "sdk.email:fieldType", "sdk.money:fieldType", "sdk.percentage:fieldType", "sdk.phone:fieldType"}
	if !reflect.DeepEqual(uses, want) {
		t.Errorf("Uses = %v", uses)
	}
}

func TestBuiltinPatternIsRE2AndBounded(t *testing.T) {
	c := builtinCatalog(t)
	pat, _ := c.Doc("sdk.phone").Value["base"].(map[string]any)["pattern"].(string)
	if len(pat) == 0 || len(pat) > 200 {
		t.Fatalf("pattern phone = %q", pat)
	}
}
