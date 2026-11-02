package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestLaunchConfigServeHasSecretKeySource: konfigurasi pratinjau (.claude/launch.json) yang menjalankan `serve` harus
// mendapat ERP_SECRET_KEY dari sumber yang tidak terlacak (--env-file ke berkas .env yang di-gitignore), karena serve
// wajib berkunci di semua mode. Kunci tetap tidak boleh ditanam di berkas terlacak.
func TestLaunchConfigServeHasSecretKeySource(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".claude", "launch.json")) //nolint:gosec // berkas repo
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Configurations []struct {
			Name        string   `json:"name"`
			RuntimeArgs []string `json:"runtimeArgs"`
		} `json:"configurations"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	literalKey := regexp.MustCompile(`ERP_SECRET_KEY\s*=\s*'?"?[A-Za-z0-9+/]{43}=`)
	serveCmd := regexp.MustCompile(`erpora[\w.-]*(\.exe)?\s+serve\b`)
	var checked int
	for _, c := range cfg.Configurations {
		joined := strings.Join(c.RuntimeArgs, " ")
		if literalKey.MatchString(joined) {
			t.Errorf("konfigurasi %q menanam ERP_SECRET_KEY tetap di berkas terlacak", c.Name)
		}
		if !serveCmd.MatchString(joined) {
			continue
		}
		checked++
		if !strings.Contains(joined, "--env-file") {
			t.Errorf("konfigurasi %q menjalankan serve tanpa --env-file (ERP_SECRET_KEY tidak tersedia, serve exit 2)", c.Name)
		}
	}
	if checked == 0 {
		t.Error("tidak ada konfigurasi yang menjalankan serve; tes ini perlu diperbarui")
	}
}
