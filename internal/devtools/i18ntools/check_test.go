package i18ntools

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo adalah workspace mini: registri id (default) dan en (fallback id), katalog platform dan ui yang sejajar.
type repo struct {
	t    *testing.T
	root string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, root: t.TempDir()}
	r.write(RegistryPath, `{"default":"id","locales":[{"code":"id","name":"Bahasa Indonesia"},{"code":"en","name":"English","fallback":"id"}]}`)
	r.write(PlatformCatalogDir+"/id.json", `{"app":{"name":"ERPora"},"greet":"Halo {name}"}`)
	r.write(PlatformCatalogDir+"/en.json", `{"app":{"name":"ERPora"},"greet":"Hello {name}"}`)
	r.write(UICatalogDir+"/id.json", `{"actions":{"save":"Simpan"}}`)
	r.write(UICatalogDir+"/en.json", `{"actions":{"save":"Save"}}`)
	return r
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

// module menulis modules/<dir>/module.json dengan bagian i18n (locales dan default) dan katalog per bahasa.
func (r *repo) module(dir, slug string, locales []string, def string, catalogs map[string]string) {
	r.t.Helper()
	loc, _ := json.Marshal(locales)
	r.write("modules/"+dir+"/module.json",
		`{"schema_version":"1","key":"acme.`+slug+`","slug":"`+slug+`","name":"X","version":"1.0.0","i18n":{"locales":`+string(loc)+`,"defaultLocale":"`+def+`"}}`)
	for code, body := range catalogs {
		r.write("modules/"+dir+"/i18n/"+code+".json", body)
	}
}

func (r *repo) check(strict bool) *Report {
	r.t.Helper()
	rep, err := Check(r.root, CheckOptions{Strict: strict})
	if err != nil {
		r.t.Fatalf("Check: %v", err)
	}
	return rep
}

func findCode(rep *Report, code string) []Finding {
	var out []Finding
	for _, f := range rep.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func wantOne(t *testing.T, rep *Report, code string, sev Severity) Finding {
	t.Helper()
	got := findCode(rep, code)
	if len(got) != 1 {
		t.Fatalf("temuan %s = %d, want 1; semua: %+v", code, len(got), rep.Findings)
	}
	if got[0].Severity != sev {
		t.Errorf("%s severity = %s, want %s", code, got[0].Severity, sev)
	}
	return got[0]
}

func TestCheck_CleanRepo(t *testing.T) {
	r := newRepo(t)
	r.module("notes", "notes", []string{"id", "en"}, "id", map[string]string{
		"id": `{"note":{"title":"Judul","count":"{count, plural, =0 {Kosong} other {# catatan}}"}}`,
		"en": `{"note":{"title":"Title","count":"{count, plural, =0 {Empty} one {# note} other {# notes}}"}}`,
	})
	rep := r.check(false)
	if len(rep.Findings) != 0 || rep.Failed() {
		t.Fatalf("repo bersih harus tanpa temuan: %+v", rep.Findings)
	}
	if rep.Findings == nil {
		t.Error("Findings harus slice kosong (bukan nil) agar --json memuat [] ")
	}
}

// Galat struktural (Requirement 8.1): setiap kasus menggagalkan (Failed) dengan kode, berkas, bahasa, dan kunci.
func TestCheck_StructuralErrors(t *testing.T) {
	cases := []struct {
		name    string
		body    string // isi platform/en.json yang dirusak
		code    string
		key     string
		detail  string // potongan Detail (opsional)
		differs bool   // true bila kunci tidak diperiksa
	}{
		{name: "json rusak", body: `{"app": `, code: "invalid_json", differs: true},
		{name: "icu tidak valid", body: `{"app":{"name":"ERPora"},"greet":"Hello {name"}`, code: "invalid_icu", key: "greet"},
		{name: "kunci ganda", body: `{"app":{"name":"A","name":"B"},"greet":"Hello {name}"}`, code: "duplicate_key", key: "app.name"},
		{name: "tabrakan perataan", body: `{"a.b":{"c":"x"},"a":{"b.c":"y"},"greet":"Hello {name}"}`, code: "flatten_collision", key: "a.b.c", detail: "a.b -> c"},
		{name: "karakter terlarang", body: `{"app":{"na me":"A"},"greet":"Hello {name}"}`, code: "invalid_segment", key: "app.na me"},
		{name: "html", body: `{"app":{"name":"<script>x</script>"},"greet":"Hello {name}"}`, code: "html_not_allowed", key: "app.name"},
		{name: "kunci bernamespace pemilik lain", body: `{"orders::title":"x","greet":"Hello {name}"}`, code: "invalid_namespace", key: "orders::title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.write(PlatformCatalogDir+"/en.json", tc.body)
			rep := r.check(false)
			if !rep.Failed() {
				t.Fatalf("harus gagal: %+v", rep.Findings)
			}
			var hit *Finding
			for i := range rep.Findings {
				if rep.Findings[i].Code == tc.code {
					hit = &rep.Findings[i]
				}
			}
			if hit == nil {
				t.Fatalf("tidak ada temuan %s: %+v", tc.code, rep.Findings)
			}
			if hit.Severity != SeverityError || hit.Namespace != "platform" || hit.Locale != "en" ||
				hit.File != PlatformCatalogDir+"/en.json" {
				t.Errorf("temuan = %+v", *hit)
			}
			if !tc.differs && hit.Key != tc.key {
				t.Errorf("key = %q, want %q", hit.Key, tc.key)
			}
		})
	}
}

