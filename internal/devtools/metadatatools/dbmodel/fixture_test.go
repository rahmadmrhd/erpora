package dbmodel

import (
	"sort"
	"strings"
	"testing"
)

func TestDerive_Fixture(t *testing.T) {
	s, ps := Derive(fixtureResources(t))
	if len(ps) != 0 {
		t.Fatalf("temuan: %v", ps)
	}
	var names []string
	for _, tb := range s.Tables {
		names = append(names, tb.Name)
		if tb.Column("tenant_id") != nil {
			t.Errorf("%s memuat tenant_id", tb.Name)
		}
		if c := tb.Column("id"); c == nil && tb.Kind != KindPivot || c != nil && c.Type.String() != "uuid" {
			t.Errorf("%s: id harus uuid", tb.Name)
		}
		if len(tb.Name) > 63 {
			t.Errorf("nama tabel terlalu panjang: %s", tb.Name)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("tabel harus terurut deterministik: %v", names)
	}
	want := "fixturecore_branches,fixturecore_customers,fixturesales_invoice_lines,fixturesales_invoice_tags,fixturesales_invoices,fixturesales_notes,fixturesales_products,fixturesales_tags"
	if strings.Join(names, ",") != want {
		t.Fatalf("tabel = %s\nwant   %s", strings.Join(names, ","), want)
	}

	inv := table(t, s, "fixturesales_invoices")
	// Blok berawalan, tipe domain, dan campuran grup.
	for _, c := range []string{"shipping_street", "shipping_postal_code", "tax_rate", "discount_rate", "customer_name", "total", "customer_id", "branch_id", "payment_term", "status"} {
		col(t, inv, c)
	}
	if inv.Column("withholding_rate") != nil {
		t.Error("withholdingRate di-omit")
	}
	if inv.Column("summary") != nil || inv.Column("line_count") != nil {
		t.Error("accessor expr/func tanpa kolom")
	}
	if inv.Lock == nil {
		t.Error("invoice dokumen: Lock harus ada")
	}
	// Salinan field adalah kolom biasa; relasi lintas modul tanpa FK.
	if len(inv.ForeignKeys) != 0 {
		t.Errorf("invoice tidak punya FK satu modul: %+v", inv.ForeignKeys)
	}

	line := table(t, s, "fixturesales_invoice_lines")
	lt := col(t, line, "line_total")
	if lt.Generated == nil {
		t.Error("line_total adalah kolom turunan stored")
	}
	if len(line.ForeignKeys) != 2 {
		t.Errorf("invoice_lines: FK invoice dan product, dapat %+v", line.ForeignKeys)
	}

	cust := table(t, s, "fixturecore_customers")
	if col(t, cust, "contact_email").Type.String() != "text" {
		t.Error("tipe domain email -> text")
	}
	// Indeks komposit dan unique yang dideklarasikan.
	hasIdx := false
	for _, ix := range cust.Indexes {
		if strings.Join(ix.Columns, ",") == "status,name" {
			hasIdx = true
		}
	}
	if !hasIdx {
		t.Errorf("customers.indexes = %+v", cust.Indexes)
	}
	pivot := table(t, s, "fixturesales_invoice_tags")
	if pivot.Kind != KindPivot {
		t.Error("pivot")
	}
}

func TestDerive_Deterministic(t *testing.T) {
	a, _ := Derive(fixtureResources(t))
	b, _ := Derive(fixtureResources(t))
	if a.Summary() != b.Summary() {
		t.Error("Derive tidak deterministik")
	}
	if !strings.Contains(a.Summary(), "fixturesales_invoices") {
		t.Errorf("Summary = %s", a.Summary())
	}
}
