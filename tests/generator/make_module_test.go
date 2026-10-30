//go:build generator

// Package generator membuktikan bahwa modul hasil `erpora make:*` lolos semua gate kualitas dan dapat di-install
// (PRD-000 FR-22 AC, exit gate M2). Test berjalan di salinan repo sementara; repo asli tidak disentuh.
package generator

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/testsupport/pgtest"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// workDir membuat direktori kerja di <root>/tmp (sudah di-.gitignore dan dilewati copyRepo), bukan di TEMP sistem:
// path TEMP Windows sering memuat spasi atau bentuk 8.3 (RAHMAD~1), yang merusak Taskfile (`for` memecah path pada spasi)
// dan lockfile Bun (path workspace dihitung dari realpath). Dihapus saat test selesai (kegagalan hapus diabaikan).
func workDir(t *testing.T, root string) string {
	t.Helper()
	base := filepath.Join(root, "tmp")
	if err := os.MkdirAll(base, 0o750); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "generator-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestMakeModule_PassesAllGates(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	repo := filepath.Join(workDir(t, root), "repo")
	copyRepo(t, root, repo)
	// Test yang butuh Bun wajib berjalan (keputusan 22): CI dan test ini tidak boleh men-skip diam-diam.
	env := []string{"ERP_REQUIRE_BUN=1"}

	// 1. Dependensi frontend terkunci, lalu bangun binary dev dari salinan.
	run(t, repo, env, "bun", "install", "--frozen-lockfile")
	binName := "erpora"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(workDir(t, root), binName)
	build := func() {
		t.Helper()
		run(t, repo, env, "go", "build", "-tags", "devtools", "-o", bin, "./cmd/erpora")
	}
	build()

	// 2. --dry-run: menampilkan rencana tetapi tidak menulis apa pun.
	sum0 := treeChecksum(t, repo)
	out := run(t, repo, env, bin, "make:module", "inventory", "--frontend", "--depends", "acme.example", "--dry-run")
	for _, want := range []string{"create modules/inventory/go.mod", "modify go.work"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run harus memuat %q:\n%s", want, out)
		}
	}
	if sum := treeChecksum(t, repo); sum != sum0 {
		t.Fatal("--dry-run mengubah pohon file")
	}

	// 3. Pembuatan sungguhan; mengulang tanpa --force ditolak dan tidak mengubah apa pun.
	run(t, repo, env, bin, "make:module", "inventory", "--frontend", "--depends", "acme.example")
	sum1 := treeChecksum(t, repo)
	out, code := runResult(t, repo, env, bin, "make:module", "inventory", "--frontend", "--depends", "acme.example")
	if code == 0 || !strings.Contains(out, "sudah ada") {
		t.Fatalf("make:module ulang tanpa --force: exit %d, keluaran harus memuat \"sudah ada\":\n%s", code, out)
	}
	if sum := treeChecksum(t, repo); sum != sum1 {
		t.Fatal("make:module yang ditolak mengubah pohon file")
	}

	// 4. Modul bernama dengan tanda hubung, lalu satu handler tambahan (route + kontrak + capability).
	run(t, repo, env, bin, "make:module", "stock-opname")
	run(t, repo, env, bin, "make:handler", "inventory", "Approve",
		"--route", "POST /items/{id}/approve", "--permission", "inventory.items.approve")

	// 5. Binary dibangun ulang (internal/app/modules.go berubah), lalu setiap gate harus lolos.
	build()
	run(t, repo, env, "task", "lint")
	run(t, repo, env, "task", "arch")
	run(t, repo, env, "task", "check:gowork-off")
	run(t, repo, env, "task", "test")
	for _, m := range []string{"inventory", "stock-opname"} {
		out := run(t, repo, env, bin, "module:check", m)
		if !strings.Contains(out, "OK") {
			t.Errorf("module:check %s: %s", m, out)
		}
	}
	run(t, repo, env, bin, "contracts:lint")
	run(t, repo, env, bin, "contracts:generate", "--check")
	// i18n: katalog modul hasil generate valid (tanpa galat maupun peringatan) dan keys.gen.ts mutakhir (i18n 8.6, M3a 13.4).
	run(t, repo, env, bin, "i18n:check", "--strict")
	run(t, repo, env, bin, "i18n:generate", "--check")
	run(t, repo, env, "bun", "install", "--frozen-lockfile")
	run(t, repo, env, "bun", "run", "--filter", "*", "typecheck")
	run(t, repo, env, "bun", "run", "--filter", "*", "lint")
	run(t, repo, env, "bun", "run", "--filter", "@erpora/web", "deps")
	// Test frontend dari root (seperti web:check): memuat module.test.ts modul hasil generate (M3a R15.4), lalu build
	// shell yang kini merakit frontendModule modul baru (anggaran bundle dan csp-check).
	run(t, repo, env, "bun", "test", "--pass-with-no-tests")
	run(t, repo, env, "bun", "run", "--filter", "@erpora/web", "build")

	// 6. Install, enable, command CLI, dan serve terhadap PostgreSQL sungguhan.
	dsn := pgtest.Start(t)
	addr := freeAddr(t)
	dbEnv := append(env, "ERP_DATABASE_URL="+dsn, "ERP_HTTP_ADDR="+addr, "ERP_ENV=development", "ERP_LOG_FORMAT=text", "OTEL_EXPORTER_OTLP_ENDPOINT=")
	run(t, repo, dbEnv, bin, "migrate", "up")
	run(t, repo, dbEnv, bin, "modules", "install", "acme.inventory", "--with-deps", "--enable")
	run(t, repo, dbEnv, bin, "modules", "install", "acme.stock-opname", "--enable")

	var status struct {
		Status string `json:"status"`
	}
	// Keluaran gabungan stdout+stderr: banner dev ada di stderr, jadi JSON dimulai dari "{" pertama.
	statusOut := run(t, repo, dbEnv, bin, "modules", "status", "acme.inventory", "--json")
	if i := strings.Index(statusOut, "{"); i >= 0 {
		statusOut = statusOut[i:]
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusOut)
	}
	if status.Status != "enabled" {
		t.Fatalf("status acme.inventory = %q, want enabled", status.Status)
	}
	if out := run(t, repo, dbEnv, bin, "inventory:recount"); !strings.Contains(out, "items:") {
		t.Fatalf("inventory:recount harus mencetak items:\n%s", out)
	}

	srv := exec.Command(bin, "serve")
	srv.Dir = repo
	srv.Env = append(os.Environ(), dbEnv...)
	var srvOut strings.Builder
	srv.Stdout, srv.Stderr = &srvOut, &srvOut
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Wait() }()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = srv.Process.Kill()
			<-done
		}
	})
	base := "http://" + addr
	waitOK(t, base+"/readyz", 30*time.Second, &srvOut)
	resp, err := http.Get(base + "/api/stock-opname/items") //nolint:gosec,noctx // alamat lokal acak milik test
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/stock-opname/items = %d: %s\nserver:\n%s", resp.StatusCode, body, srvOut.String())
	}
	var env2 map[string]json.RawMessage
	if err := json.Unmarshal(body, &env2); err != nil {
		t.Fatalf("body bukan JSON: %s", body)
	}
	for _, k := range []string{"data", "page"} {
		if _, ok := env2[k]; !ok {
			t.Errorf("amplop harus memuat %q: %s", k, body)
		}
	}
	t.Logf("total durasi: %s", time.Since(started).Round(time.Second))
}

func waitOK(t *testing.T, url string, d time.Duration, srvOut fmt.Stringer) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec,noctx // alamat lokal acak milik test
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s tidak siap dalam %s\nserver:\n%s", url, d, srvOut.String())
}
