package app

import (
	"net/http"
	"strings"
	"testing"

	"erpora/contracts"
	"erpora/internal/platform/apicontract"
	"erpora/internal/platform/modules/registry"
	"erpora/modules/example"
	"erpora/sdk/module"
)

// buildConformance merakit masukan CheckConformance dari katalog terkompilasi: kontrak common + setiap Entry.Contract,
// route registry ditambah GET /api/bootstrap (route httpserver, bukan registry) sebagai route publik platform.core.
func buildConformance(t *testing.T) (*registry.Registry, apicontract.ConformanceInput) {
	t.Helper()
	reg, err := BuildRegistry()
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	common, err := apicontract.Parse("common", contracts.CommonOpenAPI)
	if err != nil {
		t.Fatalf("common: %v", err)
	}
	docs := []*apicontract.Doc{common}
	docOf := map[string]string{}
	caps := map[string][]string{}
	for _, e := range reg.Entries() {
		caps[e.Manifest.Key] = e.Manifest.Capabilities
		if e.Contract == nil {
			continue
		}
		name := e.Manifest.Slug
		if e.Manifest.Key == registry.PlatformKey {
			name = "platform"
		}
		d, err := apicontract.Parse(name, e.Contract)
		if err != nil {
			t.Fatalf("kontrak %s: %v", e.Manifest.Key, err)
		}
		docs = append(docs, d)
		docOf[e.Manifest.Key] = name
	}
	set, err := apicontract.NewSet(docs...)
	if err != nil {
		t.Fatal(err)
	}
	var routes []apicontract.RouteSpec
	for _, r := range reg.Routes() {
		routes = append(routes, apicontract.RouteSpec{
			Module: r.Module, Method: r.Method, Pattern: r.Pattern, Permission: r.Permission,
			Public: r.Public, Classification: r.Classification,
		})
	}
	routes = append(routes, apicontract.RouteSpec{Module: registry.PlatformKey, Method: http.MethodGet, Pattern: "/api/bootstrap", Public: true})
	return reg, apicontract.ConformanceInput{
		Set: set, DocOf: docOf, Routes: routes, Capabilities: caps,
		DispatchEquivalent: map[string]bool{registry.PlatformKey: true},
	}
}

func mismatchKinds(ms []apicontract.Mismatch) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Kind + " " + m.Module + " " + m.Method + " " + m.Path + ": " + m.Detail
	}
	return out
}

func TestContracts_EveryEntryHasContract(t *testing.T) {
	reg, _ := buildConformance(t)
	for _, e := range reg.Entries() {
		if len(e.Contract) == 0 {
			t.Errorf("modul %s tidak punya kontrak OpenAPI (Entry.Contract kosong)", e.Manifest.Key)
		}
	}
}

func TestContracts_RoutesMatchOperations(t *testing.T) {
	t.Run("katalog terkompilasi", func(t *testing.T) {
		_, in := buildConformance(t)
		if got := apicontract.CheckConformance(in); len(got) != 0 {
			t.Errorf("route dan kontrak tidak sesuai:\n%s", strings.Join(mismatchKinds(got), "\n"))
		}
	})

	// AC FR-23.2: menambah Registrar.Handle tanpa operasi di kontrak menggagalkan test.
	t.Run("extra_route", func(t *testing.T) {
		restore := swapModules(func() []module.Module {
			return []module.Module{&extraRouteModule{Module: example.New()}}
		})
		defer restore()
		_, in := buildConformance(t)
		got := apicontract.CheckConformance(in)
		var missing []apicontract.Mismatch
		for _, m := range got {
			if m.Kind == "missing_operation" {
				missing = append(missing, m)
			}
		}
		if len(got) != 1 || len(missing) != 1 || missing[0].Path != "/api/example/extra" {
			t.Errorf("want tepat satu missing_operation /api/example/extra, got:\n%s", strings.Join(mismatchKinds(got), "\n"))
		}
	})

	t.Run("extra_operation", func(t *testing.T) {
		restore := swapModules(func() []module.Module {
			return []module.Module{&extraOperationModule{Module: example.New()}}
		})
		defer restore()
		_, in := buildConformance(t)
		got := apicontract.CheckConformance(in)
		if len(got) != 1 || got[0].Kind != "missing_route" || got[0].Path != "/api/example/extra-op" {
			t.Errorf("want tepat satu missing_route /api/example/extra-op, got:\n%s", strings.Join(mismatchKinds(got), "\n"))
		}
	})
}

