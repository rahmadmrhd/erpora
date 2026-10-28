package gen

import (
	"strings"
	"testing"
)

// Review Focus 1: slug berstrip menghasilkan Dir tanpa strip dan turunan lain yang konsisten.
func TestParseModuleName_Table(t *testing.T) {
	got, err := ParseModuleName("stock-opname", "acme")
	if err != nil {
		t.Fatal(err)
	}
	want := ModuleName{
		Slug: "stock-opname", Dir: "stockopname", Key: "acme.stock-opname", Snake: "stock_opname",
		Camel: "stockOpname", Pascal: "StockOpname", NPM: "@erpora/module-stockopname", GoPath: "erpora/modules/stockopname",
		Publisher: "acme", Title: "Stock Opname",
	}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	inv, err := ParseModuleName("inventory", "acme")
	if err != nil || inv.Dir != "inventory" || inv.Key != "acme.inventory" || inv.Snake != "inventory" || inv.Pascal != "Inventory" {
		t.Errorf("inventory = %+v err %v", inv, err)
	}
	if p, err := ParseModuleName("billing", "my-corp"); err != nil || p.Key != "my-corp.billing" {
		t.Errorf("publisher: %+v %v", p, err)
	}
	for _, bad := range []string{"Inventory", "stock_opname", "-x", "x-", "a--b", "1abc", "platform", "x", "api", "", "a b"} {
		if _, err := ParseModuleName(bad, "acme"); err == nil {
			t.Errorf("slug %q harus ditolak", bad)
		}
	}
	if _, err := ParseModuleName("billing", "Acme"); err == nil {
		t.Error("publisher Acme harus ditolak")
	}
	long := "a" + string(make([]byte, 0))
	for len(long) < 41 {
		long += "a"
	}
	if _, err := ParseModuleName(long, "acme"); err == nil {
		t.Error("slug 41 karakter harus ditolak")
	}
}

func TestParseTypeName_Table(t *testing.T) {
	cases := []struct {
		in                                                    string
		pascal, camel, snake, kebab, pluralKebab, pluralSnake string
	}{
		{"Item", "Item", "item", "item", "item", "items", "items"},
		{"Category", "Category", "category", "category", "category", "categories", "categories"},
		{"Box", "Box", "box", "box", "box", "boxes", "boxes"},
		{"Day", "Day", "day", "day", "day", "days", "days"},
		{"Branch", "Branch", "branch", "branch", "branch", "branches", "branches"},
		{"StockOpname", "StockOpname", "stockOpname", "stock_opname", "stock-opname", "stock-opnames", "stock_opnames"},
		{"Line2", "Line2", "line2", "line2", "line2", "line2s", "line2s"},
	}
	for _, c := range cases {
		got, err := ParseTypeName(c.in)
		want := TypeName{c.pascal, c.camel, c.snake, c.kebab, c.pluralKebab, c.pluralSnake}
		if err != nil || got != want {
			t.Errorf("%s: got %+v err %v, want %+v", c.in, got, err, want)
		}
	}
	for _, bad := range []string{"item", "_Item", "Item-X", "", "1Item", "Item Name"} {
		if _, err := ParseTypeName(bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}

// G-4: Dir yang menjadi nama package Go tidak boleh keyword, "main", atau identifier predeclared.
func TestParseModuleName_RejectsUnusablePackageNames(t *testing.T) {
	for _, bad := range []string{"go", "if", "func", "type", "map", "var", "for", "range", "chan", "main", "int", "len", "new", "copy", "nil", "any", "error", "string", "true", "false", "iota", "cap", "bool", "byte", "append", "close", "panic", "print", "min", "max", "clear"} {
		_, err := ParseModuleName(bad, "acme")
		if err == nil || !strings.Contains(err.Error(), "package Go") {
			t.Errorf("%q harus ditolak sebagai nama package Go: %v", bad, err)
		}
	}
	if _, err := ParseModuleName("go-live", "acme"); err != nil {
		t.Errorf("go-live (dir golive) harus diterima: %v", err)
	}
}

// G-4: Dir juga tidak boleh bentrok dengan identifier yang sudah dipakai internal/app/modules.go (mis. variabel ex).
func TestCheckModuleDirAgainstAssembly(t *testing.T) {
	src := []byte("package app\n\nimport (\n\t\"erpora/modules/example\"\n\t\"erpora/sdk/module\"\n)\n\nfunc compiledModules() []module.Module {\n\tex := example.New()\n\treturn []module.Module{ex}\n}\n")
	for _, bad := range []string{"ex", "example", "module", "compiledmodules"} {
		if bad == "compiledmodules" { // identifier berhuruf besar-kecil campur tidak bentrok dengan dir huruf kecil
			if err := CheckModuleDir(bad, src); err != nil {
				t.Errorf("%q harus diterima: %v", bad, err)
			}
			continue
		}
		if err := CheckModuleDir(bad, src); err == nil || !strings.Contains(err.Error(), "modules.go") {
			t.Errorf("%q harus ditolak (bentrok di modules.go): %v", bad, err)
		}
	}
	if err := CheckModuleDir("inventory", src); err != nil {
		t.Errorf("inventory harus diterima: %v", err)
	}
}

// G-3: nama yang membuat nama berkas Go bermakna khusus (_test, _<GOOS>, _<GOARCH>) ditolak; berkas itu akan
// dikecualikan dari build atau dianggap berkas test.
func TestParseTypeName_RejectsSpecialFileSuffixes(t *testing.T) {
	for _, bad := range []string{"ItemTest", "FooTest", "ItemLinux", "ItemWindows", "StockDarwin", "ItemAmd64", "ItemArm64", "ItemWasm", "ItemJs", "ItemLinuxAmd64"} {
		if _, err := ParseTypeName(bad); err == nil || !strings.Contains(err.Error(), "berkas") {
			t.Errorf("%q harus ditolak dengan alasan nama berkas: %v", bad, err)
		}
	}
	// Segmen pertama tidak dianggap sufiks oleh go/build; kata biasa yang mirip tetap diterima.
	for _, ok := range []string{"Linux", "Test", "Windows", "Testing", "Latest", "Armed", "LinuxItem", "Contest"} {
		if _, err := ParseTypeName(ok); err != nil {
			t.Errorf("%q harus diterima: %v", ok, err)
		}
	}
}

func TestCheckGoFileStem(t *testing.T) {
	for _, bad := range []string{"x_test", "x_linux", "x_amd64", "x_linux_amd64", "stock_windows"} {
		if err := CheckGoFileStem(bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
	for _, ok := range []string{"x", "linux", "test", "reindex", "recount_items", "x_testing"} {
		if err := CheckGoFileStem(ok); err != nil {
			t.Errorf("%q harus diterima: %v", ok, err)
		}
	}
}

func TestParseSnakeName(t *testing.T) {
	if v, err := ParseSnakeName("add_tags"); err != nil || v != "add_tags" {
		t.Errorf("add_tags: %q %v", v, err)
	}
	for _, bad := range []string{"AddTags", "add-tags", "1add", "", "_x"} {
		if _, err := ParseSnakeName(bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}
