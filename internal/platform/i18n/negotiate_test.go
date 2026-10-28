package i18n

import (
	"strings"
	"testing"
	"testing/quick"
)

func TestNegotiate(t *testing.T) {
	r := testRegistry(t)
	cases := map[string]string{
		"":                        "id",
		"id":                      "id",
		"en":                      "en",
		"en-GB":                   "en-GB",
		"en-US":                   "en", // wilayah tak terdaftar -> bahasa terdekat
		"en-GB,en;q=0.9":          "en-GB",
		"en;q=0.5, id;q=0.9":      "id",
		"id-ID,id;q=0.9,en;q=0.8": "id",
		"fr-FR,fr;q=0.9,en;q=0.8": "en", // fr tak terdaftar, en cocok
		"zh-CN":                   "id", // tidak ada kecocokan -> default
		"*":                       "id",
		"xx-invalid-@@@":          "id",
		"en;q=abc":                "id",
		"ms":                      "ms",
		"ms-MY":                   "ms",
	}
	for in, want := range cases {
		if got := r.Negotiate(in); got != want {
			t.Errorf("Negotiate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNegotiateIgnoresOversizedHeader(t *testing.T) {
	r := testRegistry(t)
	long := "en," + strings.Repeat("x", maxAcceptLanguageLen)
	if got := r.Negotiate(long); got != "id" {
		t.Errorf("header > 1 KB harus diabaikan (default), got %q", got)
	}
	ok := "en" + strings.Repeat(" ", maxAcceptLanguageLen-2)
	if got := r.Negotiate(ok); got != "en" {
		t.Errorf("header tepat di batas harus diproses, got %q", got)
	}
}

// P9 Negosiasi tertutup: untuk string Accept-Language apa pun, hasilnya selalu anggota registri.
func TestPropertyNegotiateClosed(t *testing.T) {
	r := testRegistry(t)
	prop := func(s string) bool { return r.Has(r.Negotiate(s)) }
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"\x00", "en\x00", ";;;", ",,,", "en;q=1e999", "\xff\xfe", "en-" + strings.Repeat("a", 200)} {
		if !prop(s) {
			t.Errorf("Negotiate(%q) bukan anggota registri", s)
		}
	}
}

func FuzzNegotiate(f *testing.F) {
	for _, s := range []string{"", "en", "en-GB,en;q=0.9", "*", "id;q=0", "fr, *;q=0.1", "en;q=", ";", "en-u-nu-arab"} {
		f.Add(s)
	}
	r, err := ParseRegistry([]byte(validRegistry))
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := r.Negotiate(s); !r.Has(got) {
			t.Fatalf("Negotiate(%q) = %q bukan anggota registri", s, got)
		}
	})
}
