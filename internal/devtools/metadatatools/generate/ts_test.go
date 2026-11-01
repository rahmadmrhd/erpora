package generate

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/resolve"
)

func fixtureWithWeb(t *testing.T) (string, Result) {
	t.Helper()
	root, cat := fixtureRepo(t)
	for _, m := range []string{"fixturecore", "fixturesales"} {
		writeFile(t, root, "modules/"+m+"/web/package.json", `{"name":"@erpora/module-`+m+`"}`)
	}
	res, err := Generate(root, cat, Options{Checker: okChecker{}, Check: true})
	if !errors.Is(err, ErrDrift) {
		t.Fatal(err)
	}
	return root, res
}

func tsFiles(r Result) []File {
	var out []File
	for _, f := range r.Files {
		if strings.HasSuffix(f.Path, ".ts") {
			out = append(out, f)
		}
	}
	return out
}

func TestTypeScriptOnlyForModulesWithWeb(t *testing.T) {
	_, res := fixtureWithWeb(t)
	ts := tsFiles(res)
	if len(ts) != 7 {
		t.Fatalf("diharapkan 7 berkas TS, ada %d", len(ts))
	}
	for _, f := range ts {
		if !strings.Contains(f.Path, "/web/src/metadata/") || !strings.HasSuffix(f.Path, ".meta.gen.ts") {
			t.Errorf("jalur TS tak terduga: %s", f.Path)
		}
		s := string(f.Content)
		if !strings.HasPrefix(s, generatedHeader+"\n") {
			t.Errorf("%s: kepala hilang", f.Path)
		}
		if !strings.Contains(s, "export const resourceMeta: ResourceMeta = {") {
			t.Errorf("%s: ekspor resourceMeta hilang", f.Path)
		}
	}
	// modul tanpa web/ tetap tanpa TS
	root, cat := fixtureRepo(t)
	writeFile(t, root, "modules/fixturecore/web/package.json", `{}`)
	res2, _ := Generate(root, cat, Options{Checker: okChecker{}, Check: true})
	for _, f := range tsFiles(res2) {
		if !strings.HasPrefix(f.Path, "modules/fixturecore/") {
			t.Errorf("TS untuk modul tanpa web/: %s", f.Path)
		}
	}
}

func TestTypeScriptGoldenFixture(t *testing.T) {
	_, res := fixtureWithWeb(t)
	assertGolden(t, filepath.Join("testdata", "golden", "fixture-web"), tsFiles(res))
}

func TestTypeScriptNeverEmitsServerOnlyOrClientHidden(t *testing.T) {
	_, res := fixtureWithWeb(t)
	for _, f := range tsFiles(res) {
		s := string(f.Content)
		for _, bad := range []string{"clientVisible", "scope", "server"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s memuat %q", f.Path, bad)
			}
		}
	}
	inv := ""
	for _, f := range tsFiles(res) {
		if strings.HasSuffix(f.Path, "/invoice.meta.gen.ts") {
			inv = string(f.Content)
		}
	}
	if strings.Contains(inv, "discountWithinCap") {
		t.Error("aturan scope: server tidak boleh dipancarkan")
	}
	if !strings.Contains(inv, "dueAfterIssue") {
		t.Error("aturan scope: both harus dipancarkan")
	}
	if strings.Contains(inv, "50000000.00") {
		t.Error("ambang yang merujuk field confidential bocor ke klien")
	}
	// Catatan pelaporan memuat apa yang sengaja tidak dipancarkan.
	notes := strings.Join(res.Omitted, "\n")
	for _, want := range []string{"discountWithinCap", "approvalNote.requiredWhen", "approvalNote.visibleWhen"} {
		if !strings.Contains(notes, want) {
			t.Errorf("catatan Omitted tidak memuat %q:\n%s", want, notes)
		}
	}
}

