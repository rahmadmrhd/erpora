package commands

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runSecretGenerate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newTestRoot(t)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"secret:generate"}, args...))
	err := root.Execute()
	return out.String(), err
}

// Keluaran: satu baris base64 standar 32 byte acak (setara `openssl rand -base64 32`), berbeda tiap pemanggilan.
func TestSecretGenerate_PrintsFreshBase64Key(t *testing.T) {
	a, err := runSecretGenerate(t)
	if err != nil {
		t.Fatal(err)
	}
	b, err := runSecretGenerate(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{a, b} {
		if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
			t.Fatalf("harus tepat satu baris: %q", out)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(out, "\n"))
		if err != nil || len(raw) != 32 {
			t.Fatalf("bukan base64 standar 32 byte: len=%d err=%v", len(raw), err)
		}
	}
	if a == b {
		t.Error("tiap pemanggilan harus menghasilkan kunci baru")
	}
}

// Command tidak menyimpan apa pun (tanpa menulis .env) dan tidak menerima argumen.
func TestSecretGenerate_WritesNothingAndTakesNoArgs(t *testing.T) {
	dir := inRepo(t)
	if _, err := runSecretGenerate(t); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "go.work" && e.Name() != "modules" {
			t.Errorf("berkas tak terduga dibuat: %s", filepath.Join(dir, e.Name()))
		}
	}
	if _, err := runSecretGenerate(t, "extra"); err == nil {
		t.Error("argumen berlebih harus ditolak")
	}
}

// Berjalan di luar repo (tidak memanggil repoRoot) dan tanpa konfigurasi/DB.
func TestSecretGenerate_WorksOutsideRepoWithoutConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ERP_DATABASE_URL", "")
	if _, err := runSecretGenerate(t); err != nil {
		t.Fatal(err)
	}
}

// Peringatan CI: keluaran command mencetak rahasia ke log; Long harus menyebut ::add-mask:: dan pola openssl di ci.yml.
func TestSecretGenerate_HelpWarnsAboutCI(t *testing.T) {
	cmd := newSecretGenerate()
	for _, want := range []string{"CI", "::add-mask::", "openssl rand -base64 32", "ci.yml"} {
		if !strings.Contains(cmd.Long, want) {
			t.Errorf("Long harus memuat %q:\n%s", want, cmd.Long)
		}
	}
}
