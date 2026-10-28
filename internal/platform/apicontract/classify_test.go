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

// K-4 (Q32): pengecualian x-erp-allow-highly-confidential tidak menurunkan klasifikasi (route tetap highly_confidential,
// sehingga Cache-Control: no-store berlaku); nilainya harus string alasan tidak kosong, selain itu error.
func TestResponseClassification_AllowHighlyConfidentialException(t *testing.T) {
	mk := func(ext string) (*Set, Operation) {
		src := `openapi: 3.1.0
info: {title: p, version: '1'}
paths:
  /api/p/people:
    get:
      operationId: listPeople
      x-erp-permission: p.people.read
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  monthlySalary: { type: string, x-erp-classification: highly_confidential` + ext + ` }
`
		d, err := Parse("p", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewSet(d)
		if err != nil {
			t.Fatal(err)
		}
		return s, d.Operations()[0]
	}
	s, op := mk(", x-erp-allow-highly-confidential: 'Modul HR menampilkan gaji kepada HR berizin'")
	if got, err := s.ResponseClassification(op); err != nil || got != module.ClassHighlyConfidential {
		t.Errorf("dengan pengecualian: got %q err %v, want highly_confidential (no-store tetap berlaku)", got, err)
	}
	for _, bad := range []string{", x-erp-allow-highly-confidential: ''", ", x-erp-allow-highly-confidential: '   '", ", x-erp-allow-highly-confidential: true", ", x-erp-allow-highly-confidential: 5"} {
		s, op := mk(bad)
		if _, err := s.ResponseClassification(op); err == nil {
			t.Errorf("%s harus error (alasan wajib string tidak kosong)", bad)
		}
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

// R-3 (Q32 butir 14): pengecualian hanya untuk properti data HR (nik, npwp, bankAccount*, salary; per token, termasuk
// bentuk gabungan) pada properti skema respons 2xx; konsisten dengan rule Spectral erp-allow-highly-confidential-valid.
func TestResponseClassification_AllowHighlyRestrictedToHRProperties(t *testing.T) {
	classify := func(t *testing.T, schema string) error {
		t.Helper()
		src := `openapi: 3.1.0
info: {title: p, version: '1'}
paths:
  /api/p/people:
    get:
      operationId: listPeople
      x-erp-permission: p.people.read
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
` + schema
		d, err := Parse("p", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewSet(d)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.ResponseClassification(d.Operations()[0])
		return err
	}
	prop := func(name string) string {
		return "                type: object\n                properties:\n                  " + name +
			": { type: string, x-erp-classification: highly_confidential, x-erp-allow-highly-confidential: 'alasan' }\n"
	}
	for _, ok := range []string{"nik", "npwp", "monthlySalary", "bankAccountNumber", "bankaccount", "salary"} {
		if err := classify(t, prop(ok)); err != nil {
			t.Errorf("%s harus lolos: %v", ok, err)
		}
	}
	for _, bad := range []string{"password", "apiKey", "privatekey", "clientSecret", "refreshToken", "bankAccountPassword", "fullName", "label"} {
		if err := classify(t, prop(bad)); err == nil {
			t.Errorf("%s harus ditolak", bad)
		}
	}
	root := "                type: object\n                x-erp-classification: highly_confidential\n                x-erp-allow-highly-confidential: 'alasan'\n                properties:\n                  nik: { type: string }\n"
	if err := classify(t, root); err == nil {
		t.Error("pengecualian pada skema akar harus ditolak")
	}
}
