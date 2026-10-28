package i18n

import (
	"slices"
	"strings"
	"testing"
	"testing/quick"
)

const validRegistry = `{
  "default": "id",
  "locales": [
    { "code": "id", "name": "Bahasa Indonesia" },
    { "code": "en", "name": "English", "fallback": "id" },
    { "code": "en-GB", "name": "English (UK)", "fallback": "en" },
    { "code": "ms", "name": "Bahasa Melayu", "fallback": "id", "partial": true }
  ]
}`

func TestParseRegistryValid(t *testing.T) {
	r, err := ParseRegistry([]byte(validRegistry))
	if err != nil {
		t.Fatalf("ParseRegistry: %v", err)
	}
	if r.Default != "id" || len(r.Locales) != 4 {
		t.Fatalf("registry = %+v", r)
	}
	if !r.Has("en-GB") || r.Has("fr") {
		t.Error("Has tidak sesuai")
	}
	if info, _ := r.Info("ms"); !info.Partial || info.Name != "Bahasa Melayu" {
		t.Errorf("Info(ms) = %+v", info)
	}
	if codes := r.Codes(); !slices.Equal(codes, []string{"id", "en", "en-GB", "ms"}) {
		t.Errorf("Codes = %v", codes)
	}
}

func TestChain(t *testing.T) {
	r, err := ParseRegistry([]byte(validRegistry))
	if err != nil {
		t.Fatal(err)
	}
	for code, want := range map[string][]string{
		"id":    {"id"},
		"en":    {"en", "id"},
		"en-GB": {"en-GB", "en", "id"},
		"ms":    {"ms", "id"},
		"zz":    {"id"}, // tak dikenal: hanya default
	} {
		if got := r.Chain(code); !slices.Equal(got, want) {
			t.Errorf("Chain(%q) = %v, want %v", code, got, want)
		}
	}
}

func TestChainAppendsDefaultForRootlessLocale(t *testing.T) {
	r, err := ParseRegistry([]byte(`{"default":"id","locales":[{"code":"id","name":"ID"},{"code":"fr","name":"Francais"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Chain("fr"); !slices.Equal(got, []string{"fr", "id"}) {
		t.Errorf("Chain(fr) = %v", got)
	}
}

func TestParseRegistryRejectsInvalid(t *testing.T) {
	loc := func(items ...string) string {
		return `{"default":"id","locales":[` + strings.Join(items, ",") + `]}`
	}
	id := `{"code":"id","name":"ID"}`
	cases := map[string]struct{ in, want string }{
		"json rusak":               {`{`, "json"},
		"field tak dikenal":        {`{"default":"id","locales":[],"x":1}`, "json"},
		"tanpa default":            {`{"locales":[` + id + `]}`, "default"},
		"default tak terdaftar":    {`{"default":"xx","locales":[` + id + `]}`, "default"},
		"kode ganda":               {loc(id, id), "ganda"},
		"kode berpola salah":       {loc(id, `{"code":"EN","name":"x","fallback":"id"}`), "pola"},
		"kode terlalu panjang":     {loc(id, `{"code":"abcd","name":"x"}`), "pola"},
		"region huruf kecil":       {loc(id, `{"code":"en-gb","name":"x"}`), "pola"},
		"nama kosong":              {loc(id, `{"code":"en","name":"","fallback":"id"}`), "name"},
		"fallback tak terdaftar":   {loc(id, `{"code":"en","name":"x","fallback":"zz"}`), "fallback"},
		"default punya fallback":   {loc(`{"code":"id","name":"ID","fallback":"en"}`, `{"code":"en","name":"x","fallback":"id"}`), "default"},
		"siklus":                   {loc(id, `{"code":"en","name":"x","fallback":"fr"}`, `{"code":"fr","name":"x","fallback":"en"}`), "siklus"},
		"fallback ke diri sendiri": {loc(id, `{"code":"en","name":"x","fallback":"en"}`), "siklus"},
		"terlalu dalam": {loc(id,
			`{"code":"aa","name":"x","fallback":"id"}`,
			`{"code":"bb","name":"x","fallback":"aa"}`,
			`{"code":"cc","name":"x","fallback":"bb"}`,
			`{"code":"dd","name":"x","fallback":"cc"}`,
			`{"code":"ee","name":"x","fallback":"dd"}`,
			`{"code":"ff","name":"x","fallback":"ee"}`), "kedalaman"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRegistry([]byte(c.in))
			if err == nil {
				t.Fatalf("ParseRegistry(%s) harus galat", c.in)
			}
			if !strings.Contains(strings.ToLower(err.Error()), c.want) {
				t.Errorf("err = %q, harus memuat %q", err, c.want)
			}
		})
	}
}

func TestParseRegistryDepthBoundaryAllowed(t *testing.T) {
	in := `{"default":"id","locales":[{"code":"id","name":"x"},
	  {"code":"aa","name":"x","fallback":"id"},{"code":"bb","name":"x","fallback":"aa"},
	  {"code":"cc","name":"x","fallback":"bb"},{"code":"dd","name":"x","fallback":"cc"}]}`
	r, err := ParseRegistry([]byte(in))
	if err != nil {
		t.Fatalf("kedalaman 4 harus lolos: %v", err)
	}
	if got := r.Chain("dd"); len(got) != 5 {
		t.Errorf("Chain(dd) = %v", got)
	}
}

func TestLoadEmbeddedRegistry(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r.Default != "id" || !r.Has("id") || !r.Has("en") {
		t.Fatalf("registri bawaan = %+v", r)
	}
	if info, _ := r.Info("en"); info.Fallback != "id" {
		t.Errorf("en harus fallback ke id: %+v", info)
	}
}

// P2 Rantai fallback berhingga dan deterministik: untuk registri valid dan semua kode (termasuk yang
// tak dikenal), Chain berhingga, tanpa duplikat, diawali kode yang dikenal, dan diakhiri default.
func TestPropertyChainFiniteDeterministic(t *testing.T) {
	r, err := ParseRegistry([]byte(validRegistry))
	if err != nil {
		t.Fatal(err)
	}
	prop := func(code string) bool {
		c1, c2 := r.Chain(code), r.Chain(code)
		if !slices.Equal(c1, c2) || len(c1) == 0 || len(c1) > 6 {
			return false
		}
		seen := map[string]bool{}
		for _, x := range c1 {
			if seen[x] || !r.Has(x) {
				return false
			}
			seen[x] = true
		}
		if r.Has(code) && c1[0] != code {
			return false
		}
		return c1[len(c1)-1] == r.Default
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatal(err)
	}
	for _, code := range r.Codes() {
		if !prop(code) {
			t.Errorf("properti gagal untuk %q", code)
		}
	}
}
