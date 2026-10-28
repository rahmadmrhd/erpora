package apicontract

import (
	"os"
	"strings"
	"testing"
)

func TestParse_Rejects(t *testing.T) {
	anchors, err := os.ReadFile("testdata/anchors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"anchor/alias":  string(anchors),
		"openapi 3.0":   "openapi: 3.0.3\ninfo: {title: x, version: '1'}\npaths: {}\n",
		"path non-api":  "openapi: 3.1.0\ninfo: {title: x, version: '1'}\npaths:\n  /v1/x:\n    get: {responses: {}}\n",
		"kunci integer": "openapi: 3.1.0\ninfo: {title: x, version: '1'}\npaths:\n  /api/x/y:\n    get:\n      responses:\n        200: {description: ok}\n",
		"yaml rusak":    "openapi: [3.1.0\n",
		"ref relatif":   "openapi: 3.1.0\ninfo: {title: x, version: '1'}\npaths: {}\ncomponents:\n  schemas:\n    A: {$ref: '../common.yaml#/components/schemas/B'}\n",
		"bukan objek":   "- a\n- b\n",
	}
	for name, src := range cases {
		if _, err := Parse("x", []byte(src)); err == nil {
			t.Errorf("%s: Parse harus gagal", name)
		}
	}
}

func TestParse_OperationsOrderedWithExtensions(t *testing.T) {
	d := loadDoc(t, "alpha")
	ops := d.Operations()
	if len(ops) != 5 {
		t.Fatalf("operasi = %d, want 5: %+v", len(ops), ops)
	}
	for i := 1; i < len(ops); i++ {
		a, b := ops[i-1], ops[i]
		if a.Path > b.Path || (a.Path == b.Path && a.Method > b.Method) {
			t.Errorf("operasi tidak terurut: %s %s sebelum %s %s", a.Method, a.Path, b.Method, b.Path)
		}
	}
	list := opOf(t, loadSet(t, "common", "alpha", "beta"), "alpha", "listAlphaItems")
	if list.Method != "GET" || list.Path != "/api/alpha/items" || list.Permission != "alpha.items.read" || list.Public || list.Deprecated {
		t.Errorf("listAlphaItems = %+v", list)
	}
	if got := strings.Join(list.DeprecatedQueryParams, ","); got != "legacy" {
		t.Errorf("DeprecatedQueryParams = %q, want legacy", got)
	}
	health := opOf(t, loadSet(t, "common", "alpha", "beta"), "alpha", "getAlphaHealth")
	if !health.Public || health.Permission != "" {
		t.Errorf("health = %+v", health)
	}
	secrets := opOf(t, loadSet(t, "common", "alpha", "beta"), "alpha", "listAlphaSecrets")
	if !secrets.Deprecated {
		t.Errorf("secrets harus deprecated: %+v", secrets)
	}
	if got := d.ExternalRefs(); strings.Join(got, ",") != "beta,common" {
		t.Errorf("ExternalRefs = %v, want [beta common]", got)
	}
}
