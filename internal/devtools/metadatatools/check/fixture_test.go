package check

import (
	"strings"
	"testing"
)

const fixtureRoot = "../testdata/repo"

func TestFixtureWorkspacePasses(t *testing.T) {
	in, probs := LoadWorkspace(fixtureRoot)
	if len(probs) != 0 {
		t.Fatalf("pemuatan fixture: %s", dump(probs))
	}
	if len(in.Modules) != 2 {
		t.Fatalf("modul = %d", len(in.Modules))
	}
	rep := Run(in)
	// Fixture fixturesales.invoiceLine.description adalah tujuan salinan (snapshot dari product.name) tetapi tidak
	// readOnly dan tidak menyatakan fieldGroup/classification eksplisit (Requirement 14.1, 14.5): cacat pada
	// fixture (testdata bukan milik pemeriksa). Hapus toleransi ini setelah fixture diperbaiki.
	var unexpected []Problem
	for _, p := range rep.Problems {
		if p.Severity == SeverityError && (p.Code == CodeSnapshotToNotReadOnly || p.Code == CodeSnapshotToExplicit) &&
			strings.Contains(p.File, "invoiceLine.resource.yaml") && strings.Contains(p.Pointer, "/fields/description") {
			continue
		}
		if p.Severity == SeverityError {
			unexpected = append(unexpected, p)
		}
	}
	if len(unexpected) != 0 {
		t.Fatalf("galat tak terduga pada fixture: %s", dump(unexpected))
	}

	inv := rep.Resource("fixturesales.invoice")
	if inv == nil || !inv.Document {
		t.Fatalf("invoice = %+v", inv)
	}
	if inv.RuleScope["dueAfterIssue"] != ScopeBoth || inv.RuleScope["discountWithinCap"] != ScopeServer {
		t.Errorf("scope aturan = %v", inv.RuleScope)
	}
	if inv.Group["number"] != "general" || inv.Group["total"] != "pricing" || inv.Class["total"] != "confidential" {
		t.Errorf("grup/klasifikasi = %v %v", inv.Group["number"], inv.Class["total"])
	}
	if inv.Class["summary"] != "internal" {
		t.Errorf("summary mewarisi internal, dapat %q", inv.Class["summary"])
	}
	line := rep.Resource("fixturesales.invoiceLine")
	if line.Class["lineTotal"] != "confidential" || line.Group["lineTotal"] != "pricing" {
		t.Errorf("lineTotal = %s/%s", line.Class["lineTotal"], line.Group["lineTotal"])
	}
	// FK tanpa indeks diperingatkan (belum ada indeks yang diawali invoiceId).
	warned := false
	for _, p := range rep.Problems {
		if p.Severity == SeverityWarning && p.Code == CodeRelationFKNoIndex && strings.Contains(p.Pointer, "/relations/invoice") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("peringatan FK tanpa indeks diharapkan: %s", dump(rep.Problems))
	}
}

func TestLoadWorkspaceReportsBrokenMetadata(t *testing.T) {
	in, probs := LoadWorkspace("../testdata/tidak-ada")
	if len(in.Modules) != 0 || len(probs) != 0 {
		t.Errorf("root tanpa modul: %v %v", in, probs)
	}
}

func TestParseManifestReadsResourcesAndGroups(t *testing.T) {
	m, err := ParseManifest([]byte(`{"key":"acme.x","slug":"x","dependencies":[{"module":"acme.y"}],"capabilities":["x.a.read"],
	"resources":[{"key":"x.a","actions":["read"],"custom_actions":[{"key":"adjust"}],"owner_field":"created_by","scope_fields":{"x.w":"w_id"},
	"field_groups":[{"key":"general","default":true},{"key":"pricing","classification":"confidential","fields":["p"]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasResources || len(m.Resources) != 1 || m.Resources[0].CustomActions[0] != "adjust" || m.Resources[0].FieldGroups[1].Fields[0] != "p" ||
		m.Dependencies[0] != "acme.y" || m.Resources[0].OwnerField != "created_by" {
		t.Errorf("manifest = %+v", m)
	}
	m2, _ := ParseManifest([]byte(`{"key":"acme.x","slug":"x"}`))
	if m2.HasResources {
		t.Error("tanpa resources: HasResources harus false")
	}
	if _, err := ParseManifest([]byte(`{`)); err == nil {
		t.Error("JSON rusak harus galat")
	}
}

func TestReportHelpers(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: integer, maxLength: 3}\n")))
	if !rep.HasErrors() || rep.ErrorCount() == 0 {
		t.Fatal("harus ada galat")
	}
	p := rep.Problems[0]
	s := p.String()
	for _, want := range []string{"m/r.resource.yaml", "error", p.Code} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q tidak memuat %q", s, want)
		}
	}
	if p.Module != "m" || p.Resource != "m.r" {
		t.Errorf("Module/Resource = %q %q", p.Module, p.Resource)
	}
}

func TestIsSensitiveName(t *testing.T) {
	for _, n := range []string{"salary", "npwp", "NIK", "apikey", "bankaccountNo", "refreshToken", "passwd"} {
		if !IsSensitiveName(n) {
			t.Errorf("%s harus sensitif", n)
		}
	}
	for _, n := range []string{"name", "tokenless", "salaries", "keyboard", "accountNo", "bank", "apiVersion"} {
		if IsSensitiveName(n) {
			t.Errorf("%s tidak sensitif", n)
		}
	}
}
