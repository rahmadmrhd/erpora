package gen

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"erpora/internal/archcheck"
)

// inventoryRepo menyalin fixture lalu menerapkan make:module inventory (tanpa Post) sebagai dasar generator lapisan.
func inventoryRepo(t *testing.T) string {
	t.Helper()
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory"})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, plan)
	return root
}

func TestMakeEntity_Golden(t *testing.T) {
	root := inventoryRepo(t)
	plan, err := MakeEntity(root, "inventory", "Category")
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/entity", plan.Ops)
	want := []string{"modules/inventory/internal/domain/category.go", "modules/inventory/internal/domain/category_test.go"}
	if got := strings.Join(opPaths(plan.Ops), ","); got != strings.Join(want, ",") {
		t.Errorf("paths = %s", got)
	}
}

func TestMakeUsecase_Golden(t *testing.T) {
	root := inventoryRepo(t)
	cmd, err := MakeUsecase(root, "inventory", "AdjustStock", false)
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/usecase_command", cmd.Ops)
	q, err := MakeUsecase(root, "inventory", "StockLevel", true)
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/usecase_query", q.Ops)
	if got := opContent(t, q.Ops, "modules/inventory/internal/application/stock_level.go"); !strings.Contains(got, "type StockLevelResult struct") {
		t.Errorf("query harus punya Result:\n%s", got)
	}
}

func TestMakeContract_Golden(t *testing.T) {
	root := inventoryRepo(t)
	plan, err := MakeContract(root, "inventory", "Stock")
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/contract", plan.Ops)
}

func TestMakeContract_NoInternalImports(t *testing.T) {
	root := inventoryRepo(t)
	plan, err := MakeContract(root, "inventory", "Stock")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range plan.Ops {
		f, err := parser.ParseFile(token.NewFileSet(), op.Path, op.Content, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(p, "/internal") {
				t.Errorf("%s mengimpor %s; contract tidak boleh mengimpor internal/", op.Path, p)
			}
		}
	}
}

func TestMakeCommand_Golden(t *testing.T) {
	root := inventoryRepo(t)
	plan, err := MakeCommand(root, "inventory", "sync-stock")
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/command", plan.Ops)
	mod := opContent(t, plan.Ops, "modules/inventory/module.go")
	if !strings.Contains(mod, "r.Command(cli.SyncStockCommand())") || !strings.Contains(mod, "internal/presentation/cli\"") {
		t.Errorf("module.go belum menyisipkan command dan impor:\n%s", mod)
	}
}

// G-3: nama command yang menghasilkan berkas _test.go atau _<GOOS>.go ditolak sebelum Plan.
func TestMakeCommand_SpecialFileSuffixRejected(t *testing.T) {
	root := inventoryRepo(t)
	for _, name := range []string{"sync-test", "sync-linux", "sync-amd64"} {
		if _, err := MakeCommand(root, "inventory", name); err == nil || !strings.Contains(err.Error(), "berkas") {
			t.Errorf("%s: err = %v, want penolakan nama berkas", name, err)
		}
	}
}

func TestMakeCommand_DuplicateNameRejected(t *testing.T) {
	root := inventoryRepo(t)
	before := checksum(t, root)
	if _, err := MakeCommand(root, "inventory", "recount"); err == nil || !strings.Contains(err.Error(), "recount") {
		t.Errorf("recount sudah ada: err = %v", err)
	}
	// Command yang baru dibuat juga terdeteksi sebagai duplikat (dibaca dari presentation/cli lewat go/ast).
	plan, err := MakeCommand(root, "inventory", "sync-stock")
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, plan)
	if _, err := MakeCommand(root, "inventory", "sync-stock"); err == nil {
		t.Error("duplikat setelah diterapkan harus ditolak")
	}
	for _, bad := range []string{"Sync", "sync_stock", "1sync", "-x"} {
		if _, err := MakeCommand(root, "inventory", bad); err == nil {
			t.Errorf("nama %q harus ditolak", bad)
		}
	}
	_ = before
}

func TestMakeRepository_RequiresEntity(t *testing.T) {
	root := inventoryRepo(t)
	_, err := MakeRepository(root, "inventory", "Category")
	if err == nil || !strings.Contains(err.Error(), "make:entity") {
		t.Errorf("err = %v, want instruksi make:entity", err)
	}
}

func TestMakeRepository_Golden(t *testing.T) {
	root := inventoryRepo(t)
	ent, err := MakeEntity(root, "inventory", "Category")
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, ent)
	plan, err := MakeRepository(root, "inventory", "Category")
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/repository", plan.Ops)
	if got := opContent(t, plan.Ops, "modules/inventory/internal/infrastructure/category_model.go"); !strings.Contains(got, `"inventory_categories"`) {
		t.Errorf("nama tabel salah:\n%s", got)
	}
}

func TestLayers_RejectUnknownModuleAndExisting(t *testing.T) {
	root := inventoryRepo(t)
	if _, err := MakeEntity(root, "nope", "X"); err == nil {
		t.Error("modul tidak ada harus error")
	}
	if _, err := MakeEntity(root, "inventory", "item"); err == nil {
		t.Error("nama bukan PascalCase harus error")
	}
	plan, _ := MakeEntity(root, "inventory", "Category")
	applyPlan(t, root, plan)
	plan2, err := MakeEntity(root, "inventory", "Category")
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(t.Context(), plan2, Options{Root: root}); !errors.Is(err, ErrExists) {
		t.Errorf("entity kedua: err = %v, want ErrExists", err)
	}
}

// Kode hasil kelima generator harus dapat dibangun, lolos vet dan test-nya sendiri, serta tidak melanggar aturan arsitektur.
func TestLayers_GeneratedCodeCompilesAndPassesArch(t *testing.T) {
	if testing.Short() {
		t.Skip("membangun modul hasil generate; dilewati pada -short")
	}
	root := inventoryRepo(t)
	apply := func(p Plan, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		applyPlan(t, root, p)
	}
	apply(MakeEntity(root, "inventory", "Category"))
	apply(MakeUsecase(root, "inventory", "AdjustStock", false))
	apply(MakeUsecase(root, "inventory", "StockLevel", true))
	apply(MakeContract(root, "inventory", "Stock"))
	apply(MakeCommand(root, "inventory", "sync-stock"))
	apply(MakeRepository(root, "inventory", "Category"))

	realSDK, err := filepath.Abs(filepath.Join("..", "..", "..", "sdk"))
	if err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "modules", "inventory")
	gomod := filepath.Join(modDir, "go.mod")
	data, _ := os.ReadFile(gomod)
	patched := strings.ReplaceAll(string(data), "=> ../../sdk", "=> "+filepath.ToSlash(realSDK))
	if err := os.WriteFile(gomod, []byte(patched), 0o600); err != nil {
		t.Fatal(err)
	}
	// Lihat modulWithAllHandlers: proxy dan sumdb dibiarkan aktif agar dependensi transitif sdk dapat diunduh di CI.
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}, {"test", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = modDir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	t.Setenv("GOWORK", "off")
	repo, err := archcheck.Load(modDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range archcheck.Check(repo) {
		switch v.Rule {
		case "D1", "D3", "D3b", "D4":
			t.Errorf("pelanggaran %s di %s: %s", v.Rule, v.Package, v.Detail)
		}
	}
}
