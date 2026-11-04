package dbmodel

import (
	"strings"
	"testing"
)

func triggerByName(tb *Table, name string) *Trigger {
	for _, tr := range tb.Triggers {
		if tr.Name == name {
			return tr
		}
	}
	return nil
}

// Resource dokumen mendapat pemicu penguncian yang parameternya dibangkitkan dari metadata: lockWhen,
// tabel transisi lifecycle bawaan, transisi sumbu status, dan pasangan migrateFrom (Requirement 13.8).
func TestDerive_DocumentLockTrigger(t *testing.T) {
	s, ps := Derive(fixtureResources(t))
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	inv := table(t, s, "fixturesales_invoices")
	tr := triggerByName(inv, "fixturesales_invoices_lock_trg")
	if tr == nil || tr.Function != "fixturesales_invoices_lock_fn" || strings.Join(tr.Events, ",") != "DELETE,UPDATE" || tr.Lock == nil || tr.Child != nil {
		t.Fatalf("pemicu = %+v", tr)
	}
	ls := tr.Lock
	if ls.Condition == nil || ls.StatusColumn != "status" || ls.VersionColumn != "version" || ls.UpdatedAtColumn != "updated_at" || ls.UpdatedByColumn != "updated_by" {
		t.Errorf("lock = %+v", ls)
	}
	var pairs []string
	for _, p := range ls.Transitions {
		pairs = append(pairs, p[0]+">"+p[1])
	}
	want := "draft>deleted,draft>accepted,draft>pending_approval,pending_approval>accepted,pending_approval>rejected,accepted>cancelled,accepted>done"
	if strings.Join(pairs, ",") != want {
		t.Errorf("transisi = %s", strings.Join(pairs, ","))
	}
	if len(ls.Axes) != 2 || ls.Axes[0].Column != "billing_status" || ls.Axes[1].Column != "delivery_status" {
		t.Fatalf("sumbu = %+v", ls.Axes)
	}
	if len(ls.Axes[0].Transitions) != 1 || len(ls.Axes[1].Transitions) != 6 {
		t.Errorf("transisi sumbu = %+v", ls.Axes)
	}
	foundBack := false
	for _, p := range ls.Axes[1].Transitions {
		if p == [2]string{"partially_delivered", "to_deliver"} {
			foundBack = true
		}
	}
	if !foundBack {
		t.Error("transisi mundur sumbu harus ikut")
	}
	// Non-dokumen tanpa pemicu.
	for _, name := range []string{"fixturecore_customers", "fixturecore_branches", "fixturesales_products", "fixturesales_tags"} {
		if n := len(table(t, s, name).Triggers); n != 0 {
			t.Errorf("%s punya %d pemicu", name, n)
		}
	}
}

// Anak embedded, pivot belongsToMany, dan hasOne dilindungi pemicu anak (Requirement 13.11).
func TestDerive_ChildLockTriggers(t *testing.T) {
	s, ps := Derive(fixtureResources(t))
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	line := table(t, s, "fixturesales_invoice_lines")
	tr := triggerByName(line, "fixturesales_invoice_lines_invoice_id_lock_trg")
	if tr == nil || tr.Child == nil || tr.Lock != nil || tr.Child.Parent.Name != "fixturesales_invoices" || tr.Child.FKColumn != "invoice_id" ||
		strings.Join(tr.Events, ",") != "DELETE,INSERT,UPDATE" || tr.Function != "fixturesales_invoice_lines_invoice_id_lock_fn" {
		t.Fatalf("pemicu anak embedded = %+v", tr)
	}
	pivot := table(t, s, "fixturesales_invoice_tags")
	if tr := triggerByName(pivot, "fixturesales_invoice_tags_invoice_id_lock_trg"); tr == nil || tr.Child == nil || tr.Child.FKColumn != "invoice_id" {
		t.Errorf("pemicu pivot = %+v", tr)
	}
	// Anak bukan embedded (morph notes) dan relasi ke master tidak dilindungi.
	if n := len(table(t, s, "fixturesales_notes").Triggers); n != 0 {
		t.Errorf("notes (morphMany) tanpa pemicu anak: %d", n)
	}
}

func TestDerive_HasOneChildLockTrigger(t *testing.T) {
	s := deriveOne(t,
		f("m/header.resource.yaml", resourceDoc("m.header", "relations:\n  doc: {type: belongsTo, to: m.doc, required: true, onDelete: cascade}\n")),
		f("m/doc.resource.yaml", resourceNoAudit("m.doc", "include:\n  - source: sdk:document\nfields:\n  n: {type: integer, label: m.doc.n}\nrelations:\n  header: {type: hasOne, to: m.header}\n")),
	)
	hd := table(t, s, "m_headers")
	if tr := triggerByName(hd, "m_headers_doc_id_lock_trg"); tr == nil || tr.Child == nil || tr.Child.FKColumn != "doc_id" || tr.Child.Parent.Name != "m_docs" {
		t.Fatalf("pemicu hasOne = %+v", tr)
	}
}

