//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/testsupport/pgtest"
	"erpora/internal/platform/testsupport/secretkey"
)

// Command CLI core (PRD-001 C1.12) pada binary RELEASE (bukan devtools): migrate up memasang core, lalu core:settings:set
// dan core:create-admin (sandi dari stdin) menyiapkan instalasi yang dapat login lewat API, persis urutan smoke CI.

const cliPassword = "kata-sandi-biner-cli-kuat-2026"

func postJSON(t *testing.T, url, body string) (*http.Response, string) {
	t.Helper()
	c := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestCoreCLI_ReleaseBinaryCreateAdminThenLoginViaAPI(t *testing.T) {
	dsn := pgtest.Start(t)
	keyEnv := []string{secretkey.Env()}
	run := func(stdin string, extraEnv []string, args ...string) (string, int) {
		return cliRunBin(t, erpBinRelease, dsn, append(append([]string{}, keyEnv...), extraEnv...), stdin, args...)
	}

	if out, code := run("", nil, "migrate", "up"); code != 0 {
		t.Fatalf("migrate up: exit %d\n%s", code, out)
	}
	// Command core terdaftar dan terpasang di binary release; migrate up memasang core (required) sehingga enabled.
	if out, code := run("", nil, "core:settings:set", "--key", "platform.public_url", "--value", "https://erp.example.com"); code != 0 {
		t.Fatalf("core:settings:set: exit %d\n%s", code, out)
	}
	allOut := ""
	out, code := run(cliPassword+"\n", nil, "core:create-admin", "--email", "Admin.Biner@Example.test", "--name", "Admin Biner")
	allOut += out
	if code != 0 {
		t.Fatalf("core:create-admin: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "admin.biner@example.test") {
		t.Errorf("keluaran = %q", out)
	}
	out, code = run("", nil, "core:rotate-keys")
	allOut += out
	if code != 0 || !strings.Contains(out, "pending") {
		t.Fatalf("core:rotate-keys: exit %d\n%s", code, out)
	}
	kid := ""
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "kunci baru: "); ok {
			kid = strings.TrimSpace(rest)
		}
	}
	if kid == "" {
		t.Fatalf("kid tidak terbaca dari keluaran:\n%s", out)
	}

	addr := freeAddr(t)
	p := startServeBin(t, erpBinRelease, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/readyz", 200, 30*time.Second)

	resp, body := postJSON(t, base+"/api/core/auth/login", `{"email":"admin.biner@example.test","password":"`+cliPassword+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d %s\n%s", resp.StatusCode, body, p.logs(t))
	}
	var lr struct {
		Data struct {
			AccessToken string `json:"accessToken"`
			MFAState    string `json:"mfaState"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &lr); err != nil || lr.Data.AccessToken == "" || lr.Data.MFAState != "enroll_required" {
		t.Fatalf("login body = %s (%v)", body, err)
	}
	if resp, _ := postJSON(t, base+"/api/core/auth/login", `{"email":"admin.biner@example.test","password":"salah-salah-salah"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sandi salah = %d", resp.StatusCode)
	}

	// Kunci pending hasil CLI terpublikasi di JWKS serve yang berjalan (belum menandatangani).
	_, jwks := getWith(t, base+"/.well-known/jwks.json", nil)
	if !strings.Contains(jwks, kid) {
		t.Errorf("kid %s tidak ada di JWKS: %s", kid, jwks)
	}

	// Sandi tidak pernah muncul di keluaran CLI maupun log serve (stdout dan stderr).
	_ = p.cmd.Process.Kill()
	_ = p.waitExit(t, 10*time.Second) // cmd.Wait selesai = goroutine penyalin stdout/stderr selesai; baru aman dibaca (-race)
	for name, s := range map[string]string{"cli": allOut, "serve stdout": p.stdout.String(), "serve stderr": p.stderr.String()} {
		if strings.Contains(s, cliPassword) {
			t.Errorf("sandi bocor di %s", name)
		}
	}
}

// Tanpa ERP_SECRET_KEY command core:* berhenti dengan exit 2 sebelum menyentuh data; di production http ditolak.
func TestCoreCLI_ReleaseBinaryGuards(t *testing.T) {
	dsn := pgtest.Start(t)
	if out, code := cliRunBin(t, erpBinRelease, dsn, []string{secretkey.Env()}, "", "migrate", "up"); code != 0 {
		t.Fatalf("migrate up: exit %d\n%s", code, out)
	}
	// os.Environ() proses uji tidak memuat ERP_SECRET_KEY (hanya Setenv tes tertentu), jadi tanpa extraEnv kunci tidak ada.
	out, code := cliRunBin(t, erpBinRelease, dsn, []string{"ERP_SECRET_KEY="}, cliPassword+"\n", "core:create-admin", "--email", "a@example.test")
	if code != 2 || !strings.Contains(out, "ERP_SECRET_KEY") {
		t.Fatalf("tanpa kunci: exit %d\n%s", code, out)
	}
	if strings.Contains(out, cliPassword) {
		t.Error("sandi bocor")
	}
	prod := []string{secretkey.Env(), "ERP_ENV=production"}
	out, code = cliRunBin(t, erpBinRelease, dsn, prod, "", "core:settings:set", "--key", "platform.public_url", "--value", "http://erp.example.com")
	if code == 0 || !strings.Contains(out, "https wajib") {
		t.Fatalf("http di production harus ditolak: exit %d\n%s", code, out)
	}
	out, code = cliRunBin(t, erpBinRelease, dsn, prod, "", "core:settings:set", "--key", "platform.public_url", "--value", "https://erp.example.com")
	if code != 0 {
		t.Fatalf("https di production: exit %d\n%s", code, out)
	}
}

// Urutan smoke CI (job smoke-linux/smoke-windows) pada binary release dan production: migrate up memasang core, lalu
// core:settings:set mengisi URL publik, lalu serve production (ERP_MAIL_DRIVER=log) siap (/readyz 200). Tanpa langkah
// core:settings:set, serve production gagal start (URL publik wajib, PRD-001 Q24).
func TestCoreCLI_ReleaseBinaryProductionSmokeSequence(t *testing.T) {
	dsn := pgtest.Start(t)
	prod := []string{secretkey.Env(), "ERP_ENV=production", "ERP_MAIL_DRIVER=log"}
	if out, code := cliRunBin(t, erpBinRelease, dsn, prod, "", "migrate", "up"); code != 0 {
		t.Fatalf("migrate up: exit %d\n%s", code, out)
	}

	// Tanpa URL publik: serve production berhenti dengan exit 2 dan pesan yang menyebut URL publik.
	addr := freeAddr(t)
	failing := exec.Command(erpBinRelease, "serve")
	failing.Env = append(baseEnv(dsn, addr), prod...)
	if out, err := failing.CombinedOutput(); err == nil || !strings.Contains(string(out), "URL publik") {
		t.Fatalf("serve production tanpa URL publik harus gagal menyebut URL publik: %v\n%s", err, out)
	}

	if out, code := cliRunBin(t, erpBinRelease, dsn, prod, "", "core:settings:set", "--key", "platform.public_url", "--value", "https://erp.example.com"); code != 0 {
		t.Fatalf("core:settings:set: exit %d\n%s", code, out)
	}
	addr = freeAddr(t)
	cmd := exec.Command(erpBinRelease, "serve")
	cmd.Env = append(baseEnv(dsn, addr), prod...)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	waitStatus(t, "http://"+addr+"/readyz", 200, 30*time.Second)
}
