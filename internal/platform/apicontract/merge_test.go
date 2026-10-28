package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func lookup(t *testing.T, node any, path ...string) any {
	t.Helper()
	cur := node
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("bukan objek pada %q (path %v)", p, path)
		}
		cur, ok = m[p]
		if !ok {
			t.Fatalf("kunci %q tidak ada (path %v)", p, path)
		}
	}
	return cur
}

// AC FR-23: dua modul dengan nama skema sama tidak bentrok pada spec gabungan.
func TestMerge_SameSchemaNameTwoModules(t *testing.T) {
	s := loadSet(t, "common", "platform", "alpha", "beta")
	spec, err := s.Merge([]string{"common", "platform", "alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	schemas := lookup(t, spec, "components", "schemas").(map[string]any)
	for _, want := range []string{"alpha.Item", "beta.Item", "Uuid", "Page", "beta.Secret"} {
		if _, ok := schemas[want]; !ok {
			t.Errorf("components.schemas tidak memuat %q: %v", want, keys(schemas))
		}
	}
	if _, ok := schemas["Item"]; ok {
		t.Error("Item tanpa namespace tidak boleh ada")
	}
	ref := lookup(t, spec, "paths", "/api/alpha/items", "get", "responses", "200", "content", "application/json", "schema", "properties", "data", "items", "$ref")
	if ref != "#/components/schemas/alpha.Item" {
		t.Errorf("ref alpha = %v", ref)
	}
	ref = lookup(t, spec, "paths", "/api/beta/items", "get", "responses", "200", "content", "application/json", "schema", "properties", "data", "items", "$ref")
	if ref != "#/components/schemas/beta.Item" {
		t.Errorf("ref beta = %v", ref)
	}
	// common tidak diberi namespace; erp:common ditulis ulang menjadi ref lokal.
	ref = lookup(t, spec, "components", "schemas", "alpha.Item", "properties", "id", "$ref")
	if ref != "#/components/schemas/Uuid" {
		t.Errorf("ref Uuid = %v", ref)
	}
	data, _ := json.Marshal(spec)
	if strings.Contains(string(data), "erp:") {
		t.Errorf("spec gabungan masih memuat rujukan erp:")
	}
	if _, ok := lookup(t, spec, "paths").(map[string]any)["/api/bootstrap"]; !ok {
		t.Error("path platform hilang")
	}
}

func TestMerge_PathConflict(t *testing.T) {
	a := loadDoc(t, "alpha")
	dupSrc := `openapi: 3.1.0
info: {title: dup, version: '1'}
paths:
  /api/alpha/items:
    get:
      operationId: dupItems
      x-erp-permission: dup.items.read
      responses: {'200': {description: ok}}
`
	dup, err := Parse("dup", []byte(dupSrc))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSet(loadDoc(t, "common"), a, dup, loadDoc(t, "beta"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Merge([]string{"common", "beta", "alpha", "dup"})
	if err == nil || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "dup") {
		t.Errorf("err = %v, want konflik menyebut alpha dan dup", err)
	}
}

func TestMerge_OperationIDConflict(t *testing.T) {
	dupSrc := `openapi: 3.1.0
info: {title: dup, version: '1'}
paths:
  /api/dup/items:
    get:
      operationId: listAlphaItems
      x-erp-permission: dup.items.read
      responses: {'200': {description: ok}}
`
	dup, err := Parse("dup", []byte(dupSrc))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSet(loadDoc(t, "common"), loadDoc(t, "alpha"), loadDoc(t, "beta"), dup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Merge([]string{"common", "beta", "alpha", "dup"}); err == nil || !strings.Contains(err.Error(), "listAlphaItems") {
		t.Errorf("err = %v, want konflik operationId", err)
	}
}

func TestBundle_InlinesErpRefs(t *testing.T) {
	s := loadSet(t, "common", "platform", "alpha", "beta")
	for _, name := range []string{"beta", "alpha", "platform"} {
		spec, err := s.Bundle(name)
		if err != nil {
			t.Fatalf("Bundle(%s): %v", name, err)
		}
		data, _ := json.Marshal(spec)
		if strings.Contains(string(data), "erp:") {
			t.Errorf("Bundle(%s) masih memuat erp:", name)
		}
	}
	spec, _ := s.Bundle("alpha")
	schemas := lookup(t, spec, "components", "schemas").(map[string]any)
	for _, want := range []string{"Item", "common.Uuid", "common.Page", "beta.Secret"} {
		if _, ok := schemas[want]; !ok {
			t.Errorf("bundle alpha tidak memuat schema %q: %v", want, keys(schemas))
		}
	}
	if ref := lookup(t, spec, "components", "schemas", "Item", "properties", "id", "$ref"); ref != "#/components/schemas/common.Uuid" {
		t.Errorf("ref id = %v", ref)
	}
	if _, ok := lookup(t, spec, "components", "parameters").(map[string]any)["common.Page"]; !ok {
		t.Error("parameter common.Page tidak tersalin")
	}
	if _, ok := lookup(t, spec, "components", "responses").(map[string]any)["common.NotFound"]; !ok {
		t.Error("response common.NotFound tidak tersalin")
	}
	// Ref lokal di dalam komponen salinan ikut ditulis ulang (common.NotFound -> common.Error).
	if _, ok := schemas["common.Error"]; !ok {
		t.Error("common.Error (rujukan transitif) tidak tersalin")
	}
	// Deterministik.
	a, _ := json.Marshal(spec)
	spec2, _ := s.Bundle("alpha")
	b, _ := json.Marshal(spec2)
	if string(a) != string(b) {
		t.Error("Bundle tidak deterministik")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
