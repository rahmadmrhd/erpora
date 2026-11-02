package generate

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"erpora/internal/devtools/metadatatools/resolve"
)

// hostile memuat kutip, garis miring, backtick, penutup komentar, "$$", interpolasi, baris baru, pemisah baris
// Unicode, dan upaya menyuntikkan kode, SQL, dan skrip.
const hostile = "\"; panic(\"pwn\") /* */ `x` $$ ${y}\n\t\\ '; DROP TABLE t; --  </script>"

func hostileResolved(t *testing.T) *resolve.Resolved {
	t.Helper()
	field := func(name string, attrs map[string]any) *resolve.Field {
		return &resolve.Field{Name: name, Attrs: attrs}
	}
	cond := map[string]any{"field": "note", "op": "eq", "value": hostile}
	return &resolve.Resolved{
		Key: "evil.thing", Module: "evil", Kind: "resource", DefaultGroup: "default",
		Description:   hostile,
		LabelTemplate: "{title} */ ` $$ \"",
		LockWhen:      map[string]any{"field": "title", "op": "ne", "value": hostile},
		Fields: []*resolve.Field{
			field("title", map[string]any{
				"type": "string", "maxLength": json.Number("10"), "pattern": "^[a-z\"`$*/ ]+$",
				"default": hostile, "description": hostile, "label": "evil." + hostile,
				"classification": "internal",
			}),
			field("status", map[string]any{"type": "enum", "values": []any{"draft", "net_15"}, "default": "draft"}),
			field("note", map[string]any{"type": "text", "nullable": true, "requiredWhen": cond, "visibleWhen": cond}),
		},
		Rules: []*resolve.Rule{{ID: "ruleOne", Attrs: map[string]any{
			"assert":  map[string]any{"field": "title", "op": "ne", "value": hostile},
			"field":   "title",
			"message": "evil." + hostile,
		}}},
	}
}

func declaredNames(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.GenDecl:
			for _, s := range x.Specs {
				switch sp := s.(type) {
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						out = append(out, n.Name)
					}
				case *ast.TypeSpec:
					out = append(out, sp.Name.Name)
				}
			}
		case *ast.FuncDecl:
			if x.Recv == nil { // metode melekat pada tipe bangkitan
				out = append(out, x.Name.Name)
			}
		}
	}
	return out
}

func stringLiterals(t *testing.T, f *ast.File) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			s, err := strconv.Unquote(bl.Value)
			if err != nil {
				t.Fatalf("literal tidak sah %s: %v", bl.Value, err)
			}
			out[s] = true
		}
		return true
	})
	return out
}

func TestGoStringIsTheOnlyQuoter(t *testing.T) {
	for _, s := range []string{hostile, "", "\x00", "\xff\xfe", "a\"b", "`", "*/", "$$", "\r\n", "日本語"} {
		q := goString(s)
		if !strings.HasPrefix(q, `"`) || strings.ContainsAny(q, "\n\r") {
			t.Errorf("literal mentah: %q", q)
		}
		back, err := strconv.Unquote(q)
		if err != nil || back != s {
			t.Errorf("round-trip gagal untuk %q: %q %v", s, back, err)
		}
	}
}

