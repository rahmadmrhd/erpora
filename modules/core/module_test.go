package core

import (
	"os"
	"strings"
	"testing"

	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
	"erpora/sdk/module/moduletest"
)

var (
	_ module.Module         = (*Module)(nil)
	_ module.ContractSource = (*Module)(nil)
	_ module.CatalogSource  = (*Module)(nil)
)

// Manifest: modul required (erp.core) tanpa migrasi sendiri (tabel identitas dimiliki platform) dan tanpa
// capability (route auth tidak memakai permission; capability RBAC datang pada C2).
func TestManifest(t *testing.T) {
	m := New().Manifest()
	if m.Key != "erp.core" || m.Slug != "core" || len(m.Raw) == 0 {
		t.Fatalf("manifest = %+v", m)
	}
	if !m.Required {
		t.Error("core wajib required: true (tidak dapat di-disable atau di-uninstall)")
	}
	if m.Migrations.Strategy != "none" {
		t.Errorf("migrations.strategy = %q, want none", m.Migrations.Strategy)
	}
	for _, c := range m.Capabilities {
		if !strings.HasPrefix(c, "core.") {
			t.Errorf("capability %q harus berawalan core.", c)
		}
	}
}

// Route: method, path, serta aturan publik vs terautentikasi sesuai PRD-001 Q29 dan daftar route sesi menunggu MFA
// milik platform (runtime/authroutes.go; path-nya harus persis sama).
func TestRegister_RoutesVisibilityAndOptions(t *testing.T) {
	r := moduletest.NewRecorder()
	if err := New().Register(r); err != nil {
		t.Fatal(err)
	}
	public := []string{"/auth/login", "/auth/refresh", "/auth/password/forgot", "/auth/password/reset"}
	own := []string{
		"/auth/logout", "/auth/password/change",
		"/auth/mfa/enroll", "/auth/mfa/confirm", "/auth/mfa/verify", "/auth/mfa/recovery", "/auth/mfa/recovery-codes",
	}
	if len(r.Routes) != len(public)+len(own) {
		t.Fatalf("jumlah route = %d, want %d: %+v", len(r.Routes), len(public)+len(own), r.Routes)
	}
	for _, p := range public {
		rt, ok := r.Route("POST", p)
		if !ok || !rt.Public || rt.Permission != "" || rt.Options.Authenticated {
			t.Errorf("POST %s harus Public tanpa Authenticated: %+v", p, rt)
		}
	}
	for _, p := range own {
		rt, ok := r.Route("POST", p)
		if !ok || rt.Public || rt.Permission != "" || !rt.Options.Authenticated {
			t.Errorf("POST %s harus Authenticated (tanpa permission, tidak Public): %+v", p, rt)
		}
	}
	// Respons yang memuat token/rahasia ditandai highly_confidential (no-store oleh platform); kontrak menegakkan kesamaannya.
	for _, p := range []string{"/auth/login", "/auth/refresh", "/auth/mfa/enroll", "/auth/mfa/confirm", "/auth/mfa/verify", "/auth/mfa/recovery", "/auth/mfa/recovery-codes"} {
		rt, _ := r.Route("POST", p)
		if rt.Options.Classification != module.ClassHighlyConfidential {
			t.Errorf("POST %s klasifikasi = %q, want highly_confidential", p, rt.Options.Classification)
		}
	}
	if rt, _ := r.Route("POST", "/auth/mfa/recovery-codes"); !rt.Options.RequireMFA {
		t.Error("recovery-codes menuntut sesi terverifikasi MFA (RequireMFA)")
	}
	for _, rt := range r.Routes {
		if rt.Options.MaxBodyBytes == 0 || rt.Options.MaxBodyBytes > 16<<10 {
			t.Errorf("%s %s batas body = %d, want kecil", rt.Method, rt.Path, rt.Options.MaxBodyBytes)
		}
	}
	// Command CLI (C1.12): lima command core:*; nama tercatat tanpa slug (registry menambahkannya).
	var names []string
	for _, c := range r.Commands {
		names = append(names, c.Name)
	}
	if got, want := strings.Join(names, ","), "create-admin,rotate-keys,reset-mfa,unlock,settings:set"; got != want {
		t.Errorf("command = %s, want %s", got, want)
	}
}

func TestOpenAPI_Embedded(t *testing.T) {
	doc := string(New().OpenAPI())
	if !strings.HasPrefix(doc, "openapi: 3.1.0") {
		t.Errorf("openapi.yaml harus diawali \"openapi: 3.1.0\"")
	}
	for _, want := range []string{"/api/core/auth/login", "x-erp-credential-response: true", "x-erp-authenticated: true"} {
		if !strings.Contains(doc, want) {
			t.Errorf("openapi.yaml tidak memuat %q", want)
		}
	}
	// Refresh token tidak boleh punya properti di kontrak (hanya cookie).
	if strings.Contains(strings.ToLower(doc), "refreshtoken:") {
		t.Error("kontrak tidak boleh mendeklarasikan properti refreshToken")
	}
}

// Katalog: sejajar antar bahasa dan semua kunci detail yang dirujuk handler ada (pesan galat errors.* dimiliki katalog platform).
func TestCatalogs_ParseAndAligned(t *testing.T) {
	m := New()
	byLocale := map[string]sdki18n.Flat{}
	for _, loc := range []string{"id", "en"} {
		f, err := m.Catalogs().Open(loc + ".json")
		if err != nil {
			t.Fatal(err)
		}
		flat, issues := sdki18n.ParseCatalog(f, sdki18n.DefaultLimits())
		_ = f.Close()
		if len(issues) > 0 {
			t.Fatalf("katalog %s: %v", loc, issues)
		}
		byLocale[loc] = flat
	}
	for k := range byLocale["id"] {
		if _, ok := byLocale["en"][k]; !ok {
			t.Errorf("kunci %q hanya ada di id", k)
		}
	}
	if len(byLocale["id"]) != len(byLocale["en"]) {
		t.Errorf("jumlah kunci id=%d en=%d", len(byLocale["id"]), len(byLocale["en"]))
	}
	for _, k := range []string{
		"password.violation.too_short", "password.violation.too_long", "password.violation.common", "password.violation.unknown",
		"detail.wrong_current_password", "detail.invalid_token", "detail.invalid_code",
	} {
		if byLocale["id"][k] == "" {
			t.Errorf("katalog id tidak memuat %q", k)
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
