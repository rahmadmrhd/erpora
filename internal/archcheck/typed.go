package archcheck

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// TypedPackage adalah satu paket dengan informasi tipe lengkap untuk aturan EGRESS dan DECODE.
type TypedPackage struct {
	ImportPath, Module string
	// Slug adalah namespace i18n modul pemilik (slug di module.json; bila tidak ada, nama terakhir path
	// Go module). Dipakai aturan I1.
	Slug  string
	Fset  *token.FileSet
	Files []*ast.File
	Info  *types.Info
	Types *types.Package
}

// LoadTyped memuat "./..." di dir dengan informasi tipe (tanpa file test). env menambah environment
// proses (mis. GOWORK=off untuk fixture).
func LoadTyped(dir string, env ...string) ([]TypedPackage, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedModule | packages.NeedImports | packages.NeedDeps,
		Tests: false,
		Dir:   dir,
		Env:   append(os.Environ(), env...),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("muat paket %s: %w", dir, err)
	}
	var out []TypedPackage
	var errs []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
		tp := TypedPackage{ImportPath: p.PkgPath, Fset: p.Fset, Files: p.Syntax, Info: p.TypesInfo, Types: p.Types}
		if p.Module != nil {
			tp.Module = p.Module.Path
			tp.Slug = moduleSlug(dir, p.Module.Path)
		}
		out = append(out, tp)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("paket %s tidak dapat dikompilasi:\n%s", dir, strings.Join(errs, "\n"))
	}
	return out, nil
}

// moduleSlug membaca slug dari <dir>/module.json; bila tidak ada, memakai nama terakhir path Go module.
func moduleSlug(dir, modPath string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "module.json")); err == nil { //nolint:gosec // path tetap di direktori modul
		var m struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(data, &m) == nil && m.Slug != "" {
			return m.Slug
		}
	}
	return modPath[strings.LastIndex(modPath, "/")+1:]
}

// LoadTypedRepo memuat setiap modul bisnis (modules/*/go.mod) di bawah root dalam mode workspace.
func LoadTypedRepo(root string) ([]TypedPackage, error) {
	mods, err := filepath.Glob(filepath.Join(root, "modules", "*", "go.mod"))
	if err != nil {
		return nil, err
	}
	var all []TypedPackage
	for _, gm := range mods {
		pkgs, err := LoadTyped(filepath.Dir(gm))
		if err != nil {
			return nil, err
		}
		all = append(all, pkgs...)
	}
	return all, nil
}

// CheckSecurity menerapkan aturan EGRESS, DECODE, dan I1 pada paket modul bisnis (Module berprefiks
// "erpora/modules/"). Detail: "<file>:<line>: <uraian>".
//
// EGRESS: modul tidak boleh memakai HTTP client mentah net/http (DefaultClient, DefaultTransport, Get, Post,
// Head, PostForm, serta Client/Transport yang diinstansiasi); satu-satunya jalur keluar adalah sdk/egress
// (security-guidelines §3.3).
//
// DECODE: input tidak boleh didekode langsung ke tipe dari paket internal/infrastructure, termasuk lewat
// slice, map, atau struct pembungkus (mass assignment, security-guidelines §3.1). json.Unmarshal/Decode di
// dalam paket infrastructure sendiri (payload jsonb/egress milik adapter) diperbolehkan; httpx.DecodeJSON
// di paket infrastructure selalu melanggar.
func CheckSecurity(pkgs []TypedPackage) []Violation {
	var out []Violation
	for _, p := range pkgs {
		if !strings.HasPrefix(p.Module, modulesBase) || p.Info == nil {
			continue
		}
		seenEgress := map[string]bool{} // EGRESS: satu temuan per baris (ident tipe + method call pada baris sama dilaporkan sekali)
		add := func(rule string, pos token.Pos, detail string) {
			pp := p.Fset.Position(pos)
			if rule == "EGRESS" {
				key := fmt.Sprintf("%s:%d", pp.Filename, pp.Line)
				if seenEgress[key] {
					return
				}
				seenEgress[key] = true
			}
			out = append(out, Violation{Rule: rule, Package: p.ImportPath,
				Detail: fmt.Sprintf("%s:%d: %s", filepath.Base(pp.Filename), pp.Line, detail)})
		}
		inInfra := isInfraPath(p.ImportPath)

		for _, f := range p.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.Ident:
					if name, ok := rawHTTPUse(p.Info.Uses[x]); ok {
						add("EGRESS", x.Pos(), "net/http."+name+" dilarang di modul; gunakan erpora/sdk/egress")
					}
					// Setiap penyebutan nama tipe net/http.Client/Transport (var, field, parameter, return, literal,
					// new): menangkap klien zero value (`var c http.Client; c.Get(u)`) yang lolos dari pemeriksaan literal.
					if tn, ok := p.Info.Uses[x].(*types.TypeName); ok && tn.Pkg() != nil && tn.Pkg().Path() == "net/http" &&
						(tn.Name() == "Client" || tn.Name() == "Transport") {
						add("EGRESS", x.Pos(), "net/http."+tn.Name()+" dilarang di modul; gunakan erpora/sdk/egress")
					}
				case *ast.SelectorExpr:
					// Pemanggilan method keluar pada klien/transport yang diperoleh dari mana pun (mis. hasil fungsi lain).
					if sel, ok := p.Info.Selections[x]; ok && sel.Kind() == types.MethodVal {
						if name, isRaw := rawHTTPType(sel.Recv()); isRaw {
							switch x.Sel.Name {
							case "Do", "Get", "Post", "Head", "PostForm", "RoundTrip":
								add("EGRESS", x.Sel.Pos(), "(*net/http."+name+")."+x.Sel.Name+" dilarang di modul; gunakan erpora/sdk/egress")
							}
						}
					}
				case *ast.CallExpr:
					checkDecodeCall(p, x, inInfra, add)
					checkI18nKey(p, x, add)
				}
				return true
			})
		}
		// httpx.DecodeJSON[T]: tipe target ada di Info.Instances.
		for id, inst := range p.Info.Instances {
			fn, ok := p.Info.Uses[id].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != httpxPath || fn.Name() != "DecodeJSON" || inst.TypeArgs.Len() == 0 {
				continue
			}
			target := inst.TypeArgs.At(0)
			switch {
			case inInfra:
				add("DECODE", id.Pos(), "httpx.DecodeJSON di paket infrastructure dilarang; decode di lapisan presentation ke DTO khusus")
			case mentionsInfra(target, map[types.Type]bool{}):
				add("DECODE", id.Pos(), "httpx.DecodeJSON ke tipe yang memuat model infrastructure ("+target.String()+"); gunakan DTO khusus")
			}
		}
	}
	return out
}

