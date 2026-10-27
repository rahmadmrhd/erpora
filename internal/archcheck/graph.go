// Package archcheck memeriksa aturan batas arsitektur D1-D11 terhadap graf paket dan go.mod repo.
package archcheck

import "strings"

// Package adalah satu paket Go (hasil `go list -json`).
type Package struct {
	ImportPath  string
	Module      string // path Go module pemilik paket
	Dir         string
	Imports     []string
	TestImports []string // impor file _test.go (internal + eksternal); hanya dipakai aturan D2, D7, D8 (Q26)
	GoFiles     []string // file non-test, relatif terhadap Dir
	// FileImports: nama file non-test (relatif Dir) -> impor file itu; gabungan semua varian build.
	// Dipakai aturan DEVTOOLS yang berlaku per file (PRD-000 FR-22.1).
	FileImports map[string][]string
}

// GoMod adalah ringkasan satu go.mod.
type GoMod struct {
	Path, Dir, GoVersion, Toolchain string
	Requires                        []string        // semua require (langsung + indirect)
	Indirect                        map[string]bool // require bertanda "// indirect"
}

// Work adalah ringkasan go.work. Nil pada Repo.Work = pemeriksaan workspace dilewati.
type Work struct {
	Enabled   bool     // mode workspace aktif (GOWORK tidak "off"/kosong)
	GoVersion string   // direktif go di go.work
	Toolchain string   // direktif toolchain di go.work
	Use       []string // direktori use (absolut, bersih)
	ModDirs   []string // direktori go.mod hasil glob sdk/go.mod dan modules/*/go.mod (absolut, bersih)
}

// WebPackage adalah ringkasan modules/<name>/web/package.json.
type WebPackage struct {
	Name       string
	ModuleDeps []string // dependency @erpora/module-* (dependencies + peerDependencies), tanpa duplikat
}

// Manifest adalah ringkasan module.json.
type Manifest struct {
	Key, Slug    string
	Dependencies []string        // key modul dari dependencies[].module
	Optional     map[string]bool // key modul yang dependencies[].optional = true
}

// Repo adalah seluruh data yang diperiksa.
type Repo struct {
	Root      string
	Packages  []Package
	Mods      []GoMod
	Manifests map[string]Manifest // by module path
	Work      *Work
	Web       map[string]WebPackage // by module path; hanya modul yang punya web/package.json
}

// Violation adalah satu pelanggaran aturan.
type Violation struct{ Rule, Package, Detail string }

const (
	rootModule  = "erpora"
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
