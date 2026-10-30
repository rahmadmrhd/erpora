package modcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheck_RepositoryClean(t *testing.T) {
	got, err := Check("../../..", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("repo harus bersih, temuan: %+v", got)
	}
}

func TestCheck_UnknownModule(t *testing.T) {
	if _, err := Check("../../..", "tidak-ada"); err == nil || !strings.Contains(err.Error(), "tidak dikenal") {
		t.Fatalf("err = %v", err)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fixture membuat workspace mini: modul alpha dan beta (beta bergantung pada alpha), keduanya valid.
// Parameter menyesuaikan satu pelanggaran per kasus.
type fixture struct {
	alphaVersion      string // default 1.0.0
	alphaCapabilities string // isi array JSON, default "alpha.items.read"
	betaDeclaresDep   bool
	alphaGo           string // direktif go, default 1.27
}

func (f fixture) build(t *testing.T) string {
	t.Helper()
	if f.alphaVersion == "" {
		f.alphaVersion = "1.0.0"
	}
	if f.alphaCapabilities == "" {
		f.alphaCapabilities = `"alpha.items.read"`
	}
	if f.alphaGo == "" {
		f.alphaGo = "1.27"
	}
	root := t.TempDir()
	write(t, root, "go.work", "go 1.27\n\ntoolchain go1.27.3\n\nuse (\n\t./modules/alpha\n\t./modules/beta\n)\n")
	manifest := func(key, slug, version, caps, deps string) string {
		return `{
  "schema_version": "1",
  "key": "` + key + `",
  "slug": "` + slug + `",
  "name": "` + slug + `",
  "version": "` + version + `",
  "publisher": { "id": "acme", "name": "ACME" },
  "trust": "builtin",
  "compatibility": { "platform": "^0.1", "sdk": "^0.1" },
  "dependencies": [
    { "module": "platform.core", "version": "^0.1", "optional": false }` + deps + `
  ],
  "capabilities": [` + caps + `],
  "migrations": { "strategy": "none" },
  "offline": { "mode": "online-only" }
}
`
	}
	write(t, root, "modules/alpha/go.mod", "module erpora/modules/alpha\n\ngo "+f.alphaGo+"\n\ntoolchain go1.27.3\n")
	write(t, root, "modules/alpha/a.go", "package alpha\n")
	write(t, root, "modules/alpha/module.json", manifest("acme.alpha", "alpha", f.alphaVersion, f.alphaCapabilities, ""))
	deps := ""
	if f.betaDeclaresDep {
		deps = `,
    { "module": "acme.alpha", "version": "^1.0", "optional": false }`
	}
	write(t, root, "modules/beta/go.mod", "module erpora/modules/beta\n\ngo 1.27\n\ntoolchain go1.27.3\n\nrequire erpora/modules/alpha v0.0.0\n")
	write(t, root, "modules/beta/b.go", "package beta\n")
	write(t, root, "modules/beta/module.json", manifest("acme.beta", "beta", "1.0.0", `"beta.items.read"`, deps))
	return root
}

func TestCheck_Fixtures(t *testing.T) {
	t.Setenv("GOWORK", "") // fixture membawa go.work sendiri
	tests := []struct {
		name     string
		fx       fixture
		wantMod  string
		wantRule string
	}{
		{"versi bukan semver", fixture{alphaVersion: "1.0", betaDeclaresDep: true}, "acme.alpha", RuleSchema},
		{"capability berawalan lain", fixture{alphaCapabilities: `"other.x.read"`, betaDeclaresDep: true}, "acme.alpha", RuleCapabilityPrefix},
		{"require tanpa dependensi manifest", fixture{betaDeclaresDep: false}, "acme.beta", RuleD8},
		{"go tidak sama", fixture{alphaGo: "1.26", betaDeclaresDep: true}, "acme.alpha", RuleGoVersion},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(tc.fx.build(t), "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range got {
				if f.Module == tc.wantMod && f.Rule == tc.wantRule {
					found = true
				}
			}
			if !found {
				t.Fatalf("temuan %s/%s tidak ada: %+v", tc.wantMod, tc.wantRule, got)
			}
		})
	}
}

