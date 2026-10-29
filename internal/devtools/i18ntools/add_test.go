package i18ntools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (r *repo) read(rel string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *repo) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(r.root, filepath.FromSlash(rel)))
	return err == nil
}

// addRepo: repo mini dengan platform/ui (katalog bersarang) dan satu modul "notes" yang mendeklarasikan id dan en.
func addRepo(t *testing.T) *repo {
	t.Helper()
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/id.json", `{"app":{"name":"ERPora"},"greet":"Halo {name}","count":"{n, plural, =0 {Kosong} other {# item}}"}`)
	r.write(PlatformCatalogDir+"/en.json", `{"app":{"name":"ERPora"},"greet":"Hello {name}","count":"{n, plural, =0 {Empty} other {# items}}"}`)
	r.module("notes", "notes", []string{"id", "en"}, "id", map[string]string{
		"id": `{"note":{"title":"Judul","status.draft":"Draf"}}`,
		"en": `{"note":{"title":"Title","status.draft":"Draft"}}`,
	})
	return r
}

func TestAdd_CreatesSkeletonAndRegistersLocale(t *testing.T) {
	r := addRepo(t)
	r.write(RegistryPath, `{
  "default": "id",
  "locales": [
    { "code": "id", "name": "Bahasa Indonesia" },
    { "code": "en", "name": "English", "fallback": "id" }
  ]
}
`)
	res, err := Add(r.root, AddOptions{Locale: "ms", Name: "Bahasa Melayu"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.RegistryUpdated || len(res.Created) != 3 {
		t.Fatalf("hasil = %+v", res)
	}

	reg := r.read(RegistryPath)
	wantReg := `{
  "default": "id",
  "locales": [
    { "code": "id", "name": "Bahasa Indonesia" },
    { "code": "en", "name": "English", "fallback": "id" },
    { "code": "ms", "name": "Bahasa Melayu", "fallback": "id" }
  ]
}
`
	if reg != wantReg {
		t.Errorf("registri =\n%s\nwant\n%s", reg, wantReg)
	}

	// kerangka: struktur bersarang dan urutan kunci sama dengan bahasa default; nilai bertanda TODO.
	got := r.read(PlatformCatalogDir + "/ms.json")
	want := "{\n  \"app\": {\n    \"name\": \"TODO: ERPora\"\n  },\n  \"greet\": \"TODO: Halo {name}\",\n  \"count\": \"TODO: {n, plural, =0 {Kosong} other {# item}}\"\n}\n"
	if got != want {
		t.Errorf("platform/ms.json =\n%s\nwant\n%s", got, want)
	}
	if !r.exists(UICatalogDir + "/ms.json") {
		t.Error("ui/ms.json tidak dibuat")
	}
	if m := r.read("modules/notes/module.json"); !strings.Contains(m, `"locales":["id","en","ms"]`) {
		t.Errorf("manifest modul tidak memuat ms:\n%s", m)
	}
	if !r.exists("modules/notes/i18n/ms.json") {
		t.Error("katalog modul tidak dibuat")
	}

	// hasil lolos i18n:check tanpa temuan dan tampil 0% di status
	if rep := r.check(true); len(rep.Findings) != 0 {
		t.Fatalf("check --strict harus bersih: %+v", rep.Findings)
	}
	rows, err := Status(r.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Locale == "ms" && (row.Translated != 0 || row.Percent != 0) {
			t.Errorf("baris ms harus 0%%: %+v", row)
		}
	}
}

func TestAdd_Idempotent(t *testing.T) {
	r := addRepo(t)
	if _, err := Add(r.root, AddOptions{Locale: "ms", Name: "Bahasa Melayu"}); err != nil {
		t.Fatal(err)
	}
	files := []string{RegistryPath, PlatformCatalogDir + "/ms.json", UICatalogDir + "/ms.json", "modules/notes/i18n/ms.json", "modules/notes/module.json"}
	before := map[string]string{}
	for _, f := range files {
		before[f] = r.read(f)
	}
	res, err := Add(r.root, AddOptions{Locale: "ms", Name: "Bahasa Melayu"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RegistryUpdated || len(res.Created) != 0 || len(res.ManifestsUpdated) != 0 || len(res.Skipped) != 3 {
		t.Fatalf("jalan kedua harus tanpa perubahan: %+v", res)
	}
	for _, f := range files {
		if r.read(f) != before[f] {
			t.Errorf("%s berubah pada jalan kedua", f)
		}
	}
}

func TestAdd_DoesNotOverwriteExistingTranslation(t *testing.T) {
	r := addRepo(t)
	r.write(RegistryPath, `{"default":"id","locales":[{"code":"id","name":"I"},{"code":"en","name":"E","fallback":"id"},{"code":"ms","name":"M","fallback":"id","partial":true}]}`)
	r.write(UICatalogDir+"/ms.json", `{"actions":{"save":"Simpan"}}`)
	res, err := Add(r.root, AddOptions{Locale: "ms"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RegistryUpdated {
		t.Error("bahasa sudah terdaftar: registri tidak boleh berubah")
	}
	if got := r.read(UICatalogDir + "/ms.json"); got != `{"actions":{"save":"Simpan"}}` {
		t.Errorf("berkas yang ada ditimpa: %s", got)
	}
	if !r.exists(PlatformCatalogDir + "/ms.json") {
		t.Error("platform/ms.json harus dibuat")
	}
}

func TestAdd_SelectedModulesOnly(t *testing.T) {
	r := addRepo(t)
	res, err := Add(r.root, AddOptions{Locale: "ms", Modules: []string{"notes"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 1 || res.Created[0] != "modules/notes/i18n/ms.json" {
		t.Fatalf("hasil = %+v", res)
	}
	if r.exists(PlatformCatalogDir+"/ms.json") || r.exists(UICatalogDir+"/ms.json") {
		t.Error("platform/ui tidak dipilih")
	}
	// ui dan platform dapat dipilih eksplisit
	if _, err := Add(r.root, AddOptions{Locale: "ms", Modules: []string{"ui"}}); err != nil {
		t.Fatal(err)
	}
	if !r.exists(UICatalogDir+"/ms.json") || r.exists(PlatformCatalogDir+"/ms.json") {
		t.Error("hanya ui yang dibuat")
	}
}

func TestAdd_ModuleWithoutManifestDeclarationKeepsManifest(t *testing.T) {
	r := addRepo(t)
	r.write("modules/legacy/module.json", `{"schema_version":"1","key":"acme.legacy","slug":"legacy","name":"X","version":"1.0.0"}`)
	r.write("modules/legacy/i18n/id.json", `{"a":"A"}`)
	before := r.read("modules/legacy/module.json")
	if _, err := Add(r.root, AddOptions{Locale: "ms", Modules: []string{"legacy"}}); err != nil {
		t.Fatal(err)
	}
	if !r.exists("modules/legacy/i18n/ms.json") || r.read("modules/legacy/module.json") != before {
		t.Error("katalog dibuat dan manifest tak berubah")
	}
}

func TestAdd_Validation(t *testing.T) {
	var ve *ValidationError
	cases := map[string]AddOptions{
		"kode tidak sah":           {Locale: "MS_my"},
		"modul tidak dikenal":      {Locale: "ms", Modules: []string{"tidakada"}},
		"fallback tidak terdaftar": {Locale: "ms", Fallback: "zz"},
		"kode kosong":              {Locale: ""},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			r := addRepo(t)
			if _, err := Add(r.root, opts); !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
			if r.exists(PlatformCatalogDir + "/ms.json") {
				t.Error("tidak boleh menulis apa pun saat validasi gagal")
			}
		})
	}
	t.Run("katalog default rusak", func(t *testing.T) {
		r := addRepo(t)
		r.write(UICatalogDir+"/id.json", `{rusak`)
		if _, err := Add(r.root, AddOptions{Locale: "ms"}); !errors.As(err, &ve) {
			t.Fatalf("err = %v", err)
		}
		if r.exists(PlatformCatalogDir + "/ms.json") {
			t.Error("tidak boleh menulis sebagian")
		}
	})
	t.Run("registri tidak ada", func(t *testing.T) {
		r := addRepo(t)
		if err := os.Remove(filepath.Join(r.root, filepath.FromSlash(RegistryPath))); err != nil {
			t.Fatal(err)
		}
		var ce *ConfigError
		if _, err := Add(r.root, AddOptions{Locale: "ms"}); !errors.As(err, &ce) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestAdd_NameDefaultsToCodeAndFallbackDefaultsToDefaultLocale(t *testing.T) {
	r := addRepo(t)
	if _, err := Add(r.root, AddOptions{Locale: "ms", Fallback: "en", Modules: []string{"ui"}}); err != nil {
		t.Fatal(err)
	}
	if reg := r.read(RegistryPath); !strings.Contains(reg, `{ "code": "ms", "name": "ms", "fallback": "en" }`) {
		t.Errorf("registri:\n%s", reg)
	}
}

func TestAppendToArray_Formats(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"satu baris dengan spasi", `{"i18n": { "locales": ["id", "en"], "defaultLocale": "id" }}`, `{"i18n": { "locales": ["id", "en", "ms"], "defaultLocale": "id" }}`},
		{"satu baris tanpa spasi", `{"i18n":{"locales":["id","en"]}}`, `{"i18n":{"locales":["id","en","ms"]}}`},
		{"banyak baris", "{\n  \"i18n\": {\n    \"locales\": [\n      \"id\",\n      \"en\"\n    ]\n  }\n}", "{\n  \"i18n\": {\n    \"locales\": [\n      \"id\",\n      \"en\",\n      \"ms\"\n    ]\n  }\n}"},
		{"array kosong", `{"i18n":{"locales":[]}}`, `{"i18n":{"locales":["ms"]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := appendToArray([]byte(tc.src), []string{"i18n", "locales"}, `"ms"`)
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %q err %v\nwant %q", got, err, tc.want)
			}
		})
	}
	if _, err := appendToArray([]byte(`{"a":1}`), []string{"a"}, `"x"`); err == nil {
		t.Error("nilai bukan array harus galat")
	}
	if _, err := appendToArray([]byte(`{"a":[]}`), []string{"b"}, `"x"`); err == nil {
		t.Error("anggota tidak ada harus galat")
	}
}
