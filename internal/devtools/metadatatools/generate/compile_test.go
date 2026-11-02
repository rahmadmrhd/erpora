package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/archcheck"
	"erpora/internal/devtools/metadatatools/internal/testrepo"
)

const behaviorTest = `package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/modules/fixturesales/internal/domain"
	"erpora/sdk/httpx"
	"erpora/sdk/metadata"
)

const validBody = ` + "`" + `{"issuedOn":"2026-01-01","dueOn":"2026-01-31","total":"100.00","customerId":"0193a6c2-7c1e-7d4a-8a9b-0c1d2e3f4a5b"}` + "`" + `

func post(body string) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), r
}

func details(t *testing.T, err error) (string, map[string]string) {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status != 422 {
		t.Fatalf("bukan 422 *httpx.Error: %v", err)
	}
	m := map[string]string{}
	for _, d := range he.Details {
		m[d.Field] = d.Code
	}
	return he.Code, m
}

func TestGeneratedCreate(t *testing.T) {
	w, r := post(validBody)
	d, err := DecodeInvoiceCreate(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(context.Background()); err != nil {
		t.Fatalf("seharusnya sah: %v", err)
	}
	if d.Total == nil || *d.Total != "100.00" || !d.Present["total"] || d.Present["branchId"] {
		t.Errorf("dto = %+v", d)
	}
}

func TestGeneratedRejectsReadOnlyAndUnknown(t *testing.T) {
	w, r := post(` + "`" + `{"number":"X","status":"accepted","bogus":1,"total":"1"}` + "`" + `)
	_, err := DecodeInvoiceCreate(w, r)
	code, m := details(t, err)
	if code != "validation_failed" || m["number"] != "read_only" || m["status"] != "read_only" || m["bogus"] != "unknown_field" {
		t.Errorf("%s %v", code, m)
	}
}

func TestGeneratedTypeAndStructure(t *testing.T) {
	w, r := post(` + "`" + `{"issuedOn":"2026-01-01","dueOn":"2026-01-31","total":100,"customerId":"x"}` + "`" + `)
	_, err := DecodeInvoiceCreate(w, r)
	if _, m := details(t, err); m["total"] != "invalid_type" {
		t.Errorf("desimal angka JSON harus ditolak: %v", m)
	}
	w, r = post(` + "`" + `{"issuedOn":"2026-01-01","dueOn":"2026-01-31","customerId":"nope"}` + "`" + `)
	d, err := DecodeInvoiceCreate(w, r)
	if err != nil {
		t.Fatal(err)
	}
	code, m := details(t, d.Validate(context.Background()))
	if code != "validation_failed" || m["total"] != "required" || m["customerId"] != "invalid_uuid" {
		t.Errorf("%s %v", code, m)
	}
}

func TestGeneratedBusinessRules(t *testing.T) {
	w, r := post(` + "`" + `{"issuedOn":"2026-02-01","dueOn":"2026-01-01","total":"1.00","customerId":"0193a6c2-7c1e-7d4a-8a9b-0c1d2e3f4a5b"}` + "`" + `)
	d, _ := DecodeInvoiceCreate(w, r)
	code, m := details(t, d.Validate(context.Background()))
	if code != "business_rule_violated" || m["dueOn"] != "due_after_issue" {
		t.Errorf("%s %v", code, m)
	}
	w, r = post(` + "`" + `{"issuedOn":"2026-01-01","dueOn":"2026-02-01","total":"1.00","discountRate":"60","customerId":"0193a6c2-7c1e-7d4a-8a9b-0c1d2e3f4a5b"}` + "`" + `)
	d, _ = DecodeInvoiceCreate(w, r)
	if _, m := details(t, d.Validate(context.Background())); m["discountRate"] != "discount_within_cap" {
		t.Errorf("%v", m)
	}
	w, r = post(` + "`" + `{"issuedOn":"2026-01-01","dueOn":"2026-02-01","total":"60000000.00","customerId":"0193a6c2-7c1e-7d4a-8a9b-0c1d2e3f4a5b"}` + "`" + `)
	d, _ = DecodeInvoiceCreate(w, r)
	if _, m := details(t, d.Validate(context.Background())); m["approvalNote"] != "required" {
		t.Errorf("requiredWhen: %v", m)
	}
}

func TestGeneratedUpdate(t *testing.T) {
	w, r := post(` + "`" + `{"dueOn":"2026-03-01","approvalNote":null}` + "`" + `)
	d, err := DecodeInvoiceUpdate(w, r)
	if err != nil {
		t.Fatal(err)
	}
	current := metadata.Record{"issuedOn": "2026-01-01", "dueOn": "2026-01-15", "status": "draft", "total": "10.00",
		"discountRate": "0", "taxRate": "11", "taxable": true, "shipToCustomer": false}
	if err := d.Validate(context.Background(), current); err != nil {
		t.Fatalf("patch sah: %v", err)
	}
	w, r = post(` + "`" + `{"dueOn":"2025-12-01"}` + "`" + `)
	d, _ = DecodeInvoiceUpdate(w, r)
	if code, _ := details(t, d.Validate(context.Background(), current)); code != "business_rule_violated" {
		t.Errorf("aturan lintas field harus memakai record gabungan: %s", code)
	}
	w, r = post(` + "`" + `{"total":null}` + "`" + `)
	d, _ = DecodeInvoiceUpdate(w, r)
	if _, m := details(t, d.Validate(context.Background(), current)); m["total"] != "null_not_allowed" {
		t.Errorf("%v", m)
	}
}

func TestGeneratedBodyLimitAndDepth(t *testing.T) {
	w, r := post(validBody)
	r = r.WithContext(httpx.WithBodyLimit(r.Context(), 20))
	if _, err := DecodeInvoiceCreate(w, r); err == nil {
		t.Error("body melebihi batas harus ditolak")
	}
	w, r = post(strings.Repeat("[", 100))
	if _, err := DecodeInvoiceCreate(w, r); err == nil {
		t.Error("JSON dalam harus ditolak")
	}
}

func TestGeneratedStatusTables(t *testing.T) {
	s := domain.InvoiceSpec
	if s.Lifecycle == nil || !s.Lifecycle.Allows(domain.InvoiceStatusDraft, domain.InvoiceStatusAccepted) ||
		!s.Lifecycle.Allows(domain.InvoiceStatusDraft, domain.InvoiceStatusNeedApproval) ||
		s.Lifecycle.Allows(domain.InvoiceStatusDraft, "submitted") || s.Lifecycle.Allows(domain.InvoiceStatusAccepted, domain.InvoiceStatusDraft) ||
		s.Lifecycle.Allows(domain.InvoiceStatusRejected, domain.InvoiceStatusDraft) {
		t.Errorf("tabel transisi lifecycle salah: %+v", s.Lifecycle)
	}
	del, ok := s.Axis(domain.InvoiceFieldDeliveryStatus)
	if !ok || del.Initial != domain.InvoiceDeliveryStatusToDeliver {
		t.Fatalf("sumbu pengiriman: %+v", del)
	}
	if !del.Allows(domain.InvoiceDeliveryStatusPartiallyDelivered, domain.InvoiceDeliveryStatusToDeliver) ||
		!del.Allows(domain.InvoiceDeliveryStatusDelivered, domain.InvoiceDeliveryStatusToDeliver) ||
		del.Allows(domain.InvoiceDeliveryStatusToDeliver, domain.InvoiceDeliveryStatusToDeliver) {
		t.Error("transisi mundur sumbu pengiriman harus sah, pasangan tak tercantum tidak")
	}
	bill, ok := s.Axis(domain.InvoiceFieldBillingStatus)
	if !ok || !bill.Allows(domain.InvoiceBillingStatusToBill, domain.InvoiceBillingStatusBilled) || bill.Allows(domain.InvoiceBillingStatusBilled, domain.InvoiceBillingStatusToBill) {
		t.Errorf("sumbu penagihan: %+v", bill)
	}
	for _, name := range []string{domain.InvoiceFieldBillingStatus, domain.InvoiceFieldDeliveryStatus, domain.InvoiceFieldStatus} {
		if _, ro := s.ReadOnlyNames()[name]; !ro {
			t.Errorf("%s harus hanya-baca", name)
		}
	}
}

func TestGeneratedRejectsStatusAxisFromClient(t *testing.T) {
	for _, field := range []string{"status", "billingStatus", "deliveryStatus"} {
		w, r := post("{\"" + field + "\":\"x\",\"total\":\"1\"}")
		_, err := DecodeInvoiceCreate(w, r)
		if _, m := details(t, err); m[field] != "read_only" {
			t.Errorf("%s harus read_only: %v", field, m)
		}
		w, r = post("{\"" + field + "\":\"x\"}")
		_, err = DecodeInvoiceUpdate(w, r)
		if _, m := details(t, err); m[field] != "read_only" {
			t.Errorf("update %s harus read_only: %v", field, m)
		}
	}
}

func TestGeneratedLockedAndConstants(t *testing.T) {
	if domain.InvoiceSpec.Locked(metadata.Record{"status": domain.InvoiceStatusDraft}) ||
		!domain.InvoiceSpec.Locked(metadata.Record{"status": domain.InvoiceStatusAccepted}) {
		t.Error("Locked")
	}
	if domain.InvoiceFieldCustomerID != "customerId" || domain.InvoicePaymentTermNet15 != "net_15" {
		t.Error("konstanta")
	}
}
`

