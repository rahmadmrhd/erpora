// Package config memuat konfigurasi proses dari environment (opsional dengan env-file).
package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

// Env adalah mode lingkungan: "development" atau "production".
type Env string

const (
	EnvDevelopment Env = "development"
	EnvProduction  Env = "production"
)

// Config adalah konfigurasi proses yang sudah tervalidasi.
type Config struct {
	DatabaseURL     string
	Env             Env
	HTTPAddr        string
	LogFormat       string
	TenancyMode     string
	DefaultTenantID *uuid.UUID
	DBSlowQuery     time.Duration
	// AuthzStub adalah mode stub otorisasi sampai PRD-001: "allow" atau "deny".
	AuthzStub string
	// ModulesAdminAPI (ERP_MODULES_ADMIN_API) mengaktifkan endpoint mutasi /api/platform/modules di luar
	// development, sampai PRD-001 menyediakan autentikasi. Default false.
	ModulesAdminAPI bool
	// DBNPlusOneThreshold (ERP_DB_NPLUSONE_THRESHOLD, default 10, 0 = mati): ambang peringatan N+1 di development.
	DBNPlusOneThreshold int
	// DBStatementTimeout (ERP_DB_STATEMENT_TIMEOUT_MS, default 30000, valid 1000..600000) membatasi tiap statement
	// pada pool `erpora serve` (Q26 butir 7). CLI tidak memakainya.
	DBStatementTimeout time.Duration
}

// ModulesAdminAPIEnabled: endpoint mutasi modul aktif bila ERP_ENV=development atau ERP_MODULES_ADMIN_API=true.
func (c Config) ModulesAdminAPIEnabled() bool { return c.Env == EnvDevelopment || c.ModulesAdminAPI }

// ValidationError menyebut variabel environment yang tidak valid.
type ValidationError struct{ Var, Reason string }

func (e ValidationError) Error() string { return e.Var + ": " + e.Reason }

type raw struct {
	DatabaseURL     string `env:"ERP_DATABASE_URL"`
	Env             string `env:"ERP_ENV" envDefault:"production"`
	HTTPAddr        string `env:"ERP_HTTP_ADDR" envDefault:"127.0.0.1:8080"`
	LogFormat       string `env:"ERP_LOG_FORMAT"`
	TenancyMode     string `env:"ERP_TENANCY_MODE" envDefault:"single"`
	DefaultTenantID string `env:"ERP_DEFAULT_TENANT_ID"`
	DBSlowQueryMS   string `env:"ERP_DB_SLOW_QUERY_MS" envDefault:"200"`
	AuthzStub       string `env:"ERP_AUTHZ_STUB"`
	ModulesAdminAPI string `env:"ERP_MODULES_ADMIN_API"`
	NPlusOne        string `env:"ERP_DB_NPLUSONE_THRESHOLD" envDefault:"10"`
	StatementMS     string `env:"ERP_DB_STATEMENT_TIMEOUT_MS" envDefault:"30000"`
}