func TestCheck_FlattenCollisionNamesBothPaths(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/id.json", `{"a.b":{"c":"x"},"a":{"b.c":"y"},"greet":"Halo {name}"}`)
	f := wantOne(t, r.check(false), "flatten_collision", SeverityError)
	if !strings.Contains(f.Detail, "a.b") || !strings.Contains(f.Detail, "b.c") {
		t.Errorf("detail harus menyebut kedua jalur: %q", f.Detail)
	}
}

func TestCheck_ModuleNamespaceAndDeclarations(t *testing.T) {
	t.Run("bahasa tidak terdaftar", func(t *testing.T) {
		r := newRepo(t)
		r.module("notes", "notes", []string{"id", "fr"}, "id", map[string]string{"id": `{"a":"A"}`, "fr": `{"a":"A"}`})
		f := wantOne(t, r.check(false), CodeUnregisteredLocale, SeverityError)
		if f.Namespace != "notes" || f.Locale != "fr" {
			t.Errorf("temuan = %+v", f)
		}
	})
	t.Run("berkas bahasa yang dideklarasikan hilang", func(t *testing.T) {
		r := newRepo(t)
		r.module("notes", "notes", []string{"id", "en"}, "id", map[string]string{"id": `{"a":"A"}`})
		f := wantOne(t, r.check(false), CodeMissingFile, SeverityError)
		if f.File != "modules/notes/i18n/en.json" || f.Locale != "en" {
			t.Errorf("temuan = %+v", f)
		}
	})
	t.Run("defaultLocale di luar locales", func(t *testing.T) {
		r := newRepo(t)
		r.module("notes", "notes", []string{"id"}, "en", map[string]string{"id": `{"a":"A"}`})
		wantOne(t, r.check(false), CodeInvalidManifest, SeverityError)
	})
	t.Run("slug modul memakai namespace tercadang", func(t *testing.T) {
		r := newRepo(t)
		r.module("ui", "ui", []string{"id"}, "id", map[string]string{"id": `{"a":"A"}`})
		wantOne(t, r.check(false), CodeInvalidNamespace, SeverityError)
	})
	t.Run("direktori i18n tanpa bagian manifest hanya peringatan", func(t *testing.T) {
		r := newRepo(t)
		r.write("modules/notes/module.json", `{"schema_version":"1","key":"acme.notes","slug":"notes","name":"X","version":"1.0.0"}`)
		r.write("modules/notes/i18n/id.json", `{"a":"A"}`)
		rep := r.check(false)
		wantOne(t, rep, CodeUndeclaredI18n, SeverityWarning)
		if rep.Failed() {
			t.Error("peringatan tidak boleh menggagalkan")
		}
	})
	t.Run("modul tanpa i18n dan tanpa direktori diabaikan", func(t *testing.T) {
		r := newRepo(t)
		r.write("modules/plain/module.json", `{"schema_version":"1","key":"acme.plain","slug":"plain","name":"X","version":"1.0.0"}`)
		if rep := r.check(false); len(rep.Findings) != 0 {
			t.Fatalf("temuan = %+v", rep.Findings)
		}
	})
}

