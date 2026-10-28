package gen

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"erpora/internal/devtools/contracttools"
	"erpora/internal/devtools/templates"
)

// ModuleOptions mengatur make:module.
type ModuleOptions struct {
	Name, Publisher string   // Publisher default "acme"
	Frontend        bool     // buat modules/<dir>/web (@erpora/module-<dir>)
	Depends         []string // key modul, mis. "acme.example"; platform.core selalu ditambahkan
}

// moduleDep adalah dependensi modul pada data template.
type moduleDep struct {
	Key, Dir, GoPath, NPM, Constraint string
	HasWeb                            bool
}

// moduleData adalah data template make:module.
type moduleData struct {
	ModuleName
	PublisherName string
	Dependencies  []moduleDep
	Frontend      bool
}

// MakeModule membangun Plan modul baru turunan example (keputusan rencana 13-15). Validasi dilakukan sebelum Plan:
// nama, bentrok Dir/slug/key dengan modul yang ada (Dir yang sudah ada -> ErrExists), dan dependensi ada di repo.
// Ops: semua file template (OpCreate) ditambah OpModify untuk go.work, go.mod root, internal/app/modules.go dan, bila
// Frontend, web/package.json dan web/src/app/modules.ts. Post: go mod tidy, contracts:generate (Frontend), bun install,
// dan Biome atas file TS.
func MakeModule(root string, o ModuleOptions) (Plan, error) {
	if o.Publisher == "" {
		o.Publisher = "acme"
	}
	name, err := ParseModuleName(o.Name, o.Publisher)
	if err != nil {
		return Plan{}, err
	}
	existing, err := listModules(root)
	if err != nil {
		return Plan{}, err
	}
	for _, m := range existing {
		switch {
		case m.Dir == name.Dir:
			return Plan{}, fmt.Errorf("%w: modules/%s sudah ada (modul %s); nama %q bentrok pada direktori %q", ErrExists, m.Dir, m.Key, name.Slug, name.Dir)
		case m.Slug == name.Slug:
			return Plan{}, fmt.Errorf("slug %q sudah dipakai modul %s", name.Slug, m.Key)
		case m.Key == name.Key:
			return Plan{}, fmt.Errorf("key %q sudah dipakai modules/%s", name.Key, m.Dir)
		}
	}
	modulesGo, err := readRepoFile(root, "internal/app/modules.go")
	if err != nil {
		return Plan{}, err
	}
	if err := CheckModuleDir(name.Dir, modulesGo); err != nil {
		return Plan{}, err
	}
	data := moduleData{ModuleName: name, PublisherName: titleCase(o.Publisher), Frontend: o.Frontend}
	for _, key := range o.Depends {
		ref, err := ResolveModule(root, key)
		if err != nil {
			return Plan{}, fmt.Errorf("dependensi %q: %w", key, err)
		}
		major, minor, _ := strings.Cut(strings.TrimPrefix(ref.Version, "v"), ".")
		minor, _, _ = strings.Cut(minor, ".")
		data.Dependencies = append(data.Dependencies, moduleDep{
			Key: ref.Key, Dir: ref.Dir, GoPath: ref.GoPath, NPM: "@erpora/module-" + ref.Dir, Constraint: "^" + major + "." + minor, HasWeb: ref.HasWeb,
		})
	}

	var plan Plan
	prefix := "modules/" + name.Dir + "/"
	err = fs.WalkDir(templates.FS, "module", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(p, "module/"), ".tmpl")
		if rel == "deps.go" && len(data.Dependencies) == 0 {
			return nil
		}
		if strings.HasPrefix(rel, "web/") && !o.Frontend {
			return nil
		}
		content, err := Render(p, data)
		if err != nil {
			return err
		}
		out := strings.ReplaceAll(rel, "__snake__", name.Snake)
		if strings.HasSuffix(out, ".go") {
			if content, err = FormatGo(out, content); err != nil {
				return err
			}
		}
		plan.Ops = append(plan.Ops, FileOp{Path: prefix + out, Kind: OpCreate, Content: content})
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	sort.Slice(plan.Ops, func(i, j int) bool { return plan.Ops[i].Path < plan.Ops[j].Path })

	mods, err := modifyRepoFiles(root, name, o.Frontend)
	if err != nil {
		return Plan{}, err
	}
	plan.Ops = append(plan.Ops, mods...)
	plan.Post = modulePost(name, o.Frontend)
	plan.NoForce = true
	return plan, nil
}