func TestTypeScriptClientVisibleFalseAndUnsafeRefs(t *testing.T) {
	r := hostileResolved(t)
	r.Fields = append(r.Fields,
		fieldNamed("secretNote", map[string]any{"type": "string", "clientVisible": false, "clientVisibleReason": "sensitif", "classification": "internal"}),
		fieldNamed("gate", map[string]any{"type": "string", "nullable": true, "visibleWhen": map[string]any{"field": "secretNote", "op": "eq", "value": "x"}}),
		fieldNamed("amount", map[string]any{"type": "string", "classification": "confidential", "fieldGroup": "money"}),
		fieldNamed("other", map[string]any{"type": "string", "nullable": true, "readonlyWhen": map[string]any{"field": "amount", "op": "eq", "value": "a"}}),
	)
	r.LockWhen = map[string]any{"field": "amount", "op": "eq", "value": "a"}
	m, err := buildModel(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, omitted, err := buildTS(m, func(string) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"secretNote", "sensitif", "clientVisible", "lockWhen"} {
		if strings.Contains(s, bad) {
			t.Errorf("keluaran memuat %q", bad)
		}
	}
	notes := strings.Join(omitted, "\n")
	for _, want := range []string{"secretNote", "gate.visibleWhen", "other.readonlyWhen", "lockWhen"} {
		if !strings.Contains(notes, want) {
			t.Errorf("catatan tidak memuat %q:\n%s", want, notes)
		}
	}
}

func TestTypeScriptLayoutDeterministicAndFolded(t *testing.T) {
	_, a := fixtureWithWeb(t)
	_, b := fixtureWithWeb(t)
	for i, f := range tsFiles(a) {
		if string(f.Content) != string(tsFiles(b)[i].Content) {
			t.Errorf("%s tidak deterministik", f.Path)
		}
		for n, line := range strings.Split(string(f.Content), "\n") {
			if len(line) > 100 && !strings.Contains(line, `"`) && !strings.Contains(line, `'`) {
				t.Errorf("%s:%d melebihi 100 kolom tanpa literal string", f.Path, n+1)
			}
			if strings.HasSuffix(line, " ") || strings.Contains(line, "\t") {
				t.Errorf("%s:%d spasi ekor atau tab", f.Path, n+1)
			}
		}
	}
}

// TestTypeScriptPassesClientLoader menjalankan bun test atas keluaran golden: checkClientMeta (pagar chunk
// publik) dan validateRecord dari paket metadata frontend. Dilewati bila bun tidak ada.
func TestTypeScriptPassesClientLoader(t *testing.T) {
	if testing.Short() {
		t.Skip("bun test")
	}
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun tidak ada")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("testdata", "golden", "fixture-web")); err != nil {
		t.Skip("golden TS belum ada")
	}
	cmd := exec.Command(bun, "test", "./internal/devtools/metadatatools/generate/testdata/ts/")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bun test gagal: %v\n%s", err, out)
	}
}

