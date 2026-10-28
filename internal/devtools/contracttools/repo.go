// Package contracttools berisi alat kontrak API untuk developer tools: lint (Spectral lewat Bun), generate tipe TS,
// dan deteksi breaking change (oasdiff).
package contracttools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"erpora/internal/platform/apicontract"
)

// ModuleInfo meringkas satu modules/<dir>.
type ModuleInfo struct {
	Dir, Key, Slug, Version string
	Dependencies            []string // key modul, tanpa platform.core
	Capabilities            []string
	HasWeb                  bool // modules/<dir>/web/package.json ada
	HasContract             bool // modules/<dir>/openapi.yaml ada
}

// Repo adalah kontrak dan modul sebuah repo.
type Repo struct {
	Root string
	// ToolsRoot adalah repo tempat web/contracts-lint.ts dan contracts/spectral.yaml berada; sama dengan Root
	// kecuali pada fixture test.
	ToolsRoot string
	Set       *apicontract.Set
	Modules   []ModuleInfo
	// Files memetakan nama dokumen ("common", "platform", slug) ke path relatif Root (slash).
	Files map[string]string
}

// LoadRepo membaca contracts/openapi/{common,platform}.yaml, modules/*/module.json, dan modules/*/openapi.yaml.
// Modul tanpa openapi.yaml tetap tercatat; Set tidak memuatnya. Urutan modul: Dir.
func LoadRepo(root string) (*Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r := &Repo{Root: abs, ToolsRoot: abs, Files: map[string]string{}}
	var docs []*apicontract.Doc
	for _, name := range []string{"common", "platform"} {
		rel := "contracts/openapi/" + name + ".yaml"
		d, err := parseFile(abs, name, rel)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
		r.Files[name] = rel
	}
	entries, err := os.ReadDir(filepath.Join(abs, "modules"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	keyToSlug := map[string]string{}
	type pending struct {
		info ModuleInfo
		deps []string
	}
	var mods []pending
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(abs, "modules", e.Name(), "module.json")) //nolint:gosec // path dari ReadDir repo
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var mf struct {
			Key, Slug, Version string
			Capabilities       []string
			Dependencies       []struct{ Module string }
		}
		if err := json.Unmarshal(raw, &mf); err != nil {
			return nil, fmt.Errorf("modules/%s/module.json: %w", e.Name(), err)
		}
		info := ModuleInfo{Dir: e.Name(), Key: mf.Key, Slug: mf.Slug, Version: mf.Version, Capabilities: mf.Capabilities}
		for _, d := range mf.Dependencies {
			if d.Module != "platform.core" {
				info.Dependencies = append(info.Dependencies, d.Module)
			}
		}
		if _, err := os.Stat(filepath.Join(abs, "modules", e.Name(), "web", "package.json")); err == nil {
			info.HasWeb = true
		}
		keyToSlug[mf.Key] = mf.Slug
		mods = append(mods, pending{info: info})
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].info.Dir < mods[j].info.Dir })
	for _, m := range mods {
		rel := "modules/" + m.info.Dir + "/openapi.yaml"
		if _, err := os.Stat(filepath.Join(abs, filepath.FromSlash(rel))); err == nil {
			d, err := parseFile(abs, m.info.Slug, rel)
			if err != nil {
				return nil, err
			}
			docs = append(docs, d)
			r.Files[m.info.Slug] = rel
			m.info.HasContract = true
		}
		r.Modules = append(r.Modules, m.info)
	}
	set, err := apicontract.NewSet(docs...)
	if err != nil {
		return nil, err
	}
	r.Set = set
	return r, nil
}

func parseFile(root, name, rel string) (*apicontract.Doc, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // path dari repo
	if err != nil {
		return nil, err
	}
	return apicontract.Parse(name, data)
}

// Module mencari modul berdasarkan Dir, Slug, atau Key.
func (r *Repo) Module(arg string) (ModuleInfo, bool) {
	for _, m := range r.Modules {
		if m.Dir == arg || m.Slug == arg || m.Key == arg {
			return m, true
		}
	}
	return ModuleInfo{}, false
}

// slugOfKey memetakan key modul ke slug-nya ("" bila tidak dikenal).
func (r *Repo) slugOfKey(key string) string {
	for _, m := range r.Modules {
		if m.Key == key {
			return m.Slug
		}
	}
	return ""
}

// ModuleNames mendaftar Dir modul untuk pesan error.
func (r *Repo) ModuleNames() string {
	names := make([]string, len(r.Modules))
	for i, m := range r.Modules {
		names[i] = m.Dir
	}
	return strings.Join(names, ", ")
}
