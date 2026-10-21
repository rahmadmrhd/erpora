//go:build integration && !windows

package integration

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rahmadmrhd/erp-go/internal/platform/testsupport/pgtest"
)

// Request yang sedang berjalan (header baru terkirim sebagian) harus selesai saat SIGINT.
func TestServe_GracefulShutdown(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	addr := freeAddr(t)
	p := startServe(t, dsn, addr)
	waitStatus(t, "http://"+addr+"/readyz", 200, 20*time.Second)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // pastikan koneksi berstatus aktif di server
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := conn.Write([]byte("\r\n")); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if err := p.waitExit(t, 30*time.Second); err != nil {
		t.Fatalf("exit: %v (stderr: %s)", err, strings.TrimSpace(p.stderr.String()))
	}
}