func TestCheck_RegistryErrors(t *testing.T) {
	cases := map[string]string{
		"fallback tidak terdaftar": `{"default":"id","locales":[{"code":"id","name":"I"},{"code":"en","name":"E","fallback":"zz"}]}`,
		"siklus fallback":          `{"default":"id","locales":[{"code":"id","name":"I"},{"code":"en","name":"E","fallback":"ms"},{"code":"ms","name":"M","fallback":"en"}]}`,
		"default punya fallback":   `{"default":"id","locales":[{"code":"id","name":"I","fallback":"en"},{"code":"en","name":"E"}]}`,
		"kode tidak sesuai pola":   `{"default":"id","locales":[{"code":"id","name":"I"},{"code":"EN_us","name":"E"}]}`,
		"json rusak":               `{"default":`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRepo(t)
			r.write(RegistryPath, body)
			f := wantOne(t, r.check(false), CodeInvalidRegistry, SeverityError)
			if f.File != RegistryPath {
				t.Errorf("file = %q", f.File)
			}
		})
	}
}

// Kegagalan prasyarat/konfigurasi dikembalikan sebagai *ConfigError (exit 3), bukan temuan validasi.
func TestCheck_ConfigFailures(t *testing.T) {
	var ce *ConfigError
	t.Run("registri tidak ada", func(t *testing.T) {
		r := newRepo(t)
		if err := os.Remove(filepath.Join(r.root, filepath.FromSlash(RegistryPath))); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(r.root, CheckOptions{}); !errors.As(err, &ce) {
			t.Fatalf("err = %v, want *ConfigError", err)
		}
	})
	t.Run("manifest tidak terbaca", func(t *testing.T) {
		r := newRepo(t)
		r.write("modules/notes/module.json", `{bukan json`)
		if _, err := Check(r.root, CheckOptions{}); !errors.As(err, &ce) {
			t.Fatalf("err = %v, want *ConfigError", err)
		}
	})
	t.Run("root bukan direktori", func(t *testing.T) {
		if _, err := Check(filepath.Join(t.TempDir(), "tidak-ada"), CheckOptions{}); !errors.As(err, &ce) {
			t.Fatalf("err = %v, want *ConfigError", err)
		}
	})
}

// Ketidaksesuaian antar bahasa hanya peringatan (Requirement 8.1, 8.3).
func TestCheck_ParityWarningsDoNotFail(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/id.json", `{"app":{"name":"ERPora"},"greet":"Halo {name}","only":"saja id","count":"{n} item"}`)
	r.write(PlatformCatalogDir+"/en.json", `{"app":{"name":"ERPora"},"greet":"Hello {who}","extra":"only en","count":"{n} items"}`)
	rep := r.check(false)
	if rep.Failed() {
		t.Fatalf("peringatan tidak boleh menggagalkan: %+v", rep.Findings)
	}
	miss := wantOne(t, rep, CodeMissingKey, SeverityWarning)
	if miss.Key != "only" || miss.Locale != "en" || miss.File != PlatformCatalogDir+"/en.json" || miss.Namespace != "platform" {
		t.Errorf("missing = %+v", miss)
	}
	if extra := wantOne(t, rep, CodeExtraKey, SeverityWarning); extra.Key != "extra" || extra.Locale != "en" {
		t.Errorf("extra = %+v", extra)
	}
	if pm := wantOne(t, rep, CodeParamMismatch, SeverityWarning); pm.Key != "greet" || pm.Locale != "en" {
		t.Errorf("param = %+v", pm)
	}
	if rep.Errors != 0 || rep.Warnings != 3 {
		t.Errorf("errors=%d warnings=%d", rep.Errors, rep.Warnings)
	}
}

