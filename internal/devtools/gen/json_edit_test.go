package gen

import (
	"strings"
	"testing"
)

func TestAppendJSONArrayString(t *testing.T) {
	inline := "{\n  \"key\": \"x\",\n  \"capabilities\": [\"a.b.c\", \"a.d.e\"],\n  \"z\": 1\n}\n"
	got, err := AppendJSONArrayString([]byte(inline), "capabilities", "a.f.g")
	if err != nil {
		t.Fatal(err)
	}
	if want := "  \"capabilities\": [\"a.b.c\", \"a.d.e\", \"a.f.g\"],\n"; !strings.Contains(string(got), want) {
		t.Errorf("inline:\n%s", got)
	}
	if !strings.HasPrefix(string(got), "{\n  \"key\": \"x\",\n") || !strings.HasSuffix(string(got), "  \"z\": 1\n}\n") {
		t.Errorf("bagian lain berubah:\n%s", got)
	}
	again, err := AppendJSONArrayString(got, "capabilities", "a.f.g")
	if err != nil || string(again) != string(got) {
		t.Errorf("nilai sudah ada harus tak berubah: %v\n%s", err, again)
	}

	multi := "{\n  \"caps\": [\n    \"a\",\n    \"b\"\n  ],\n  \"n\": 2\n}\n"
	got, err = AppendJSONArrayString([]byte(multi), "caps", "c")
	if err != nil || string(got) != "{\n  \"caps\": [\n    \"a\",\n    \"b\",\n    \"c\"\n  ],\n  \"n\": 2\n}\n" {
		t.Errorf("multiline: %v\n%s", err, got)
	}

	empty, err := AppendJSONArrayString([]byte("{\"caps\": []}"), "caps", "x")
	if err != nil || string(empty) != "{\"caps\": [\"x\"]}" {
		t.Errorf("array kosong: %v %s", err, empty)
	}
	if _, err := AppendJSONArrayString([]byte(inline), "tidak-ada", "x"); err == nil {
		t.Error("kunci tidak ada harus error")
	}
	if _, err := AppendJSONArrayString([]byte(inline), "key", "x"); err == nil {
		t.Error("kunci bukan array harus error")
	}
}

func TestAddJSONDependency(t *testing.T) {
	src := "{\n  \"name\": \"@erpora/web\",\n  \"devDependencies\": {\n    \"typescript\": \"6.0.3\"\n  }\n}\n"
	got, err := AddJSONDependency([]byte(src), "dependencies", "@erpora/module-inventory", "workspace:*")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"name\": \"@erpora/web\",\n  \"dependencies\": {\n    \"@erpora/module-inventory\": \"workspace:*\"\n  },\n  \"devDependencies\": {\n    \"typescript\": \"6.0.3\"\n  }\n}\n"
	if string(got) != want {
		t.Errorf("seksi baru:\n%s\nwant:\n%s", got, want)
	}
	got, err = AddJSONDependency(got, "dependencies", "@erpora/module-billing", "workspace:*")
	if err != nil || !strings.Contains(string(got), "\"@erpora/module-billing\": \"workspace:*\",\n    \"@erpora/module-inventory\"") {
		t.Errorf("urut alfabet (sisip di depan): %v\n%s", err, got)
	}
	got, err = AddJSONDependency(got, "dependencies", "@erpora/module-stock", "workspace:*")
	if err != nil || !strings.Contains(string(got), "\"@erpora/module-inventory\": \"workspace:*\",\n    \"@erpora/module-stock\": \"workspace:*\"\n  }") {
		t.Errorf("urut alfabet (sisip di akhir): %v\n%s", err, got)
	}
	again, err := AddJSONDependency(got, "dependencies", "@erpora/module-stock", "workspace:*")
	if err != nil || string(again) != string(got) {
		t.Errorf("sudah ada harus tak berubah: %v", err)
	}
	if _, err := AddJSONDependency([]byte("bukan json"), "dependencies", "a", "b"); err == nil {
		t.Error("JSON rusak harus error")
	}
}
