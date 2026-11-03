package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoAPIDevActorInProduction memastikan aktor tetap `api:dev` (stub PRD-000) sudah pensiun: sejak PRD-001 C1.13 aktor
// aksi API selalu `user:<id>` dari Principal. Hanya kode produksi dan kontrak yang diperiksa (bukan *_test.go, testdata,
// docs, atau hasil build); sebutan historis di dokumen dan rencana dibiarkan.
func TestNoAPIDevActorInProduction(t *testing.T) {
	root := filepath.Join("..", "..")
	skipDir := map[string]bool{".git": true, "node_modules": true, "docs": true, "testdata": true, "dist": true, "bin": true, ".claude": true, ".worktrees": true}
	isTest := func(name string) bool {
		for _, s := range []string{"_test.go", ".test.ts", ".test.tsx"} {
			if strings.HasSuffix(name, s) {
				return true
			}
		}
		return false
	}
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".go", ".ts", ".tsx", ".yaml", ".yml", ".json", ".sql":
		default:
			return nil
		}
		if isTest(d.Name()) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "api:dev") {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Errorf("aktor `api:dev` sudah pensiun (PRD-001 C1.13); masih ada di kode produksi/kontrak: %v", hits)
	}
}
