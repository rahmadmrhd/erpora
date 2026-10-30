package exampleaddon

import (
	"slices"
	"testing"

	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var _ module.CatalogSource = (*Module)(nil)

// Katalog terjemahan modul (PRD-000 FR-25): satu berkas per bahasa yang dideklarasikan manifest, valid, dan
// sejajar antar bahasa.
func TestCatalogs_MatchManifestAndAreConsistent(t *testing.T) {
	m := New(fakeNotes{})
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
}

// Kunci yang dirujuk kode diperiksa lewat moduletest.I18nContext: StrictMissing menggagalkan test bila kunci salah
// ketik atau hilang di bahasa dasar. Jangan memakai t.Parallel() pada test yang memakai I18nContext (StrictMissing
// global proses). en-GB tidak punya berkas sendiri dan jatuh ke en, lalu bahasa dasar (rantai fallback helper).
func TestCatalogs_KeysResolveUnderStrictMissing(t *testing.T) {
	for _, loc := range []sdki18n.Locale{"id", "en", "en-GB"} {
		t.Run(string(loc), func(t *testing.T) {
			ctx := moduletest.I18nContext(t, New(fakeNotes{}), loc)
			if got := sdki18n.T(ctx, "page.summary.title"); got == "" || got == "page.summary.title" {
				t.Errorf("page.summary.title = %q", got)
			}
		})
	}
}
