package apicontract

import (
	"testing"

	"erpora/sdk/module"
)

// Review Focus 5: klasifikasi respons dihitung rekursif (properti bersarang, array, $ref lintas dokumen, siklus).
func TestResponseClassification_Nested(t *testing.T) {
	s := loadSet(t, "common", "alpha", "beta", "cyclic")
	cases := []struct {
		doc, op string
		want    module.Classification
		wantErr bool
	}{
		{"alpha", "listAlphaCustomers", module.ClassConfidential, false},     // data.items[].customer.address
		{"alpha", "listAlphaSecrets", module.ClassHighlyConfidential, false}, // $ref ke erp:beta#/.../Secret
		{"alpha", "listAlphaItems", module.ClassInternal, false},             // tanpa anotasi
		{"alpha", "getAlphaHealth", module.ClassInternal, false},
		{"cyclic", "listNodes", module.ClassInternal, false}, // siklus selesai tanpa overflow
		{"alpha", "getAlphaBadClass", "", true},              // nilai tidak valid
	}
	for _, c := range cases {
		got, err := s.ResponseClassification(opOf(t, s, c.doc, c.op))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.op, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("%s: klasifikasi = %q, want %q", c.op, got, c.want)
		}
	}
}

// K-12: klasifikasi menelusuri SEMUA keyword subskema JSON Schema 2020-12 dan properti yang bersanding dengan $ref
// (OpenAPI 3.1 mengizinkannya), bukan hanya properties/items/allOf.
func TestResponseClassification_AllSubschemaKeywords(t *testing.T) {
	const secret = "{ type: string, x-erp-classification: highly_confidential }"
	cases := map[string]string{
		"properties":                "properties: { v: " + secret + " }",
		"patternProperties":         "patternProperties: { '^x-': " + secret + " }",
		"dependentSchemas":          "dependentSchemas: { a: { properties: { v: " + secret + " } } }",
		"items":                     "items: " + secret,
		"prefixItems":               "prefixItems: [" + secret + "]",
		"additionalProperties":      "additionalProperties: " + secret,
		"unevaluatedProperties":     "unevaluatedProperties: " + secret,
		"unevaluatedItems":          "unevaluatedItems: " + secret,
		"contains":                  "contains: " + secret,
		"not":                       "not: " + secret,
		"if":                        "if: " + secret,
		"then":                      "then: " + secret,
		"else":                      "else: " + secret,
		"allOf":                     "allOf: [" + secret + "]",
		"oneOf":                     "oneOf: [" + secret + "]",
		"anyOf":                     "anyOf: [" + secret + "]",
		"sibling properties of ref": "$ref: '#/components/schemas/Plain'\n                properties: { v: " + secret + " }",
		"sibling class of ref":      "$ref: '#/components/schemas/Plain'\n                x-erp-classification: highly_confidential",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			src := `openapi: 3.1.0
info: {title: p, version: '1'}
paths:
  /api/p/things:
    get:
      operationId: listThings
      x-erp-permission: p.things.read
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                ` + body + `
components:
  schemas:
    Plain: { type: object, properties: { name: { type: string } } }
`
			d, err := Parse("p", []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			s, err := NewSet(d)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.ResponseClassification(d.Operations()[0])
			if err != nil || got != module.ClassHighlyConfidential {
				t.Errorf("got %q err %v, want highly_confidential", got, err)
			}
		})
	}
}

func TestResponseClassification_AllPublicStaysPublic(t *testing.T) {
	src := `openapi: 3.1.0
info: {title: p, version: '1'}
paths:
  /api/p/things:
    get:
      operationId: listThings
      x-erp-permission: p.things.read
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                x-erp-classification: public
                properties:
                  name: { type: string }
`
	d, err := Parse("p", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSet(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ResponseClassification(d.Operations()[0])
	if err != nil || got != module.ClassPublic {
		t.Errorf("got %q err %v, want public", got, err)
	}
}
