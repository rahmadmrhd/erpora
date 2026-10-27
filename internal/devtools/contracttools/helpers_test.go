package contracttools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realRoot adalah root repo ERPora (dua tingkat di atas internal/devtools/contracttools).
const realRoot = "../../.."

// requireBun mengembalikan runner bun. Tanpa bun, test di-skip kecuali ERP_REQUIRE_BUN=1 (CI) yang membuatnya gagal.
func requireBun(t *testing.T) BunRunner {
	t.Helper()
	b, err := FindBun()
	if err != nil {
		if os.Getenv("ERP_REQUIRE_BUN") == "1" {
			t.Fatalf("ERP_REQUIRE_BUN=1 tetapi bun tidak ada: %v", err)
		}
		t.Skip("bun tidak ada; test dilewati")
	}
	return b
}

// writeFile menulis file (membuat direktori) di bawah root.
func writeFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// copyRepoSubset menyalin contracts/openapi dan modules/<dirs>/{module.json,openapi.yaml,web/package.json}
// dari repo nyata ke root baru.
func copyRepoSubset(t *testing.T, dst string, dirs ...string) {
	t.Helper()
	rels := []string{"contracts/openapi/common.yaml", "contracts/openapi/platform.yaml"}
	for _, d := range dirs {
		for _, f := range []string{"module.json", "openapi.yaml", "web/package.json"} {
			if _, err := os.Stat(filepath.Join(realRoot, "modules", d, filepath.FromSlash(f))); err == nil {
				rels = append(rels, "modules/"+d+"/"+f)
			}
		}
	}
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(realRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, dst, rel, data)
	}
}

// editFile mengubah file di root dengan serangkaian pasangan {lama, baru}; setiap lama harus ada tepat sekali.
func editFile(t *testing.T, root, rel string, edits ...[2]string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, e := range edits {
		if strings.Count(s, e[0]) != 1 {
			t.Fatalf("%s: teks %q muncul %d kali, harus tepat 1", rel, e[0], strings.Count(s, e[0]))
		}
		s = strings.Replace(s, e[0], e[1], 1)
	}
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}
