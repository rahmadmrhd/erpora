package resolve

import (
	"fmt"
	"reflect"
	"testing"
)

// impactCatalog membangun katalog uji: tipe email dan blok alamat bersama dipakai beberapa resource di tiga modul.
// Parameter mengubah dokumen bersama agar dapat dibandingkan dengan versi dasar.
type variant struct {
	emailMax    string // maxLength tipe sdk.email
	addrFields  string // field tambahan blok crm.address
	dropAddress bool
}

func impactCatalog(t *testing.T, v variant) *Catalog {
	t.Helper()
	if v.emailMax == "" {
		v.emailMax = "254"
	}
	c := NewCatalog()
	files := []file{
		f("sdk/audit.abstract.yaml", abstract("sdk.audit", "export: true\nfields:\n  id: {type: uuid, readOnly: true}\n")),
		f("sdk/email.type.yaml", fieldType("sdk.email", fmt.Sprintf("export: true\nbase: {type: string, maxLength: %s, format: email}\nclassification: confidential\nwidget: email\n", v.emailMax))),
	}
	addModule(t, c, Module{Slug: "sdk"}, files...)
	crm := []file{
		f("crm/customer.resource.yaml", resource("crm.customer", "include:\n  - {source: crm.address, prefix: home}\nfields:\n  mail: {type: sdk:email}\n")),
		f("crm/note.resource.yaml", resource("crm.note", "fields:\n  body: {type: text}\n")),
	}
	if !v.dropAddress {
		crm = append(crm, f("crm/address.abstract.yaml", abstract("crm.address", "export: true\nfields:\n  street: {type: string, maxLength: 200}\n"+v.addrFields)))
	} else {
		crm[0] = f("crm/customer.resource.yaml", resource("crm.customer", "fields:\n  mail: {type: sdk:email}\n"))
	}
	addModule(t, c, Module{Slug: "crm"}, crm...)
	sales := f("sales/order.resource.yaml", resource("sales.order", "include:\n  - {source: crm.address, prefix: ship}\nfields:\n  contact: {type: sdk:email}\n"))
	if v.dropAddress {
		sales = f("sales/order.resource.yaml", resource("sales.order", "include:\n  - {source: crm.address, prefix: ship}\nfields:\n  contact: {type: sdk:email}\n"))
	}
	addModule(t, c, Module{Slug: "sales", Dependencies: []string{"crm"}}, sales,
		f("sales/plain.resource.yaml", resource("sales.plain", "include:\n  - source: sdk:audit\nfields:\n  x: {type: text}\n")))
	return c
}

func userIndex(d DocImpact) map[string]UserImpact {
	m := map[string]UserImpact{}
	for _, u := range d.Users {
		m[u.Resource] = u
	}
	return m
}

func findDoc(r *ImpactReport, key string) *DocImpact {
	for i := range r.Docs {
		if r.Docs[i].Key == key {
			return &r.Docs[i]
		}
	}
	return nil
}

func TestImpactUnchangedIsEmpty(t *testing.T) {
	rep := Impacts(impactCatalog(t, variant{}), impactCatalog(t, variant{}))
	if len(rep.Docs) != 0 || len(rep.Problems) != 0 {
		t.Fatalf("laporan = %+v", rep)
	}
}

func TestImpactSharedDomainTypeChangeReportsAllUsers(t *testing.T) {
	rep := Impacts(impactCatalog(t, variant{}), impactCatalog(t, variant{emailMax: "300"}))
	if len(rep.Problems) != 0 {
		t.Fatalf("problems = %v", rep.Problems)
	}
	d := findDoc(rep, "sdk.email")
	if d == nil || len(rep.Docs) != 1 {
		t.Fatalf("laporan = %+v", rep.Docs)
	}
	if d.Module != "sdk" || d.Kind != "fieldType" || !d.Exported {
		t.Errorf("doc = %+v", d)
	}
	users := userIndex(*d)
	if len(users) != 2 {
		t.Fatalf("pemakai = %v", users)
	}
	cust, ord := users["crm.customer"], users["sales.order"]
	if cust.Module != "crm" || !cust.CrossModule || ord.Module != "sales" || !ord.CrossModule {
		t.Errorf("CrossModule: %+v %+v", cust, ord)
	}
	if !reflect.DeepEqual(cust.Via, []string{"sdk.email"}) {
		t.Errorf("Via = %v", cust.Via)
	}
	want := []FieldChange{{Field: "mail", Kind: ChangeChanged, Attrs: []string{"maxLength"}}}
	if !reflect.DeepEqual(cust.Changes, want) {
		t.Errorf("perubahan crm.customer = %+v", cust.Changes)
	}
	if got := ord.Changes; len(got) != 1 || got[0].Field != "contact" || got[0].Attrs[0] != "maxLength" {
		t.Errorf("perubahan sales.order = %+v", got)
	}
	if users["sales.plain"].Resource != "" || users["crm.note"].Resource != "" {
		t.Error("resource yang tidak memakai tipe tidak boleh ikut dilaporkan")
	}
}

func TestImpactBlockChangeIncludesTransitiveAndSameModule(t *testing.T) {
	rep := Impacts(impactCatalog(t, variant{}), impactCatalog(t, variant{addrFields: "  city: {type: string, maxLength: 50}\n"}))
	d := findDoc(rep, "crm.address")
	if d == nil || len(rep.Docs) != 1 {
		t.Fatalf("laporan = %+v", rep.Docs)
	}
	users := userIndex(*d)
	if len(users) != 2 {
		t.Fatalf("pemakai = %v", users)
	}
	if users["crm.customer"].CrossModule || !users["sales.order"].CrossModule {
		t.Errorf("CrossModule salah: %+v", users)
	}
	ch := users["sales.order"].Changes
	if len(ch) != 1 || ch[0].Field != "shipCity" || ch[0].Kind != ChangeAdded {
		t.Errorf("perubahan = %+v", ch)
	}
	if ch[0].PotentiallyBreaking() {
		t.Error("penambahan field bukan perubahan merusak")
	}
	if !(FieldChange{Kind: ChangeRemoved}).PotentiallyBreaking() || !(FieldChange{Kind: ChangeChanged, Attrs: []string{"type"}}).PotentiallyBreaking() {
		t.Error("penghapusan dan perubahan tipe berpotensi merusak")
	}
}

func TestImpactRemovedSharedDocBreaksUsers(t *testing.T) {
	rep := Impacts(impactCatalog(t, variant{}), impactCatalog(t, variant{dropAddress: true}))
	d := findDoc(rep, "crm.address")
	if d == nil {
		t.Fatalf("laporan = %+v", rep.Docs)
	}
	if !d.Removed {
		t.Error("Removed harus true")
	}
	u := userIndex(*d)["sales.order"]
	if !u.Broken || !u.CrossModule {
		t.Errorf("sales.order = %+v", u)
	}
	if len(rep.Problems) == 0 {
		t.Error("resolusi katalog baru yang rusak harus dilaporkan")
	}
}

func TestResultUsers(t *testing.T) {
	res := impactCatalog(t, variant{}).ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatal(res.Problems)
	}
	var keys []string
	for _, u := range res.Users("sdk.email") {
		keys = append(keys, u.Key)
	}
	if !reflect.DeepEqual(keys, []string{"crm.customer", "sales.order"}) {
		t.Errorf("Users = %v", keys)
	}
}