func TestTypeScriptAccessorExprOnlyWhenRefsAreClientSafe(t *testing.T) {
	r := hostileResolved(t)
	expr := func(refs ...string) map[string]any {
		var args []any
		for _, f := range refs {
			args = append(args, map[string]any{"field": f})
		}
		return map[string]any{"concat": args}
	}
	acc := func(kind string, e any) map[string]any {
		a := map[string]any{"kind": kind}
		if e != nil {
			a["expr"] = e
		}
		return a
	}
	r.Fields = append(r.Fields,
		fieldNamed("cost", map[string]any{"type": "decimal", "precision": json.Number("18"), "scale": json.Number("2"), "classification": "confidential", "fieldGroup": "money"}),
		fieldNamed("safeLabel", map[string]any{"type": "string", "nullable": true, "accessor": acc("expr", expr("title"))}),
		fieldNamed("costLabel", map[string]any{"type": "string", "nullable": true, "accessor": acc("expr", expr("title", "cost"))}),
		fieldNamed("bigCost", map[string]any{"type": "string", "nullable": true, "accessor": acc("expr", map[string]any{
			"if":   map[string]any{"field": "cost", "op": "gte", "value": "1000000"},
			"then": map[string]any{"lit": "mahal"}, "else": map[string]any{"lit": "murah"}})}),
		fieldNamed("storedMargin", map[string]any{"type": "decimal", "precision": json.Number("18"), "scale": json.Number("2"), "nullable": true,
			"accessor": acc("stored", map[string]any{"mul": []any{map[string]any{"field": "cost"}, map[string]any{"dec": "2"}}, "scale": json.Number("2"), "round": "half_up"})}),
		fieldNamed("storedSafe", map[string]any{"type": "string", "nullable": true, "accessor": acc("stored", expr("title"))}),
		fieldNamed("viaFunc", map[string]any{"type": "string", "nullable": true, "accessor": acc("func", nil)}),
	)
	m, err := buildModel(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, omitted, err := buildTS(m, func(string) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	section := func(name string) string {
		i := strings.Index(s, `name: "`+name+`"`)
		if i < 0 {
			t.Fatalf("field %s tidak dipancarkan", name)
		}
		rest := s[i:]
		if j := strings.Index(rest[10:], "name: "); j >= 0 {
			rest = rest[:10+j]
		}
		return rest
	}
	if sec := section("safeLabel"); !strings.Contains(sec, `kind: "expr"`) || !strings.Contains(sec, "concat") {
		t.Errorf("expr aman harus dipancarkan:\n%s", sec)
	}
	for _, name := range []string{"costLabel", "bigCost"} {
		if sec := section(name); strings.Contains(sec, "accessor") || strings.Contains(sec, "concat") || !strings.Contains(sec, "readOnly: true") {
			t.Errorf("%s: accessor yang merujuk field confidential tidak boleh dipancarkan:\n%s", name, sec)
		}
	}
	for _, name := range []string{"storedMargin", "storedSafe"} {
		sec := section(name)
		if !strings.Contains(sec, `kind: "stored"`) || strings.Contains(sec, "expr:") || strings.Contains(sec, "mul") {
			t.Errorf("%s: stored hanya kind, tanpa expr:\n%s", name, sec)
		}
	}
	if sec := section("viaFunc"); !strings.Contains(sec, `kind: "func"`) {
		t.Errorf("func harus tetap:\n%s", sec)
	}
	if strings.Contains(s, "1000000") || strings.Contains(s, "mahal") {
		t.Error("ambang atau literal dari accessor yang tidak dipancarkan bocor")
	}
	notes := strings.Join(omitted, "\n")
	for _, want := range []string{"costLabel: accessor expr", "bigCost: accessor expr", "storedMargin: ekspresi accessor stored", "storedSafe: ekspresi accessor stored"} {
		if !strings.Contains(notes, want) {
			t.Errorf("catatan tidak memuat %q:\n%s", want, notes)
		}
	}
}

func TestTypeScriptPivotFieldsFilteredByClassificationAndConditions(t *testing.T) {
	r := hostileResolved(t)
	r.Relations = []*resolve.Relation{{Name: "tags", Attrs: map[string]any{
		"type": "belongsToMany", "to": "evil.tag",
		"pivotFields": map[string]any{
			"remark":  map[string]any{"type": "string", "nullable": true, "maxLength": json.Number("100")},
			"secret":  map[string]any{"type": "string", "nullable": true, "classification": "confidential"},
			"hidden":  map[string]any{"type": "string", "nullable": true, "clientVisible": false, "clientVisibleReason": "x"},
			"grouped": map[string]any{"type": "string", "nullable": true, "fieldGroup": "money"},
			"gated":   map[string]any{"type": "string", "nullable": true, "visibleWhen": map[string]any{"field": "secret", "op": "eq", "value": "x"}},
			"okGate":  map[string]any{"type": "string", "nullable": true, "visibleWhen": map[string]any{"field": "remark", "op": "isnull", "value": false}},
		},
	}}}
	m, err := buildModel(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, omitted, err := buildTS(m, func(ptr string) int { return len(ptr) })
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`name: "remark"`, `name: "gated"`, `name: "okGate"`} {
		if !strings.Contains(s, want) {
			t.Errorf("tidak memuat %s", want)
		}
	}
	for _, bad := range []string{`name: "secret"`, `name: "hidden"`, `name: "grouped"`} {
		if strings.Contains(s, bad) {
			t.Errorf("pivot field terlarang dipancarkan: %s", bad)
		}
	}
	if strings.Contains(fieldSection(t, s, "gated"), "visibleWhen") {
		t.Error("kondisi yang merujuk pivot field confidential tidak boleh dipancarkan")
	}
	if !strings.Contains(fieldSection(t, s, "okGate"), "visibleWhen") {
		t.Error("kondisi aman pada pivot field harus dipancarkan")
	}
	notes := strings.Join(omitted, "\n")
	for _, want := range []string{"pivotFields.secret", "pivotFields.hidden", "pivotFields.grouped", "gated.visibleWhen"} {
		if !strings.Contains(notes, want) {
			t.Errorf("catatan tidak memuat %q:\n%s", want, notes)
		}
	}
}

// fieldSection mengembalikan potongan keluaran TS dari `name: "<nama>"` sampai field berikutnya.
func fieldSection(t *testing.T, s, name string) string {
	t.Helper()
	i := strings.Index(s, `name: "`+name+`"`)
	if i < 0 {
		t.Fatalf("field %s tidak dipancarkan", name)
	}
	rest := s[i:]
	if j := strings.Index(rest[10:], "name: "); j >= 0 {
		rest = rest[:10+j]
	}
	return rest
}
