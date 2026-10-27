//go:build integration

package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql untuk memeriksa DB dari test

	"erpora/internal/platform/testsupport/pgtest"
)

// cliRun menjalankan `erpora <args>` dan mengembalikan keluaran gabungan (stdout+stderr) serta exit code.
func cliRun(t *testing.T, dsn string, extraEnv []string, stdin string, args ...string) (string, int) {
	t.Helper()
	return cliRunBin(t, erpBin, dsn, extraEnv, stdin, args...)
}

func cliRunBin(t *testing.T, bin, dsn string, extraEnv []string, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(baseEnv(dsn, "127.0.0.1:0"), extraEnv...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), exitCodeOf(t, err)
}

func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var xe *exec.ExitError
	if errors.As(err, &xe) {
		return xe.ExitCode()
	}
	t.Fatalf("gagal menjalankan perintah: %v", err)
	return -1
}

func mustCLI(t *testing.T, dsn string, want int, args ...string) string {
	t.Helper()
	out, code := cliRun(t, dsn, nil, "", args...)
	if code != want {
		t.Fatalf("erpora %s: exit %d, want %d\n%s", strings.Join(args, " "), code, want, out)
	}
	return out
}

func dbQuery(t *testing.T, dsn, query string, args ...any) int64 {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func bootstrapState(t *testing.T, base string) (revision int64, etag string, status map[string]string) {
	t.Helper()
	resp, body := getWith(t, base+"/api/bootstrap", nil)
	var b struct {
		Data struct {
			Modules  []struct{ Key, Status string }
			Revision int64
		}
	}
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		t.Fatalf("bootstrap %q: %v", body, err)
	}
	status = map[string]string{}
	for _, m := range b.Data.Modules {
		status[m.Key] = m.Status
	}
	return b.Data.Revision, resp.Header.Get("ETag"), status
}

func waitCode(t *testing.T, url string, want int, d time.Duration) {
	t.Helper()
	waitStatusJSON(t, url, want, d)
}

// waitStatusJSON seperti waitStatus tetapi tanpa header Accept: text/html (endpoint API).
func waitStatusJSON(t *testing.T, url string, want int, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	last := 0
	for time.Now().Before(deadline) {
		resp, body := getWith(t, url, nil)
		last = resp.StatusCode
		if last == want {
			return body
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("%s tidak mencapai status %d dalam %v (terakhir %d)", url, want, d, last)
	return ""
}

// Exit criteria M2b: kedua modul referensi dikelola penuh lewat CLI sambil `serve` berjalan (propagasi lintas proses).
func TestCLI_ManageBothModules(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)

	out := mustCLI(t, dsn, 0, "modules", "list")
	for _, key := range []string{"acme.example", "acme.example-addon"} {
		if !regexp.MustCompile(regexp.QuoteMeta(key) + `\s+\S+\s+-\s+available`).MatchString(out) {
			t.Fatalf("%s harus available:\n%s", key, out)
		}
	}
	plan := mustCLI(t, dsn, 0, "modules", "plan", "install", "acme.example-addon", "--with-deps")
	i, j := strings.Index(plan, "acme.example  install"), strings.Index(plan, "acme.example-addon  install")
	if i < 0 || j < 0 || i > j {
		t.Fatalf("plan harus memuat acme.example sebelum acme.example-addon:\n%s", plan)
	}

	mustCLI(t, dsn, 0, "modules", "install", "acme.example-addon", "--with-deps", "--enable")

	addr := freeAddr(t)
	startServe(t, dsn, addr)
	base := "http://" + addr
	waitStatus(t, base+"/readyz", 200, 20*time.Second)
	waitCode(t, base+"/api/example-addon/summary", 200, 5*time.Second)
	rev0, etag0, st := bootstrapState(t, base)
	if st["acme.example"] != "enabled" || st["acme.example-addon"] != "enabled" || rev0 == 0 {
		t.Fatalf("bootstrap awal: rev=%d %v", rev0, st)
	}

	out = mustCLI(t, dsn, 3, "modules", "disable", "acme.example")
	if !strings.Contains(out, "dependents_active") || !strings.Contains(out, "acme.example-addon") {
		t.Fatalf("disable example harus ditolak dengan dependents_active:\n%s", out)
	}
	out = mustCLI(t, dsn, 0, "example:recount")
	if !strings.Contains(out, "notes: 3") {
		t.Fatalf("recount:\n%s", out)
	}

	mustCLI(t, dsn, 0, "modules", "disable", "acme.example-addon")
	waitCode(t, base+"/api/example-addon/summary", 403, 6*time.Second) // watcher lintas proses (<= 3 x 2s)

	mustCLI(t, dsn, 0, "modules", "disable", "acme.example")
	out = mustCLI(t, dsn, 3, "example:recount")
	if !strings.Contains(out, "disabled") {
		t.Fatalf("recount saat disabled harus menyebut disabled:\n%s", out)
	}

	// Dependen yang masih ter-install (walau disabled) menahan uninstall (keputusan 15): uninstall addon lebih dulu.
	if out := mustCLI(t, dsn, 3, "modules", "uninstall", "acme.example", "--yes"); !strings.Contains(out, "dependents_active") {
		t.Fatalf("uninstall example saat addon ter-install: %s", out)
	}
	mustCLI(t, dsn, 0, "modules", "uninstall", "acme.example-addon", "--yes")
	mustCLI(t, dsn, 0, "modules", "uninstall", "acme.example", "--yes")
	if h := mustCLI(t, dsn, 0, "modules", "history", "acme.example"); !strings.Contains(h, "uninstalled") {
		t.Fatalf("history:\n%s", h)
	}
	mustCLI(t, dsn, 0, "modules", "install", "acme.example")
	mustCLI(t, dsn, 0, "modules", "enable", "acme.example")
	if out := mustCLI(t, dsn, 0, "modules", "enable", "acme.example"); !strings.Contains(out, "tidak ada perubahan") {
		t.Fatalf("enable kedua harus no-op:\n%s", out)
	}

	deadline := time.Now().Add(6 * time.Second)
	for {
		rev, etag, st := bootstrapState(t, base)
		if st["acme.example"] == "enabled" && rev > rev0 && etag != etag0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bootstrap tidak mengikuti CLI: rev %d -> %d, etag %s -> %s, %v", rev0, rev, etag0, etag, st)
		}
		time.Sleep(150 * time.Millisecond)
	}
	if out := mustCLI(t, dsn, 0, "migrate", "status"); !strings.Contains(out, "module:acme.example") {
		t.Fatalf("migrate status tanpa baris modul:\n%s", out)
	}
}