// Load membaca konfigurasi. envFile "" = tanpa file; environment proses menang atas isi file.
// Variabel OTEL_* dari file diteruskan ke environment proses (bila belum ada) agar terbaca SDK OpenTelemetry.
func Load(envFile string) (Config, error) {
	vars := map[string]string{}
	if envFile != "" {
		data, err := os.ReadFile(envFile) //nolint:gosec // path env-file diberikan operator lewat flag --env-file
		if err != nil {
			return Config{}, fmt.Errorf("baca env-file %q: %w", envFile, err)
		}
		data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
		parsed, err := godotenv.Unmarshal(string(data))
		if err != nil {
			// Teks error godotenv menyertakan potongan isi file (bisa memuat rahasia), jadi tidak diteruskan.
			return Config{}, fmt.Errorf("parse env-file %q: sintaks tidak valid (isi file tidak ditampilkan)", envFile)
		}
		for k, v := range parsed {
			vars[k] = v
			// SDK OpenTelemetry membaca OTEL_* langsung dari environment proses (endpoint, protokol, sampler,
			// header, timeout, dst.), jadi nilai dari file diteruskan ke proses bila belum ditetapkan di sana.
			if strings.HasPrefix(k, "OTEL_") {
				if _, set := os.LookupEnv(k); !set {
					if err := os.Setenv(k, v); err != nil {
						return Config{}, fmt.Errorf("set %s dari env-file: %w", k, err)
					}
				}
			}
		}
	}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			vars[k] = v
		}
	}

	var r raw
	if err := env.ParseWithOptions(&r, env.Options{Environment: vars}); err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}

	c := Config{
		DatabaseURL: r.DatabaseURL, Env: Env(r.Env), HTTPAddr: r.HTTPAddr,
		LogFormat: r.LogFormat, TenancyMode: r.TenancyMode,
	}
	if c.DatabaseURL == "" {
		return Config{}, ValidationError{"ERP_DATABASE_URL", "wajib diisi"}
	}
	if c.Env != EnvDevelopment && c.Env != EnvProduction {
		return Config{}, ValidationError{"ERP_ENV", "harus development atau production"}
	}
	if c.LogFormat == "" {
		c.LogFormat = "json"
		if c.Env == EnvDevelopment {
			c.LogFormat = "text"
		}
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return Config{}, ValidationError{"ERP_LOG_FORMAT", "harus json atau text"}
	}
	if c.TenancyMode != "single" {
		return Config{}, ValidationError{"ERP_TENANCY_MODE", "hanya mode single yang didukung"}
	}
	if r.DefaultTenantID != "" {
		id, err := uuid.Parse(r.DefaultTenantID)
		if err != nil {
			return Config{}, ValidationError{"ERP_DEFAULT_TENANT_ID", "bukan UUID valid"}
		}
		c.DefaultTenantID = &id
	}
	c.AuthzStub = r.AuthzStub
	if c.AuthzStub == "" {
		c.AuthzStub = "deny"
		if c.Env == EnvDevelopment {
			c.AuthzStub = "allow"
		}
	}
	if c.AuthzStub != "allow" && c.AuthzStub != "deny" {
		return Config{}, ValidationError{"ERP_AUTHZ_STUB", "harus allow atau deny"}
	}
	if c.AuthzStub == "allow" && c.Env == EnvProduction {
		return Config{}, ValidationError{"ERP_AUTHZ_STUB", "stub allow dilarang saat ERP_ENV=production: autentikasi dan otorisasi nyata baru tersedia lewat PRD-001, jadi gunakan deny (atau development untuk lokal)"}
	}
	switch r.ModulesAdminAPI {
	case "", "false":
	case "true":
		c.ModulesAdminAPI = true
	default:
		return Config{}, ValidationError{"ERP_MODULES_ADMIN_API", "harus true atau false"}
	}
	var ms int
	if _, err := fmt.Sscanf(r.DBSlowQueryMS, "%d", &ms); err != nil || ms < 0 || fmt.Sprint(ms) != strings.TrimSpace(r.DBSlowQueryMS) {
		return Config{}, ValidationError{"ERP_DB_SLOW_QUERY_MS", "harus bilangan bulat >= 0 (milidetik)"}
	}
	c.DBSlowQuery = time.Duration(ms) * time.Millisecond
	var np int
	if _, err := fmt.Sscanf(r.NPlusOne, "%d", &np); err != nil || np < 0 || fmt.Sprint(np) != strings.TrimSpace(r.NPlusOne) {
		return Config{}, ValidationError{"ERP_DB_NPLUSONE_THRESHOLD", "harus bilangan bulat >= 0 (0 = mati)"}
	}
	c.DBNPlusOneThreshold = np
	var st int
	if _, err := fmt.Sscanf(r.StatementMS, "%d", &st); err != nil || fmt.Sprint(st) != strings.TrimSpace(r.StatementMS) || st < 1000 || st > 600000 {
		return Config{}, ValidationError{"ERP_DB_STATEMENT_TIMEOUT_MS", "harus bilangan bulat 1000 sampai 600000 (milidetik)"}
	}
	c.DBStatementTimeout = time.Duration(st) * time.Millisecond
	return c, nil
}
