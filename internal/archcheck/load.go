package archcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
)

// Load membaca workspace di root: paket (go list -json ./... per modul), go.mod, dan module.json.
func Load(root string) (Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Repo{}, err
	}
	repo := Repo{Root: abs, Manifests: map[string]Manifest{}, Web: map[string]WebPackage{}}
	work, err := loadWork(abs)
	if err != nil {
		return repo, err
	}
	repo.Work = work
	out, err := run(abs, "go", "list", "-m", "-f", "{{.Dir}}")
	if err != nil {
		return repo, err
	}
	// Satu direktori per baris: path boleh memuat spasi (mis. "C:\Users\Nama Pengguna\...").
	for _, dir := range strings.Split(string(out), "\n") {
		dir = strings.TrimSpace(strings.TrimSuffix(dir, "\r"))
		if dir == "" {
			continue
		}
		gm, err := loadGoMod(dir)
		if err != nil {
			return repo, err
		}
		repo.Mods = append(repo.Mods, gm)
		mf, ok, err := loadManifest(dir)
		if err != nil {
			return repo, err
		}
		if ok {
			repo.Manifests[gm.Path] = mf
		}
		// Hanya modul bisnis (modules/*) yang punya modules/<x>/web; web/ di root adalah app shell (F3).
		if strings.HasPrefix(gm.Path, modulesBase) {
			wp, ok, err := loadWebPackage(dir)
			if err != nil {
				return repo, err
			}
			if ok {
				repo.Web[gm.Path] = wp
			}
		}
		pkgs, err := listPackages(dir, gm.Path)
		if err != nil {
			return repo, err
		}
		repo.Packages = append(repo.Packages, pkgs...)
	}
	return repo, nil
}

func run(dir, name string, args ...string) ([]byte, error) { return runEnv(dir, nil, name, args...) }

func runEnv(dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // dipanggil hanya dengan "go" dan argumen tetap
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s (dir %s): %w\n%s", name, strings.Join(args, " "), dir, err, stderr.String())
	}
	return out, nil
}

func loadGoMod(dir string) (GoMod, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod")) //nolint:gosec // dir berasal dari `go list -m`
	if err != nil {
		return GoMod{}, err
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return GoMod{}, err
	}
	gm := GoMod{Dir: dir}
	if f.Module != nil {
		gm.Path = f.Module.Mod.Path
	}
	if f.Go != nil {
		gm.GoVersion = f.Go.Version
	}
	if f.Toolchain != nil {
		gm.Toolchain = f.Toolchain.Name
	}
	for _, r := range f.Require {
		gm.Requires = append(gm.Requires, r.Mod.Path)
		if r.Indirect {
			if gm.Indirect == nil {
				gm.Indirect = map[string]bool{}
			}
			gm.Indirect[r.Mod.Path] = true
		}
	}
	return gm, nil
}

// loadWork membaca go.work aktif (go env GOWORK) dan menemukan semua go.mod repo lewat glob
// sdk/go.mod dan modules/*/go.mod. Mode workspace nonaktif ditandai Enabled=false (dilaporkan oleh Check).
func loadWork(root string) (*Work, error) {
	w := &Work{}
	out, err := run(root, "go", "env", "GOWORK")
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(out))
	if path != "" && path != "off" {
		data, err := os.ReadFile(path) //nolint:gosec // path dari `go env GOWORK`
		if err != nil {
			return nil, err
		}
		wf, err := modfile.ParseWork("go.work", data, nil)
		if err != nil {
			return nil, err
		}
		w.Enabled = true
		if wf.Go != nil {
			w.GoVersion = wf.Go.Version
		}
		if wf.Toolchain != nil {
			w.Toolchain = wf.Toolchain.Name
		}
		base := filepath.Dir(path)
		for _, u := range wf.Use {
			d := u.Path
			if !filepath.IsAbs(d) {
				d = filepath.Join(base, d)
			}
			w.Use = append(w.Use, filepath.Clean(d))
		}
	}
	for _, pattern := range []string{filepath.Join(root, "sdk", "go.mod"), filepath.Join(root, "modules", "*", "go.mod")} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			w.ModDirs = append(w.ModDirs, filepath.Clean(filepath.Dir(m)))
		}
	}
	return w, nil
}

// loadWebPackage membaca web/package.json modul (bila ada) dan mengumpulkan dependency @erpora/module-*.
func loadWebPackage(dir string) (WebPackage, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "web", "package.json")) //nolint:gosec // dir berasal dari `go list -m`
	if os.IsNotExist(err) {
		return WebPackage{}, false, nil
	}
	if err != nil {
		return WebPackage{}, false, err
	}
	return parseWebPackage(data)
}

// platformModuleSDK adalah paket SDK frontend platform; berawalan @erpora/module- tetapi bukan modul.
const platformModuleSDK = "@erpora/module-sdk"

