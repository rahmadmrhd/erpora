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