func titleCase(s string) string {
	words := strings.Split(s, "-")
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func readRepoFile(root, rel string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // path tetap di bawah root repo
	if err != nil {
		return nil, fmt.Errorf("baca %s: %w", rel, err)
	}
	return b, nil
}

// modifyRepoFiles mengubah berkas repo secara terstruktur (keputusan 14): go.work dan go.mod lewat modfile, modules.go
// lewat penanda + astutil, package.json lewat splice JSON, dan modules.ts lewat penanda.
func modifyRepoFiles(root string, n ModuleName, frontend bool) ([]FileOp, error) {
	var ops []FileOp

	work, err := readRepoFile(root, "go.work")
	if err != nil {
		return nil, err
	}
	wf, err := modfile.ParseWork("go.work", work, nil)
	if err != nil {
		return nil, fmt.Errorf("uraikan go.work: %w", err)
	}
	if err := wf.AddUse("./modules/"+n.Dir, ""); err != nil {
		return nil, err
	}
	wf.SortBlocks()
	ops = append(ops, FileOp{Path: "go.work", Kind: OpModify, Content: modfile.Format(wf.Syntax)})

	gm, err := readRepoFile(root, "go.mod")
	if err != nil {
		return nil, err
	}
	mf, err := modfile.Parse("go.mod", gm, nil)
	if err != nil {
		return nil, fmt.Errorf("uraikan go.mod: %w", err)
	}
	if err := mf.AddRequire(n.GoPath, "v0.0.0"); err != nil {
		return nil, err
	}
	if err := mf.AddReplace(n.GoPath, "", "./modules/"+n.Dir, ""); err != nil {
		return nil, err
	}
	mf.SortBlocks()
	mf.Cleanup()
	gmOut, err := mf.Format()
	if err != nil {
		return nil, err
	}
	ops = append(ops, FileOp{Path: "go.mod", Kind: OpModify, Content: gmOut})

	app, err := readRepoFile(root, "internal/app/modules.go")
	if err != nil {
		return nil, err
	}
	// Penanda diperiksa lebih dulu agar kegagalan tidak meninggalkan hasil sebagian.
	appOut, err := InsertAtMarker("internal/app/modules.go", app, "// erp:generated:modules", n.Dir+".New(),")
	if err != nil {
		return nil, err
	}
	if appOut, err = AddGoImport("internal/app/modules.go", appOut, "", n.GoPath); err != nil {
		return nil, err
	}
	ops = append(ops, FileOp{Path: "internal/app/modules.go", Kind: OpModify, Content: appOut})

	if frontend {
		pkg, err := readRepoFile(root, "web/package.json")
		if err != nil {
			return nil, err
		}
		pkgOut, err := AddJSONDependency(pkg, "dependencies", n.NPM, "workspace:*")
		if err != nil {
			return nil, fmt.Errorf("web/package.json: %w", err)
		}
		ops = append(ops, FileOp{Path: "web/package.json", Kind: OpModify, Content: pkgOut})

		ts, err := readRepoFile(root, "web/src/app/modules.ts")
		if err != nil {
			return nil, err
		}
		ident := n.Camel + "Info"
		tsOut, err := InsertAtMarker("web/src/app/modules.ts", ts, "// erp:generated:web-imports",
			fmt.Sprintf("import { moduleInfo as %s } from %q;", ident, n.NPM))
		if err != nil {
			return nil, err
		}
		if tsOut, err = InsertAtMarker("web/src/app/modules.ts", tsOut, "// erp:generated:web-modules", ident+","); err != nil {
			return nil, err
		}
		ops = append(ops, FileOp{Path: "web/src/app/modules.ts", Kind: OpModify, Content: tsOut})
	}
	return ops, nil
}

// modulePost menyusun langkah setelah file ditulis.
func modulePost(n ModuleName, frontend bool) []PostStep {
	steps := []PostStep{{
		Name: "go mod tidy (modules/" + n.Dir + ")",
		Run: func(ctx context.Context, root string) error {
			return runCmd(ctx, filepath.Join(root, "modules", n.Dir), []string{"GOWORK=off"}, "go", "mod", "tidy")
		},
	}}
	if !frontend {
		return steps
	}
	return append(steps,
		generatePost(n.Dir),
		PostStep{Name: "bun install", Run: func(ctx context.Context, root string) error {
			return runCmd(ctx, root, nil, "bun", "install")
		}},
		PostStep{Name: "biome (modules/" + n.Dir + "/web, web/src/app/modules.ts)", Run: func(ctx context.Context, root string) error {
			biome := filepath.Join(root, "web", "node_modules", "@biomejs", "biome", "bin", "biome")
			if _, err := os.Stat(biome); err != nil {
				biome = filepath.Join(root, "node_modules", "@biomejs", "biome", "bin", "biome")
			}
			return runCmd(ctx, root, nil, "bun", biome, "check", "--write", path.Join("modules", n.Dir, "web"), "web/src/app/modules.ts")
		}},
	)
}

// generatePost membangkitkan ulang tipe TS kontrak untuk satu modul ber-web/ (sama dengan `contracts:generate <dir>`).
func generatePost(dir string) PostStep {
	return PostStep{Name: "contracts:generate " + dir, Run: func(ctx context.Context, root string) error {
		bun, err := contracttools.FindBun()
		if err != nil {
			return err
		}
		repo, err := contracttools.LoadRepo(root)
		if err != nil {
			return err
		}
		files, err := contracttools.Generate(ctx, repo, dir, bun)
		if err != nil {
			return err
		}
		return contracttools.WriteGenerated(root, files)
	}}
}

// runCmd menjalankan perintah dan menyertakan stderr/stdout pada error.
func runCmd(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // perintah tetap dari kode devtools
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(out.String()))
	}
	return nil
}
