package main

import (
	"encoding/json"
	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "perbarui berkas golden")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("baca golden (jalankan dengan -update): %v", err)
	}
	if string(want) != got {
		t.Errorf("output berbeda dari %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestInspectRoutes_Golden(t *testing.T) {
	clearEnv(t) // tanpa ERP_DATABASE_URL: inspect tidak butuh DB/env
	out, err := run(t, "inspect", "routes")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "inspect_routes.golden", out)
	alias, err := run(t, "route:list")
	if err != nil {
		t.Fatal(err)
	}
	if alias != out {
		t.Errorf("route:list berbeda dari inspect routes:\n%s", alias)
	}
}

func TestInspectPermissions_AliasSameOutput(t *testing.T) {
	clearEnv(t)
	out, err := run(t, "inspect", "permissions")
	if err != nil {
		t.Fatal(err)
	}
	alias, err := run(t, "permission:list")
	if err != nil || alias != out {
		t.Fatalf("alias berbeda (err=%v):\n%s\nvs\n%s", err, alias, out)
	}
	for _, want := range []string{"acme.example", "example.ping.read", "example-addon.summary.read"} {
		if !strings.Contains(out, want) {
			t.Errorf("permissions tanpa %q:\n%s", want, out)
		}
	}
}

func TestInspectModules_JSON(t *testing.T) {
	clearEnv(t)
	out, err := run(t, "inspect", "modules", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var mods []map[string]any
	if err := json.Unmarshal([]byte(out), &mods); err != nil {
		t.Fatalf("bukan JSON valid: %v\n%s", err, out)
	}
	if len(mods) != 3 || mods[0]["key"] != "platform.core" {
		t.Errorf("modules = %v", mods)
	}
}

func TestInspectModules_Table(t *testing.T) {
	clearEnv(t)
	out, err := run(t, "inspect", "modules")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"KEY", "COMPATIBLE", "acme.example-addon", "platform.core"} {
		if !strings.Contains(out, want) {
			t.Errorf("tabel tanpa %q:\n%s", want, out)
		}
	}
}

func TestInspectRoutes_JSON(t *testing.T) {
	clearEnv(t)
	out, err := run(t, "inspect", "routes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var routes []map[string]any
	if err := json.Unmarshal([]byte(out), &routes); err != nil || len(routes) != 3 {
		t.Fatalf("err=%v routes=%v", err, routes)
	}
}

func TestList_ShowsInspectAliasesInReleaseFlavor(t *testing.T) {
	out, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"route:list", "permission:list"} {
		if !strings.Contains(out, want) {
			t.Errorf("list tanpa %q:\n%s", want, out)
		}
	}
}

// Q26 butir 10: --json mencetak [] (bukan null) untuk daftar kosong.
func TestInspectViews_EmptyListsAreJSONArrays(t *testing.T) {
	reg, err := registry.Build(registry.Options{PlatformVersion: buildinfo.PlatformVersion, SDKVersion: module.SDKVersion})
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{"routes": routeViews(reg), "permissions": permissionViews(reg), "modules-nonnil": moduleViews(reg)} {
		var sb strings.Builder
		if err := writeJSON(&sb, v); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sb.String(), "null") {
			t.Errorf("%s: JSON memuat null: %s", name, sb.String())
		}
	}
	var sb strings.Builder
	_ = writeJSON(&sb, routeViews(reg))
	if strings.TrimSpace(sb.String()) != "[]" {
		t.Errorf("routes kosong = %q, want []", sb.String())
	}
}
