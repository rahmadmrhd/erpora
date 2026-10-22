//go:build integration

package integration

import (
	"bytes"
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

var erpBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "erpora-it-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	name := "erpora"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	erpBin = filepath.Join(dir, name)
	build := exec.Command("go", "build", "-tags", "devtools,integration", "-o", erpBin, "../../cmd/erpora")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build erp:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func baseEnv(dsn, addr string) []string {
	env := os.Environ()
	return append(env,
		"ERP_DATABASE_URL="+dsn, "ERP_HTTP_ADDR="+addr, "ERP_ENV=development",
		"ERP_LOG_FORMAT=text", "OTEL_EXPORTER_OTLP_ENDPOINT=",
	)
}

func migrateUp(t *testing.T, dsn string) {
	t.Helper()
	cmd := exec.Command(erpBin, "migrate", "up")
	cmd.Env = baseEnv(dsn, "127.0.0.1:0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("migrate up: %v\n%s", err, out)
	}
}

type proc struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	done   chan error
}

func startServe(t *testing.T, dsn, addr string) *proc {
	t.Helper()
	return startServeBin(t, erpBin, dsn, addr)
}

func startServeBin(t *testing.T, bin, dsn, addr string) *proc {
	t.Helper()
	cmd := exec.Command(bin, "serve")
	cmd.Env = baseEnv(dsn, addr)
	p := &proc{cmd: cmd, stderr: &bytes.Buffer{}, done: make(chan error, 1)}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = cmd.Process.Kill()
			<-p.done
		}
	})
	return p
}

func (p *proc) waitExit(t *testing.T, d time.Duration) error {
	t.Helper()
	select {
	case err := <-p.done:
		p.done <- err
		return err
	case <-time.After(d):
		t.Fatalf("proses tidak keluar dalam %v; stderr:\n%s", d, p.stderr)
		return nil
	}
}

func get(url string) (int, string, error) {
	c := http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "text/html")
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

func waitStatus(t *testing.T, url string, want int, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	var last string
	for time.Now().Before(deadline) {
		code, body, err := get(url)
		if err == nil && code == want {
			return body
		}
		last = fmt.Sprintf("code=%d err=%v", code, err)
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("%s tidak mencapai status %d dalam %v (terakhir: %s)", url, want, d, last)
	return ""
}

func TestServe_HealthAndReady(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	addr := freeAddr(t)
	startServe(t, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/healthz", 200, 20*time.Second)
	waitStatus(t, base+"/readyz", 200, 5*time.Second)
	body := waitStatus(t, base+"/", 200, 5*time.Second)
	if !strings.Contains(strings.ToLower(body), "<html") {
		t.Fatalf("/ bukan HTML: %s", body)
	}
}

func TestServe_UnmigratedDatabase(t *testing.T) {
	dsn := pgtest.Start(t)
	p := startServe(t, dsn, freeAddr(t))
	if err := p.waitExit(t, 20*time.Second); err == nil {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(p.stderr.String(), "erpora migrate up") {
		t.Fatalf("stderr = %s", p.stderr)
	}
}

func TestServe_DatabaseUnreachable(t *testing.T) {
	closed := freeAddr(t)
	dsn := "postgres://erp:rahasia@" + closed + "/erp?sslmode=disable"
	start := time.Now()
	p := startServe(t, dsn, freeAddr(t))
	if err := p.waitExit(t, 10*time.Second); err == nil {
		t.Fatal("expected non-zero exit")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("too slow")
	}
	s := p.stderr.String()
	if !strings.Contains(s, "host=") || strings.Contains(s, "rahasia") {
		t.Fatalf("stderr = %s", s)
	}
}

func TestServe_ReadyzFailsWhenDatabaseStops(t *testing.T) {
	dsn, stop := pgtest.StartWithStop(t)
	migrateUp(t, dsn)
	addr := freeAddr(t)
	startServe(t, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/readyz", 200, 20*time.Second)
	stop()
	waitStatus(t, base+"/readyz", 503, 10*time.Second)
	waitStatus(t, base+"/healthz", 200, 3*time.Second)
}

func TestServe_PortInUse(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	addr := l.Addr().String()
	p := startServe(t, dsn, addr)
	if err := p.waitExit(t, 20*time.Second); err == nil {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(p.stderr.String(), addr) {
		t.Fatalf("stderr = %s", p.stderr)
	}
}

// Route uji (/__test/slow) hanya boleh ada pada biner bertag `integration`, tidak pada biner normal.
func TestServe_NormalBinaryHasNoTestRoute(t *testing.T) {
	name := "erpora-plain"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-tags", "devtools", "-o", bin, "../../cmd/erpora")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatal(err)
	}
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	addr := freeAddr(t)
	startServeBin(t, bin, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/readyz", 200, 20*time.Second)
	// Tanpa route uji, path ini jatuh ke fallback SPA; yang penting bukan respons route lambat.
	_, body, err := get(base + "/__test/slow?ms=10")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "slow-done") {
		t.Fatalf("biner normal memuat route uji: %s", body)
	}
}
