//go:build integration

package app

import (
	"strconv"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"erpora/internal/platform/apicontract"
)

// validateResponse memeriksa status didokumentasikan dan body lolos skema respons kontrak (FR-23.2).
func validateResponse(t *testing.T, set *apicontract.Set, op apicontract.Operation, code int, body string) {
	t.Helper()
	responses, _ := op.Node["responses"].(map[string]any)
	if _, ok := responses[strconv.Itoa(code)]; !ok {
		if _, ok := responses["default"]; !ok {
			t.Errorf("%s %s: status %d tidak didokumentasikan di kontrak", op.Method, op.Path, code)
			return
		}
	}
	sch, err := set.ResponseSchema(op, code)
	if err != nil {
		t.Errorf("%s %s: %v", op.Method, op.Path, err)
		return
	}
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(body))
	if err != nil {
		t.Errorf("%s %s: body bukan JSON: %v\n%s", op.Method, op.Path, err, body)
		return
	}
	if err := sch.Validate(v); err != nil {
		t.Errorf("%s %s: body tidak sesuai kontrak (%d): %v\n%s", op.Method, op.Path, code, err, body)
	}
}

func TestContractResponses_ReferenceModules(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.Router
	_, in := buildConformance(t)

	for _, key := range []string{"acme.example", "acme.example-addon"} {
		if rec := call(h, "POST", "/api/platform/modules/"+key+"/install", ""); rec.Code != 200 {
			t.Fatalf("install %s = %d %s", key, rec.Code, rec.Body)
		}
		if rec := call(h, "POST", "/api/platform/modules/"+key+"/enable", ""); rec.Code != 200 {
			t.Fatalf("enable %s = %d %s", key, rec.Code, rec.Body)
		}
	}

	// Setiap operasi kontrak: path contoh menghasilkan 200 dan body lolos skemanya.
	for _, doc := range []string{"example", "example-addon"} {
		ops := in.Set.Operations(doc)
		if len(ops) == 0 {
			t.Fatalf("kontrak %s tidak punya operasi", doc)
		}
		for _, op := range ops {
			if strings.Contains(op.Path, "{") {
				t.Fatalf("%s %s: path berparameter perlu nilai contoh di test ini", op.Method, op.Path)
			}
			rec := call(h, op.Method, op.Path, "")
			if rec.Code != 200 {
				t.Errorf("%s %s = %d %s, want 200", op.Method, op.Path, rec.Code, rec.Body)
				continue
			}
			validateResponse(t, in.Set, op, rec.Code, rec.Body.String())
		}
	}

	var notes apicontract.Operation
	for _, op := range in.Set.Operations("example") {
		if op.OperationID == "listExampleNotes" {
			notes = op
		}
	}
	if notes.Path == "" {
		t.Fatal("operasi listExampleNotes tidak ada")
	}

	// Kasus error terdokumentasi: parameter salah -> 400 berbentuk Error.
	rec := call(h, "GET", "/api/example/notes?page=0", "")
	if rec.Code != 400 {
		t.Fatalf("page=0 = %d %s, want 400", rec.Code, rec.Body)
	}
	validateResponse(t, in.Set, notes, rec.Code, rec.Body.String())

	// Modul disabled -> 403 berbentuk Error (skema erp:common Error).
	if rec := call(h, "POST", "/api/platform/modules/acme.example-addon/disable", ""); rec.Code != 200 {
		t.Fatalf("disable addon = %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, "POST", "/api/platform/modules/acme.example/disable", ""); rec.Code != 200 {
		t.Fatalf("disable example = %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "GET", "/api/example/notes", "")
	if rec.Code != 403 || errCodeOf(t, rec) != "module_disabled" {
		t.Fatalf("notes saat disabled = %d %s, want 403 module_disabled", rec.Code, rec.Body)
	}
	validateResponse(t, in.Set, notes, rec.Code, rec.Body.String())
}