func TestHostileInputGoOutputIsDataOnly(t *testing.T) {
	m, err := buildModel(hostileResolved(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	// nilai enum jahat langsung ke emitter (lapisan ini harus aman walau masukan lolos validasi hulu)
	for _, f := range m.Fields {
		if f.Name == "status" {
			f.Enum = append(f.Enum, hostile, "*/ x", "`")
		}
	}
	dom, err := renderDomain(m, "domain")
	if err != nil {
		t.Fatal(err)
	}
	dto, err := renderDTO(m, "httpapi", "erpora/modules/evil/internal/domain")
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string][]byte{"domain": dom, "dto": dto} {
		f, err := parser.ParseFile(token.NewFileSet(), name+".go", src, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s bukan Go yang sah: %v\n%s", name, err, src)
		}
		for _, n := range declaredNames(f) {
			if !strings.HasPrefix(n, "Thing") && !strings.HasPrefix(n, "Decode") && n != "decodeBody" {
				t.Errorf("%s: deklarasi tak terduga %q", name, n)
			}
		}
		for _, cg := range f.Comments {
			if strings.Contains(cg.Text(), "pwn") || strings.Contains(cg.Text(), "DROP") {
				t.Errorf("%s: masukan masuk ke komentar: %q", name, cg.Text())
			}
		}
		if name == "domain" {
			lits := stringLiterals(t, f)
			for _, want := range []string{hostile, "evil." + hostile, "{title} */ ` $$ \"", "*/ x", "`", "^[a-z\"`$*/ ]+$"} {
				if !lits[want] {
					t.Errorf("literal %q tidak ada sebagai literal string utuh", want)
				}
			}
		}
	}
	assertGolden(t, filepath.Join("testdata", "golden", "hostile-go"), []File{
		{Path: "domain/thing_meta.gen.go", Content: dom}, {Path: "http/thing_dto.gen.go", Content: dto},
	})
}

func TestHostileInputOpenAPIRoundTrips(t *testing.T) {
	m, err := buildModel(hostileResolved(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Fields {
		if f.Name == "status" {
			f.Enum = append(f.Enum, hostile)
		}
	}
	schemas, err := buildSchemas(m)
	if err != nil {
		t.Fatal(err)
	}
	out, err := SpliceSchemas([]byte("openapi: 3.1.0\npaths: {}\n"), schemas)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("hasil bukan YAML sah: %v\n%s", err, out)
	}
	if len(doc) != 3 {
		t.Errorf("kunci tingkat atas bertambah: %v", doc)
	}
	allowed := map[string]bool{"type": true, "description": true, "required": true, "additionalProperties": true,
		"minProperties": true, "properties": true, generatedMarkerKey: true}
	sch := doc["components"].(map[string]any)["schemas"].(map[string]any)
	if len(sch) != 3 {
		t.Fatalf("skema = %v", sch)
	}
	for name, v := range sch {
		for k := range v.(map[string]any) {
			if !allowed[k] {
				t.Errorf("%s: kunci tak terduga %q", name, k)
			}
		}
	}
	read := sch["Thing"].(map[string]any)
	if read["description"] != hostile {
		t.Errorf("deskripsi resource berubah: %q", read["description"])
	}
	title := read["properties"].(map[string]any)["title"].(map[string]any)
	if title["description"] != hostile || title["pattern"] != "^[a-z\"`$*/ ]+$" {
		t.Errorf("title = %v", title)
	}
	create := sch["ThingCreate"].(map[string]any)["properties"].(map[string]any)["title"].(map[string]any)
	if create["default"] != hostile {
		t.Errorf("default berubah: %q", create["default"])
	}
	enum := read["properties"].(map[string]any)["status"].(map[string]any)["enum"].([]any)
	if enum[len(enum)-1] != hostile {
		t.Errorf("enum berubah: %v", enum)
	}
	assertGolden(t, filepath.Join("testdata", "golden", "hostile-openapi"), []File{{Path: "openapi.yaml", Content: out}})
}

var tsString = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)

// decodeTSString menguraikan literal string JavaScript (kutip ganda atau tunggal) yang dipancarkan pembangkit.
func decodeTSString(lit string) (string, error) {
	if strings.HasPrefix(lit, "'") {
		inner := lit[1 : len(lit)-1]
		var sb strings.Builder
		for i := 0; i < len(inner); i++ {
			switch {
			case inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '\'':
				sb.WriteByte('\'')
				i++
			case inner[i] == '\\' && i+1 < len(inner):
				sb.WriteByte(inner[i])
				sb.WriteByte(inner[i+1])
				i++
			case inner[i] == '"':
				sb.WriteString(`\"`)
			default:
				sb.WriteByte(inner[i])
			}
		}
		lit = `"` + sb.String() + `"`
	}
	var s string
	err := json.Unmarshal([]byte(lit), &s)
	return s, err
}

func TestHostileInputTypeScriptIsDataOnly(t *testing.T) {
	m, err := buildModel(hostileResolved(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Fields {
		if f.Name == "status" {
			f.Enum = append(f.Enum, hostile)
		}
	}
	out, _, err := buildTS(m, func(string) int { return 0 }, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	decoded := map[string]bool{}
	rest := tsString.ReplaceAllStringFunc(text, func(lit string) string {
		s, err := decodeTSString(lit)
		if err != nil {
			t.Fatalf("literal bukan string sah: %s", lit)
		}
		decoded[s] = true
		return `""`
	})
	for _, want := range []string{hostile, "evil." + hostile, "{title} */ ` $$ \"", "^[a-z\"`$*/ ]+$"} {
		if !decoded[want] {
			t.Errorf("literal %q tidak ada sebagai string JSON utuh", want)
		}
	}
	// Di luar literal string tidak boleh ada teks masukan: hanya struktur objek.
	for _, bad := range []string{"`", "*/", "$$", "${", "pwn", "DROP", "<script", " "} {
		if strings.Contains(rest, bad) {
			t.Errorf("teks masukan %q bocor di luar literal string:\n%s", bad, rest)
		}
	}
	if strings.Contains(text, "description") {
		t.Error("deskripsi tidak boleh dipancarkan ke klien")
	}
	assertGolden(t, filepath.Join("testdata", "golden", "hostile-ts"), []File{{Path: "thing.meta.gen.ts", Content: out}})
}

func TestBuildModelRejectsHostileIdentifiersAndEnums(t *testing.T) {
	bad := func(mut func(r *resolve.Resolved)) {
		t.Helper()
		r := hostileResolved(t)
		mut(r)
		if _, err := buildModel(r, nil); !errors.Is(err, ErrInput) {
			t.Errorf("harus ErrInput: %v", err)
		}
	}
	bad(func(r *resolve.Resolved) { r.Key = "evil.thing\"; drop" })
	bad(func(r *resolve.Resolved) { r.Key = "evil.Thing" })
	bad(func(r *resolve.Resolved) { r.Fields[0].Name = "ti\"tle" })
	bad(func(r *resolve.Resolved) { r.Fields[0].Name = "Title" })
	bad(func(r *resolve.Resolved) { r.Fields[1].Attrs["values"] = []any{"ok", "bad\nvalue"} })
	bad(func(r *resolve.Resolved) { r.Fields[1].Attrs["values"] = []any{"UPPER"} })
	bad(func(r *resolve.Resolved) { r.Fields[0].Attrs["type"] = "string; drop" })
	bad(func(r *resolve.Resolved) { r.Fields[0].Attrs["classification"] = "highly_confidential" })
	bad(func(r *resolve.Resolved) { r.Fields[0].Attrs["pattern"] = "(?=lookahead)" })
	bad(func(r *resolve.Resolved) { r.Fields[0].Attrs["minimum"] = "1e5" })
	bad(func(r *resolve.Resolved) { r.Fields[0].Attrs["default"] = json.Number("5") }) // default angka pada string
	bad(func(r *resolve.Resolved) {
		r.Fields = append(r.Fields, &resolve.Field{Name: "title", Attrs: map[string]any{"type": "string"}})
	})
	bad(func(r *resolve.Resolved) { r.Fields[0].Name = "present" })
	bad(func(r *resolve.Resolved) { r.LabelTemplate = "{unterminated" })
	bad(func(r *resolve.Resolved) { r.Rules[0].ID = "Rule One" })
}

func TestCheckRel(t *testing.T) {
	for _, rel := range []string{"../x", "modules/a/../../x", "/etc/passwd", "modules/a/x/", "modules\\a\\x", "modules/b/x", "modules/a", "", "modules/a/./x", "modules/ab/x"} {
		if err := checkRel(rel, "a"); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("%q harus ditolak: %v", rel, err)
		}
	}
	if err := checkRel("modules/a/internal/domain/x.gen.go", "a"); err != nil {
		t.Error(err)
	}
}

// link membuat symlink (atau junction di Windows); false bila tidak dapat dibuat.
func link(t *testing.T, target, name string) bool {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", name, target).CombinedOutput(); err != nil {
			t.Logf("junction tidak dapat dibuat: %v %s", err, out)
			return false
		}
		return true
	}
	if err := os.Symlink(target, name); err != nil {
		t.Logf("symlink tidak dapat dibuat: %v", err)
		return false
	}
	return true
}

func TestSymlinkedOutputDirIsRejectedWithoutWriting(t *testing.T) {
	root, cat := fixtureRepo(t)
	outside := t.TempDir()
	internal := filepath.Join(root, "modules", "fixturesales", "internal")
	if err := os.MkdirAll(internal, 0o750); err != nil {
		t.Fatal(err)
	}
	domain := filepath.Join(internal, "domain")
	if !link(t, outside, domain) {
		t.Skip("tautan tidak tersedia")
	}
	for _, check := range []bool{false, true} {
		_, err := Generate(root, cat, Options{Checker: okChecker{}, Check: check})
		if !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("check=%v err=%v", check, err)
		}
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Errorf("menulis lewat symlink ke luar modul: %v", entries)
	}
}