func TestCheck_ArgumentAndPluralAreCompatible(t *testing.T) {
	r := newRepo(t)
	r.write(UICatalogDir+"/id.json", `{"n":"{n} item"}`)
	r.write(UICatalogDir+"/en.json", `{"n":"{n, plural, one {# item} other {# items}}"}`)
	if rep := r.check(false); len(rep.Findings) != 0 {
		t.Fatalf("argumen sederhana dan plural harus kompatibel: %+v", rep.Findings)
	}
}

func TestCheck_StrictTurnsWarningsIntoErrors(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/en.json", `{"app":{"name":"ERPora"}}`) // greet hilang
	if rep := r.check(false); rep.Failed() || rep.Warnings != 1 {
		t.Fatalf("mode biasa: %+v", rep)
	}
	rep := r.check(true)
	if !rep.Failed() || rep.Errors != 1 || rep.Warnings != 0 {
		t.Fatalf("strict: errors=%d warnings=%d", rep.Errors, rep.Warnings)
	}
	if rep.Findings[0].Severity != SeverityError || rep.Findings[0].Code != CodeMissingKey {
		t.Errorf("temuan = %+v", rep.Findings[0])
	}
}

// Bahasa partial (Requirement 8.4): kunci hilang tidak dilaporkan (juga di --strict); kunci berlebih dan
// argumen tak cocok tetap dilaporkan.
func TestCheck_PartialLocale(t *testing.T) {
	r := newRepo(t)
	r.write(RegistryPath, `{"default":"id","locales":[{"code":"id","name":"I"},{"code":"ms","name":"M","fallback":"id","partial":true}]}`)
	r.write(PlatformCatalogDir+"/ms.json", `{"app":{"name":"ERPora"},"extra":"x"}`) // greet hilang, extra berlebih
	r.write(UICatalogDir+"/ms.json", `{"actions":{"save":"Simpan"}}`)
	for _, rel := range []string{PlatformCatalogDir + "/en.json", UICatalogDir + "/en.json"} { // en tidak terdaftar lagi
		if err := os.Remove(filepath.Join(r.root, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}
	rep := r.check(false)
	if rep.Failed() {
		t.Fatalf("tidak boleh gagal: %+v", rep.Findings)
	}
	for _, f := range rep.Findings {
		if f.Code == CodeMissingKey {
			t.Errorf("bahasa partial tidak boleh melaporkan kunci hilang: %+v", f)
		}
	}
	if f := wantOne(t, rep, CodeExtraKey, SeverityWarning); f.Locale != "ms" || f.Key != "extra" {
		t.Errorf("extra = %+v", f)
	}
	// param tak cocok pada partial tetap dilaporkan
	r.write(PlatformCatalogDir+"/ms.json", `{"app":{"name":"ERPora"},"greet":"Halo {siapa}"}`)
	wantOne(t, r.check(false), CodeParamMismatch, SeverityWarning)
	// strict tidak menggagalkan karena kunci hilang pada partial
	r.write(PlatformCatalogDir+"/ms.json", `{"app":{"name":"ERPora"}}`)
	if rep := r.check(true); rep.Failed() {
		t.Fatalf("strict + partial hanya kunci hilang: %+v", rep.Findings)
	}
}

func TestCheck_UndeclaredCatalogFileIsWarning(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/fr.json", `{"app":{"name":"ERPora"}}`) // fr tidak terdaftar
	rep := r.check(false)
	f := wantOne(t, rep, CodeUndeclaredFile, SeverityWarning)
	if f.File != PlatformCatalogDir+"/fr.json" || rep.Failed() {
		t.Errorf("temuan = %+v failed=%v", f, rep.Failed())
	}
}

func TestCheck_DefaultLocaleFileMissingForPlatformIsError(t *testing.T) {
	r := newRepo(t)
	if err := os.Remove(filepath.Join(r.root, filepath.FromSlash(UICatalogDir+"/id.json"))); err != nil {
		t.Fatal(err)
	}
	f := wantOne(t, r.check(false), CodeMissingFile, SeverityError)
	if f.Namespace != "ui" || f.Locale != "id" {
		t.Errorf("temuan = %+v", f)
	}
}

func TestReport_JSONAndText(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/en.json", `{"app":{"name":"ERPora"}}`)
	rep := r.check(false)

	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Findings []map[string]any `json:"findings"`
		Errors   int              `json:"errors"`
		Warnings int              `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON tidak valid: %v\n%s", err, buf.String())
	}
	if got.Warnings != 1 || len(got.Findings) != 1 {
		t.Fatalf("json = %+v", got)
	}
	f := got.Findings[0]
	for k, want := range map[string]string{
		"severity": "warning", "code": "missing_key", "namespace": "platform", "locale": "en",
		"file": PlatformCatalogDir + "/en.json", "key": "greet",
	} {
		if f[k] != want {
			t.Errorf("json %s = %v, want %s", k, f[k], want)
		}
	}

	buf.Reset()
	rep.WriteText(&buf)
	txt := buf.String()
	for _, want := range []string{PlatformCatalogDir + "/en.json", "en", "greet", "missing_key", "0 galat, 1 peringatan"} {
		if !strings.Contains(txt, want) {
			t.Errorf("teks tidak memuat %q:\n%s", want, txt)
		}
	}
}

func TestStatus_Percentages(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/id.json", `{"a":"A","b":"B","c":"C","d":"D"}`)
	r.write(PlatformCatalogDir+"/en.json", `{"a":"A","b":"B","c":"TODO: C"}`) // d hilang, c masih TODO
	rows, err := Status(r.root)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]StatusRow{}
	for _, row := range rows {
		byKey[row.Namespace+"/"+row.Locale] = row
	}
	if got := byKey["platform/id"]; got.Total != 4 || got.Translated != 4 || got.Percent != 100 || !got.Default {
		t.Errorf("platform/id = %+v", got)
	}
	if got := byKey["platform/en"]; got.Total != 4 || got.Translated != 2 || got.Percent != 50 || got.Default {
		t.Errorf("platform/en = %+v", got)
	}
	if got := byKey["ui/en"]; got.Percent != 100 {
		t.Errorf("ui/en = %+v", got)
	}
}

func TestStatus_IncludesModulesAndPartial(t *testing.T) {
	r := newRepo(t)
	r.write(RegistryPath, `{"default":"id","locales":[{"code":"id","name":"I"},{"code":"ms","name":"M","fallback":"id","partial":true}]}`)
	r.write(PlatformCatalogDir+"/ms.json", `{"app":{"name":"ERPora"}}`)
	r.write(UICatalogDir+"/ms.json", `{"actions":{"save":"Simpan"}}`)
	r.module("notes", "notes", []string{"id", "ms"}, "id", map[string]string{"id": `{"x":"X","y":"Y"}`, "ms": `{"x":"X"}`})
	rows, err := Status(r.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Namespace == "notes" && row.Locale == "ms" {
			if row.Total != 2 || row.Translated != 1 || row.Percent != 50 || !row.Partial {
				t.Errorf("notes/ms = %+v", row)
			}
			return
		}
	}
	t.Fatalf("baris notes/ms tidak ada: %+v", rows)
}

func TestStatus_InvalidCatalogIsValidationError(t *testing.T) {
	r := newRepo(t)
	r.write(PlatformCatalogDir+"/en.json", `{rusak`)
	var ve *ValidationError
	if _, err := Status(r.root); !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
}