const (
	httpxPath = "erpora/sdk/httpx"
	infraSeg  = "/internal/infrastructure"
)

func isInfraPath(path string) bool {
	return strings.Contains(path+"/", infraSeg+"/")
}

// rawHTTPUse mengenali objek package-level net/http yang membuka klien mentah (bukan method).
func rawHTTPUse(obj types.Object) (string, bool) {
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "net/http" {
		return "", false
	}
	switch o := obj.(type) {
	case *types.Var:
		if o.IsField() || o.Parent() != o.Pkg().Scope() {
			return "", false
		}
		if o.Name() == "DefaultClient" || o.Name() == "DefaultTransport" {
			return o.Name(), true
		}
	case *types.Func:
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			return "", false
		}
		switch o.Name() {
		case "Get", "Post", "Head", "PostForm":
			return o.Name(), true
		}
	}
	return "", false
}

// rawHTTPType mengenali net/http.Client dan net/http.Transport (juga lewat pointer).
func rawHTTPType(t types.Type) (string, bool) {
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "net/http" {
		return "", false
	}
	if n := named.Obj().Name(); n == "Client" || n == "Transport" {
		return n, true
	}
	return "", false
}

// i18nPath adalah paket penerjemah yang dipakai modul.
const i18nPath = "erpora/sdk/i18n"

// checkI18nKey (aturan I1): argumen kunci (indeks 1) pada i18n.T dan i18n.Lookup yang bernilai konstan
// (literal, konstanta bernama, atau lipatan konstanta) tidak boleh berbentuk "<ns>::kunci" dengan ns
// bukan namespace modul sendiri, "ui", atau "platform". Kunci dinamis tidak dapat diperiksa statis;
// penegakannya di runtime (T menolak namespace modul lain).
func checkI18nKey(p TypedPackage, call *ast.CallExpr, add func(string, token.Pos, string)) {
	var id *ast.Ident
	switch f := call.Fun.(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return
	}
	fn, ok := p.Info.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != i18nPath || (fn.Name() != "T" && fn.Name() != "Lookup") || len(call.Args) < 2 {
		return
	}
	tv, ok := p.Info.Types[call.Args[1]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return
	}
	key := constant.StringVal(tv.Value)
	ns, _, found := strings.Cut(key, "::")
	if !found || ns == "ui" || ns == "platform" || ns == p.Slug {
		return
	}
	add("I1", call.Args[1].Pos(), "kunci i18n bernamespace modul lain ("+strconv.Quote(ns)+"); modul hanya boleh memakai namespace sendiri, ui, dan platform")
}

// checkDecodeCall memeriksa json.Unmarshal dan (*json.Decoder).Decode.
func checkDecodeCall(p TypedPackage, call *ast.CallExpr, inInfra bool, add func(string, token.Pos, string)) {
	var id *ast.Ident
	switch f := call.Fun.(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return
	}
	fn, ok := p.Info.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "encoding/json" {
		return
	}
	var target ast.Expr
	sig, _ := fn.Type().(*types.Signature)
	switch {
	case fn.Name() == "Unmarshal" && sig != nil && sig.Recv() == nil && len(call.Args) == 2:
		target = call.Args[1]
	case fn.Name() == "Decode" && sig != nil && sig.Recv() != nil && len(call.Args) == 1:
		target = call.Args[0]
	default:
		return
	}
	if inInfra {
		return
	}
	if t := p.Info.TypeOf(target); t != nil && mentionsInfra(t, map[types.Type]bool{}) {
		add("DECODE", call.Pos(), "json."+fn.Name()+" ke tipe yang memuat model infrastructure ("+t.String()+"); gunakan DTO khusus")
	}
}

// mentionsInfra menelusuri tipe (pointer, slice, array, map, field struct termasuk embedded, underlying
// tipe bernama) dan melaporkan apakah ada tipe bernama dari paket internal/infrastructure.
func mentionsInfra(t types.Type, seen map[types.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	switch x := types.Unalias(t).(type) {
	case *types.Named:
		if pkg := x.Obj().Pkg(); pkg != nil && isInfraPath(pkg.Path()) {
			return true
		}
		return mentionsInfra(x.Underlying(), seen)
	case *types.Pointer:
		return mentionsInfra(x.Elem(), seen)
	case *types.Slice:
		return mentionsInfra(x.Elem(), seen)
	case *types.Array:
		return mentionsInfra(x.Elem(), seen)
	case *types.Map:
		return mentionsInfra(x.Key(), seen) || mentionsInfra(x.Elem(), seen)
	case *types.Struct:
		for f := range x.Fields() {
			if mentionsInfra(f.Type(), seen) {
				return true
			}
		}
	}
	return false
}
