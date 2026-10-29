package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	i18nRegistry = "internal/platform/i18n/locales.json"
	i18nPlatform = "internal/platform/i18n/catalog"
	i18nUI       = "packages/ui/i18n"
)

func writeIn(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// i18nRepo membuat repo mini valid (go.work + registri + katalog platform dan ui sejajar).
func i18nRepo(t *testing.T) string {
	t.Helper()
	dir := inRepo(t)
	writeIn(t, dir, i18nRegistry, `{"default":"id","locales":[{"code":"id","name":"Bahasa Indonesia"},{"code":"en","name":"English","fallback":"id"}]}`)
	writeIn(t, dir, i18nPlatform+"/id.json", `{"greet":"Halo {name}"}`)
	writeIn(t, dir, i18nPlatform+"/en.json", `{"greet":"Hello {name}"}`)
	writeIn(t, dir, i18nUI+"/id.json", `{"save":"Simpan"}`)
	writeIn(t, dir, i18nUI+"/en.json", `{"save":"Save"}`)
	return dir
}

func runI18n(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newTestRoot(t)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// Exit code i18n:check (Requirement 8.6): 0 sukses/peringatan, 2 galat validasi, 3 prasyarat gagal.
func TestI18nCheck_ExitCodes(t *testing.T) {
	t.Run("hijau", func(t *testing.T) {
		i18nRepo(t)
		out, err := runI18n(t, "i18n:check")
		if exitCodeOf(err) != 0 || !strings.Contains(out, "0 galat, 0 peringatan") {
			t.Fatalf("exit=%d err=%v out=%s", exitCodeOf(err), err, out)
		}
	})
	t.Run("peringatan tidak menggagalkan", func(t *testing.T) {
		dir := i18nRepo(t)
		writeIn(t, dir, i18nPlatform+"/en.json", `{}`)
		out, err := runI18n(t, "i18n:check")
		if exitCodeOf(err) != 0 || !strings.Contains(out, "missing_key") {
			t.Fatalf("exit=%d err=%v out=%s", exitCodeOf(err), err, out)
		}
	})
	t.Run("strict menggagalkan peringatan", func(t *testing.T) {
		dir := i18nRepo(t)
		writeIn(t, dir, i18nPlatform+"/en.json", `{}`)
		_, err := runI18n(t, "i18n:check", "--strict")
		if exitCodeOf(err) != 2 {
			t.Fatalf("exit=%d err=%v", exitCodeOf(err), err)
		}
	})
	t.Run("galat struktural exit 2", func(t *testing.T) {
		dir := i18nRepo(t)
		writeIn(t, dir, i18nUI+"/en.json", `{"save": `)
		out, err := runI18n(t, "i18n:check")
		if exitCodeOf(err) != 2 || !strings.Contains(out, "invalid_json") {
			t.Fatalf("exit=%d err=%v out=%s", exitCodeOf(err), err, out)
		}
	})
	t.Run("registri tidak ada exit 3", func(t *testing.T) {
		inRepo(t)
		_, err := runI18n(t, "i18n:check")
		if exitCodeOf(err) != 3 {
			t.Fatalf("exit=%d err=%v", exitCodeOf(err), err)
		}
	})
	t.Run("argumen tak dikenal", func(t *testing.T) {
		i18nRepo(t)
		if _, err := runI18n(t, "i18n:check", "extra"); err == nil {
			t.Fatal("harus galat")
		}
	})
}

func TestI18nCheck_JSONOutput(t *testing.T) {
	dir := i18nRepo(t)
	writeIn(t, dir, i18nPlatform+"/en.json", `{}`)
	out, err := runI18n(t, "i18n:check", "--json")
	if exitCodeOf(err) != 0 {
		t.Fatalf("exit=%d err=%v", exitCodeOf(err), err)
	}
	var rep struct {
		Findings []struct{ Code, File, Locale, Key string }
		Warnings int
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("bukan JSON: %v\n%s", err, out)
	}
	if rep.Warnings != 1 || len(rep.Findings) != 1 || rep.Findings[0].Code != "missing_key" ||
		rep.Findings[0].Key != "greet" || rep.Findings[0].Locale != "en" || !strings.HasSuffix(rep.Findings[0].File, "catalog/en.json") {
		t.Fatalf("json = %+v", rep)
	}
}

func TestI18nStatus_Output(t *testing.T) {
	dir := i18nRepo(t)
	writeIn(t, dir, i18nPlatform+"/id.json", `{"a":"A","b":"B"}`)
	writeIn(t, dir, i18nPlatform+"/en.json", `{"a":"A"}`)
	out, err := runI18n(t, "i18n:status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAMESPACE", "platform", "50.0%", "default"} {
		if !strings.Contains(out, want) {
			t.Errorf("keluaran tidak memuat %q:\n%s", want, out)
		}
	}
	// katalog rusak -> exit 2; registri tidak ada -> exit 3
	writeIn(t, dir, i18nPlatform+"/en.json", `{rusak`)
	if _, err := runI18n(t, "i18n:status"); exitCodeOf(err) != 2 {
		t.Fatalf("exit=%d err=%v", exitCodeOf(err), err)
	}
	inRepo(t)
	if _, err := runI18n(t, "i18n:status"); exitCodeOf(err) != 3 {
		t.Fatalf("exit=%d err=%v", exitCodeOf(err), err)
	}
}

func TestI18nAdd_CreatesIdempotentAndExitCodes(t *testing.T) {
	dir := i18nRepo(t)
	out, err := runI18n(t, "i18n:add", "ms", "--name", "Bahasa Melayu")
	if err != nil {
		t.Fatalf("err=%v out=%s", err, out)
	}
	for _, want := range []string{"registri:", "buat internal/platform/i18n/catalog/ms.json", "buat packages/ui/i18n/ms.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("keluaran tidak memuat %q:\n%s", want, out)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, i18nPlatform, "ms.json"))
	if err != nil || !strings.Contains(string(data), "TODO: Halo {name}") {
		t.Fatalf("ms.json = %s err=%v", data, err)
	}
	out, err = runI18n(t, "i18n:add", "ms", "--name", "Bahasa Melayu")
	if err != nil || !strings.Contains(out, "tidak ada perubahan") {
		t.Fatalf("jalan kedua: err=%v out=%s", err, out)
	}
	if out, err = runI18n(t, "i18n:check", "--strict"); err != nil {
		t.Fatalf("check --strict setelah add: %v\n%s", err, out)
	}
	// galat penggunaan
	if _, err := runI18n(t, "i18n:add"); err == nil {
		t.Error("tanpa argumen harus galat")
	}
	if _, err := runI18n(t, "i18n:add", "ZZ_zz"); exitCodeOf(err) != 2 {
		t.Errorf("kode tidak sah: exit=%d", exitCodeOf(err))
	}
	if _, err := runI18n(t, "i18n:add", "fr", "tidakada"); exitCodeOf(err) != 2 {
		t.Errorf("modul tidak dikenal: exit=%d", exitCodeOf(err))
	}
	inRepo(t)
	if _, err := runI18n(t, "i18n:add", "fr"); exitCodeOf(err) != 3 {
		t.Errorf("registri tidak ada: exit=%d", exitCodeOf(err))
	}
}

func TestI18nGenerate_WritesAndChecksDrift(t *testing.T) {
	dir := i18nRepo(t)
	// --check sebelum generate: basi (exit 2), tidak menulis apa pun
	out, err := runI18n(t, "i18n:generate", "--check")
	if exitCodeOf(err) != 2 || !strings.Contains(out, "basi: packages/ui/src/i18n/keys.gen.ts") {
		t.Fatalf("check awal: exit=%d err=%v out=%s", exitCodeOf(err), err, out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "packages/ui/src/i18n/keys.gen.ts")); statErr == nil {
		t.Fatal("--check tidak boleh menulis")
	}
	out, err = runI18n(t, "i18n:generate")
	if err != nil || !strings.Contains(out, "tulis packages/platform-web/src/i18n/keys.platform.gen.ts") {
		t.Fatalf("generate: err=%v out=%s", err, out)
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "packages/platform-web/src/i18n/keys.platform.gen.ts"))
	if readErr != nil || !strings.Contains(string(data), `greet: { name: string | number };`) {
		t.Fatalf("keys.platform.gen.ts = %s err=%v", data, readErr)
	}
	if out, err = runI18n(t, "i18n:generate", "--check"); err != nil || !strings.Contains(out, "mutakhir") {
		t.Fatalf("check sesudah generate: err=%v out=%s", err, out)
	}
	// katalog berubah -> basi lagi
	writeIn(t, dir, i18nUI+"/id.json", `{"save":"Simpan","load":"{n} muat"}`)
	writeIn(t, dir, i18nUI+"/en.json", `{"save":"Save","load":"{n} load"}`)
	if _, err = runI18n(t, "i18n:generate", "--check"); exitCodeOf(err) != 2 {
		t.Fatalf("exit=%d", exitCodeOf(err))
	}
	// katalog rusak -> exit 2; registri tidak ada -> exit 3
	writeIn(t, dir, i18nUI+"/en.json", `{rusak`)
	if _, err = runI18n(t, "i18n:generate"); exitCodeOf(err) != 2 {
		t.Fatalf("katalog rusak: exit=%d", exitCodeOf(err))
	}
	inRepo(t)
	if _, err = runI18n(t, "i18n:generate"); exitCodeOf(err) != 3 {
		t.Fatalf("tanpa registri: exit=%d", exitCodeOf(err))
	}
}
