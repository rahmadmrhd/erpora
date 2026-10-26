package archcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// Versi go/toolchain wajib seluruh repo (CLAUDE.md): semua go.mod dan go.work.
const (
	requiredGo        = "1.27"
	requiredToolchain = "go1.27.3"
)

var routerPrefixes = []string{
	"github.com/go-chi/", "github.com/gorilla/mux", "github.com/gin-gonic/",
	"github.com/labstack/echo", "github.com/gofiber/", "github.com/julienschmidt/httprouter",
	"github.com/uptrace/bunrouter", "github.com/dimfeld/httptreemux", "goji.io",
	"github.com/go-zoo/bone", "github.com/bmizerany/pat", "github.com/beego/beego",
	"github.com/go-martini/martini",
}

// isDBOpenImport: impor driver/pool database (seluruh pgx termasuk pgconn, lib/pq, driver GORM);
// hanya internal/platform/database yang boleh membuka koneksi (FR-21).
func isDBOpenImport(imp string) bool {
	return strings.HasPrefix(imp, "github.com/jackc/pgx/") || strings.HasPrefix(imp, "github.com/lib/pq") ||
		strings.HasPrefix(imp, "gorm.io/driver/")
}

// isSQLImport: database/sql (dan subpaketnya) memungkinkan sql.Open; allowlist eksplisit di bawah.
func isSQLImport(imp string) bool {
	return imp == "database/sql" || strings.HasPrefix(imp, "database/sql/")
}

func isInfraOrPresentation(imp string) bool {
	return hasSeg(imp, "infrastructure") || hasSeg(imp, "presentation")
}

