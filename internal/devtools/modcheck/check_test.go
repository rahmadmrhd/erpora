package modcheck

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/platform/modules/modtest"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
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
	alphaResources    string // array JSON resources alpha; kosong = diturunkan dari capabilities
}

// fixtureResources membangun array JSON resources dari isi array capabilities ("a.b.read", ...): satu resource
// per <slug>.<resource>, sehingga fixture lolos aturan FR-110 (permission turunan resource x aksi).
func fixtureResources(caps string) string {
	var perms []string
	for _, c := range strings.Split(caps, ",") {
		if c = strings.Trim(strings.TrimSpace(c), `"`); c != "" {
			perms = append(perms, c)
		}
	}
	b, err := json.Marshal(modtest.Resources(perms...))
	if err != nil {
		panic(err)
	}
	return string(b)
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
	alphaRes := f.alphaResources
	if alphaRes == "" {
		alphaRes = fixtureResources(f.alphaCapabilities)
	}
	write(t, root, "go.work", "go 1.27\n\ntoolchain go1.27.3\n\nuse (\n\t./modules/alpha\n\t./modules/beta\n)\n")
	manifest := func(key, slug, version, caps, deps, res string) string {
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
  "resources": ` + res + `,
  "migrations": { "strategy": "none" },
  "offline": { "mode": "online-only" }
}
`
	}
	write(t, root, "modules/alpha/go.mod", "module erpora/modules/alpha\n\ngo "+f.alphaGo+"\n\ntoolchain go1.27.3\n")
	write(t, root, "modules/alpha/a.go", "package alpha\n")
	write(t, root, "modules/alpha/module.json", manifest("acme.alpha", "alpha", f.alphaVersion, f.alphaCapabilities, "", alphaRes))
	deps := ""
	if f.betaDeclaresDep {
		deps = `,
    { "module": "acme.alpha", "version": "^1.0", "optional": false }`
	}
	write(t, root, "modules/beta/go.mod", "module erpora/modules/beta\n\ngo 1.27\n\ntoolchain go1.27.3\n\nrequire erpora/modules/alpha v0.0.0\n")
	write(t, root, "modules/beta/b.go", "package beta\n")
	write(t, root, "modules/beta/module.json", manifest("acme.beta", "beta", "1.0.0", `"beta.items.read"`, deps, fixtureResources(`"beta.items.read"`)))
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
		write(t, root, "modules/alpha/i18n/"+f, `{"resource":{"items":"Item"},"fieldGroup":{"general":"Umum"}}`)
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

// deskFixture membangun fixture bersih lalu menambahkan, pada alpha, direktori web/ (bila withWeb), bagian
// "desk" manifest (bila deskJSON != "") beserta katalog id.json (bagian i18n ikut dideklarasikan bila catalog != "").
func deskFixture(t *testing.T, withWeb bool, deskJSON, catalog string) string {
	t.Helper()
	root := fixture{betaDeclaresDep: true}.build(t)
	write(t, root, "internal/platform/i18n/locales.json", testLocalesJSON)
	extra := ""
	if catalog != "" {
		extra += `,
  "i18n": {"locales":["id"],"defaultLocale":"id"}`
		write(t, root, "modules/alpha/i18n/id.json", catalog)
	}
	if deskJSON != "" {
		extra += `,
  "desk": ` + deskJSON
	}
	if extra != "" {
		p := filepath.Join(root, "modules", "alpha", "module.json")
		b, err := os.ReadFile(p) //nolint:gosec // path uji
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, "modules/alpha/module.json", strings.Replace(string(b), `"offline": { "mode": "online-only" }`, `"offline": { "mode": "online-only" }`+extra, 1))
	}
	if withWeb {
		write(t, root, "modules/alpha/web/package.json", `{"name":"@erpora/module-alpha"}`)
	}
	return root
}

const (
	deskCatalog   = `{"desk":{"title":"Alpha"},"nav":{"items":"Item"},"resource":{"items":"Item"},"fieldGroup":{"general":"Umum"}}`
	deskValidJSON = `{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.items","to":"/items","permission":"alpha.items.read"}]}`
)

func TestCheck_Desk(t *testing.T) {
	t.Setenv("GOWORK", "")
	tests := []struct {
		name    string
		withWeb bool
		desk    string
		catalog string
		want    []string // potongan Detail yang harus ada pada temuan DESK; kosong = bersih
	}{
		{"modul tanpa web dan tanpa desk lolos", false, "", "", nil},
		{"modul tanpa web boleh punya desk", false, deskValidJSON, deskCatalog, nil},
		{"modul web dengan desk valid lolos", true, deskValidJSON, deskCatalog, nil},
		{"modul web tanpa desk ditolak", true, "", "", []string{"module.json", `"desk"`, "modules/alpha/web"}},
		{"item access authenticated tanpa capability lolos (Q43)", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.items","to":"/items","access":"authenticated"}]}`,
			deskCatalog, nil},
		{"permission dan access bersamaan ditolak (Q43)", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.items","to":"/items","permission":"alpha.items.read","access":"authenticated"}]}`,
			deskCatalog, []string{"desk"}},
		{"permission di luar capabilities", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.items","to":"/items","permission":"alpha.items.write"}]}`,
			deskCatalog, []string{"/desk/menus/0/permission", "desk_unknown_permission"}},
		{"label tidak ada di katalog", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.tidakAda","to":"/items","permission":"alpha.items.read"}]}`,
			deskCatalog, []string{"/desk/menus/0/label", "desk_label_unknown"}},
		{"to tidak valid", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.items","to":"/a/../b","permission":"alpha.items.read"}]}`,
			deskCatalog, []string{"/desk/menus/0/to", "desk_invalid_to"}},
		{"dashboard dideklarasikan sebagai menu", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"dashboard","label":"nav.items","to":"/items","permission":"alpha.items.read"}]}`,
			deskCatalog, []string{"/desk/menus/0/id", "desk_dashboard_locked"}},
		{"id ganda", true,
			`{"label":"desk.title","icon":"file-text","menus":[{"id":"items","label":"nav.items","to":"/a","permission":"alpha.items.read"},{"id":"items","label":"nav.items","to":"/b","permission":"alpha.items.read"}]}`,
			deskCatalog, []string{"/desk/menus/1/id", "desk_duplicate_id"}},
		{"desk tanpa katalog", true, deskValidJSON, "", []string{"desk_label_unknown"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(deskFixture(t, tc.withWeb, tc.desk, tc.catalog), "alpha")
			if err != nil {
				t.Fatal(err)
			}
			var desk []Finding
			for _, f := range got {
				if f.Rule == RuleDesk {
					desk = append(desk, f)
				}
			}
			if len(tc.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("harus bersih: %+v", got)
				}
				return
			}
			for _, w := range tc.want {
				found := false
				for _, f := range desk {
					if strings.Contains(f.Detail, w) {
						found = true
					}
				}
				if !found {
					t.Errorf("temuan DESK %q tidak ada: %+v", w, desk)
				}
			}
		})
	}
}

// metadataFixture menambah resources/field_groups pada manifest alpha dan metadata resource alpha.item.
func metadataFixture(t *testing.T, pricingFields string, nameGroup string) string {
	t.Helper()
	root := fixture{betaDeclaresDep: true}.build(t)
	p := filepath.Join(root, "modules", "alpha", "module.json")
	b, err := os.ReadFile(p) //nolint:gosec // path uji
	if err != nil {
		t.Fatal(err)
	}
	base := fixtureResources(`"alpha.items.read"`)
	res := `[` + strings.TrimSuffix(strings.TrimPrefix(base, "["), "]") + `,{"key":"alpha.item","label":"resource.item","actions":["read"],"field_groups":[
    {"key":"general","label":"fieldGroup.general","default":true},
    {"key":"pricing","label":"fieldGroup.pricing","classification":"confidential"` + pricingFields + `}]}]`
	if !strings.Contains(string(b), base) {
		t.Fatalf("resources dasar tidak ditemukan di manifest alpha")
	}
	write(t, root, "modules/alpha/module.json", strings.Replace(string(b), base, res, 1))
	write(t, root, "modules/alpha/metadata/item.resource.yaml", `schemaVersion: "1"
kind: resource
key: alpha.item
fields:
  name: {type: text`+nameGroup+`}
  unitPrice: {type: decimal, precision: 10, scale: 2, fieldGroup: pricing}
`)
	return root
}

func TestCheck_MetadataFieldGroups(t *testing.T) {
	t.Setenv("GOWORK", "")
	// kapabilitas alpha.items.read tetap ada; resource baru tidak dipersoalkan oleh module:check.
	root := metadataFixture(t, `,"fields":["unit_price"]`, "")
	if got, err := Check(root, ""); err != nil || len(got) != 0 {
		t.Fatalf("fields sama dengan metadata harus bersih: %+v err=%v", got, err)
	}
	root = metadataFixture(t, `,"fields":["unit_price","name"]`, "")
	got, err := Check(root, "alpha")
	if err != nil || len(got) != 1 || got[0].Rule != RuleMetadata || !strings.Contains(got[0].Detail, "manifest_fields_mismatch") {
		t.Fatalf("got = %+v err=%v", got, err)
	}
	root = metadataFixture(t, ``, ", fieldGroup: ghost")
	got, err = Check(root, "")
	if err != nil || len(got) != 1 || got[0].Rule != RuleMetadata || !strings.Contains(got[0].Detail, "manifest_group_unknown") ||
		!strings.Contains(got[0].Detail, "modules/alpha/metadata/item.resource.yaml") {
		t.Fatalf("got = %+v err=%v", got, err)
	}
	// modul tanpa metadata/ tidak terpengaruh.
	if got, err := Check(fixture{betaDeclaresDep: true}.build(t), ""); err != nil || len(got) != 0 {
		t.Fatalf("got = %+v err=%v", got, err)
	}
}

// fakeModule membungkus manifest hasil baca module.json agar dapat dimasukkan ke registry.Build.
type fakeModule struct{ m module.Manifest }

func (f fakeModule) Manifest() module.Manifest       { return f.m }
func (fakeModule) Register(r module.Registrar) error { return nil }

// Paritas FR-110: module:check dan registry.Build memberi temuan otorisasi yang sama (kode dan JSON pointer) untuk
// module.json yang sama, karena keduanya memakai manifest.ValidateAuthorization.
func TestCheck_AuthorizationParityWithRegistry(t *testing.T) {
	t.Setenv("GOWORK", "")
	grp := `"field_groups":[{"key":"general","label":"fieldGroup.general","default":true}]`
	tests := map[string]fixture{
		"aksi kustom bentrok": {alphaCapabilities: `"alpha.items.read"`, betaDeclaresDep: true, alphaResources: `[{"key":"alpha.items","label":"resource.items","actions":["read"],
			"custom_actions":[{"key":"approve","label":"action.approve"}],` + grp + `}]`},
		"dimensi tak terdaftar": {betaDeclaresDep: true, alphaResources: `[{"key":"alpha.items","label":"resource.items","actions":["read"],
			"scope_fields":{"alpha.zone":"zone_id"},` + grp + `}]`},
		"tanpa grup default": {betaDeclaresDep: true, alphaResources: `[{"key":"alpha.items","label":"resource.items","actions":["read"],
			"field_groups":[{"key":"general","label":"fieldGroup.general"}]}]`},
		"capability tanpa resource": {alphaCapabilities: `"alpha.items.read"`, betaDeclaresDep: true, alphaResources: `[]`},
	}
	for name, fx := range tests {
		t.Run(name, func(t *testing.T) {
			root := fx.build(t)
			got, err := Check(root, "alpha")
			if err != nil {
				t.Fatal(err)
			}
			authz := map[string]bool{}
			for _, f := range got {
				if f.Rule != RuleSchema {
					continue
				}
				for _, code := range authzCodes {
					if strings.HasSuffix(f.Detail, "("+code+")") {
						field, _, _ := strings.Cut(f.Detail, ": ")
						authz[field+"|"+code] = true
					}
				}
			}
			if len(authz) == 0 {
				t.Fatalf("module:check harus melaporkan pelanggaran otorisasi: %+v", got)
			}

			raw, err := os.ReadFile(filepath.Join(root, "modules", "alpha", "module.json")) //nolint:gosec // path uji
			if err != nil {
				t.Fatal(err)
			}
			mf, err := module.ParseManifest(raw)
			if err != nil {
				t.Fatal(err)
			}
			_, berr := registry.Build(registry.Options{PlatformVersion: "0.1.0", SDKVersion: "0.1.0"}, fakeModule{m: mf})
			var re *registry.Error
			if !errors.As(berr, &re) {
				t.Fatalf("registry.Build harus menolak: %v", berr)
			}
			regSet := map[string]bool{}
			for _, p := range re.Problems {
				for _, code := range authzCodes {
					if p.Code == code {
						regSet[p.Field+"|"+code] = true
					}
				}
			}
			if len(regSet) != len(authz) {
				t.Fatalf("temuan berbeda: module:check=%v registry=%v", authz, regSet)
			}
			for k := range authz {
				if !regSet[k] {
					t.Errorf("temuan %s hanya ada di module:check (registry=%v)", k, regSet)
				}
			}
		})
	}
}

var authzCodes = []string{"invalid_resource", "invalid_scope_dimension", "invalid_default_role", "capability_not_derived"}
