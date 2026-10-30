package moduletest

import (
	"context"

	"erpora/sdk/i18n"
	"erpora/sdk/module"
)

// I18nContext membangun context uji untuk handler, hook, atau command modul yang memakai i18n.T: Resolver berasal
// dari katalog modul sendiri (module.CatalogSource), scope = slug modul, locale = locale, dan i18n.StrictMissing(t)
// aktif sehingga salah ketik kunci menggagalkan test (Requirement 3.3). Kunci hilang pada locale jatuh ke bahasa dasar
// manifest (sama dengan fallback platform). Namespace "ui" dan "platform" milik platform dan tidak dapat diperiksa di
// sini: kuncinya dianggap ada (teks = kunci itu sendiri). Katalog yang rusak atau berkas bahasa yang hilang
// dilaporkan ke t. t biasanya *testing.T.
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
	for _, l := range []i18n.Locale{r.def, locale} {
		if _, done := r.flats[l]; done {
			continue
		}
		f, err := cs.Catalogs().Open(string(l) + ".json")
		if err != nil {
			t.Errorf("moduletest: berkas katalog %s.json tidak ditemukan: %v", l, err)
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
		for _, l := range []i18n.Locale{loc, r.def} {
			if src, ok := r.flats[l][key]; ok {
				return src, true
			}
		}
	}
	return "", false
}