// Check menjalankan semua aturan dan mengembalikan pelanggaran (terurut stabil).
func Check(r Repo) []Violation {
	var v []Violation
	add := func(rule, pkg, detail string) { v = append(v, Violation{rule, pkg, detail}) }

	modPaths := map[string]bool{}
	for _, m := range r.Mods {
		modPaths[m.Path] = true
	}
	for p := range r.Manifests {
		modPaths[p] = true
	}
	// owningModule: modul modules/* terpanjang yang memuat import path.
	owningModule := func(imp string) string {
		best := ""
		for p := range modPaths {
			if strings.HasPrefix(p, modulesBase) && underPath(imp, p) && len(p) > len(best) {
				best = p
			}
		}
		return best
	}
	allowedByManifest := func(x, y string) bool {
		my, ok := r.Manifests[y]
		if !ok {
			return false
		}
		for _, d := range r.Manifests[x].Dependencies {
			if d == my.Key {
				return true
			}
		}
		return false
	}

	for _, p := range r.Packages {
		rp, inRepo := rel(p.ImportPath)
		isPlatform := inRepo && strings.HasPrefix(rp, "internal/platform/")
		isModulePkg := inRepo && strings.HasPrefix(rp, "modules/")
		isSDK := inRepo && underPath(rp, "sdk")
		isApp := inRepo && underPath(rp, "internal/app")
		isArch := inRepo && underPath(rp, "internal/archcheck")
		isDomain := hasSeg(p.ImportPath, "internal/domain")
		isApplication := hasSeg(p.ImportPath, "internal/application")
		gormOK := isArch ||
			(isModulePkg && hasSeg(rp, "internal/infrastructure")) ||
			underPath(rp, "internal/platform/database") ||
			underPath(rp, "internal/platform/modules/store") ||
			underPath(rp, "sdk/data")
		dbOK := isArch || underPath(rp, "internal/platform/database")
		sqlOK := dbOK || underPath(rp, "internal/platform/migrate")

		// D2, D7, D8 juga berlaku untuk impor file test (Q26 butir 1); aturan lain hanya non-test.
		for _, imp := range unionStrings(append([]string{}, p.Imports...), p.TestImports) {
			if isPlatform && strings.HasPrefix(imp, modulesBase) {
				add("D2", p.ImportPath, "internal/platform tidak boleh mengimpor modul: "+imp)
			}
			if inRepo && strings.HasPrefix(imp, modulesBase) && !isPlatform && !isModulePkg && !isApp {
				add("D7", p.ImportPath, "hanya internal/app yang boleh mengimpor modul: "+imp)
			}
			if isModulePkg {
				if y := owningModule(imp); y != "" && p.Module != "" && y != p.Module && !allowedByManifest(p.Module, y) {
					add("D8", p.ImportPath, "impor modul "+y+" yang tidak ada di dependencies manifest: "+imp)
				}
			}
		}

		for _, imp := range p.Imports {
			_, impInRepo := rel(imp)
			if isDomain {
				if imp == "net/http" || strings.HasPrefix(imp, "net/http/") ||
					strings.HasPrefix(imp, "gorm.io/") || strings.HasPrefix(imp, "github.com/jackc/pgx") ||
					imp == "database/sql" || strings.HasPrefix(imp, "database/sql/") ||
					(impInRepo && isInfraOrPresentation(imp)) {
					add("D3", p.ImportPath, "domain tidak boleh mengimpor "+imp)
				}
			}
			if strings.HasPrefix(imp, "gorm.io/") && !gormOK {
				add("D3b", p.ImportPath, "impor gorm.io hanya boleh di infrastructure/database/store/sdk-data: "+imp)
			}
			if isApplication && impInRepo && isInfraOrPresentation(imp) {
				add("D4", p.ImportPath, "application tidak boleh mengimpor "+imp)
			}
			if isModulePkg {
				// D1: antarmodul hanya lewat contract/.
				if y := owningModule(imp); y != "" && p.Module != "" && y != p.Module && !underPath(imp, y+"/contract") {
					add("D1", p.ImportPath, "modul hanya boleh mengimpor "+y+"/contract, bukan "+imp)
				}
				// D3b (Q26 butir 2): sdk/data hanya dari internal/infrastructure.
				if underPath(imp, sdkModule+"/data") && !hasSeg(rp, "internal/infrastructure") {
					add("D3b", p.ImportPath, "erpora/sdk/data hanya boleh diimpor dari internal/infrastructure modul: "+imp)
				}
			}
			if isModulePkg || isSDK {
				for _, rp2 := range routerPrefixes {
					if strings.HasPrefix(imp, rp2) {
						add("D10", p.ImportPath, "router pihak ketiga dilarang (pakai net/http.ServeMux): "+imp)
						break
					}
				}
			}
			// database/sql hanya di internal/platform/database dan internal/platform/migrate (menerima *sql.DB
			// untuk goose tanpa membuka koneksi); modul mengakses DB lewat sdk/data.
			if isSQLImport(imp) && !sqlOK {
				add("DB-OPEN", p.ImportPath, "database/sql hanya boleh di internal/platform/database dan internal/platform/migrate: "+imp)
			}
			if isDBOpenImport(imp) && !dbOK {
				add("DB-OPEN", p.ImportPath, "hanya internal/platform/database yang boleh membuka koneksi: "+imp)
			}
		}

		for _, f := range p.GoFiles {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, f), nil, 0)
			if err != nil {
				continue
			}
			found := false
			ast.Inspect(file, func(n ast.Node) bool {
				// Setiap rujukan selector (termasuk method value m := db.AutoMigrate), bukan hanya call.
				if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "AutoMigrate" {
					found = true
				}
				return !found
			})
			if found {
				add("AUTOMIGRATE", p.ImportPath, "AutoMigrate dilarang di kode non-test: "+f)
			}
		}
	}

	// Aturan level go.mod.
	vers, tcs := map[string][]string{}, map[string][]string{}
	for _, m := range r.Mods {
		if m.Path == sdkModule {
			for _, q := range m.Requires {
				if q != m.Path && underPath(q, rootModule) {
					add("D5", m.Path, "sdk tidak boleh me-require modul repo lain: "+q)
				}
			}
		}
		if strings.HasPrefix(m.Path, modulesBase) {
			for _, q := range m.Requires {
				if q == rootModule {
					add("D9", m.Path, "modul tidak boleh me-require root module")
				}
				// Q26 butir 8: require indirect diabaikan; require langsung wajib ada di manifest.
				if strings.HasPrefix(q, modulesBase) && q != m.Path && !m.Indirect[q] && !allowedByManifest(m.Path, q) {
					add("D8", m.Path, "go.mod me-require modul "+q+" yang tidak ada di dependencies manifest")
				}
			}
			// Arah sebaliknya: dependency manifest ke modul repo harus ada di require go.mod.
			for _, dep := range r.Manifests[m.Path].Dependencies {
				if strings.HasPrefix(dep, "platform.") {
					continue
				}
				for y, my := range r.Manifests {
					if my.Key == dep && y != m.Path && !contains(m.Requires, y) {
						add("D8", m.Path, "manifest bergantung pada "+dep+" ("+y+") tetapi go.mod tidak me-require-nya")
					}
				}
			}
		}
		if m.GoVersion != requiredGo {
			add("GOVERSION", m.Path, "direktif go harus '"+requiredGo+"', ditemukan '"+m.GoVersion+"'")
		}
		if m.Toolchain != requiredToolchain {
			add("GOVERSION", m.Path, "direktif toolchain harus '"+requiredToolchain+"', ditemukan '"+m.Toolchain+"'")
		}
		vers[m.GoVersion] = append(vers[m.GoVersion], m.Path)
		tcs[m.Toolchain] = append(tcs[m.Toolchain], m.Path)
	}
	if len(vers) > 1 {
		add("GOVERSION", "", "versi go tidak seragam: "+describe(vers))
	}
	if len(tcs) > 1 {
		add("GOVERSION", "", "toolchain tidak seragam: "+describe(tcs))
	}

	// go.work: versi + kelengkapan use.
	if w := r.Work; w != nil {
		if !w.Enabled {
			add("GOWORK", "", "mode workspace tidak aktif (GOWORK=off atau go.work tidak ditemukan): aturan arsitektur hanya bermakna pada workspace penuh")
		} else {
			if w.GoVersion != requiredGo {
				add("GOVERSION", "go.work", "direktif go harus '"+requiredGo+"', ditemukan '"+w.GoVersion+"'")
			}
			if w.Toolchain != requiredToolchain {
				add("GOVERSION", "go.work", "direktif toolchain harus '"+requiredToolchain+"', ditemukan '"+w.Toolchain+"'")
			}
			for _, d := range w.ModDirs {
				if !contains(w.Use, d) {
					add("GOWORK", d, "go.mod tidak tercantum di use go.work")
				}
			}
		}
	}

	// F2: dependencies package.json frontend modul == dependencies manifest (key -> @erpora/module-<nama dir>).
	for y, wp := range r.Web {
		want := map[string]bool{}
		for _, dep := range r.Manifests[y].Dependencies {
			if strings.HasPrefix(dep, "platform.") {
				continue
			}
			for my, m := range r.Manifests {
				if m.Key == dep {
					want[webPackageName(my)] = true
				}
			}
		}
		have := map[string]bool{}
		for _, d := range wp.ModuleDeps {
			have[d] = true
		}
		for d := range have {
			if !want[d] && d != webPackageName(y) {
				add("F2", y, "web/package.json bergantung pada "+d+" yang tidak ada di dependencies manifest")
			}
		}
		for d := range want {
			if !have[d] {
				add("F2", y, "manifest bergantung pada modul tetapi web/package.json tidak memuat "+d)
			}
		}
	}

	// D11: siklus pada graf Requires antar modul repo.
	graph := map[string][]string{}
	for _, m := range r.Mods {
		for _, q := range m.Requires {
			if modPaths[q] && q != m.Path {
				graph[m.Path] = append(graph[m.Path], q)
			}
		}
	}
	for _, cyc := range findCycles(r.Mods, graph) {
		add("D11", cyc[0], "siklus require antar modul: "+strings.Join(cyc, " -> "))
	}

	sort.SliceStable(v, func(i, j int) bool {
		if v[i].Rule != v[j].Rule {
			return v[i].Rule < v[j].Rule
		}
		if v[i].Package != v[j].Package {
			return v[i].Package < v[j].Package
		}
		return v[i].Detail < v[j].Detail
	})
	return v
}

// webPackageName memetakan path Go module modul repo ke nama package frontend (@erpora/module-<nama dir>).
func webPackageName(modPath string) string {
	return "@erpora/module-" + modPath[strings.LastIndex(modPath, "/")+1:]
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func describe(m map[string][]string) string {
	var parts []string
	for k, mods := range m {
		sort.Strings(mods)
		parts = append(parts, "'"+k+"' ("+strings.Join(mods, ", ")+")")
	}
	sort.Strings(parts)
	return strings.Join(parts, " vs ")
}

// findCycles mengembalikan siklus yang ditemukan lewat DFS (satu per back-edge).
func findCycles(mods []GoMod, g map[string][]string) [][]string {
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var out [][]string
	var visit func(n string)
	visit = func(n string) {
		color[n] = grey
		stack = append(stack, n)
		for _, m := range g[n] {
			switch color[m] {
			case white:
				visit(m)
			case grey:
				i := len(stack) - 1
				for stack[i] != m {
					i--
				}
				cyc := append([]string{}, stack[i:]...)
				out = append(out, append(cyc, m))
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
	}
	paths := make([]string, 0, len(mods))
	for _, m := range mods {
		paths = append(paths, m.Path)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if color[p] == white {
			visit(p)
		}
	}
	return out
}