func goTool(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-p=1 -mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TestGeneratedCodeCompilesAndBehaves membuktikan bahwa keluaran Go kompilasi, lolos vet dan archcheck tanpa
// pengecualian baru, dan berperilaku sesuai Requirement 9 (modul fixture sungguhan, salinan sementara).
func TestGeneratedCodeCompilesAndBehaves(t *testing.T) {
	if testing.Short() {
		t.Skip("kompilasi modul fixture (lambat)")
	}
	root := testrepo.Copy(t, true)
	cat := fixtureCatalog(t, root)
	if _, err := Generate(root, cat, Options{Checker: okChecker{}}); err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "modules", "fixturesales")
	writeFile(t, root, "modules/fixturesales/internal/presentation/http/behavior_test.go", behaviorTest)

	goTool(t, modDir, "build", "./...")
	goTool(t, modDir, "vet", "./...")
	goTool(t, modDir, "test", "./...")
	coreDir := filepath.Join(root, "modules", "fixturecore")
	goTool(t, coreDir, "build", "./...")

	t.Setenv("GOWORK", "off")
	repo, err := archcheck.Load(modDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range archcheck.Check(repo) {
		switch v.Rule {
		case "D1", "D2", "D3", "D3b", "D4", "D7", "D9", "D10", "AUTOMIGRATE", "DB-OPEN":
			t.Errorf("pelanggaran %s di %s: %s", v.Rule, v.Package, v.Detail)
		}
	}
}
