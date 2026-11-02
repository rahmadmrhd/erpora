// Package generate adalah pembangkit kode dari metadata resource (spec resource-metadata, task 6.1 sampai 6.5):
// skema OpenAPI (disuntikkan ke modules/<dir>/openapi.yaml), kode Go (domain/<nama>_meta.gen.go dan
// presentation/http/<nama>_dto.gen.go beserta runtime per modul), dan metadata TypeScript
// (web/src/metadata/<nama>.meta.gen.ts).
//
// Keamanan (Requirement 8.4, 8.6, 19.3 sampai 19.10): pembangkit hanya menulis di bawah modules/<dir>/, tidak
// mengikuti symlink, gagal tanpa menulis apa pun bila pemeriksa metadata gagal atau penanda "Code generated"
// pada berkas keluaran hilang, dan semua literal melewati satu fungsi kutip (goString, jsonLit, dan pengodean
// node YAML). Keluaran deterministik dan tidak memuat cap waktu. Paket ini tidak mengakses database dan hanya
// dipanggil dari flavor dev (perintah metadata:generate).
package generate

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"erpora/internal/devtools/gen"
	"erpora/internal/devtools/metadatatools/check"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/internal/devtools/templates/resource"
)

// Galat sentinel (gunakan errors.Is).
var (
	// ErrNoChecker: Options.Checker kosong; pembangkit tidak boleh berjalan tanpa metadata:check.
	ErrNoChecker = errors.New("pemeriksa metadata (check) wajib disediakan")
	// ErrCheckFailed: metadata:check atau resolusi menemukan masalah; tidak ada yang ditulis.
	ErrCheckFailed = errors.New("metadata:check gagal; tidak ada yang dibangkitkan")
	// ErrDrift: keluaran bangkitan berbeda dari yang ada di disk (mode Check).
	ErrDrift = errors.New("keluaran bangkitan tidak sinkron dengan metadata (jalankan metadata:generate)")
	// ErrHandEdited: berkas keluaran kehilangan penanda "Code generated" (diubah tangan atau milik orang lain).
	ErrHandEdited = errors.New("berkas keluaran tidak berpenanda Code generated (diubah tangan?)")
	// ErrUnsafePath: jalur di luar direktori modul, symlink, atau berkas tidak biasa.
	ErrUnsafePath = errors.New("jalur keluaran tidak aman")
	// ErrConflict: skema OpenAPI tulis tangan bernama sama dengan skema bangkitan.
	ErrConflict = errors.New("bentrok dengan skema tulis tangan")
	// ErrInput: masukan metadata tidak dapat dibangkitkan (pertahanan berlapis di samping check).
	ErrInput = errors.New("masukan metadata tidak dapat dibangkitkan")
)

// Checker menjalankan pemeriksaan metadata (metadata:check) atas katalog; mengembalikan semua temuan. Antarmuka
// kecil ini memisahkan pembangkit dari paket check.
type Checker interface {
	Check(root string, cat *resolve.Catalog) []load.Problem
}

// Options mengatur Generate.
type Options struct {
	// Checker wajib; temuan apa pun menghentikan pembangkitan sebelum menulis.
	Checker Checker
	// Modules membatasi pada direktori modul (modules/<dir>); kosong berarti semua modul pada katalog yang
	// punya resource.
	Modules []string
	// Check hanya membandingkan dan tidak menulis (metadata:generate --check).
	Check bool
}

// File adalah satu keluaran yang diharapkan; Path relatif root dengan garis miring biasa.
type File struct {
	Path    string
	Content []byte
}

// Result adalah hasil Generate.
type Result struct {
	// Files adalah seluruh keluaran yang diharapkan, terurut menurut Path.
	Files []File
	// Written dan Removed diisi pada mode tulis; Stale pada mode Check (berbeda, belum ada, atau yatim).
	Written, Removed, Stale []string
	// Omitted mencatat bagian yang sengaja tidak dipancarkan ke klien (scope server, clientVisible: false).
	Omitted []string
	// Problems memuat temuan check atau resolusi.
	Problems []load.Problem
}

