package contracttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Severity temuan linter: error, warn, atau info. Hanya error dan warn menggagalkan lint; info adalah daftar untuk reviewer
// (mis. pengecualian x-erp-allow-highly-confidential).
const (
	SeverityError = "error"
	SeverityWarn  = "warn"
	SeverityInfo  = "info"
)

// LintFailed melaporkan apakah ada temuan selain info.
func LintFailed(rs []LintResult) bool {
	for _, r := range rs {
		if r.Severity != SeverityInfo {
			return true
		}
	}
	return false
}

// LintResult adalah satu temuan linter.
type LintResult struct {
	Severity string `json:"severity"`
	Target   string `json:"target"`
	File     string `json:"file"`
	Rule     string `json:"rule"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
}

type lintTarget struct {
	Name         string   `json:"name"`
	File         string   `json:"file"`
	Slug         string   `json:"slug"`
	Dependencies []string `json:"dependencies"` // slug dependensi manifest (tanpa platform.core)
}

type lintCatalog struct {
	Root    string            `json:"root"`
	Ruleset string            `json:"ruleset"`
	Targets []lintTarget      `json:"targets"`
	Docs    map[string]string `json:"docs"`
}

// Lint menulis katalog JSON ke file sementara lalu menjalankan `bun web/contracts-lint.ts --catalog <file>`;
// stdout adalah JSON []LintResult. only "" = common + platform + semua modul; selain itu satu modul (Dir, slug, atau key).
func Lint(ctx context.Context, repo *Repo, only string, bun BunRunner) ([]LintResult, error) {
	tools, err := filepath.Abs(repo.ToolsRoot)
	if err != nil {
		return nil, err
	}
	cat := lintCatalog{
		Root:    repo.Root,
		Ruleset: filepath.Join(tools, "contracts", "spectral.yaml"),
		Docs:    map[string]string{},
	}
	for name, file := range repo.Files {
		cat.Docs[name] = file
	}
	if only == "" {
		cat.Targets = append(cat.Targets,
			lintTarget{Name: "common", File: repo.Files["common"]},
			lintTarget{Name: "platform", File: repo.Files["platform"], Slug: "platform"},
		)
		for _, m := range repo.Modules {
			if m.HasContract {
				cat.Targets = append(cat.Targets, repo.lintTarget(m))
			}
		}
	} else {
		m, ok := repo.Module(only)
		if !ok {
			return nil, fmt.Errorf("modul %q tidak dikenal (modul: %s)", only, repo.ModuleNames())
		}
		if !m.HasContract {
			return nil, fmt.Errorf("modul %s tidak punya modules/%s/openapi.yaml", m.Key, m.Dir)
		}
		cat.Targets = append(cat.Targets, repo.lintTarget(m))
	}

	data, err := json.Marshal(cat)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "erpora-lint-catalog-*.json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	out, err := bun.Run(ctx, tools, "web/contracts-lint.ts", "--catalog", f.Name())
	if err != nil {
		return nil, fmt.Errorf("linter kontrak gagal dijalankan: %w", err)
	}
	var res []LintResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("keluaran linter bukan JSON: %w", err)
	}
	sort.SliceStable(res, func(i, j int) bool {
		a, b := res[i], res[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
	return res, nil
}

func (r *Repo) lintTarget(m ModuleInfo) lintTarget {
	t := lintTarget{Name: m.Slug, File: r.Files[m.Slug], Slug: m.Slug}
	for _, key := range m.Dependencies {
		if slug := r.slugOfKey(key); slug != "" {
			t.Dependencies = append(t.Dependencies, slug)
		}
	}
	return t
}
