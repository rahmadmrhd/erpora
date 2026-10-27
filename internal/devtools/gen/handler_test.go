package gen

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"erpora/contracts"
	"erpora/internal/devtools/contracttools"
	"erpora/internal/platform/apicontract"
	"erpora/sdk/module"
)

// moduleRepo menyalin fixture lalu menerapkan make:module <name> (tanpa Post).
func moduleRepo(t *testing.T, name string) string {
	t.Helper()
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, plan)
	return root
}

func TestMakeHandler_Golden(t *testing.T) {
	cases := []struct {
		golden, module string
		opts           HandlerOptions
	}{
		{"handler_get_item", "inventory", HandlerOptions{Module: "inventory", Name: "GetItem", Route: "GET /items/{id}", Permission: "inventory.items.read"}},
		{"handler_post_action", "inventory", HandlerOptions{Module: "inventory", Name: "ApproveItem", Route: "POST /items/{id}/approve", Permission: "inventory.items.approve"}},
		{"handler_collection_hyphenated", "stock-opname", HandlerOptions{Module: "stock-opname", Name: "ListStockCounts", Route: "GET /stock-counts", Permission: "stock-opname.stock-counts.read"}},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			root := moduleRepo(t, c.module)
			plan, err := MakeHandler(root, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			AssertGolden(t, filepath.Join("testdata", "golden", c.golden), plan.Ops)
		})
	}
}

// Modul ber-web/ menyimpan tipe TS hasil generate yang di-commit: make:handler menambah operasi, jadi tipe itu
// harus dibangkitkan ulang (contracts:generate --check tidak boleh basi). Modul tanpa web/ tidak punya langkah itu.
func TestMakeHandler_RegeneratesTypesForWebModules(t *testing.T) {
	opts := func(m string) HandlerOptions {
		return HandlerOptions{Module: m, Name: "ApproveItem", Route: "POST /items/{id}/approve", Permission: m + ".items.approve"}
	}
	root := fixtureRepo(t)
	mp, err := MakeModule(root, ModuleOptions{Name: "inventory", Frontend: true})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, mp)
	np, err := MakeModule(root, ModuleOptions{Name: "stock-opname"})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, np)

	withWeb, err := MakeHandler(root, opts("inventory"))
	if err != nil {
		t.Fatal(err)
	}
	if len(withWeb.Post) != 1 || withWeb.Post[0].Name != "contracts:generate inventory" {
		t.Errorf("Post modul ber-web = %+v, want satu langkah contracts:generate inventory", withWeb.Post)
	}
	noWeb, err := MakeHandler(root, opts("stock-opname"))
	if err != nil {
		t.Fatal(err)
	}
	if len(noWeb.Post) != 0 {
		t.Errorf("modul tanpa web/ tidak boleh punya langkah Post: %+v", noWeb.Post)
	}
}

// Review Focus 2: operasi yang sudah ada di kontrak ditolak sebelum Plan; direktori tidak berubah.
func TestMakeHandler_ExistingOperationRejected(t *testing.T) {
	root := moduleRepo(t, "stock-opname")
	before := checksum(t, root)
	_, err := MakeHandler(root, HandlerOptions{Module: "stock-opname", Name: "ListItems2", Route: "GET /items", Permission: "stock-opname.items.read"})
	if err == nil || !strings.Contains(err.Error(), "sudah ada") {
		t.Errorf("err = %v, want operasi sudah ada", err)
	}
	if checksum(t, root) != before {
		t.Error("direktori berubah")
	}
}

