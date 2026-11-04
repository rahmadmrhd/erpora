package registry

import (
	"testing"
	"testing/fstest"

	"erpora/sdk/module"
)

// K-C2.5: label deklarasi otorisasi adalah kunci i18n yang harus ada di katalog bahasa dasar modul.
func TestAuthorizationLabelsMustExistInCatalog(t *testing.T) {
	mod := func(catalog string) module.Module {
		base := newFake(manifestSpec{key: "acme.notes", slug: "notes", caps: []string{"notes.items.read", "notes.items.adjust"}, i18n: i18nSpec("id")}, nil)
		return catalogFake{fakeModule: base, fsys: fstest.MapFS{"id.json": {Data: []byte(catalog)}}}
	}
	full := `{"resource":{"items":"Barang"},"fieldGroup":{"general":"Umum"},"action":{"adjust":"Sesuaikan"}}`
	if _, err := Build(testOpts(), mod(full)); err != nil {
		t.Fatalf("katalog lengkap harus lolos: %v", err)
	}
	_, err := Build(testOpts(), mod(`{"resource":{"items":"Barang"}}`))
	ps := problemsOf(t, err)
	got := map[string]bool{}
	for _, p := range ps {
		if p.Code == CodeAuthzLabelUnknown {
			got[p.Field] = true
		}
	}
	for _, f := range []string{"/resources/0/field_groups/0/label", "/resources/0/custom_actions/0/label"} {
		if !got[f] {
			t.Errorf("label tak ada di katalog tidak dilaporkan di %s: %+v", f, ps)
		}
	}
	if got["/resources/0/label"] {
		t.Errorf("label resource ada di katalog: %+v", ps)
	}
}

func TestAuthorizationLabelsSkippedWithoutI18n(t *testing.T) {
	m := newFake(spec("acme.a", "a", "a.x.read"), nil)
	if ps := CheckAuthorizationLabels(m.m, nil); len(ps) != 0 {
		t.Fatalf("modul tanpa i18n dilewati: %+v", ps)
	}
}