func TestDerive_MigrateFromPairsInLockSpec(t *testing.T) {
	s := deriveOne(t, f("m/doc.resource.yaml", resourceNoAudit("m.doc", `
include:
  - source: sdk:document
fields:
  phone: {type: string, maxLength: 20, nullable: true, deprecated: {since: "1.0.0", replacedBy: phoneE164}, label: m.doc.phone}
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.doc.phoneE164}
`)))
	tr := triggerByName(table(t, s, "m_docs"), "m_docs_lock_trg")
	if tr == nil || len(tr.Lock.MigratePairs) != 1 || tr.Lock.MigratePairs[0] != [2]string{"phone_e164", "phone"} {
		t.Fatalf("pemicu = %+v", tr)
	}
}

func TestDerive_DocumentWithoutAxesHasNoAxes(t *testing.T) {
	s := deriveOne(t, f("m/doc.resource.yaml", resourceNoAudit("m.doc", "include:\n  - source: sdk:document\nfields:\n  n: {type: integer, label: m.doc.n}\n")))
	tr := triggerByName(table(t, s, "m_docs"), "m_docs_lock_trg")
	if tr == nil || len(tr.Lock.Axes) != 0 || len(tr.Lock.Transitions) != 7 {
		t.Fatalf("pemicu = %+v", tr)
	}
}

// migrateFrom ke field yang sudah dihapus dari metadata (rilis MAJOR, K18) tetap menjadi pasangan pemicu: kolom
// asal dibaca dinamis sehingga fungsi tidak rusak setelah kolom lama dihapus.
func TestDerive_MigrateFromPairWithoutOldFieldStillInLockSpec(t *testing.T) {
	s := deriveOne(t, f("m/doc.resource.yaml", resourceNoAudit("m.doc", `
include:
  - source: sdk:document
fields:
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, label: m.doc.phoneE164}
`)))
	tb := table(t, s, "m_docs")
	tr := triggerByName(tb, "m_docs_lock_trg")
	if tb.Column("phone") != nil || tr == nil || len(tr.Lock.MigratePairs) != 1 || tr.Lock.MigratePairs[0] != [2]string{"phone_e164", "phone"} {
		t.Fatalf("pemicu = %+v", tr)
	}
}

// Resource dokumen juga mendapat penjaga statement-level BEFORE TRUNCATE dengan predikat kunci yang sama
// dengan pemicu baris (TRUNCATE tidak memicu pemicu baris).
func TestDerive_DocumentTruncateGuard(t *testing.T) {
	s, ps := Derive(fixtureResources(t))
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	inv := table(t, s, "fixturesales_invoices")
	tr := triggerByName(inv, "fixturesales_invoices_truncate_lock_trg")
	if tr == nil || tr.Function != "fixturesales_invoices_truncate_lock_fn" || strings.Join(tr.Events, ",") != "TRUNCATE" || tr.Truncate == nil || tr.Lock != nil || tr.Child != nil {
		t.Fatalf("pemicu = %+v", tr)
	}
	if tr.Truncate.Condition == nil || tr.Truncate.Condition != inv.Lock {
		t.Error("kondisi TRUNCATE harus kondisi kunci yang sama dengan pemicu baris")
	}
	for _, name := range []string{"fixturecore_customers", "fixturesales_products", "fixturesales_tags"} {
		for _, tg := range table(t, s, name).Triggers {
			if tg.Truncate != nil {
				t.Errorf("%s (non-dokumen) tidak boleh punya penjaga TRUNCATE", name)
			}
		}
	}
}

// Tabel anak dokumen (baris embedded dan pivot belongsToMany) mendapat penjaga TRUNCATE statement-level dengan
// kondisi kunci induk yang sama dengan pemicu anak baris.
func TestDerive_ChildTruncateGuards(t *testing.T) {
	s, ps := Derive(fixtureResources(t))
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	inv := table(t, s, "fixturesales_invoices")
	for _, c := range []struct{ table, trg, fk string }{
		{"fixturesales_invoice_lines", "fixturesales_invoice_lines_invoice_id_truncate_lock_trg", "invoice_id"},
		{"fixturesales_invoice_tags", "fixturesales_invoice_tags_invoice_id_truncate_lock_trg", "invoice_id"},
	} {
		tr := triggerByName(table(t, s, c.table), c.trg)
		if tr == nil || strings.Join(tr.Events, ",") != "TRUNCATE" || tr.Truncate == nil || tr.Truncate.Child == nil || tr.Truncate.Condition != nil || tr.Lock != nil || tr.Child != nil {
			t.Fatalf("%s: pemicu = %+v", c.table, tr)
		}
		if tr.Truncate.Child.Parent != inv || tr.Truncate.Child.FKColumn != c.fk {
			t.Errorf("%s: ChildLock = %+v", c.table, tr.Truncate.Child)
		}
		if tr.Function != strings.TrimSuffix(c.trg, "_trg")+"_fn" {
			t.Errorf("%s: fungsi = %s", c.table, tr.Function)
		}
	}
}