func TestMakeHandler_Validation(t *testing.T) {
	root := moduleRepo(t, "stock-opname")
	ok := HandlerOptions{Module: "stock-opname", Name: "GetCount", Route: "GET /counts/{id}", Permission: "stock-opname.counts.read"}
	if _, err := MakeHandler(root, ok); err != nil {
		t.Fatalf("opsi valid ditolak: %v", err)
	}
	bad := map[string]HandlerOptions{
		"permission prefiks salah (TestMakeHandler_PermissionPrefix)": {Module: "stock-opname", Name: "GetCount", Route: "GET /counts/{id}", Permission: "inventory.items.read"},
		"permission format":             {Module: "stock-opname", Name: "GetCount", Route: "GET /counts/{id}", Permission: "stock-opname.counts"},
		"PUT ditolak (§3)":              {Module: "stock-opname", Name: "PutCount", Route: "PUT /counts/{id}", Permission: "stock-opname.counts.write"},
		"method tidak dikenal":          {Module: "stock-opname", Name: "OptCount", Route: "OPTIONS /counts", Permission: "stock-opname.counts.read"},
		"path tanpa slash awal":         {Module: "stock-opname", Name: "GetCount", Route: "GET counts/{id}", Permission: "stock-opname.counts.read"},
		"path trailing slash":           {Module: "stock-opname", Name: "GetCount", Route: "GET /counts/", Permission: "stock-opname.counts.read"},
		"segmen bukan kebab":            {Module: "stock-opname", Name: "GetCount", Route: "GET /stockCounts", Permission: "stock-opname.counts.read"},
		"koleksi tunggal":               {Module: "stock-opname", Name: "GetCount", Route: "GET /count/{id}", Permission: "stock-opname.counts.read"},
		"parameter bukan {id}":          {Module: "stock-opname", Name: "GetCount", Route: "GET /counts/{countId}", Permission: "stock-opname.counts.read"},
		"bersarang dua tingkat":         {Module: "stock-opname", Name: "GetLine", Route: "GET /counts/{id}/lines/{line}", Permission: "stock-opname.counts.read"},
		"POST pada resource tanpa aksi": {Module: "stock-opname", Name: "PostCount", Route: "POST /counts/{id}", Permission: "stock-opname.counts.write"},
		"nama bukan PascalCase":         {Module: "stock-opname", Name: "getCount", Route: "GET /counts/{id}", Permission: "stock-opname.counts.read"},
		"modul tidak ada":               {Module: "nope", Name: "GetCount", Route: "GET /counts/{id}", Permission: "nope.counts.read"},
		"route tanpa method":            {Module: "stock-opname", Name: "GetCount", Route: "/counts/{id}", Permission: "stock-opname.counts.read"},
		"PATCH koleksi":                 {Module: "stock-opname", Name: "PatchCounts", Route: "PATCH /counts", Permission: "stock-opname.counts.write"},
		"aksi bukan kebab":              {Module: "stock-opname", Name: "DoIt", Route: "POST /counts/{id}/doIt", Permission: "stock-opname.counts.write"},
	}
	for name, o := range bad {
		if _, err := MakeHandler(root, o); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
	}
	// Pesan PUT menyebut api-guidelines section 3.
	if _, err := MakeHandler(root, bad["PUT ditolak (§3)"]); err == nil || !strings.Contains(err.Error(), "PATCH") {
		t.Errorf("pesan PUT: %v", err)
	}
}

func TestAddOperation_PreservesComments(t *testing.T) {
	src := "# komentar kepala\nopenapi: 3.1.0\npaths:\n  # komentar path\n  /api/x/items:\n    get:\n      operationId: listItems # komentar baris\n      responses: { '200': { description: ok } }\ncomponents:\n  schemas: {}\n"
	var op yaml.Node
	if err := yaml.Unmarshal([]byte("operationId: createItem\nresponses: { '201': { description: dibuat } }\n"), &op); err != nil {
		t.Fatal(err)
	}
	got, err := AddOperation([]byte(src), "/api/x/items", "post", op.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# komentar kepala", "# komentar path", "# komentar baris", "operationId: createItem", "post:", "components:"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("hasil tidak memuat %q:\n%s", want, got)
		}
	}
	if strings.Index(string(got), "get:") > strings.Index(string(got), "post:") {
		t.Errorf("urutan operasi berubah:\n%s", got)
	}
	// Bagian yang tidak disentuh tetap persis sama (gaya flow, spasi, komentar).
	for _, line := range strings.Split(src, "\n") {
		if line != "" && !strings.Contains(string(got), line+"\n") {
			t.Errorf("baris asli berubah atau hilang: %q\n%s", line, got)
		}
	}
	again, err := AddOperation(got, "/api/x/items", "post", op.Content[0])
	if err == nil {
		t.Errorf("operasi ganda harus ditolak:\n%s", again)
	}
	// Path baru dibuat bila belum ada.
	withNew, err := AddOperation([]byte(src), "/api/x/items/{id}", "get", op.Content[0])
	if err != nil || !strings.Contains(string(withNew), "/api/x/items/{id}:") {
		t.Errorf("path baru: %v\n%s", err, withNew)
	}
}

// allKinds adalah enam bentuk handler yang didukung, pada resource orders.
func allKinds() []HandlerOptions {
	m := "inventory"
	return []HandlerOptions{
		{Module: m, Name: "ListOrders", Route: "GET /orders", Permission: "inventory.orders.read"},
		{Module: m, Name: "CreateOrder", Route: "POST /orders", Permission: "inventory.orders.create"},
		{Module: m, Name: "GetOrder", Route: "GET /orders/{id}", Permission: "inventory.orders.read"},
		{Module: m, Name: "UpdateOrder", Route: "PATCH /orders/{id}", Permission: "inventory.orders.update"},
		{Module: m, Name: "DeleteOrder", Route: "DELETE /orders/{id}", Permission: "inventory.orders.delete"},
		{Module: m, Name: "ApproveOrder", Route: "POST /orders/{id}/approve", Permission: "inventory.orders.approve"},
	}
}

