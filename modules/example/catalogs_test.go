package example

import (
	"slices"
	"testing"

	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
)

var _ module.CatalogSource = (*Module)(nil)

// Katalog terjemahan modul (PRD-000 FR-25): satu berkas per bahasa yang dideklarasikan manifest, valid,
// sejajar antar bahasa, dan memuat errors.bad_request yang dipakai API notes (message galat dilokalkan,
// code tetap stabil).
func TestCatalogs_MatchManifestAndAreConsistent(t *testing.T) {
	m := New()
	decl := m.Manifest().I18n
	if decl == nil || !slices.Equal(decl.Locales, []string{"id", "en"}) || decl.DefaultLocale != "id" {
		t.Fatalf("manifest i18n = %+v", decl)
	}
	byLocale := map[string]sdki18n.Flat{}
	for _, loc := range decl.Locales {
		f, err := m.Catalogs().Open(loc + ".json")
		if err != nil {
			t.Fatalf("katalog %s: %v", loc, err)
		}
		flat, issues := sdki18n.ParseCatalog(f, sdki18n.DefaultLimits())
		_ = f.Close()
		if len(issues) > 0 {
			t.Fatalf("katalog %s tidak valid: %v", loc, issues)
		}
		byLocale[loc] = flat
	}
	for k := range byLocale["id"] {
		if _, ok := byLocale["en"][k]; !ok {
			t.Errorf("kunci %q ada di id tetapi tidak di en", k)
		}
	}
	if len(byLocale["en"]) != len(byLocale["id"]) {
		t.Errorf("jumlah kunci id=%d en=%d", len(byLocale["id"]), len(byLocale["en"]))
	}
	for _, loc := range decl.Locales {
		if byLocale[loc]["errors.bad_request"] == "" {
			t.Errorf("katalog %s tidak memuat errors.bad_request", loc)
		}
	}
}