func TestErrorsDoNotLeakContentOutsideModule(t *testing.T) {
	root, cat := fixtureRepo(t)
	outside := t.TempDir()
	const secret = "RAHASIA-DI-LUAR-MODUL"
	if err := os.WriteFile(filepath.Join(outside, "package.json"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if !link(t, outside, filepath.Join(root, "modules", "fixturesales", "web")) {
		t.Skip("tautan tidak tersedia")
	}
	_, err := Generate(root, cat, Options{Checker: okChecker{}})
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), outside) {
		t.Errorf("galat membocorkan isi atau lokasi di luar modul: %v", err)
	}
}

func TestModulesOptionRejectsTraversal(t *testing.T) {
	root, cat := fixtureRepo(t)
	for _, m := range []string{"../fixturesales", "modules/x", "a/b", "..", "."} {
		if _, err := Generate(root, cat, Options{Checker: okChecker{}, Modules: []string{m}}); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("%q: %v", m, err)
		}
	}
}

func TestModuleDirIsSymlinkRejected(t *testing.T) {
	root, cat := fixtureRepo(t)
	outside := t.TempDir()
	real := filepath.Join(root, "modules", "fixturecore")
	if err := os.Rename(real, filepath.Join(outside, "fixturecore")); err != nil {
		t.Fatal(err)
	}
	if !link(t, filepath.Join(outside, "fixturecore"), real) {
		t.Skip("tautan tidak tersedia")
	}
	if _, err := Generate(root, cat, Options{Checker: okChecker{}, Modules: []string{"fixturecore"}}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "fixturecore", "internal")); err == nil {
		t.Error("tidak boleh menulis lewat symlink modul")
	}
}
