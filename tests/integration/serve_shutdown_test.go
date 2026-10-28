//go:build integration && !windows

package integration

import (
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"erpora/internal/platform/testsupport/pgtest"
)

// FR-03: Shutdown menunggu request AKTIF (handler masih berjalan) selesai. Memakai route lambat yang
// hanya ada pada biner bertag `integration` (internal/app/testroutes_integration.go).
func TestServe_GracefulShutdown(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	addr := freeAddr(t)
	p := startServe(t, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/readyz", 200, 20*time.Second)

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		c := http.Client{Timeout: 20 * time.Second}
		resp, err := c.Get(base + "/__test/slow?ms=3000")
		if err != nil {
			done <- result{err: err}
			return
		}
		resp.Body.Close()
		done <- result{code: resp.StatusCode}
	}()

	time.Sleep(700 * time.Millisecond) // pastikan handler sudah berjalan di server
	sent := time.Now()
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	// Listener ditutup segera saat shutdown dimulai: koneksi baru harus ditolak.
	refused := false
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err != nil {
			refused = true
			break
		}
		conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	if !refused {
		t.Error("koneksi baru masih diterima setelah shutdown dimulai")
	}

	select {
	case r := <-done:
		if r.err != nil || r.code != 200 {
			t.Fatalf("request yang sedang berjalan harus selesai 200, got code=%d err=%v", r.code, r.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("request yang sedang berjalan tidak selesai")
	}
	if err := p.waitExit(t, 25*time.Second); err != nil {
		t.Fatalf("exit: %v (stderr: %s)", err, strings.TrimSpace(p.stderr.String()))
	}
	if d := time.Since(sent); d > 25*time.Second {
		t.Fatalf("shutdown melewati batas 25 detik: %v", d)
	}
}

// Sinyal kedua saat graceful shutdown tertahan (request lambat) harus mengakhiri proses segera,
// jauh sebelum timeout graceful 25 detik (Q26 butir 6).
func TestServe_SecondSignalTerminatesDuringShutdown(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	addr := freeAddr(t)
	p := startServe(t, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/readyz", 200, 20*time.Second)

	go func() { // request lambat menahan shutdown; hasilnya tidak relevan (koneksi diputus saat proses mati)
		c := http.Client{Timeout: 30 * time.Second}
		if resp, err := c.Get(base + "/__test/slow?ms=20000"); err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(700 * time.Millisecond) // pastikan handler sudah berjalan

	start := time.Now()
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond) // shutdown anggun sudah dimulai dan tertahan
	select {
	case <-p.done:
		t.Fatal("proses berhenti sebelum sinyal kedua padahal request lambat masih berjalan")
	default:
	}
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// Berakhir karena sinyal (handler default), bukan keluar bersih; harus jauh di bawah 25 detik.
	if err := p.waitExit(t, 5*time.Second); err == nil {
		t.Fatal("proses seharusnya berakhir karena sinyal kedua (exit non-nol)")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("sinyal kedua tidak segera mengakhiri proses: %v", d)
	}
}
