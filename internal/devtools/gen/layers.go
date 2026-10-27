package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// layerMod adalah data modul pada template lapisan.
type layerMod struct{ Slug, Dir, GoPath, Snake string }

// layerData adalah data template make:entity/usecase/contract/command/repository.
type layerData struct {
	Mod    layerMod
	T      TypeName
	Name   string // nama command (kebab) untuk make:command
	Pascal string // PascalCase dari Name
}

func resolveLayerModule(root, module string) (ModuleRef, layerMod, error) {
	ref, err := ResolveModule(root, module)
	if err != nil {
		return ModuleRef{}, layerMod{}, err
	}
	return ref, layerMod{Slug: ref.Slug, Dir: ref.Dir, GoPath: ref.GoPath, Snake: strings.ReplaceAll(ref.Slug, "-", "_")}, nil
}

// renderOp merender satu template menjadi FileOp OpCreate (file Go diformat).
func renderOp(path, tmpl string, data any) (FileOp, error) {
	content, err := Render(tmpl, data)
	if err != nil {
		return FileOp{}, err
	}
	if strings.HasSuffix(path, ".go") {
		if content, err = FormatGo(path, content); err != nil {
			return FileOp{}, fmt.Errorf("%s: %w", tmpl, err)
		}
	}
	return FileOp{Path: path, Kind: OpCreate, Content: content}, nil
}

func renderOps(files [][2]string, data any) (Plan, error) {
	var plan Plan
	for _, f := range files {
		op, err := renderOp(f[0], f[1], data)
		if err != nil {
			return Plan{}, err
		}
		plan.Ops = append(plan.Ops, op)
	}
	return plan, nil
}

// MakeEntity membuat internal/domain/<snake>.go dan <snake>_test.go.
func MakeEntity(root, module, name string) (Plan, error) {
	ref, mod, err := resolveLayerModule(root, module)
	if err != nil {
		return Plan{}, err
	}
	t, err := ParseTypeName(name)
	if err != nil {
		return Plan{}, err
	}
	base := "modules/" + ref.Dir + "/internal/domain/" + t.Snake
	return renderOps([][2]string{
		{base + ".go", "entity/entity.go.tmpl"},
		{base + "_test.go", "entity/entity_test.go.tmpl"},
	}, layerData{Mod: mod, T: t})
}

// MakeUsecase membuat use case di internal/application/<snake>.go: command (Command + Handler, Handle(ctx, cmd) error)
// atau, dengan query, Query + Result + Handler (Handle(ctx, q) (Result, error)); beserta test-nya.
func MakeUsecase(root, module, name string, query bool) (Plan, error) {
	ref, mod, err := resolveLayerModule(root, module)
	if err != nil {
		return Plan{}, err
	}
	t, err := ParseTypeName(name)
	if err != nil {
		return Plan{}, err
	}
	kind := "command"
	if query {
		kind = "query"
	}
	base := "modules/" + ref.Dir + "/internal/application/" + t.Snake
	return renderOps([][2]string{
		{base + ".go", "usecase/" + kind + ".go.tmpl"},
		{base + "_test.go", "usecase/" + kind + "_test.go.tmpl"},
	}, layerData{Mod: mod, T: t})
}

// MakeContract membuat contract/<snake>.go: interface <Name> dan <Name>DTO, tanpa impor internal/.
func MakeContract(root, module, name string) (Plan, error) {
	ref, mod, err := resolveLayerModule(root, module)
	if err != nil {
		return Plan{}, err
	}
	t, err := ParseTypeName(name)
	if err != nil {
		return Plan{}, err
	}
	return renderOps([][2]string{{"modules/" + ref.Dir + "/contract/" + t.Snake + ".go", "contract/contract.go.tmpl"}}, layerData{Mod: mod, T: t})
}

var commandNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// MakeCommand membuat internal/presentation/cli/<snake>.go (+ test) dan mendaftarkannya di module.go: impor cli lewat
// astutil dan `r.Command(cli.<Pascal>Command())` di atas penanda erp:generated:commands. Nama yang sudah ada (dibaca
// lewat go/ast dari module.go dan presentation/cli) ditolak.
func MakeCommand(root, module, name string) (Plan, error) {
	ref, mod, err := resolveLayerModule(root, module)
	if err != nil {
		return Plan{}, err
	}
	if !commandNameRe.MatchString(name) || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return Plan{}, fmt.Errorf("nama command %q tidak valid: kebab-case huruf kecil (^[a-z][a-z0-9-]*$)", name)
	}
	names, err := existingCommandNames(filepath.Join(root, "modules", ref.Dir))
	if err != nil {
		return Plan{}, err
	}
	if names[name] {
		return Plan{}, fmt.Errorf("command %q sudah ada di modul %s", name, ref.Key)
	}
	pascal := ""
	for _, w := range strings.Split(name, "-") {
		if w != "" {
			pascal += strings.ToUpper(w[:1]) + w[1:]
		}
	}
	data := layerData{Mod: mod, Name: name, Pascal: pascal}
	file := strings.ReplaceAll(name, "-", "_")
	plan, err := renderOps([][2]string{
		{"modules/" + ref.Dir + "/internal/presentation/cli/" + file + ".go", "command/command.go.tmpl"},
		{"modules/" + ref.Dir + "/internal/presentation/cli/" + file + "_test.go", "command/command_test.go.tmpl"},
	}, data)
	if err != nil {
		return Plan{}, err
	}
	modPath := "modules/" + ref.Dir + "/module.go"
	src, err := readRepoFile(root, modPath)
	if err != nil {
		return Plan{}, err
	}
	out, err := InsertAtMarker(modPath, src, "// erp:generated:commands", "r.Command(cli."+pascal+"Command())")
	if err != nil {
		return Plan{}, err
	}
	if out, err = AddGoImport(modPath, out, "", ref.GoPath+"/internal/presentation/cli"); err != nil {
		return Plan{}, err
	}
	plan.Ops = append(plan.Ops, FileOp{Path: modPath, Kind: OpModify, Content: out})
	return plan, nil
}

// existingCommandNames mengumpulkan literal `Name: "<x>"` pada composite literal module.Command di module.go dan
// internal/presentation/cli/*.go.
func existingCommandNames(modDir string) (map[string]bool, error) {
	files := []string{filepath.Join(modDir, "module.go")}
	cliFiles, _ := filepath.Glob(filepath.Join(modDir, "internal", "presentation", "cli", "*.go"))
	files = append(files, cliFiles...)
	names := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f) //nolint:gosec // path di bawah root repo
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, src, 0)
		if err != nil {
			return nil, fmt.Errorf("uraikan %s: %w", f, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Command" {
				return true
			}
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Name" {
					if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
						if s, err := strconv.Unquote(bl.Value); err == nil {
							names[s] = true
						}
					}
				}
			}
			return true
		})
	}
	return names, nil
}

// MakeRepository membuat port repository (domain), model GORM, repository (data.DB(ctx)), mapping, dan test mapping
// (unit, tanpa DB; keputusan rencana M2d butir 2 opsi B). Entity harus sudah ada (make:entity).
func MakeRepository(root, module, name string) (Plan, error) {
	ref, mod, err := resolveLayerModule(root, module)
	if err != nil {
		return Plan{}, err
	}
	t, err := ParseTypeName(name)
	if err != nil {
		return Plan{}, err
	}
	entity := filepath.Join(root, "modules", ref.Dir, "internal", "domain", t.Snake+".go")
	if _, err := os.Stat(entity); err != nil {
		return Plan{}, fmt.Errorf("entity %s belum ada (modules/%s/internal/domain/%s.go); jalankan `erpora make:entity %s %s` dulu",
			t.Pascal, ref.Dir, t.Snake, ref.Slug, t.Pascal)
	}
	dom := "modules/" + ref.Dir + "/internal/domain/" + t.Snake
	inf := "modules/" + ref.Dir + "/internal/infrastructure/" + t.Snake
	plan, err := renderOps([][2]string{
		{dom + "_repository.go", "repository/repository.go.tmpl"},
		{inf + "_model.go", "repository/model.go.tmpl"},
		{inf + "_repo.go", "repository/repo.go.tmpl"},
		{inf + "_mapping.go", "repository/mapping.go.tmpl"},
		{inf + "_mapping_test.go", "repository/mapping_test.go.tmpl"},
	}, layerData{Mod: mod, T: t})
	if err != nil {
		return Plan{}, err
	}
	sort.Slice(plan.Ops, func(i, j int) bool { return plan.Ops[i].Path < plan.Ops[j].Path })
	return plan, nil
}