func TestCLI_ExitCodes(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  int
	}{
		{"key tidak dikenal", "", []string{"modules", "enable", "acme.nope"}, 2},
		{"retry operasi tidak ada", "", []string{"modules", "retry", "00000000-0000-0000-0000-000000000000"}, 2},
		{"retry bukan uuid", "", []string{"modules", "retry", "bukan-uuid"}, 2},
		{"tenant lain", "", []string{"modules", "--tenant", uuid.NewString(), "list"}, 2},
		{"uninstall tanpa --yes dan stdin kosong", "", []string{"modules", "uninstall", "acme.example"}, 2},
		{"transisi tidak sah", "", []string{"modules", "enable", "acme.example"}, 3},
	}
	for _, tc := range cases {
		out, code := cliRun(t, dsn, nil, tc.stdin, tc.args...)
		if code != tc.want {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, code, tc.want, out)
		}
	}
}

// Review Focus 2 + AC "Crash": proses mati setelah langkah migrate.
func TestCLI_CrashAfterMigrateThenRetry(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)

	out, code := cliRun(t, dsn, []string{"ERP_TEST_CRASH_AFTER_STEP=migrate"}, "", "modules", "install", "acme.example")
	if code != 99 {
		t.Fatalf("install crash: exit %d, want 99\n%s", code, out)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM module_operations WHERE status = 'running'"); n != 1 {
		t.Fatalf("operasi running = %d, want 1", n)
	}
	migsBefore := dbQuery(t, dsn, "SELECT count(*) FROM module_migrations")

	// Startup serve berikutnya menandai operasi terputus.
	addr := freeAddr(t)
	p := startServe(t, dsn, addr)
	waitStatus(t, "http://"+addr+"/readyz", 200, 20*time.Second)
	_ = p.cmd.Process.Kill()
	_ = p.waitExit(t, 10*time.Second)

	status := mustCLI(t, dsn, 0, "modules", "status", "acme.example")
	if !strings.Contains(status, "status: failed") || !strings.Contains(status, "interrupted") {
		t.Fatalf("status setelah crash:\n%s", status)
	}
	m := regexp.MustCompile(`last operation: ([0-9a-f-]{36})`).FindStringSubmatch(status)
	if m == nil {
		t.Fatalf("tidak ada ID operasi terakhir:\n%s", status)
	}
	mustCLI(t, dsn, 0, "modules", "retry", m[1])
	if s := mustCLI(t, dsn, 0, "modules", "status", "acme.example"); !strings.Contains(s, "status: installed") {
		t.Fatalf("status setelah retry:\n%s", s)
	}
	if v := dbQuery(t, dsn, "SELECT max(version_id) FROM goose_module_example WHERE is_applied"); v != 2 {
		t.Errorf("versi goose = %d, want 2", v)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM module_migrations"); n != migsBefore {
		t.Errorf("module_migrations menggandakan baris: %d -> %d", migsBefore, n)
	}
}

// Review Focus 1 + AC "Konkurensi": dua proses enable bersamaan.
func TestCLI_ConcurrentEnableTwoProcesses(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	mustCLI(t, dsn, 0, "modules", "install", "acme.example")

	var wg sync.WaitGroup
	var outA string
	var codeA int
	wg.Add(1)
	go func() {
		defer wg.Done()
		outA, codeA = cliRun(t, dsn, []string{"ERP_TEST_STEP_DELAY=hook:enable:7s"}, "", "modules", "enable", "acme.example")
	}()
	// Tunggu sampai proses A memegang lock (operasi running tercatat).
	deadline := time.Now().Add(20 * time.Second)
	for dbQuery(t, dsn, "SELECT count(*) FROM module_operations WHERE status = 'running'") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("proses A tidak pernah memulai operasi")
		}
		time.Sleep(100 * time.Millisecond)
	}
	outB, codeB := cliRun(t, dsn, nil, "", "modules", "enable", "acme.example")
	wg.Wait()

	if codeA != 0 {
		t.Errorf("proses A: exit %d\n%s", codeA, outA)
	}
	if codeB != 3 || !strings.Contains(outB, "operation_in_progress") {
		t.Errorf("proses B: exit %d, want 3 + operation_in_progress\n%s", codeB, outB)
	}
	if s := mustCLI(t, dsn, 0, "modules", "status", "acme.example"); !strings.Contains(s, "status: enabled") {
		t.Errorf("status akhir:\n%s", s)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM module_lifecycle_events WHERE module_key = 'acme.example' AND event = 'enabled'"); n != 1 {
		t.Errorf("event enabled = %d, want 1", n)
	}
}
