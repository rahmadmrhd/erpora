package moduletest

import (
	"context"
	"strings"

	"erpora/sdk/i18n"
	"erpora/sdk/module"
)

// I18nContext membangun context uji untuk handler, hook, atau command modul yang memakai i18n.T: Resolver berasal
// dari katalog modul sendiri (module.CatalogSource), scope = slug modul, locale = locale, dan i18n.StrictMissing(t)
// aktif sehingga salah ketik kunci menggagalkan test (Requirement 3.3). t biasanya *testing.T.
//
// Rantai fallback (disederhanakan dari registri platform, yang tidak tersedia di sini): locale, lalu induk BCP 47
// dengan membuang subtag terakhir (en-GB, lalu en), lalu bahasa dasar manifest. Registri platform sebenarnya
// memakai fallback yang dikonfigurasi per locale (internal/platform/i18n/locales.json), jadi pada modul yang mengandalkan fallback
// khusus (mis. pt-BR ke es) hasil test dapat berbeda dari produksi; uji fallback itu di tingkat platform.
// Berkas katalog yang dideklarasikan di manifest (i18n.locales) tetapi hilang, serta katalog yang rusak,
// dilaporkan ke t; locale regional yang tidak dideklarasikan dan tidak punya berkas dilewati diam-diam
// (seperti produksi).
//
// Namespace "ui" dan "platform" milik platform dan tidak dapat diperiksa di sini: kuncinya dianggap ada (teks =
// kunci itu sendiri).
//
// PERINGATAN: i18n.StrictMissing bersifat global proses, jadi test modul yang memakai I18nContext tidak boleh
// memakai t.Parallel() (kunci hilang di test lain dapat dilaporkan ke test yang salah, dan cleanup satu test
// mematikan mode ketat test lain yang masih berjalan).
func I18nContext(t i18n.TestReporter, mod module.Module, locale i18n.Locale) context.Context {
	t.Helper()
	mf := mod.Manifest()
	r := &catalogResolver{slug: mf.Slug, flats: map[i18n.Locale]i18n.Flat{}}
	ctx := i18n.WithScope(i18n.WithLocale(context.Background(), locale), mf.Slug)
	cs, ok := mod.(module.CatalogSource)
	if !ok || mf.I18n == nil {
		t.Errorf("moduletest: modul %q tidak punya katalog (module.CatalogSource dan bagian i18n manifest)", mf.Key)
		return i18n.WithResolver(ctx, r)
	}
	r.def = i18n.Locale(mf.I18n.DefaultLocale)
	declared := map[i18n.Locale]bool{r.def: true}
	for _, l := range mf.I18n.Locales {
		declared[i18n.Locale(l)] = true
	}
	for _, l := range localeChain(locale, r.def) {
		if _, done := r.flats[l]; done {
			continue
		}
		f, err := cs.Catalogs().Open(string(l) + ".json")
		if err != nil {
			if declared[l] { // locale yang dideklarasikan wajib punya berkas; regional tak dideklarasikan boleh tidak ada
				t.Errorf("moduletest: berkas katalog %s.json tidak ditemukan: %v", l, err)
			}
			continue
		}
		flat, issues := i18n.ParseCatalog(f, i18n.DefaultLimits())
		_ = f.Close()
		for _, is := range issues {
			t.Errorf("moduletest: katalog %s.json: %s", l, is.Error())
		}
		r.flats[l] = flat
	}
	i18n.StrictMissing(t)
	return i18n.WithResolver(ctx, r)
}

// localeChain mengembalikan locale, induk BCP 47 (subtag terakhir dibuang berulang), lalu bahasa dasar (tanpa duplikat).
func localeChain(loc, def i18n.Locale) []i18n.Locale {
	var chain []i18n.Locale
	seen := map[i18n.Locale]bool{}
	add := func(l i18n.Locale) {
		if l != "" && !seen[l] {
			seen[l] = true
			chain = append(chain, l)
		}
	}
	for l := string(loc); l != ""; {
		add(i18n.Locale(l))
		i := strings.LastIndex(l, "-")
		if i < 0 {
			break
		}
		l = l[:i]
	}
	add(def)
	return chain
}

type catalogResolver struct {
	slug  string
	def   i18n.Locale
	flats map[i18n.Locale]i18n.Flat
}

func (r *catalogResolver) Resolve(_ context.Context, ns, key string, loc i18n.Locale) (string, bool) {
	switch ns {
	case i18n.NamespaceUI, i18n.NamespacePlatform:
		return ns + "::" + key, true
	case r.slug:
		for _, l := range localeChain(loc, r.def) {
			if src, ok := r.flats[l][key]; ok {
				return src, true
			}
		}
	}
	return "", false
}
