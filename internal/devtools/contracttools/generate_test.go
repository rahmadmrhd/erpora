package contracttools

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const genHeaderPrefix = "// Dibangkitkan oleh `erpora contracts:generate` dari "

func genFileMap(files []GenFile) map[string]string {
	m := map[string]string{}
	for _, f := range files {
		m[f.Path] = string(f.Content)
	}
	return m
}

func TestGenerate_Deterministic(t *testing.T) {
	bun := requireBun(t)
	repo, err := LoadRepo(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Generate(context.Background(), repo, "", bun)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(context.Background(), repo, "", bun)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("jumlah file %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) {
			t.Errorf("file %s tidak deterministik", a[i].Path)
		}
		if i > 0 && a[i-1].Path >= a[i].Path {
			t.Errorf("urutan tidak terurut: %s sebelum %s", a[i-1].Path, a[i].Path)
		}
	}
	m := genFileMap(a)
	for _, want := range []string{
		"packages/platform-contracts/src/common.gen.ts",
		"packages/platform-contracts/src/platform.gen.ts",
		"packages/module-sdk/src/manifest.gen.ts",
		"contracts/dist/openapi.json",
	} {
		if _, ok := m[want]; !ok {
			t.Errorf("file %s tidak dibangkitkan; ada: %v", want, keysOf(m))
		}
	}
	for p, c := range m {
		if strings.HasSuffix(p, ".gen.ts") && !strings.HasPrefix(c, genHeaderPrefix) {
			t.Errorf("%s tidak diawali header generate", p)
		}
	}
	dist := m["contracts/dist/openapi.json"]
	for _, want := range []string{`"x-erp-generated"`, "example.Note", "example-addon"} {
		if !strings.Contains(dist, want) {
			t.Errorf("contracts/dist/openapi.json tidak memuat %q", want)
		}
	}
	if !strings.Contains(m["packages/platform-contracts/src/platform.gen.ts"], "/api/platform/modules") {
		t.Error("platform.gen.ts tidak memuat path platform")
	}
	if !strings.Contains(m["packages/module-sdk/src/manifest.gen.ts"], "ModuleManifest") {
		t.Error("manifest.gen.ts tidak memuat tipe ModuleManifest")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// minimalRepo menyalin kontrak dan skema manifest ke root sementara.
func minimalRepo(t *testing.T, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	copyRepoSubset(t, root, dirs...)
	data, err := os.ReadFile(filepath.Join(realRoot, "contracts", "module.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "contracts/module.schema.json", data)
	return root
}

// AC FR-23: mengubah kontrak tanpa contracts:generate harus terdeteksi.
func TestCheckGenerated_DetectsStale(t *testing.T) {
	bun := requireBun(t)
	root := minimalRepo(t, "example", "exampleaddon")
	load := func() *Repo {
		repo, err := LoadRepo(root)
		if err != nil {
			t.Fatal(err)
		}
		repo.ToolsRoot = realRoot
		return repo
	}
	files, err := Generate(context.Background(), load(), "", bun)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteGenerated(root, files); err != nil {
		t.Fatal(err)
	}
	stale, err := CheckGenerated(root, files)
	if err != nil || len(stale) != 0 {
		t.Fatalf("setelah menulis: stale = %v err = %v, want kosong", stale, err)
	}

	editFile(t, root, "contracts/openapi/platform.yaml", [2]string{"summary: Daftar modul beserta status", "summary: Daftar modul beserta status (diubah)"})
	files2, err := Generate(context.Background(), load(), "", bun)
	if err != nil {
		t.Fatal(err)
	}
	stale, err = CheckGenerated(root, files2)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0] != "packages/platform-contracts/src/platform.gen.ts" {
		t.Fatalf("stale = %v, want [packages/platform-contracts/src/platform.gen.ts]", stale)
	}
	if err := WriteGenerated(root, files2); err != nil {
		t.Fatal(err)
	}
	if stale, _ = CheckGenerated(root, files2); len(stale) != 0 {
		t.Errorf("setelah generate ulang: stale = %v", stale)
	}

	// File *.gen.ts yatim (tanpa sumber) ikut dilaporkan.
	writeFile(t, root, "modules/example/web/api/contract.gen.ts", []byte("// yatim\n"))
	stale, err = CheckGenerated(root, files2)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0] != "modules/example/web/api/contract.gen.ts" {
		t.Errorf("yatim: stale = %v", stale)
	}
}

func TestGenerate_ModuleWithWeb(t *testing.T) {
	bun := requireBun(t)
	root := minimalRepo(t, "example", "exampleaddon")
	writeFile(t, root, "modules/example/web/package.json", []byte(`{"name":"@erpora/module-example"}`))
	repo, err := LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	repo.ToolsRoot = realRoot
	files, err := Generate(context.Background(), repo, "", bun)
	if err != nil {
		t.Fatal(err)
	}
	m := genFileMap(files)
	c, ok := m["modules/example/web/api/contract.gen.ts"]
	if !ok {
		t.Fatalf("contract.gen.ts modul tidak dibangkitkan: %v", keysOf(m))
	}
	for _, want := range []string{"createClient", "/api/example/notes", "export const client"} {
		if !strings.Contains(c, want) {
			t.Errorf("contract.gen.ts tidak memuat %q", want)
		}
	}
	if _, ok := m["modules/exampleaddon/web/api/contract.gen.ts"]; ok {
		t.Error("modul tanpa web/ tidak boleh dibangkitkan")
	}
}