func parseWebPackage(data []byte) (WebPackage, bool, error) {
	var raw struct {
		Name             string            `json:"name"`
		Dependencies     map[string]string `json:"dependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return WebPackage{}, false, fmt.Errorf("web/package.json: %w", err)
	}
	wp := WebPackage{Name: raw.Name}
	seen := map[string]bool{}
	for _, deps := range []map[string]string{raw.Dependencies, raw.PeerDependencies} {
		for name := range deps {
			// @erpora/module-sdk adalah paket platform yang BOLEH diimpor frontend modul (F2), bukan frontend modul.
			if strings.HasPrefix(name, "@erpora/module-") && name != platformModuleSDK && !seen[name] {
				seen[name] = true
				wp.ModuleDeps = append(wp.ModuleDeps, name)
			}
		}
	}
	sort.Strings(wp.ModuleDeps)
	return wp, true, nil
}

func loadManifest(dir string) (Manifest, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "module.json")) //nolint:gosec // dir berasal dari `go list -m`
	if os.IsNotExist(err) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	var raw struct {
		Key          string `json:"key"`
		Slug         string `json:"slug"`
		Dependencies []struct {
			Module   string `json:"module"`
			Optional bool   `json:"optional"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, false, fmt.Errorf("module.json %s: %w", dir, err)
	}
	m := Manifest{Key: raw.Key, Slug: raw.Slug}
	for _, d := range raw.Dependencies {
		m.Dependencies = append(m.Dependencies, d.Module)
		if d.Optional {
			if m.Optional == nil {
				m.Optional = map[string]bool{}
			}
			m.Optional[d.Module] = true
		}
	}
	return m, true, nil
}

// buildVariant adalah satu kombinasi build untuk `go list`. Aturan arsitektur harus berlaku untuk
// SEMUA flavor (devtools, integration) dan file khusus-OS, bukan hanya build default host.
type buildVariant struct {
	goos string // "" = default host
	tags string
}

var buildVariants = makeBuildVariants()

// makeBuildVariants: kombinasi tag {none, devtools, integration, devtools+integration} x GOOS
// {linux, windows, darwin}, agar file dengan constraint saling-eksklusif (mis. `devtools && !integration`,
// `windows && !devtools`) pun terbaca.
func makeBuildVariants() []buildVariant {
	var out []buildVariant
	for _, goos := range []string{"linux", "windows", "darwin"} {
		for _, tags := range []string{"", "devtools", "integration", "devtools,integration"} {
			out = append(out, buildVariant{goos: goos, tags: tags})
		}
	}
	return out
}

type listedPackage struct {
	ImportPath   string
	Dir          string
	Imports      []string
	TestImports  []string
	XTestImports []string
	GoFiles      []string
	Module       *struct{ Path string }
}

// listPackages menjalankan `go list -json ./...` per varian build dan menggabungkan hasilnya
// (union impor dan GoFiles per import path).
func listPackages(dir, defaultModule string) ([]Package, error) {
	results := make([][]listedPackage, len(buildVariants))
	errs := make([]error, len(buildVariants))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12) // batasi jumlah proses go list yang berjalan bersamaan
	for i, v := range buildVariants {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			args := []string{"list", "-json=ImportPath,Dir,Imports,TestImports,XTestImports,GoFiles,Module"}
			if v.tags != "" {
				args = append(args, "-tags", v.tags)
			}
			args = append(args, "./...")
			var env []string
			if v.goos != "" {
				env = []string{"GOOS=" + v.goos}
			}
			out, err := runEnv(dir, env, "go", args...)
			if err != nil {
				errs[i] = err
				return
			}
			dec := json.NewDecoder(bytes.NewReader(out))
			for {
				var p listedPackage
				if err := dec.Decode(&p); err == io.EOF {
					return
				} else if err != nil {
					errs[i] = fmt.Errorf("parse go list %s: %w", dir, err)
					return
				}
				results[i] = append(results[i], p)
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	byPath := map[string]*Package{}
	var order []string
	for _, res := range results {
		for _, p := range res {
			cur, ok := byPath[p.ImportPath]
			if !ok {
				mod := defaultModule
				if p.Module != nil {
					mod = p.Module.Path
				}
				cur = &Package{ImportPath: p.ImportPath, Module: mod, Dir: p.Dir}
				byPath[p.ImportPath] = cur
				order = append(order, p.ImportPath)
			}
			cur.Imports = unionStrings(cur.Imports, p.Imports)
			cur.TestImports = unionStrings(cur.TestImports, unionStrings(append([]string{}, p.TestImports...), p.XTestImports))
			cur.GoFiles = unionStrings(cur.GoFiles, p.GoFiles)
		}
	}
	out := make([]Package, 0, len(order))
	for _, ip := range order {
		p := byPath[ip]
		p.FileImports = fileImports(p.Dir, p.GoFiles)
		out = append(out, *p)
	}
	return out, nil
}

// fileImports membaca impor setiap file (parser.ImportsOnly); file yang tidak terbaca dilewati.
func fileImports(dir string, files []string) map[string][]string {
	out := map[string][]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, f), nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range file.Imports {
			if p, err := strconv.Unquote(imp.Path.Value); err == nil {
				out[f] = append(out[f], p)
			}
		}
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		if !seen[s] {
			a = append(a, s)
			seen[s] = true
		}
	}
	return a
}
