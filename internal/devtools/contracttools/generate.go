package contracttools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GenFile adalah satu file hasil generate; Path relatif root repo, dengan slash.
type GenFile struct {
	Path    string
	Content []byte
}

const (
	platformContractsDir = "packages/platform-contracts/src"
	moduleSDKDir         = "packages/module-sdk/src"
	distSpec             = "contracts/dist/openapi.json"
	generatedMarker      = "erpora contracts:generate; jangan diedit"
)

type genJob struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Spec        any    `json:"spec,omitempty"`
	SchemaFile  string `json:"schemaFile,omitempty"`
	Name        string `json:"name,omitempty"`
	Header      string `json:"header"`
	Footer      string `json:"footer,omitempty"`
	VirtualPath string `json:"virtualPath"`
}

func tsHeader(source string) string {
	return "// Dibangkitkan oleh `erpora contracts:generate` dari " + source + ". JANGAN DIEDIT."
}

// Generate membangkitkan, tanpa menulis: (1) packages/platform-contracts/src/{common,platform}.gen.ts dari
// Bundle("common"/"platform"); (2) modules/<dir>/web/api/contract.gen.ts untuk modul ber-web/ (Bundle(slug) +
// `export const client = createClient<paths>()`); (3) packages/module-sdk/src/manifest.gen.ts dari
// contracts/module.schema.json; (4) contracts/dist/openapi.json dari Merge. Semua TS diformat Biome lokal oleh
// runner; urutan GenFile terurut Path; deterministik. only != "" membatasi ke file kontrak satu modul.
func Generate(ctx context.Context, repo *Repo, only string, bun BunRunner) ([]GenFile, error) {
	tools, err := filepath.Abs(repo.ToolsRoot)
	if err != nil {
		return nil, err
	}
	var jobs []genJob
	paths := map[string]string{} // id job -> path tujuan
	add := func(j genJob, path string) {
		j.ID, j.VirtualPath = path, path
		jobs = append(jobs, j)
		paths[path] = path
	}

	moduleJob := func(m ModuleInfo) error {
		spec, err := repo.Set.Bundle(m.Slug)
		if err != nil {
			return err
		}
		add(genJob{
			Kind: "openapi", Spec: spec,
			Header: tsHeader(repo.Files[m.Slug]) + "\nimport { createClient } from \"@erpora/platform-contracts\";",
			Footer: "\nexport const client = createClient<paths>();",
		}, "modules/"+m.Dir+"/web/api/contract.gen.ts")
		return nil
	}

	if only != "" {
		m, ok := repo.Module(only)
		if !ok {
			return nil, fmt.Errorf("modul %q tidak dikenal (modul: %s)", only, repo.ModuleNames())
		}
		if !m.HasContract || !m.HasWeb {
			return nil, fmt.Errorf("modul %s tidak punya modules/%s/openapi.yaml dan web/package.json; tidak ada tipe TS yang dibangkitkan", m.Key, m.Dir)
		}
		if err := moduleJob(m); err != nil {
			return nil, err
		}
	} else {
		for _, name := range []string{"common", "platform"} {
			spec, err := repo.Set.Bundle(name)
			if err != nil {
				return nil, err
			}
			add(genJob{Kind: "openapi", Spec: spec, Header: tsHeader(repo.Files[name])}, platformContractsDir+"/"+name+".gen.ts")
		}
		for _, m := range repo.Modules {
			if m.HasContract && m.HasWeb {
				if err := moduleJob(m); err != nil {
					return nil, err
				}
			}
		}
		schema, err := filepath.Abs(filepath.Join(repo.Root, "contracts", "module.schema.json"))
		if err != nil {
			return nil, err
		}
		add(genJob{Kind: "jsonschema", SchemaFile: schema, Name: "ModuleManifest", Header: tsHeader("contracts/module.schema.json")},
			moduleSDKDir+"/manifest.gen.ts")
	}

	out, err := runGenerate(ctx, tools, jobs, bun)
	if err != nil {
		return nil, err
	}
	var files []GenFile
	for _, j := range jobs {
		content, ok := out[j.ID]
		if !ok {
			return nil, fmt.Errorf("runner tidak mengembalikan %s", j.ID)
		}
		files = append(files, GenFile{Path: j.ID, Content: []byte(content)})
	}

	if only == "" {
		order := []string{"common", "platform"}
		for _, m := range repo.Modules {
			if m.HasContract {
				order = append(order, m.Slug)
			}
		}
		merged, err := repo.Set.Merge(order)
		if err != nil {
			return nil, err
		}
		merged["x-erp-generated"] = generatedMarker
		data, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			return nil, err
		}
		files = append(files, GenFile{Path: distSpec, Content: append(data, '\n')})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func runGenerate(ctx context.Context, tools string, jobs []genJob, bun BunRunner) (map[string]string, error) {
	data, err := json.Marshal(map[string]any{"root": tools, "jobs": jobs})
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "erpora-generate-job-*.json")
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
	raw, err := bun.Run(ctx, tools, "web/contracts-generate.ts", "--job", f.Name())
	if err != nil {
		return nil, fmt.Errorf("pembangkit tipe TS gagal dijalankan: %w", err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("keluaran pembangkit bukan JSON: %w", err)
	}
	return out, nil
}

// WriteGenerated menulis file hasil generate (LF) di bawah root.
func WriteGenerated(root string, files []GenFile) error {
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(p, bytes.ReplaceAll(f.Content, []byte("\r\n"), []byte("\n")), 0o644); err != nil { //nolint:gosec // file sumber repo, bukan rahasia
			return err
		}
	}
	return nil
}

// CheckGenerated mengembalikan path (terurut) yang isinya berbeda atau belum ada di disk, ditambah file *.gen.ts
// yatim (tidak lagi punya sumber). contracts/dist/** (artifact build) dilewati.
func CheckGenerated(root string, files []GenFile) ([]string, error) {
	want := map[string]bool{}
	var stale []string
	for _, f := range files {
		want[f.Path] = true
		if strings.HasPrefix(f.Path, "contracts/dist/") {
			continue
		}
		disk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path))) //nolint:gosec // path dari daftar generate
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err != nil || !bytes.Equal(bytes.ReplaceAll(disk, []byte("\r\n"), []byte("\n")), f.Content) {
			stale = append(stale, f.Path)
		}
	}
	for _, pattern := range []string{"packages/*/src/*.gen.ts", "modules/*/web/api/*.gen.ts"} {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			rel, err := filepath.Rel(root, m)
			if err != nil {
				return nil, err
			}
			rel = filepath.ToSlash(rel)
			if !want[rel] {
				stale = append(stale, rel)
			}
		}
	}
	sort.Strings(stale)
	return stale, nil
}
