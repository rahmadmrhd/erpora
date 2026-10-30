// Package modcheck memeriksa modul tanpa menjalankan seluruh gate: manifest (schema + aturan), aturan arsitektur
// D8 dan GOVERSION, serta awalan capability (PRD-000 FR-22.2 `module:check`).
package modcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"erpora/internal/archcheck"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/modules/manifest"
	"erpora/sdk/module"
)

// Rule yang dilaporkan.
const (
	RuleSchema           = "SCHEMA"
	RuleD8               = "D8"
	RuleCapabilityPrefix = "CAPABILITY_PREFIX"
	RuleGoVersion        = "GOVERSION"
	RuleI18n             = "I18N"
)

// Finding adalah satu temuan pada satu modul.
type Finding struct{ Module, Rule, Detail string }

type entry struct {
	dir      string // nama direktori di modules/
	manifest module.Manifest
	parseErr error
}

// match: arg cocok dengan nama direktori, slug, atau key modul.
func (e entry) match(arg string) bool {
	return e.dir == arg || e.manifest.Slug == arg || e.manifest.Key == arg
}

// Check memeriksa setiap modules/*/module.json di root (atau satu modul bila only != "": dicocokkan terhadap
// direktori, slug, atau key). Manifest divalidasi lewat manifest.Validate; archcheck dijalankan untuk modul itu
// dan hanya temuan D8 dan GOVERSION yang dilaporkan; setiap capability harus berawalan "<slug>.".
// only yang tidak dikenal menghasilkan error. Hasil terurut stabil (modul, rule, detail).
func Check(root, only string) ([]Finding, error) {
	files, err := filepath.Glob(filepath.Join(root, "modules", "*", "module.json"))
	if err != nil {
		return nil, err
	}
	var entries []entry
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // path dari glob di root repo
		if err != nil {
			return nil, err
		}
		m, perr := module.ParseManifest(data)
		entries = append(entries, entry{dir: filepath.Base(filepath.Dir(f)), manifest: m, parseErr: perr})
	}
	if only != "" {
		var picked []entry
		for _, e := range entries {
			if e.match(only) {
				picked = append(picked, e)
			}
		}
		if len(picked) == 0 {
			var names []string
			for _, e := range entries {
				names = append(names, e.dir)
			}
			return nil, fmt.Errorf("modul %q tidak dikenal; modul yang ada: %s", only, strings.Join(names, ", "))
		}
		entries = picked
	}

	reg := loadLocaleRegistry(root)
	var out []Finding
	for _, e := range entries {
		out = append(out, manifestFindings(e)...)
		out = append(out, i18nFindings(root, e, reg)...)
	}

	repo, err := archcheck.Load(root)
	if err != nil {
		return nil, err
	}
	viol := archcheck.Check(repo)
	for _, e := range entries {
		modPath := "erpora/modules/" + e.dir
		for _, v := range viol {
			if v.Rule != RuleD8 && v.Rule != RuleGoVersion {
				continue
			}
			if v.Package == modPath || strings.HasPrefix(v.Package, modPath+"/") {
				out = append(out, Finding{Module: label(e), Rule: v.Rule, Detail: v.Detail})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Detail < b.Detail
	})
	return out, nil
}

func label(e entry) string {
	if e.manifest.Key != "" {
		return e.manifest.Key
	}
	return e.dir
}

func manifestFindings(e entry) []Finding {
	if e.parseErr != nil {
		return []Finding{{Module: label(e), Rule: RuleSchema, Detail: "module.json: " + e.parseErr.Error()}}
	}
	var out []Finding
	for _, p := range manifest.Validate(e.manifest) {
		field := p.Field
		if field == "" {
			field = "/"
		}
		out = append(out, Finding{Module: label(e), Rule: RuleSchema, Detail: fmt.Sprintf("%s: %s (%s)", field, p.Message, p.Code)})
	}
	prefix := e.manifest.Slug + "."
	for _, c := range e.manifest.Capabilities {
		if !strings.HasPrefix(c, prefix) {
			out = append(out, Finding{Module: label(e), Rule: RuleCapabilityPrefix, Detail: fmt.Sprintf("capability %q harus berawalan %q", c, prefix)})
		}
	}
	return out
}

// loadLocaleRegistry membaca registri bahasa repo; nil bila berkasnya tidak ada atau rusak (kerusakan
// registri dilaporkan oleh i18n:check dan oleh startup, bukan oleh module:check).
func loadLocaleRegistry(root string) *i18n.Registry {
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "i18n", "locales.json")) //nolint:gosec // path tetap di root repo
	if err != nil {
		return nil
	}
	reg, err := i18n.ParseRegistry(data)
	if err != nil {
		return nil
	}
	return reg
}

// i18nFindings memeriksa bagian "i18n" manifest (PRD-000 FR-25, Requirement 1.3 dan 3.8): bahasa terdaftar di
// registri (bila registri tersedia), defaultLocale termasuk locales, berkas "<locale>.json" ada, dan modul yang
// membawa direktori i18n/ wajib mendeklarasikan bagian "i18n".
func i18nFindings(root string, e entry, reg *i18n.Registry) []Finding {
	if e.parseErr != nil {
		return nil
	}
	f := func(format string, a ...any) Finding {
		return Finding{Module: label(e), Rule: RuleI18n, Detail: fmt.Sprintf(format, a...)}
	}
	dir := filepath.Join(root, "modules", e.dir, "i18n")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = ""
	}
	decl := e.manifest.I18n
	if decl == nil {
		if dir != "" {
			return []Finding{f("modul membawa direktori i18n/ tetapi manifest tidak punya bagian \"i18n\"")}
		}
		return nil
	}
	var out []Finding
	if !slices.Contains(decl.Locales, decl.DefaultLocale) {
		out = append(out, f("defaultLocale %q tidak ada di locales %v", decl.DefaultLocale, decl.Locales))
	}
	for _, loc := range decl.Locales {
		if reg != nil && !reg.Has(loc) {
			out = append(out, f("bahasa %q tidak terdaftar di registri bahasa (internal/platform/i18n/locales.json)", loc))
		}
		if _, err := os.Stat(filepath.Join(dir, loc+".json")); dir == "" || err != nil {
			out = append(out, f("berkas katalog modules/%s/i18n/%s.json untuk bahasa %q tidak ditemukan", e.dir, loc, loc))
		}
	}
	return out
}
