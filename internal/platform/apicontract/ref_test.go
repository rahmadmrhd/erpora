package apicontract

import "testing"

func TestParseRef_Table(t *testing.T) {
	cases := []struct {
		ref     string
		want    Ref
		wantErr bool
	}{
		{"#/x", Ref{"here", "/x"}, false},
		{"#/components/schemas/A~1B", Ref{"here", "/components/schemas/A~1B"}, false},
		{"erp:common#/components/schemas/Uuid", Ref{"common", "/components/schemas/Uuid"}, false},
		{"erp:example-addon#/a", Ref{"example-addon", "/a"}, false},
		{"../x.yaml#/a", Ref{}, true},
		{"x.yaml#/a", Ref{}, true},
		{"https://x/y#/a", Ref{}, true},
		{"erp:#/a", Ref{}, true},
		{"erp:common", Ref{}, true},
		{"erp:common#", Ref{}, true},
		{"#", Ref{}, true},
		{"", Ref{}, true},
	}
	for _, c := range cases {
		got, err := ParseRef("here", c.ref)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseRef(%q): err = %v, wantErr %v", c.ref, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("ParseRef(%q) = %+v, want %+v", c.ref, got, c.want)
		}
	}
}

func TestSet_ResolveAndDuplicates(t *testing.T) {
	s := loadSet(t, "common", "alpha")
	doc, node, err := s.Resolve("alpha", "erp:common#/components/schemas/Uuid")
	if err != nil || doc != "common" {
		t.Fatalf("Resolve: doc=%q err=%v", doc, err)
	}
	if m, ok := node.(map[string]any); !ok || m["format"] != "uuid" {
		t.Errorf("node = %#v", node)
	}
	if _, _, err := s.Resolve("alpha", "#/components/schemas/Hilang"); err == nil {
		t.Error("ref hilang harus error")
	} else if msg := err.Error(); !contains(msg, "alpha") || !contains(msg, "Hilang") {
		t.Errorf("error harus menyebut asal dan ref: %v", err)
	}
	if _, _, err := s.Resolve("alpha", "erp:tidakada#/a"); err == nil {
		t.Error("doc tidak ada harus error")
	}
	if _, err := NewSet(loadDoc(t, "alpha"), loadDoc(t, "alpha")); err == nil {
		t.Error("nama ganda harus error")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
