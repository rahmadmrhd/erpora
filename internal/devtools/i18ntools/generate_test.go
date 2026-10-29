package i18ntools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const notesDefault = `{"note":{
  "title":"Judul",
  "count":"{count, plural, =0 {Kosong} other {# catatan}}",
  "status":"{state, select, draft {Draf} approved {Disetujui} other {Lainnya}}",
  "greet":"Halo {name}, <b>selamat</b> datang",
  "n":"{n} item"}}`

func genRepo(t *testing.T) *repo {
	t.Helper()
	r := newRepo(t)
	r.module("notes", "notes", []string{"id", "en"}, "id", map[string]string{
		"id": notesDefault,
		"en": `{"note":{"title":"Title"}}`, // bahasa non-default tidak memengaruhi tipe
	})
	r.write("modules/notes/web/package.json", `{"name":"@erpora/module-notes"}`)
	return r
}

func genMap(t *testing.T, r *repo) map[string]string {
	t.Helper()
	files, err := Generate(r.root)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, f := range files {
		m[f.Path] = string(f.Content)
	}
	return m
}

func TestGenerate_ModuleKeysFile(t *testing.T) {
	r := genRepo(t)
	got := genMap(t, r)["modules/notes/web/src/i18n/keys.gen.ts"]
	want := "// Dibangkitkan oleh `erpora i18n:generate` dari modules/notes/i18n/id.json. JANGAN DIEDIT.\n" +
		"import type { RichTag } from \"@erpora/platform-web\";\n" +
		"\n" +
		"declare module \"@erpora/platform-web\" {\n" +
		"  interface I18nCatalogs {\n" +
		"    notes: {\n" +
		"      \"note.count\": { count: number };\n" +
		"      \"note.greet\": { name: string | number; b: RichTag };\n" +
		"      \"note.n\": { n: string | number };\n" +
		"      \"note.status\": { state: \"draft\" | \"approved\" | \"other\" };\n" +
		"      \"note.title\": {};\n" +
		"    };\n" +
		"  }\n" +
		"}\n"
	if got != want {
		t.Fatalf("keys.gen.ts =\n%s\nwant\n%s", got, want)
	}
}

func TestGenerate_Targets(t *testing.T) {
	r := genRepo(t)
	r.write("modules/noweb/module.json", `{"schema_version":"1","key":"acme.noweb","slug":"noweb","name":"X","version":"1.0.0","i18n":{"locales":["id"],"defaultLocale":"id"}}`)
	r.write("modules/noweb/i18n/id.json", `{"a":"A"}`)
	got := genMap(t, r)
	for _, p := range []string{
		"packages/platform-web/src/i18n/keys.platform.gen.ts",
		"packages/ui/src/i18n/keys.gen.ts",
		"modules/notes/web/src/i18n/keys.gen.ts",
	} {
		if _, ok := got[p]; !ok {
			t.Errorf("target %s tidak dibangkitkan; ada: %v", p, keysOf(got))
		}
	}
	if _, ok := got["modules/noweb/web/src/i18n/keys.gen.ts"]; ok {
		t.Error("modul tanpa web/ tidak boleh dibangkitkan")
	}
	if len(got) != 3 {
		t.Errorf("jumlah berkas = %d, want 3: %v", len(got), keysOf(got))
	}
	// platform-web: augmentasi namespace platform dan ui lewat modul relatif ./types (paket tidak me-resolve
	// namanya sendiri); platform lebih dulu.
	g := got["packages/platform-web/src/i18n/keys.platform.gen.ts"]
	for _, frag := range []string{
		"declare module \"./types\" {\n",
		"    platform: {\n      \"app.name\": {};\n      \"greet\": { name: string | number };\n    };\n",
		"    ui: {\n      \"actions.save\": {};\n    };\n",
	} {
		if !strings.Contains(g, frag) {
			t.Errorf("keys.platform.gen.ts tidak memuat %q:\n%s", frag, g)
		}
	}
	if strings.Index(g, "    platform: {") > strings.Index(g, "    ui: {") {
		t.Error("namespace platform harus lebih dulu dari ui")
	}
	// ui: tipe kunci mandiri, tanpa augmentasi dan tanpa impor paket repo (F4)
	u := got["packages/ui/src/i18n/keys.gen.ts"]
	for _, frag := range []string{"export interface UiCatalog {\n  \"actions.save\": {};\n}\n", "export type UiKey = keyof UiCatalog;"} {
		if !strings.Contains(u, frag) {
			t.Errorf("ui keys.gen.ts tidak memuat %q:\n%s", frag, u)
		}
	}
	if strings.Contains(u, "declare module") || strings.Contains(u, "@erpora/") {
		t.Errorf("ui keys.gen.ts tidak boleh mengaugmentasi atau mengimpor paket repo:\n%s", u)
	}
}

