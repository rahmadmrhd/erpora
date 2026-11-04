package authz

import "testing"

func TestCanonicalField(t *testing.T) {
	cases := map[string]string{
		"costPrice":    "cost_price",
		"cost_price":   "cost_price",
		"title":        "title",
		"address2Line": "address2_line",
		"pointX":       "point_x",
		"a1B2":         "a1_b2",
		"createdBy":    "created_by",
		"id":           "id",
		// tak dikenal -> "" (tidak pernah jatuh ke grup default)
		"":           "",
		"taxID":      "",
		"HTTPServer": "",
		"Title":      "",
		"_x":         "",
		"x-y":        "",
		"x y":        "",
		"cost_Price": "",
		"é":          "",
	}
	for in, want := range cases {
		if got := CanonicalField(in); got != want {
			t.Errorf("CanonicalField(%q) = %q, want %q", in, got, want)
		}
	}
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if CanonicalField(string(long)) != "" {
		t.Error("lebih dari 63 byte harus ditolak")
	}
	// camelCase yang hasil snake-nya melebihi 63 byte juga ditolak
	over := ""
	for i := 0; i < 32; i++ {
		over += "aB"
	}
	if CanonicalField(over) != "" {
		t.Errorf("hasil > 63 byte harus ditolak: %q", CanonicalField(over))
	}
}
