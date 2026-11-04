package authz

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestImportsAreStdlibGormAndSDKLeaves: sdk/authz hanya mengimpor stdlib, gorm.io/gorm, dan paket daun sdk
// (identity); tidak ada platform, modul, atau pustaka pihak ketiga lain (D5, D9; D3b mengizinkan gorm di sini).
func TestImportsAreStdlibGormAndSDKLeaves(t *testing.T) {
	allowed := map[string]bool{
		"gorm.io/gorm":        true,
		"gorm.io/gorm/clause": true,
		"erpora/sdk/identity": true,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			first, _, _ := strings.Cut(path, "/")
			if !strings.Contains(first, ".") && first != "erpora" {
				continue // stdlib
			}
			if !allowed[path] {
				t.Errorf("%s mengimpor %q: hanya stdlib, gorm.io/gorm, dan erpora/sdk/identity yang diizinkan", e.Name(), path)
			}
		}
	}
}