func keysOf(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestGenerate_NamespaceWithHyphenIsQuoted(t *testing.T) {
	r := genRepo(t)
	r.module("addon", "example-addon", []string{"id"}, "id", map[string]string{"id": `{"a":"A"}`})
	r.write("modules/addon/web/package.json", `{}`)
	g := genMap(t, r)["modules/addon/web/src/i18n/keys.gen.ts"]
	if !strings.Contains(g, "    \"example-addon\": {\n") {
		t.Errorf("namespace bertanda hubung harus dikutip:\n%s", g)
	}
}

// ui adalah daun (F4): tag rich memakai tipe fungsi inline, bukan impor @erpora/platform-web.
func TestGenerate_UIRichTagDoesNotImportPlatformWeb(t *testing.T) {
	r := genRepo(t)
	r.write(UICatalogDir+"/id.json", `{"hint":"Tekan <kbd>Esc</kbd> untuk menutup"}`)
	r.write(UICatalogDir+"/en.json", `{"hint":"Press <kbd>Esc</kbd> to close"}`)
	all := genMap(t, r)
	g := all["packages/ui/src/i18n/keys.gen.ts"]
	if strings.Contains(g, "import type { RichTag }") || !strings.Contains(g, `import type { ReactNode } from "react";`) ||
		!strings.Contains(g, "  hint: { kbd: (chunks: ReactNode) => ReactNode };\n") { // semua kunci pengenal sah: tanpa kutip (Biome asNeeded)
		t.Errorf("ui keys.gen.ts:\n%s", g)
	}
	// augmentasi di platform-web memakai RichTag dari ./types
	pw := all["packages/platform-web/src/i18n/keys.platform.gen.ts"]
	if !strings.Contains(pw, `import type { RichTag } from "./types";`) || !strings.Contains(pw, "      hint: { kbd: RichTag };\n") {
		t.Errorf("keys.platform.gen.ts:\n%s", pw)
	}
}

// Baris lebih dari 100 karakter diformat seperti Biome (objek dipecah, gabungan literal dipecah) agar
// biome check pada hasil generate lolos.
func TestGenerate_WrapsLongEntriesLikeBiome(t *testing.T) {
	r := genRepo(t)
	long := `{"x":{"y":"{alpha_long_name} {beta_long_name} {gamma_long_name} {delta_long_name}",` +
		`"z":"{kind, select, satu_panjang {A} dua_panjang {B} tiga_panjang {C} empat_panjang {D} lima_panjang {E} other {F}}"}}`
	r.write("modules/notes/i18n/id.json", long)
	r.write("modules/notes/i18n/en.json", long)
	g := genMap(t, r)["modules/notes/web/src/i18n/keys.gen.ts"]
	for _, line := range strings.Split(g, "\n") {
		if strings.HasPrefix(line, "//") {
			continue
		}
		if len([]rune(line)) > 100 {
			t.Errorf("baris > 100 karakter: %q", line)
		}
	}
	for _, want := range []string{
		"      \"x.y\": {\n        alpha_long_name: string | number;\n",
		"        kind:\n          | \"satu_panjang\"\n          | \"dua_panjang\"\n",
		"          | \"other\";\n      };\n",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("tidak memuat %q:\n%s", want, g)
		}
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	r := genRepo(t)
	a, b := genMap(t, r), genMap(t, r)
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s tidak deterministik", k)
		}
	}
}

func TestGenerate_InvalidCatalogIsValidationError(t *testing.T) {
	r := genRepo(t)
	r.write("modules/notes/i18n/id.json", `{"a":"{rusak"}`)
	var ve *ValidationError
	if _, err := Generate(r.root); !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	var ce *ConfigError
	r2 := genRepo(t)
	if err := os.Remove(filepath.Join(r2.root, filepath.FromSlash(RegistryPath))); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(r2.root); !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *ConfigError", err)
	}
}

func TestWriteAndCheckGenerated(t *testing.T) {
	r := genRepo(t)
	files, err := Generate(r.root)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := CheckGenerated(r.root, files)
	if err != nil || len(stale) != len(files) {
		t.Fatalf("sebelum tulis semua basi: stale=%v err=%v", stale, err)
	}
	if err := WriteGenerated(r.root, files); err != nil {
		t.Fatal(err)
	}
	if stale, err = CheckGenerated(r.root, files); err != nil || len(stale) != 0 {
		t.Fatalf("setelah tulis: stale=%v err=%v", stale, err)
	}
	// CRLF di disk (autocrlf) tetap dianggap sama
	p := filepath.Join(r.root, "packages/ui/src/i18n/keys.gen.ts")
	data, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(string(data), "\n", "\r\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	if stale, _ = CheckGenerated(r.root, files); len(stale) != 0 {
		t.Fatalf("CRLF dianggap basi: %v", stale)
	}
	// katalog ui berubah -> basi untuk berkas ui dan augmentasi platform-web
	r.write(UICatalogDir+"/id.json", `{"actions":{"save":"Simpan","load":"{n} muat"}}`)
	r.write(UICatalogDir+"/en.json", `{"actions":{"save":"Save","load":"{n} load"}}`)
	files2, _ := Generate(r.root)
	stale, _ = CheckGenerated(r.root, files2)
	if len(stale) != 2 || stale[0] != "packages/platform-web/src/i18n/keys.platform.gen.ts" || stale[1] != "packages/ui/src/i18n/keys.gen.ts" { // ui juga memengaruhi augmentasi di platform-web
		t.Fatalf("stale = %v", stale)
	}
	// berkas yatim: modul tanpa katalog lagi tetapi keys.gen.ts masih ada
	r.write("modules/ghost/web/src/i18n/keys.gen.ts", "// sisa\n")
	stale, _ = CheckGenerated(r.root, files2)
	if !strings.Contains(strings.Join(stale, ","), "modules/ghost/web/src/i18n/keys.gen.ts") {
		t.Fatalf("berkas yatim tidak dilaporkan: %v", stale)
	}
}
