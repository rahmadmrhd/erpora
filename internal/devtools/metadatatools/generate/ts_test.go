package generate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
