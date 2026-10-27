//go:build integration

package integration

import (
	"strings"
	"testing"

	"erpora/internal/platform/testsupport/pgtest"
)

// prodEnv: ERP_ENV=production wajib berpasangan dengan stub deny (config menolak allow di production).
var prodEnv = []string{"ERP_ENV=production", "ERP_AUTHZ_STUB=deny"}

// Review Focus 4: command tulis binary dev ditolak (exit 3) pada DB berpenanda production, tanpa mengubah data;
// command read-only tetap berjalan; binary release tidak terpengaruh.
func TestDevBinary_ProductionMarker(t *testing.T) {
	dsn := pgtest.Start(t)

	// 1. Binary release menulis penanda production.
	if out, code := cliRunBin(t, erpBinRelease, dsn, prodEnv, "", "migrate", "up"); code != 0 {
		t.Fatalf("release migrate up: exit %d\n%s", code, out)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM platform_state WHERE key = 'environment' AND value = 'production'"); n != 1 {
		t.Fatalf("penanda production tidak tertulis (n=%d)", n)
	}
	installs := dbQuery(t, dsn, "SELECT count(*) FROM module_installations")
	ops := dbQuery(t, dsn, "SELECT count(*) FROM module_operations")

	// 2. Binary dev (ERP_ENV=development, kasus salah-konfigurasi paling berbahaya) menolak semua command tulis.
	addr := freeAddr(t)
	writes := [][]string{
		{"serve"},
		{"migrate", "up"},
		{"modules", "enable", "acme.example"},
		{"example:recount"},
	}
	for _, args := range writes {
		cmd := append([]string{}, args...)
		out, code := cliRunBin(t, erpBin, dsn, []string{"ERP_HTTP_ADDR=" + addr}, "", cmd...)
		if code != 3 {
			t.Errorf("dev %s: exit %d, want 3\n%s", strings.Join(args, " "), code, out)
		}
		if !strings.Contains(out, "berpenanda production") {
			t.Errorf("dev %s: keluaran tidak memuat \"berpenanda production\":\n%s", strings.Join(args, " "), out)
		}
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM module_installations"); n != installs {
		t.Errorf("module_installations berubah: %d -> %d", installs, n)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM module_operations"); n != ops {
		t.Errorf("module_operations berubah: %d -> %d", ops, n)
	}

	// 3. Command read-only tetap berjalan.
	mustCLI(t, dsn, 0, "inspect", "routes")
	mustCLI(t, dsn, 0, "modules", "list")

	// 4. Binary release tidak terpengaruh penanda.
	if out, code := cliRunBin(t, erpBinRelease, dsn, nil, "", "modules", "install", "acme.example"); code != 0 {
		t.Fatalf("release modules install: exit %d\n%s", code, out)
	}
}

// Keputusan 4: DB belum berpenanda + ERP_ENV=production pada binary dev tidak boleh menginisialisasi DB.
func TestDevBinary_UnmarkedProductionRefused(t *testing.T) {
	dsn := pgtest.Start(t)
	out, code := cliRunBin(t, erpBin, dsn, prodEnv, "", "migrate", "up")
	if code != 3 {
		t.Fatalf("exit %d, want 3\n%s", code, out)
	}
	if n := dbQuery(t, dsn, "SELECT count(*) FROM (SELECT to_regclass('platform_state') AS r) x WHERE r IS NULL"); n != 1 {
		t.Fatal("platform_state tidak boleh terbentuk")
	}
}

func TestInspectConfig_ShowsDatabaseEnvironment(t *testing.T) {
	dsn := pgtest.Start(t)
	migrateUp(t, dsn)
	out := mustCLI(t, dsn, 0, "inspect", "config")
	if !strings.Contains(out, "database.environment=development") {
		t.Errorf("database.environment tidak tampil:\n%s", out)
	}

	// Tanpa DB: tetap exit 0, nilai unknown tanpa membocorkan DSN.
	dead := "postgres://erp:rahasia@127.0.0.1:1/erp?sslmode=disable&connect_timeout=1"
	out, code := cliRun(t, dead, nil, "", "inspect", "config")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "database.environment=unknown (") {
		t.Errorf("want database.environment=unknown (...):\n%s", out)
	}
	if strings.Contains(out, "rahasia") {
		t.Errorf("password bocor:\n%s", out)
	}
}
