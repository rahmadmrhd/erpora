package stockopname

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"

	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var (
	_ module.Module          = (*Module)(nil)
	_ module.MigrationSource = (*Module)(nil)
	_ module.ContractSource  = (*Module)(nil)
	_ module.Installer       = (*Module)(nil)
	_ module.HealthChecker   = (*Module)(nil)
	_ module.DevSeeder       = (*Module)(nil)
	_ module.CatalogSource   = (*Module)(nil)
)

func TestManifest_Parses(t *testing.T) {
	m := New().Manifest()
	if m.Key != "acme.stock-opname" || m.Slug != "stock-opname" || len(m.Raw) == 0 {
		t.Fatalf("manifest = %+v", m)
	}
}

// Katalog terjemahan modul (PRD-000 FR-25): satu berkas per bahasa yang dideklarasikan manifest, valid, dan
// sejajar antar bahasa; errors.bad_request melokalkan message galat route daftar (code tetap stabil).
func TestCatalogs_MatchManifestAndAreConsistent(t *testing.T) {
	m := New()
	decl := m.Manifest().I18n
	if decl == nil || len(decl.Locales) != 2 || decl.DefaultLocale != "id" {
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

func TestMigrations_Embedded(t *testing.T) {
	names, err := fs.Glob(New().Migrations(), "*.sql")
	if err != nil || len(names) != 1 || names[0] != "0001_create_stock_opname_items.sql" {
		t.Fatalf("names = %v err = %v", names, err)
	}
}

// Route, permission, dan command yang didaftarkan harus sesuai manifest (capabilities).
func TestRegister_RoutesAndCommand(t *testing.T) {
	r := moduletest.NewRecorder()
	if err := New().Register(r); err != nil {
		t.Fatal(err)
	}
	if r.Permission("GET", "/items") != "stock-opname.items.read" {
		t.Errorf("routes = %+v", r.Routes)
	}
	caps := map[string]bool{}
	for _, c := range New().Manifest().Capabilities {
		caps[c] = true
	}
	for _, rt := range r.Routes {
		if !rt.Public && !caps[rt.Permission] {
			t.Errorf("permission %q tidak ada di capabilities manifest", rt.Permission)
		}
	}
	// Command tambahan dari `erpora make:command` boleh bertambah; yang wajib ada hanya recount, dan semuanya punya Run.
	var recount *module.Command
	for i, c := range r.Commands {
		if c.Run == nil {
			t.Errorf("command %q tidak punya Run", c.Name)
		}
		if c.Name == "recount" {
			recount = &r.Commands[i]
		}
	}
	if recount == nil {
		t.Fatalf("command recount tidak terdaftar: %+v", r.Commands)
	}
	// Tanpa konteks tenant, command gagal dengan error (bukan panic).
	if err := recount.Run(context.Background(), module.Invocation{Out: &bytes.Buffer{}}); err == nil {
		t.Error("recount tanpa tenant harus error")
	}
}

// Kontrak OpenAPI di-embed; kesesuaiannya dengan route diperiksa oleh test di internal/app (modul tidak boleh
// mengimpor paket platform, aturan D9).
func TestOpenAPI_Embedded(t *testing.T) {
	doc := string(New().OpenAPI())
	if !strings.HasPrefix(doc, "openapi: 3.1.0") {
		t.Errorf("openapi.yaml harus diawali \"openapi: 3.1.0\"")
	}
	for _, want := range []string{"/api/stock-opname/items", "x-erp-permission: stock-opname.items.read"} {
		if !strings.Contains(doc, want) {
			t.Errorf("openapi.yaml tidak memuat %q", want)
		}
	}
}

// Generator make:* menyisipkan registrasi di atas penanda; penanda tidak boleh hilang dari module.go.
func TestModuleGo_GeneratorMarkers(t *testing.T) {
	src, err := os.ReadFile("module.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"// erp:generated:routes", "// erp:generated:commands"} {
		if strings.Count(string(src), marker) != 1 {
			t.Errorf("penanda %q harus tepat satu di module.go", marker)
		}
	}
}

func TestCount_NoTenantContext(t *testing.T) {
	if _, err := New().svc.Count(context.Background()); err == nil {
		t.Error("Count tanpa tenant harus error")
	}
}

// Kunci yang dirujuk kode diperiksa lewat moduletest.I18nContext: StrictMissing menggagalkan test bila kunci salah
// ketik atau hilang di bahasa dasar. Jangan memakai t.Parallel() pada test yang memakai I18nContext (StrictMissing
// global proses). en-GB tidak punya berkas sendiri dan jatuh ke en, lalu bahasa dasar (rantai fallback helper).
func TestCatalogs_KeysResolveUnderStrictMissing(t *testing.T) {
	for _, loc := range []sdki18n.Locale{"id", "en", "en-GB"} {
		t.Run(string(loc), func(t *testing.T) {
			ctx := moduletest.I18nContext(t, New(), loc)
			if got := sdki18n.T(ctx, "errors.bad_request"); got == "" || strings.Contains(got, "::") {
				t.Errorf("errors.bad_request = %q", got)
			}
		})
	}
}
