// Package archcheck memeriksa aturan batas arsitektur D1-D11 terhadap graf paket dan go.mod repo.
package archcheck

import "strings"

// Package adalah satu paket Go (hasil `go list -json`).
type Package struct {
	ImportPath string
	Module     string // path Go module pemilik paket
	Dir        string
	Imports    []string
	GoFiles    []string // file non-test, relatif terhadap Dir
}

// GoMod adalah ringkasan satu go.mod.
type GoMod struct {
	Path, Dir, GoVersion, Toolchain string
	Requires                        []string
}

// Manifest adalah ringkasan module.json.
type Manifest struct {
	Key, Slug    string
	Dependencies []string // key modul dari dependencies[].module
}

// Repo adalah seluruh data yang diperiksa.
type Repo struct {
	Root      string
	Packages  []Package
	Mods      []GoMod
	Manifests map[string]Manifest // by module path
}

// Violation adalah satu pelanggaran aturan.
type Violation struct{ Rule, Package, Detail string }

const (
	rootModule  = "github.com/rahmadmrhd/erp-go"
	modulesBase = rootModule + "/modules/"
	sdkModule   = rootModule + "/sdk"
)

// underPath: p sama dengan base atau berada di bawahnya (batas segmen).
func underPath(p, base string) bool {
	return p == base || strings.HasPrefix(p, base+"/")
}

// hasSeg: path memuat segmen seg (mis. "internal/domain") sebagai rangkaian segmen utuh.
func hasSeg(p, seg string) bool {
	return strings.HasSuffix(p, "/"+seg) || strings.Contains(p, "/"+seg+"/") || p == seg || strings.HasPrefix(p, seg+"/")
}

// rel mengembalikan path relatif terhadap root module.
func rel(importPath string) (string, bool) {
	if importPath == rootModule {
		return "", true
	}
	if strings.HasPrefix(importPath, rootModule+"/") {
		return importPath[len(rootModule)+1:], true
	}
	return "", false
}
