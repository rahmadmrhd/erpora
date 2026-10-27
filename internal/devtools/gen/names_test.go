package gen

import "testing"

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