func TestCheck_FixtureCleanAndOnly(t *testing.T) {
	t.Setenv("GOWORK", "")
	root := fixture{betaDeclaresDep: true}.build(t)
	got, err := Check(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("fixture dasar harus bersih: %+v", got)
	}
	// only dicocokkan terhadap direktori, slug, atau key; hanya modul itu yang dilaporkan.
	root = fixture{alphaVersion: "1.0", betaDeclaresDep: true}.build(t)
	for _, only := range []string{"alpha", "acme.alpha"} {
		got, err = Check(root, only)
		if err != nil || len(got) == 0 {
			t.Fatalf("only=%s: got=%+v err=%v", only, got, err)
		}
	}
	got, err = Check(root, "beta")
	if err != nil || len(got) != 0 {
		t.Fatalf("only=beta harus bersih: got=%+v err=%v", got, err)
	}
}

const testLocalesJSON = `{"default":"id","locales":[{"code":"id","name":"ID"},{"code":"en","name":"EN","fallback":"id"}]}`

// i18nFixture membangun fixture bersih lalu menambahkan registri bahasa dan perubahan per kasus pada alpha.
func i18nFixture(t *testing.T, i18nManifest string, files ...string) string {
	t.Helper()
	root := fixture{betaDeclaresDep: true}.build(t)
	write(t, root, "internal/platform/i18n/locales.json", testLocalesJSON)
	if i18nManifest != "" {
		p := filepath.Join(root, "modules", "alpha", "module.json")
		b, err := os.ReadFile(p) //nolint:gosec // path uji
		if err != nil {
			t.Fatal(err)
		}
		s := strings.Replace(string(b), `"offline": { "mode": "online-only" }`, `"offline": { "mode": "online-only" },
  "i18n": `+i18nManifest, 1)
		write(t, root, "modules/alpha/module.json", s)
	}
	for _, f := range files {
		write(t, root, "modules/alpha/i18n/"+f, `{}`)
	}
	return root
}

func TestCheck_I18n(t *testing.T) {
	t.Setenv("GOWORK", "")
	tests := []struct {
		name     string
		manifest string
		files    []string
		want     string // potongan Detail yang diharapkan; kosong = bersih
	}{
		{"modul tanpa i18n bersih", "", nil, ""},
		{"i18n lengkap bersih", `{"locales":["id","en"],"defaultLocale":"id"}`, []string{"id.json", "en.json"}, ""},
		{"bahasa tak terdaftar", `{"locales":["id","fr"],"defaultLocale":"id"}`, []string{"id.json", "fr.json"}, `"fr" tidak terdaftar`},
		{"defaultLocale di luar locales", `{"locales":["id"],"defaultLocale":"en"}`, []string{"id.json"}, "defaultLocale"},
		{"berkas bahasa hilang", `{"locales":["id","en"],"defaultLocale":"id"}`, []string{"id.json"}, "en.json"},
		{"direktori i18n tanpa bagian manifest", "", []string{"id.json"}, `bagian "i18n"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(i18nFixture(t, tc.manifest, tc.files...), "alpha")
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("harus bersih: %+v", got)
				}
				return
			}
			for _, f := range got {
				if f.Rule == RuleI18n && strings.Contains(f.Detail, tc.want) {
					return
				}
			}
			t.Fatalf("temuan I18N %q tidak ada: %+v", tc.want, got)
		})
	}
}

func TestCheck_I18nSkipsRegistryCheckWithoutRegistryFile(t *testing.T) {
	t.Setenv("GOWORK", "")
	root := i18nFixture(t, `{"locales":["fr"],"defaultLocale":"fr"}`, "fr.json")
	if err := os.Remove(filepath.Join(root, "internal", "platform", "i18n", "locales.json")); err != nil {
		t.Fatal(err)
	}
	got, err := Check(root, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("tanpa registri, pemeriksaan registri dilewati: %+v", got)
	}
}
