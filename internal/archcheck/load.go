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
	for _, dir := range strings.Fields(string(out)) {
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
		pk, err := run(dir, "go", "list", "-json", "./...")
		if err != nil {
			return repo, err
		}
		dec := json.NewDecoder(bytes.NewReader(pk))
		for {
			var p struct {
				ImportPath string
				Dir        string
				Imports    []string
				GoFiles    []string
				Module     *struct{ Path string }
			}
			if err := dec.Decode(&p); err == io.EOF {
				break
			} else if err != nil {
				return repo, fmt.Errorf("parse go list %s: %w", dir, err)
			}
			mod := gm.Path
			if p.Module != nil {
				mod = p.Module.Path
			}
			repo.Packages = append(repo.Packages, Package{
				ImportPath: p.ImportPath, Module: mod, Dir: p.Dir, Imports: p.Imports, GoFiles: p.GoFiles,
			})
		}
	}
	return repo, nil
}

func run(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // dipanggil hanya dengan "go" dan argumen tetap
	cmd.Dir = dir
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
