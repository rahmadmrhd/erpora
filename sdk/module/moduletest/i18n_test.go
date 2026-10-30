package moduletest

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/sdk/i18n"
	"erpora/sdk/module"
)

// catalogModule adalah modul uji ber-katalog.
type catalogModule struct {
	mf module.Manifest
	fs fs.FS
}

func (c catalogModule) Manifest() module.Manifest     { return c.mf }
func (catalogModule) Register(module.Registrar) error { return nil }
func (c catalogModule) Catalogs() fs.FS               { return c.fs }

func newCatalogModule(t *testing.T, files map[string]string) catalogModule {
	t.Helper()
	mf, err := module.ParseManifest([]byte(`{"schema_version":"1","key":"acme.notes","slug":"notes","name":"Notes","version":"1.0.0",
		"publisher":{"id":"acme","name":"Acme"},"trust":"builtin","compatibility":{"platform":"^0.1","sdk":"^0.1"},
		"dependencies":[],"capabilities":[],"migrations":{"strategy":"none"},"offline":{"mode":"online-only"},
		"i18n":{"locales":["id","en"],"defaultLocale":"id"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return catalogModule{mf: mf, fs: m}
}

type recReporter struct {
	errs     []string
	cleanups []func()
}

func (r *recReporter) Helper()                   {}
func (r *recReporter) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recReporter) Cleanup(fn func())         { r.cleanups = append(r.cleanups, fn) }
func (r *recReporter) finish() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}

// F-12: I18nContext memberi modul konteks penerjemah dari katalognya sendiri dengan StrictMissing aktif, sehingga salah
// ketik kunci di handler modul menggagalkan test modul (Requirement 3.3).
func TestI18nContextStrictForModuleCatalogs(t *testing.T) {
	m := newCatalogModule(t, map[string]string{
		"id.json": `{"note":{"title":"Catatan","only_id":"Hanya id"}}`,
		"en.json": `{"note":{"title":"Note"}}`,
	})
	rep := &recReporter{}
	ctx := I18nContext(rep, m, "en")
	if got := i18n.T(ctx, "note.title"); got != "Note" {
		t.Errorf("en = %q", got)
	}
	if got := i18n.T(ctx, "note.only_id"); got != "Hanya id" {
		t.Errorf("fallback ke bahasa default = %q", got)
	}
	if got := i18n.T(ctx, "ui::actions.save"); got == "" {
		t.Error("namespace platform/ui tidak dapat diperiksa; tidak boleh dianggap hilang")
	}
	if len(rep.errs) != 0 {
		t.Fatalf("kunci yang ada tidak boleh gagal: %v", rep.errs)
	}
	_ = i18n.T(ctx, "note.titel") // salah ketik
	if len(rep.errs) != 1 || !strings.Contains(rep.errs[0], "notes::note.titel") {
		t.Errorf("salah ketik harus menggagalkan test: %v", rep.errs)
	}
	rep.finish()
	_ = i18n.T(ctx, "note.lagi.salah")
	if len(rep.errs) != 1 {
		t.Errorf("setelah cleanup tidak ada pelaporan lagi: %v", rep.errs)
	}
}

func TestI18nContextReportsBrokenOrMissingCatalogs(t *testing.T) {
	rep := &recReporter{}
	_ = I18nContext(rep, newCatalogModule(t, map[string]string{"id.json": `{"a": "x", "a": "y"}`, "en.json": `{}`}), "en")
	if len(rep.errs) == 0 {
		t.Error("katalog rusak (kunci ganda) harus dilaporkan")
	}
	rep2 := &recReporter{}
	_ = I18nContext(rep2, newCatalogModule(t, map[string]string{"id.json": `{}`}), "en") // en.json tidak ada
	if len(rep2.errs) == 0 {
		t.Error("berkas bahasa hilang harus dilaporkan")
	}
}