func TestContracts_PermissionsDeclaredInManifest(t *testing.T) {
	_, in := buildConformance(t)
	for _, m := range apicontract.CheckConformance(in) {
		if m.Kind == "undeclared_permission" {
			t.Errorf("%s %s %s: %s", m.Module, m.Method, m.Path, m.Detail)
		}
	}
	// Setiap permission pada operasi harus berbentuk <slug>.<resource>.<action> (platform: platform.<resource>.<action>).
	for key, name := range in.DocOf {
		for _, op := range in.Set.Operations(name) {
			if op.Permission == "" {
				continue
			}
			parts := strings.Split(op.Permission, ".")
			slug := name
			if len(parts) != 3 || parts[0] != slug {
				t.Errorf("%s %s %s: permission %q tidak berbentuk %s.<resource>.<action>", key, op.Method, op.Path, op.Permission, slug)
			}
		}
	}
}

func TestContracts_ClassificationMatchesRoute(t *testing.T) {
	_, in := buildConformance(t)
	for _, m := range apicontract.CheckConformance(in) {
		if m.Kind == "classification_mismatch" {
			t.Errorf("%s %s %s: %s", m.Module, m.Method, m.Path, m.Detail)
		}
	}
	// Klasifikasi tiap operasi dapat dihitung (tanpa error) untuk seluruh kontrak.
	for key, name := range in.DocOf {
		for _, op := range in.Set.Operations(name) {
			if _, err := in.Set.ResponseClassification(op); err != nil {
				t.Errorf("%s %s %s: %v", key, op.Method, op.Path, err)
			}
		}
	}
}

func TestContracts_ErpRefsOnlyToDependencies(t *testing.T) {
	reg, in := buildConformance(t)
	for _, e := range reg.Entries() {
		name, ok := in.DocOf[e.Manifest.Key]
		if !ok {
			continue
		}
		allowed := map[string]bool{"common": true}
		for _, dep := range e.Manifest.Dependencies {
			if dep.Module == registry.PlatformKey {
				continue // platform.core: bagian dari erp:common/erp:platform tidak dirujuk modul
			}
			if de, ok := reg.Entry(dep.Module); ok {
				allowed[de.Manifest.Slug] = true
			}
		}
		doc, _ := in.Set.Doc(name)
		for _, ref := range doc.ExternalRefs() {
			if !allowed[ref] {
				t.Errorf("kontrak %s merujuk erp:%s yang bukan common atau dependensi manifest (FR-23.1)", e.Manifest.Key, ref)
			}
		}
	}
}

// swapModules mengganti katalog terkompilasi untuk satu test; hasilnya fungsi pemulih.
func swapModules(fn func() []module.Module) func() {
	old := compiledModulesFn
	compiledModulesFn = fn
	return func() { compiledModulesFn = old }
}

// extraRouteModule membungkus example dan menambah route yang tidak ada di kontrak.
type extraRouteModule struct{ *example.Module }

func (m *extraRouteModule) Register(r module.Registrar) error {
	if err := m.Module.Register(r); err != nil {
		return err
	}
	r.Handle(http.MethodGet, "/extra", "example.ping.read", http.NotFoundHandler())
	return nil
}

// extraOperationModule membungkus example dan menambah operasi di kontrak yang tidak punya route.
type extraOperationModule struct{ *example.Module }

func (m *extraOperationModule) OpenAPI() []byte {
	src := string(m.Module.OpenAPI())
	extra := "  /api/example/extra-op:\n    x-erp-singleton: true\n    get:\n      operationId: getExampleExtraOp\n      x-erp-permission: example.ping.read\n      responses:\n        '200': { description: ok }\ncomponents:\n"
	return []byte(strings.Replace(src, "components:\n", extra, 1))
}
