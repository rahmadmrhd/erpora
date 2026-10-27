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