type module struct {
	slug, dir, goPath string
	resources         []*resModel
	hasWeb            bool
}

func moduleDirOf(slug string) string { return strings.ReplaceAll(slug, "-", "") }

// importPathRe membatasi path import Go dari go.mod (tanpa kutip, spasi, atau karakter kontrol).
var importPathRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]*$`)

// remoteLookupFor membangun pencari field resource seberang dari hasil resolusi (dan laporan check bila ada).
func remoteLookupFor(rs *resolve.Result, info map[string]*check.ResourceInfo) remoteLookup {
	return func(key, field string) (remoteField, bool) {
		r := rs.Resource(key)
		if r == nil {
			return remoteField{}, false
		}
		f := r.Field(field)
		if f == nil {
			return remoteField{}, false
		}
		class, group := f.Classification(), manifestGroup(r.GroupOf(f))
		if inf := info[key]; inf != nil {
			if c := inf.Class[field]; c != "" {
				class = c
			}
			if g := inf.Group[field]; g != "" {
				group = g
			}
		}
		def := manifestGroup(r.DefaultGroup)
		if def == "" {
			def = resolve.DefaultGroupName
		}
		visible := true
		if v, ok := f.Attrs["clientVisible"].(bool); ok && !v {
			visible = false
		}
		return remoteField{Class: class, Group: group, DefaultGroup: def, ClientVisible: visible}, true
	}
}

// Generate membangkitkan (atau, dengan Options.Check, memeriksa) keluaran semua modul pada katalog. Pemeriksa
// diberikan lewat Options.Checker (wajib); untuk alur produksi yang memakai paket check lihat GenerateWorkspace.
func Generate(root string, cat *resolve.Catalog, opts Options) (Result, error) {
	var res Result
	if opts.Checker == nil {
		return res, ErrNoChecker
	}
	if probs := opts.Checker.Check(root, cat); len(probs) > 0 {
		res.Problems = probs
		return res, fmt.Errorf("%w: %d temuan", ErrCheckFailed, len(probs))
	}
	rs := cat.ResolveAll()
	if len(rs.Problems) > 0 {
		res.Problems = rs.Problems
		return res, fmt.Errorf("%w: %d temuan resolusi", ErrCheckFailed, len(rs.Problems))
	}
	return run(root, rs, cat.Doc, nil, nil, opts)
}

// GenerateWorkspace menjalankan metadata:check (check.LoadWorkspace dan check.Run) atas root, gagal tanpa menulis
// apa pun bila ada galat (peringatan tidak menggagalkan), lalu membangkitkan memakai laporan check: klasifikasi
// dan grup efektif (accessor mewarisi), serta scope aturan dan kondisi (server tidak dipancarkan ke klien).
// Options.Checker diabaikan.
func GenerateWorkspace(root string, opts Options) (Result, error) {
	var res Result
	in, loadProblems := check.LoadWorkspace(root)
	rep := check.Run(in)
	all := append(append([]check.Problem(nil), loadProblems...), rep.Problems...)
	var errs []load.Problem
	for _, p := range all {
		if p.Severity == check.SeverityError {
			errs = append(errs, load.Problem{File: p.File, Line: p.Line, Column: p.Column, Pointer: p.Pointer, Code: p.Code, Message: p.Message})
		}
	}
	if len(errs) > 0 {
		res.Problems = errs
		return res, fmt.Errorf("%w: %d galat", ErrCheckFailed, len(errs))
	}
	docs := map[string]*load.Document{}
	for _, m := range in.Modules {
		for _, d := range m.Docs {
			docs[d.Key] = d
		}
	}
	return run(root, rep.Resolved, func(key string) *load.Document { return docs[key] }, rep.Resources, rep.ModuleDirs, opts)
}

func run(root string, rs *resolve.Result, docOf func(string) *load.Document, info map[string]*check.ResourceInfo, dirs map[string]string, opts Options) (Result, error) {
	var res Result
	cat := docSource(docOf)
	mods, err := collectModules(root, rs, info, dirs, opts.Modules)
	if err != nil {
		return res, err
	}

	files := map[string][]byte{}
	var omitted []string
	var orphanCandidates []string
	remote := remoteLookupFor(rs, info)
	handled := map[string]bool{}
	for _, mod := range mods {
		o, err := buildModule(root, cat, mod, files, remote)
		if err != nil {
			return res, err
		}
		omitted = append(omitted, o...)
		oc, err := orphanScan(root, mod.dir)
		if err != nil {
			return res, err
		}
		orphanCandidates = append(orphanCandidates, oc...)
		handled[mod.dir] = true
	}
	if len(opts.Modules) == 0 {
		// Modul yang tidak lagi punya resource tetap dipindai: keluaran bangkitan dan skema x-erp-generated yatim.
		stale, err := staleModules(root, handled, files)
		if err != nil {
			return res, err
		}
		orphanCandidates = append(orphanCandidates, stale...)
	}
	res.Omitted = omitted
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		res.Files = append(res.Files, File{Path: p, Content: files[p]})
	}

	// Perbandingan dengan disk; berkas tanpa penanda menggagalkan semuanya sebelum menulis.
	var stale, handEdited []string
	for _, f := range res.Files {
		existing, ok, err := readExisting(root, f.Path)
		if err != nil {
			return res, err
		}
		if ok && !strings.HasSuffix(f.Path, "openapi.yaml") && !hasHeader(existing) {
			handEdited = append(handEdited, f.Path)
			continue
		}
		if !ok || string(existing) != string(f.Content) {
			stale = append(stale, f.Path)
		}
	}
	if len(handEdited) > 0 {
		sort.Strings(handEdited)
		return res, fmt.Errorf("%w: %s", ErrHandEdited, strings.Join(handEdited, ", "))
	}
	var orphans []string
	for _, p := range orphanCandidates {
		if _, expected := files[p]; expected {
			continue
		}
		existing, ok, err := readExisting(root, p)
		if err != nil {
			return res, err
		}
		if ok && hasHeader(existing) {
			orphans = append(orphans, p)
		}
	}
	sort.Strings(orphans)

	if opts.Check {
		res.Stale = append(append([]string{}, stale...), orphans...)
		sort.Strings(res.Stale)
		if len(res.Stale) > 0 {
			return res, fmt.Errorf("%w: %s", ErrDrift, strings.Join(res.Stale, ", "))
		}
		return res, nil
	}

	plan := gen.Plan{}
	for _, p := range stale {
		kind := gen.OpCreate
		if _, ok, _ := readExisting(root, p); ok {
			kind = gen.OpModify
		}
		plan.Ops = append(plan.Ops, gen.FileOp{Path: p, Kind: kind, Content: files[p]})
	}
	if len(plan.Ops) > 0 {
		if err := gen.Apply(context.Background(), plan, gen.Options{Root: root}); err != nil {
			return res, fmt.Errorf("tulis keluaran: %w", err)
		}
	}
	res.Written = append(res.Written, stale...)
	for _, p := range orphans {
		if err := removeFile(root, p); err != nil {
			return res, err
		}
		res.Removed = append(res.Removed, p)
	}
	return res, nil
}

func collectModules(root string, rs *resolve.Result, info map[string]*check.ResourceInfo, dirs map[string]string, only []string) ([]*module, error) {
	bySlug := map[string]*module{}
	for _, r := range rs.Resources {
		m, err := buildModel(r, info[r.Key])
		if err != nil {
			return nil, err
		}
		mod := bySlug[r.Module]
		if mod == nil {
			dir, err := resolveModuleDir(r.Module, dirs)
			if err != nil {
				return nil, err
			}
			mod = &module{slug: r.Module, dir: dir}
			bySlug[r.Module] = mod
		}
		mod.resources = append(mod.resources, m)
	}
	var mods []*module
	for _, mod := range bySlug {
		mods = append(mods, mod)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].dir < mods[j].dir })
	if len(only) > 0 {
		want := map[string]bool{}
		for _, d := range only {
			if !moduleDirRe.MatchString(d) {
				return nil, unsafePath(d, "nama direktori modul tidak sah")
			}
			want[d] = true
		}
		var filtered []*module
		for _, m := range mods {
			if want[m.dir] {
				filtered = append(filtered, m)
				delete(want, m.dir)
			}
		}
		if len(want) > 0 {
			return nil, inputErr("modul %s tidak punya metadata resource pada katalog", strings.Join(sortedKeys(want), ", "))
		}
		mods = filtered
	}
	for _, mod := range mods {
		rel := "modules/" + mod.dir
		ok, err := isDir(root, rel)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, inputErr("direktori %s tidak ada", rel)
		}
		data, found, err := readExisting(root, rel+"/go.mod")
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, inputErr("%s/go.mod tidak ada", rel)
		}
		mod.goPath = modfile.ModulePath(data)
		if mod.goPath == "" {
			return nil, inputErr("%s/go.mod tidak memuat direktif module", rel)
		}
		if !importPathRe.MatchString(mod.goPath) {
			return nil, inputErr("%s/go.mod: path module tidak sah untuk impor Go", rel)
		}
		_, mod.hasWeb, err = readExisting(root, rel+"/web/package.json")
		if err != nil {
			return nil, err
		}
		sort.Slice(mod.resources, func(i, j int) bool { return mod.resources[i].Res.Key < mod.resources[j].Res.Key })
	}
	return mods, nil
}

// detectPackage membaca nama package berkas tulis tangan di direktori; bawaan def bila tidak ada.
func detectPackage(root, relDir, def string) (string, error) {
	names, err := listDir(root, relDir)
	if err != nil {
		return "", err
	}
	found := ""
	for _, n := range names {
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || strings.HasSuffix(n, ".gen.go") {
			continue
		}
		data, ok, err := readExisting(root, relDir+"/"+n)
		if err != nil || !ok {
			continue
		}
		f, perr := parser.ParseFile(token.NewFileSet(), n, data, parser.PackageClauseOnly)
		if perr != nil {
			continue
		}
		if found != "" && found != f.Name.Name {
			return "", inputErr("%s memuat lebih dari satu nama package", relDir)
		}
		found = f.Name.Name
	}
	if found == "" {
		return def, nil
	}
	return found, nil
}

func buildModule(root string, cat docSource, mod *module, files map[string][]byte, remote remoteLookup) ([]string, error) {
	base := "modules/" + mod.dir
	domainDir := base + "/internal/domain"
	httpDir := base + "/internal/presentation/http"
	domainPkg, err := detectPackage(root, domainDir, "domain")
	if err != nil {
		return nil, err
	}
	httpPkg, err := detectPackage(root, httpDir, "httpapi")
	if err != nil {
		return nil, err
	}
	put := func(rel string, content []byte) error {
		if err := checkRel(rel, mod.dir); err != nil {
			return err
		}
		if _, dup := files[rel]; dup {
			return unsafePath(rel, "keluaran ganda")
		}
		files[rel] = normalizeLF(content)
		return nil
	}
	dr, err := runtimeSource(resource.DomainRuntimeSource, resource.DomainRuntimePackage, domainPkg)
	if err != nil {
		return nil, err
	}
	if err := put(domainDir+"/meta_runtime.gen.go", dr); err != nil {
		return nil, err
	}
	hr, err := runtimeSource(resource.HTTPRuntimeSource, resource.HTTPRuntimePackage, httpPkg)
	if err != nil {
		return nil, err
	}
	if err := put(httpDir+"/meta_runtime.gen.go", hr); err != nil {
		return nil, err
	}
	var omitted []string
	var schemas []namedSchema
	stems := map[string]string{}
	for _, m := range mod.resources {
		if prev, dup := stems[m.Snake]; dup {
			return nil, inputErr("resource %s dan %s menghasilkan nama berkas yang sama", m.Res.Key, prev)
		}
		stems[m.Snake] = m.Res.Key
		dom, err := renderDomain(m, domainPkg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Res.Key, err)
		}
		if err := put(domainDir+"/"+m.Snake+"_meta.gen.go", dom); err != nil {
			return nil, err
		}
		dto, err := renderDTO(m, httpPkg, mod.goPath+"/internal/domain")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Res.Key, err)
		}
		if err := put(httpDir+"/"+m.Snake+"_dto.gen.go", dto); err != nil {
			return nil, err
		}
		ss, err := buildSchemas(m)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Res.Key, err)
		}
		schemas = append(schemas, ss...)
		if mod.hasWeb {
			doc := cat(m.Res.Key)
			pos := func(ptr string) int {
				if doc == nil {
					return 0
				}
				return doc.Pos(ptr).Line
			}
			ts, om, err := buildTS(m, pos, remote)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", m.Res.Key, err)
			}
			omitted = append(omitted, om...)
			if err := put(base+"/web/src/metadata/"+m.Name+".meta.gen.ts", ts); err != nil {
				return nil, err
			}
		}
	}
	if err := checkIdentifierCollisions(root, []string{domainDir, httpDir}, files); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range schemas {
		if seen[s.Name] {
			return nil, inputErr("modul %s: nama skema %s ganda", mod.dir, s.Name)
		}
		seen[s.Name] = true
	}
	specPath := base + "/openapi.yaml"
	src, ok, err := readExisting(root, specPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, inputErr("%s tidak ada; modul bermetadata wajib punya kontrak (make:module membuatnya)", specPath)
	}
	out, err := SpliceSchemas(src, schemas)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", specPath, err)
	}
	if err := put(specPath, out); err != nil {
		return nil, err
	}
	return omitted, nil
}

func normalizeLF(b []byte) []byte {
	if !strings.Contains(string(b), "\r\n") {
		return b
	}
	return []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
}

// orphanScan mendaftar berkas bangkitan yang mungkin yatim di direktori keluaran modul (dir = nama direktori modul);
// berkas yang termasuk keluaran yang diharapkan dikecualikan oleh pemanggil.
func orphanScan(root, dir string) ([]string, error) {
	base := "modules/" + dir
	var out []string
	for _, d := range []struct{ dir, suffix string }{
		{base + "/internal/domain", "_meta.gen.go"},
		{base + "/internal/domain", "meta_runtime.gen.go"},
		{base + "/internal/presentation/http", "_dto.gen.go"},
		{base + "/internal/presentation/http", "meta_runtime.gen.go"},
		{base + "/web/src/metadata", ".meta.gen.ts"},
	} {
		names, err := listDir(root, d.dir)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if strings.HasSuffix(n, d.suffix) {
				out = append(out, d.dir+"/"+n)
			}
		}
	}
	return out, nil
}

// docSource mencari dokumen metadata menurut kunci (untuk urutan sumber, mis. pivotFields).
type docSource func(key string) *load.Document

// staleModules memindai modul yang tidak (lagi) punya resource pada katalog: berkas bangkitan yatim dikembalikan
// sebagai kandidat yatim (dihapus atau dilaporkan basi), dan skema x-erp-generated yatim pada openapi.yaml dibuang
// lewat keluaran yang diharapkan. Modul tanpa artefak bangkitan tidak tersentuh dan galat bentuk di berkasnya
// tidak menggagalkan apa pun.
func staleModules(root string, handled map[string]bool, files map[string][]byte) ([]string, error) {
	ok, err := isDir(root, "modules")
	if err != nil || !ok {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "modules"))
	if err != nil {
		return nil, unsafePath("modules", "direktori tidak dapat dibaca")
	}
	var out []string
	for _, e := range entries {
		dir := e.Name()
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || !moduleDirRe.MatchString(dir) || handled[dir] {
			continue
		}
		oc, err := orphanScan(root, dir)
		if err != nil {
			return nil, err
		}
		out = append(out, oc...)
		spec := "modules/" + dir + "/openapi.yaml"
		src, found, err := readExisting(root, spec)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if names, nerr := GeneratedSchemaNames(src); nerr != nil || len(names) == 0 {
			continue
		}
		cleaned, err := SpliceSchemas(src, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec, err)
		}
		if err := checkRel(spec, dir); err != nil {
			return nil, err
		}
		files[spec] = normalizeLF(cleaned)
	}
	return out, nil
}

// topLevelNames mengembalikan nama deklarasi tingkat paket (tanpa metode) pada satu berkas Go.
func topLevelNames(name string, src []byte) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		return nil, err
	}
	var out []string
	add := func(id *ast.Ident) {
		if id.Name != "_" && id.Name != "init" {
			out = append(out, id.Name)
		}
	}
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if x.Recv == nil {
				add(x.Name)
			}
		case *ast.GenDecl:
			for _, s := range x.Specs {
				switch sp := s.(type) {
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						add(n)
					}
				case *ast.TypeSpec:
					add(sp.Name)
				}
			}
		}
	}
	return out, nil
}

// checkIdentifierCollisions memastikan identifier tingkat paket pada berkas bangkitan (termasuk runtime yang disalin
// per modul) tidak bentrok dengan kode tulis tangan (berkas .go bukan .gen.go, termasuk test) di direktori yang sama.
// Bentrok menggagalkan pembangkitan sebelum menulis apa pun.
func checkIdentifierCollisions(root string, dirs []string, files map[string][]byte) error {
	for _, dir := range dirs {
		generated := map[string]string{}
		for p, c := range files {
			if path.Dir(p) != dir || !strings.HasSuffix(p, ".gen.go") {
				continue
			}
			names, err := topLevelNames(p, c)
			if err != nil {
				return fmt.Errorf("%w: keluaran %s tidak dapat diurai", ErrInput, path.Base(p))
			}
			for _, n := range names {
				generated[n] = path.Base(p)
			}
		}
		existing, err := listDir(root, dir)
		if err != nil {
			return err
		}
		var clashes []string
		for _, n := range existing {
			if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, ".gen.go") {
				continue
			}
			data, ok, err := readExisting(root, dir+"/"+n)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			names, perr := topLevelNames(n, data)
			if perr != nil {
				continue // berkas tulis tangan yang tidak dapat diurai bukan urusan pembangkit
			}
			for _, nm := range names {
				if g, hit := generated[nm]; hit {
					clashes = append(clashes, fmt.Sprintf("%s (%s dan %s)", nm, n, g))
				}
			}
		}
		if len(clashes) > 0 {
			sort.Strings(clashes)
			return inputErr("identifier bangkitan bentrok dengan kode tulis tangan di %s: %s; ganti nama kode tulis tangan atau resource", dir, strings.Join(clashes, ", "))
		}
	}
	return nil
}

// resolveModuleDir menentukan direktori tujuan tulis sebuah modul. Pada jalur workspace dirs (dari check) adalah
// satu-satunya sumber: slug yang tidak ada di sana (gagal pemeriksaan identitas) ditolak, dan direktori harus sama
// dengan slug tanpa tanda hubung sehingga modul bermanifest slug lain tidak dapat menulis ke direktori modul lain
// (Requirement 19.3). Tanpa dirs (jalur katalog) direktori diturunkan dari slug.
func resolveModuleDir(slug string, dirs map[string]string) (string, error) {
	want := moduleDirOf(slug)
	if dirs != nil {
		d, ok := dirs[slug]
		if !ok {
			return "", inputErr("modul bermanifest slug %q tidak dikenal oleh pemeriksaan identitas modul", slug)
		}
		base, found := strings.CutPrefix(d, "modules/")
		if !found || base != want {
			return "", inputErr("direktori modul %q tidak sama dengan slug %q (diharapkan modules/%s)", d, slug, want)
		}
	}
	if !moduleDirRe.MatchString(want) {
		return "", inputErr("slug modul %q tidak memberi direktori modul yang sah", slug)
	}
	return want, nil
}
