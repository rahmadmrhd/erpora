package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clearEnv mengosongkan variabel ERP_* agar test deterministik.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ERP_DATABASE_URL", "ERP_ENV", "ERP_HTTP_ADDR", "ERP_LOG_FORMAT",
		"ERP_TENANCY_MODE", "ERP_DEFAULT_TENANT_ID", "ERP_DB_SLOW_QUERY_MS", "ERP_MODULES_ADMIN_API", "ERP_DB_NPLUSONE_THRESHOLD", "ERP_DB_STATEMENT_TIMEOUT_MS",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func wantVar(t *testing.T, err error, v string) {
	t.Helper()
	var ve ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if ve.Var != v {
		t.Fatalf("Var = %q, want %q", ve.Var, v)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearEnv(t)
	_, err := Load("")
	wantVar(t, err, "ERP_DATABASE_URL")
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "production" || c.HTTPAddr != "127.0.0.1:8080" || c.LogFormat != "json" ||
		c.TenancyMode != "single" || c.DBSlowQuery != 200*time.Millisecond || c.DefaultTenantID != nil {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestLoad_DevelopmentDefaultsToTextLogs(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ENV", "development")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.LogFormat != "text" {
		t.Fatalf("LogFormat = %q", c.LogFormat)
	}
}

func TestLoad_InvalidEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ENV", "staging")
	_, err := Load("")
	wantVar(t, err, "ERP_ENV")
}

func TestLoad_TenancyModeMulti(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_TENANCY_MODE", "multi")
	_, err := Load("")
	wantVar(t, err, "ERP_TENANCY_MODE")
}

func TestLoad_InvalidDefaultTenantID(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_DEFAULT_TENANT_ID", "not-a-uuid")
	_, err := Load("")
	wantVar(t, err, "ERP_DEFAULT_TENANT_ID")
}

func TestLoad_ValidDefaultTenantIDAndSlowQuery(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_DEFAULT_TENANT_ID", "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	t.Setenv("ERP_DB_SLOW_QUERY_MS", "50")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultTenantID == nil || c.DefaultTenantID.String() != "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b" {
		t.Fatalf("tenant = %v", c.DefaultTenantID)
	}
	if c.DBSlowQuery != 50*time.Millisecond {
		t.Fatalf("slow = %v", c.DBSlowQuery)
	}
	t.Setenv("ERP_DB_SLOW_QUERY_MS", "abc")
	_, err = Load("")
	wantVar(t, err, "ERP_DB_SLOW_QUERY_MS")
}

func TestLoad_EnvFileCRLFAndBOM(t *testing.T) {
	clearEnv(t)
	p := filepath.Join(t.TempDir(), ".env")
	body := "\xEF\xBB\xBF" + "ERP_DATABASE_URL=postgres://u:p@h/db\r\nERP_ENV=development\r\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.DatabaseURL != "postgres://u:p@h/db" || c.Env != "development" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_ProcessEnvOverridesFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_HTTP_ADDR", ":9000")
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte("ERP_DATABASE_URL=postgres://u:p@h/db\nERP_HTTP_ADDR=:7000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q", c.HTTPAddr)
	}
}

func TestLoad_MissingEnvFile(t *testing.T) {
	clearEnv(t)
	if _, err := Load(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Fatal("expected error for missing env file")
	}
}

func TestRedactDSN_SpecialCharacters(t *testing.T) {
	out := RedactDSN("postgres://erp:p%40ss%3Aw%2Frd%25@db.local:5432/erp?sslmode=disable")
	if strings.Contains(out, "p%40ss") || strings.Contains(out, "p@ss") {
		t.Fatalf("password leaked: %s", out)
	}
	if !strings.Contains(out, "erp:****@db.local:5432/erp") {
		t.Fatalf("unexpected: %s", out)
	}
	if RedactDSN("postgres://%zz") != "<invalid dsn>" {
		t.Fatal("invalid dsn not handled")
	}
}

