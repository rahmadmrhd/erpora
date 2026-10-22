package archcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	repo := Repo{Root: abs, Manifests: map[string]Manifest{}}
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
	}
	return gm, nil
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
			Module string `json:"module"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, false, fmt.Errorf("module.json %s: %w", dir, err)
	}
	m := Manifest{Key: raw.Key, Slug: raw.Slug}
	for _, d := range raw.Dependencies {
		m.Dependencies = append(m.Dependencies, d.Module)
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
	ImportPath string
	Dir        string
	Imports    []string
	GoFiles    []string
	Module     *struct{ Path string }
}

// listPackages menjalankan `go list -json ./...` per varian build dan menggabungkan hasilnya
// (union impor dan GoFiles per import path).
func listPackages(dir, defaultModule string) ([]Package, error) {
	results := make([][]listedPackage, len(buildVariants))
	errs := make([]error, len(buildVariants))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12) // batasi proses go list paralel
	for i, v := range buildVariants {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			args := []string{"list", "-json=ImportPath,Dir,Imports,GoFiles,Module"}
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
			cur.GoFiles = unionStrings(cur.GoFiles, p.GoFiles)
		}
	}
	out := make([]Package, 0, len(order))
	for _, ip := range order {
		out = append(out, *byPath[ip])
	}
	return out, nil
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