// modulWithAllHandlers menerapkan keenam handler dan menyiapkan modul agar dapat dibangun terhadap sdk asli.
func modulWithAllHandlers(t *testing.T) (root, modDir string, env []string) {
	t.Helper()
	root = moduleRepo(t, "inventory")
	for _, o := range allKinds() {
		plan, err := MakeHandler(root, o)
		if err != nil {
			t.Fatalf("%s: %v", o.Name, err)
		}
		applyPlan(t, root, plan)
	}
	realSDK, err := filepath.Abs(filepath.Join("..", "..", "..", "sdk"))
	if err != nil {
		t.Fatal(err)
	}
	modDir = filepath.Join(root, "modules", "inventory")
	gomod := filepath.Join(modDir, "go.mod")
	data, _ := os.ReadFile(gomod)
	if err := os.WriteFile(gomod, []byte(strings.ReplaceAll(string(data), "=> ../../sdk", "=> "+filepath.ToSlash(realSDK))), 0o600); err != nil {
		t.Fatal(err)
	}
	env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	return root, modDir, env
}

func TestMakeHandler_AllKindsCompileAndPassTests(t *testing.T) {
	if testing.Short() {
		t.Skip("membangun modul hasil generate; dilewati pada -short")
	}
	_, modDir, env := modulWithAllHandlers(t)
	for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}, {"test", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = modDir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// routesProgram mencetak route yang didaftarkan modul sebagai JSON (lewat moduletest.Recorder).
const routesProgram = `package main

import (
	"encoding/json"
	"fmt"

	"erpora/modules/inventory"
	"erpora/sdk/module/moduletest"
)

func main() {
	r := moduletest.NewRecorder()
	if err := inventory.New().Register(r); err != nil {
		panic(err)
	}
	type route struct {
		Method, Path, Permission string
		Public                   bool
		Classification           string
	}
	var out []route
	for _, rt := range r.Routes {
		out = append(out, route{rt.Method, rt.Path, rt.Permission, rt.Public, string(rt.Options.Classification)})
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}
`

// Hasil make:handler harus sesuai kontrak: route terdaftar, openapi.yaml, dan capabilities module.json (FR-23.2).
func TestMakeHandler_ResultConforms(t *testing.T) {
	if testing.Short() {
		t.Skip("menjalankan program Go; dilewati pada -short")
	}
	root, modDir, env := modulWithAllHandlers(t)
	progDir := filepath.Join(modDir, "internal", "zzroutes")
	if err := os.MkdirAll(progDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(progDir, "main.go"), []byte(routesProgram), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./internal/zzroutes")
	cmd.Dir, cmd.Env = modDir, env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, stderr.String())
	}
	var routes []struct {
		Method, Path, Permission string
		Public                   bool
		Classification           string
	}
	if err := json.Unmarshal(out, &routes); err != nil {
		t.Fatalf("keluaran: %v\n%s", err, out)
	}
	var specs []apicontract.RouteSpec
	for _, r := range routes {
		specs = append(specs, apicontract.RouteSpec{
			Module: "acme.inventory", Method: r.Method, Pattern: "/api/inventory" + r.Path, Permission: r.Permission, Public: r.Public,
			Classification: module.Classification(r.Classification),
		})
	}
	if len(specs) != 7 { // /items dari make:module + enam handler
		t.Fatalf("route terdaftar = %d, want 7: %+v", len(specs), routes)
	}

	common, err := apicontract.Parse("common", contracts.CommonOpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	specData, err := os.ReadFile(filepath.Join(modDir, "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := apicontract.Parse("inventory", specData)
	if err != nil {
		t.Fatalf("openapi.yaml hasil edit tidak valid: %v", err)
	}
	set, err := apicontract.NewSet(common, doc)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := ResolveModule(root, "inventory")
	if err != nil {
		t.Fatal(err)
	}
	mism := apicontract.CheckConformance(apicontract.ConformanceInput{
		Set: set, DocOf: map[string]string{"acme.inventory": "inventory"}, Routes: specs,
		Capabilities: map[string][]string{"acme.inventory": ref.Capabilities},
	})
	for _, m := range mism {
		t.Errorf("%s %s %s: %s", m.Kind, m.Method, m.Path, m.Detail)
	}
}

func TestMakeHandler_ResultLints(t *testing.T) {
	bun, err := contracttools.FindBun()
	if err != nil {
		if os.Getenv("ERP_REQUIRE_BUN") == "1" {
			t.Fatalf("ERP_REQUIRE_BUN=1 tetapi bun tidak ada: %v", err)
		}
		t.Skip("bun tidak ada")
	}
	root := moduleRepo(t, "inventory")
	for _, o := range allKinds() {
		plan, err := MakeHandler(root, o)
		if err != nil {
			t.Fatal(err)
		}
		applyPlan(t, root, plan)
	}
	realRoot, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	for _, f := range []string{"contracts/openapi/common.yaml", "contracts/openapi/platform.yaml"} {
		data, err := os.ReadFile(filepath.Join(realRoot, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "contracts", "openapi"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := contracttools.LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	repo.ToolsRoot = realRoot
	res, err := contracttools.Lint(context.Background(), repo, "inventory", bun)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		t.Errorf("%s:%d %s %s", r.File, r.Line, r.Rule, r.Message)
	}
}