func TestLoad_OTelVarsFromEnvFileReachProcessEnv(t *testing.T) {
	clearEnv(t)
	for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "OTEL_TRACES_SAMPLER"} {
		t.Setenv(k, "") // pulihkan nilai awal saat test selesai
		os.Unsetenv(k)
	}
	t.Setenv("OTEL_SERVICE_NAME", "dari-proses") // environment proses menang atas file
	p := filepath.Join(t.TempDir(), ".env")
	body := "ERP_DATABASE_URL=postgres://u:p@h/db\nOTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318\nOTEL_SERVICE_NAME=dari-file\nOTEL_TRACES_SAMPLER=always_off\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); got != "http://127.0.0.1:4318" {
		t.Errorf("endpoint = %q", got)
	}
	if got := os.Getenv("OTEL_TRACES_SAMPLER"); got != "always_off" {
		t.Errorf("sampler = %q", got)
	}
	if got := os.Getenv("OTEL_SERVICE_NAME"); got != "dari-proses" {
		t.Errorf("service name = %q (proses harus menang)", got)
	}
}

// security-guidelines §5: error parse env-file tidak boleh memuat isi file (mis. password DSN).
func TestLoad_EnvFileParseErrorDoesNotLeakSecrets(t *testing.T) {
	cases := map[string]string{
		"karakter tak valid pada nama variabel": "ERP_DATABASE_URL=postgres://u:rahasia123@h/db\nBAD KEY$=x rahasia123\n",
		"nilai berkutip tidak ditutup":          "ERP_DATABASE_URL=postgres://u:rahasia123@h/db\nERP_X=\"rahasia123 tanpa penutup\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			p := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(p)
			if err == nil {
				t.Fatal("expected parse error")
			}
			if strings.Contains(err.Error(), "rahasia123") {
				t.Fatalf("error membocorkan isi file: %v", err)
			}
			if !strings.Contains(err.Error(), "parse env-file") {
				t.Fatalf("error tidak menyebut file: %v", err)
			}
		})
	}
}

func TestLoad_ModulesAdminAPI(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")

	t.Setenv("ERP_MODULES_ADMIN_API", "yes")
	_, err := Load("")
	wantVar(t, err, "ERP_MODULES_ADMIN_API")

	t.Setenv("ERP_ENV", "production")
	t.Setenv("ERP_MODULES_ADMIN_API", "")
	c, err := Load("")
	if err != nil || c.ModulesAdminAPI || c.ModulesAdminAPIEnabled() {
		t.Fatalf("production default: %+v, %v", c, err)
	}

	t.Setenv("ERP_MODULES_ADMIN_API", "true")
	c, err = Load("")
	if err != nil || !c.ModulesAdminAPI || !c.ModulesAdminAPIEnabled() {
		t.Fatalf("production + true: %+v, %v", c, err)
	}

	t.Setenv("ERP_MODULES_ADMIN_API", "false")
	c, err = Load("")
	if err != nil || c.ModulesAdminAPIEnabled() {
		t.Fatalf("production + false: %+v, %v", c, err)
	}

	t.Setenv("ERP_ENV", "development")
	t.Setenv("ERP_MODULES_ADMIN_API", "")
	c, err = Load("")
	if err != nil || c.ModulesAdminAPI || !c.ModulesAdminAPIEnabled() {
		t.Fatalf("development: %+v, %v", c, err)
	}
}

func TestLoad_DBNPlusOneThreshold(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	c, err := Load("")
	if err != nil || c.DBNPlusOneThreshold != 10 {
		t.Fatalf("default: %+v, %v", c, err)
	}
	t.Setenv("ERP_DB_NPLUSONE_THRESHOLD", "0")
	if c, err = Load(""); err != nil || c.DBNPlusOneThreshold != 0 {
		t.Fatalf("0 (mati): %+v, %v", c, err)
	}
	t.Setenv("ERP_DB_NPLUSONE_THRESHOLD", "25")
	if c, err = Load(""); err != nil || c.DBNPlusOneThreshold != 25 {
		t.Fatalf("25: %+v, %v", c, err)
	}
	for _, bad := range []string{"-1", "abc", "1.5"} {
		t.Setenv("ERP_DB_NPLUSONE_THRESHOLD", bad)
		_, err := Load("")
		wantVar(t, err, "ERP_DB_NPLUSONE_THRESHOLD")
	}
}
