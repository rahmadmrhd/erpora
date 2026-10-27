package gen

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

// ModuleRef meringkas satu modul yang sudah ada (dari module.json dan go.mod).
type ModuleRef struct {
	Dir, Slug, Key, Version, GoPath string
	Strategy                        string // migrations.strategy: versioned | none
	HasWeb                          bool
	Capabilities                    []string
}

// ResolveModule mencari modules/<dir> berdasarkan Dir, slug, atau key. Tidak ditemukan -> error berisi daftar modul.
func ResolveModule(root, arg string) (ModuleRef, error) {
	refs, err := listModules(root)
	if err != nil {
		return ModuleRef{}, err
	}
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		if r.Dir == arg || r.Slug == arg || r.Key == arg {
			return r, nil
		}
		names = append(names, r.Dir)
	}
	return ModuleRef{}, fmt.Errorf("modul %q tidak ditemukan; modul yang ada: %s", arg, strings.Join(names, ", "))
}

func listModules(root string) ([]ModuleRef, error) {
	entries, err := os.ReadDir(filepath.Join(root, "modules"))
	if err != nil {
		return nil, fmt.Errorf("baca modules/: %w", err)
	}
	var out []ModuleRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, "modules", e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "module.json")) //nolint:gosec // path dari ReadDir repo
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var mf struct {
			Key, Slug, Version string
			Capabilities       []string
			Migrations         struct{ Strategy string }
		}
		if err := json.Unmarshal(raw, &mf); err != nil {
			return nil, fmt.Errorf("modules/%s/module.json: %w", e.Name(), err)
		}
		ref := ModuleRef{Dir: e.Name(), Slug: mf.Slug, Key: mf.Key, Version: mf.Version, Strategy: mf.Migrations.Strategy, Capabilities: mf.Capabilities}
		if gm, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil { //nolint:gosec // path dari ReadDir repo
			if f, err := modfile.Parse("go.mod", gm, nil); err == nil && f.Module != nil {
				ref.GoPath = f.Module.Mod.Path
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "web", "package.json")); err == nil {
			ref.HasWeb = true
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

var migrationFileRe = regexp.MustCompile(`^(\d{4})_`)

// NextMigrationNumber mengembalikan nomor migrasi berikutnya (maksimum prefix ^\d{4}_ ditambah 1; kosong -> 1).
func NextMigrationNumber(fsys fs.FS) (int, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, err
	}
	maxN := 0
	for _, e := range entries {
		if m := migrationFileRe.FindStringSubmatch(e.Name()); m != nil {
			n, _ := strconv.Atoi(m[1])
			maxN = max(maxN, n)
		}
	}
	return maxN + 1, nil
}
