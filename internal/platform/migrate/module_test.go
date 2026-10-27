package migrate

import "testing"

func TestModuleTable(t *testing.T) {
	cases := map[string]string{
		"example":       "goose_module_example",
		"example-addon": "goose_module_example_addon",
		"a-b-c":         "goose_module_a_b_c",
	}
	for slug, want := range cases {
		if got := ModuleTable(slug); got != want {
			t.Errorf("ModuleTable(%q) = %q, want %q", slug, got, want)
		}
	}
}
