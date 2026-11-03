// Package config memuat konfigurasi proses dari environment (opsional dengan env-file).
package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"erpora/internal/platform/crypto"
	"erpora/internal/platform/identity/password"
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
	// SecretKey (ERP_SECRET_KEY, 32 byte base64) menyegel rahasia saat diam lewat crypto.Sealer. Format divalidasi bila
	// terisi; kehadirannya diwajibkan oleh RequireSecretKey (serve dan jalur penyegel, semua mode termasuk development).
	// Buat dengan `erpora secret:generate` (build pengembang) atau `openssl rand -base64 32`. Nilainya tidak pernah tercetak.
	SecretKey Secret
	// SecretKeyPrevious (ERP_SECRET_KEY_PREVIOUS, opsional) adalah kunci lama saat rotasi: hanya untuk membuka data
	// tersegel sebelumnya (id kunci pada header segel memilih kunci). Kosong (nil) bila tidak sedang rotasi; pakai
	// crypto.NewSealerFromMaterial(SecretKey, SecretKeyPrevious), yang melewati entri kosong.
	SecretKeyPrevious Secret
	// Argon2 adalah parameter hash sandi argon2id (ERP_ARGON2_MEMORY_KIB, ERP_ARGON2_TIME, ERP_ARGON2_PARALLELISM). Default
	// m=19456 KiB, t=2, p=1 mengikuti OWASP Password Storage Cheat Sheet (konfigurasi minimum argon2id; alternatif setara:
	// m=46 MiB t=1 p=1); batas bawah/atas divalidasi password.Params.ValidateConfig (konfigurasi tidak boleh melemahkan hash
	// di bawah minimum OWASP). Hash lama dengan parameter berbeda tetap terverifikasi dan di-hash ulang saat login (needsRehash).
	Argon2 password.Params
	// Argon2MaxConcurrent (ERP_ARGON2_MAX_CONCURRENT, 1..64, default min(GOMAXPROCS, 4)) membatasi jumlah hash argon2id
	// (Hash, Verify, dan verifikasi tiruan) yang berjalan serentak dalam satu proses; permintaan berikutnya menunggu
	// sebentar lalu dijawab 429 rate_limited. Memori terburuk = batas x ERP_ARGON2_MEMORY_KIB (default 19 MiB per hash).
	Argon2MaxConcurrent int
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
	SecretKey       string `env:"ERP_SECRET_KEY"`
	SecretKeyPrev   string `env:"ERP_SECRET_KEY_PREVIOUS"`
	Argon2Memory    string `env:"ERP_ARGON2_MEMORY_KIB"`
	Argon2Time      string `env:"ERP_ARGON2_TIME"`
	Argon2Parallel  string `env:"ERP_ARGON2_PARALLELISM"`
	Argon2MaxConc   string `env:"ERP_ARGON2_MAX_CONCURRENT"`
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
	if err := loadArgon2(&c, r.Argon2Memory, r.Argon2Time, r.Argon2Parallel); err != nil {
		return Config{}, err
	}
	if err := loadArgon2MaxConcurrent(&c, r.Argon2MaxConc); err != nil {
		return Config{}, err
	}
	if err := loadSecretKeys(&c, r.SecretKey, r.SecretKeyPrev); err != nil {
		// Config yang sudah terisi (tanpa kunci) tetap dikembalikan bersama galat ini agar `erpora doctor` masih
		// dapat memeriksa PostgreSQL dan melaporkan kunci sebagai temuan tersendiri. Pemanggil lain wajib menghentikan proses.
		return c, err
	}
	return c, nil
}

// keyCreationHint adalah petunjuk pembuatan kunci pada pesan galat; tidak pernah memuat nilai apa pun. Command
// `secret:generate` hanya ada di build pengembang (tag devtools), jadi alternatif untuk binary release disertakan
// (keputusan pemilik produk 3 Nov 2026, ADR-0013 §3). Get-Random PowerShell bukan CSPRNG dan tidak boleh disarankan.
const keyCreationHint = "buat 32 byte acak base64 satu kali dengan salah satu: `erpora secret:generate` (hanya build pengembang), " +
	"`openssl rand -base64 32`, atau PowerShell (Windows PowerShell 5.1 dan 7+) `$b = New-Object byte[] 32; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)` " +
	"(jangan Get-Random: bukan CSPRNG); lalu isi ERP_SECRET_KEY. Rincian di docs/operations/deployment.md"

// previousKeyHint menjelaskan ERP_SECRET_KEY_PREVIOUS (rotasi); tidak pernah memuat nilai apa pun.
const previousKeyHint = "ERP_SECRET_KEY_PREVIOUS hanya dipakai saat rotasi kunci: isi dengan nilai ERP_SECRET_KEY LAMA persis (32 byte base64) atau kosongkan bila tidak sedang rotasi"

// parseSecretKeyVar menguraikan satu variabel kunci (base64 standar, tepat 32 byte). Kosong = tidak diset (nil, nil).
// Pesan galat tidak pernah memuat nilainya.
func parseSecretKeyVar(name, rawKey string) (Secret, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, nil
	}
	// Petunjuk menyesuaikan variabel: kunci aktif dibuat dengan secret:generate; kunci lama adalah nilai aktif sebelumnya.
	hint := keyCreationHint
	if name == "ERP_SECRET_KEY_PREVIOUS" {
		hint = previousKeyHint
	}
	key, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil {
		return nil, ValidationError{name, "bukan base64 standar yang valid (nilai tidak ditampilkan); " + hint}
	}
	if len(key) != 32 {
		return nil, ValidationError{name, fmt.Sprintf("harus tepat 32 byte setelah base64-decode (diterima %d byte; nilai tidak ditampilkan); %s", len(key), hint)}
	}
	return key, nil
}

// loadSecretKeys menguraikan ERP_SECRET_KEY dan ERP_SECRET_KEY_PREVIOUS. Format SELALU divalidasi bila terisi (semua
// mode); kehadiran kunci TIDAK diwajibkan di sini: hanya `serve` dan jalur penyegel yang memanggil RequireSecretKey,
// sehingga migrate, inspect, doctor, modules, dan perintah lain berjalan tanpa kunci.
func loadSecretKeys(c *Config, rawActive, rawPrevious string) error {
	active, err := parseSecretKeyVar("ERP_SECRET_KEY", rawActive)
	if err != nil {
		return err
	}
	previous, err := parseSecretKeyVar("ERP_SECRET_KEY_PREVIOUS", rawPrevious)
	if err != nil {
		return err
	}
	if active != nil && previous != nil && bytes.Equal(active, previous) {
		return ValidationError{"ERP_SECRET_KEY_PREVIOUS", "tidak boleh sama dengan ERP_SECRET_KEY: kunci lama hanya dipakai saat rotasi untuk membuka data tersegel sebelum rotasi; kosongkan bila tidak sedang rotasi"}
	}
	if active != nil && previous != nil && crypto.KeyIDOf(active) == crypto.KeyIDOf(previous) {
		// Id kunci (1 byte hash material) bertabrakan, peluang 1/256: header segel tak dapat membedakan keduanya.
		return ValidationError{"ERP_SECRET_KEY_PREVIOUS", "id kunci pada header segel bertabrakan dengan ERP_SECRET_KEY (peluang 1/256, bukan kunci yang sama) sehingga rotasi tidak dapat membedakan keduanya: ganti ERP_SECRET_KEY dengan kunci baru (`erpora secret:generate`, hanya build pengembang, atau `openssl rand -base64 32`; lihat docs/operations/deployment.md) lalu ulangi rotasi"}
	}
	c.SecretKey, c.SecretKeyPrevious = active, previous
	return nil
}

// RequireSecretKey memastikan ERP_SECRET_KEY terisi. Dipanggil `serve` dan command yang menyegel atau membuka data
// (NFR-104 PRD-001, ADR-0013): wajib di semua mode termasuk development, tanpa kunci sementara otomatis. Format sudah
// divalidasi oleh Load. Pesan tidak memuat nilai.
func (c Config) RequireSecretKey() error {
	if len(c.SecretKey) == 0 {
		return ValidationError{"ERP_SECRET_KEY", "wajib diisi untuk serve dan command yang menyegel data, di semua mode termasuk development (32 byte acak base64); " + keyCreationHint}
	}
	return nil
}

// parseArgon2Var menguraikan satu variabel argon2: bilangan bulat desimal tanpa tanda, <= limit (pemeriksaan sebelum
// konversi tipe agar nilai besar tidak melingkar). Kosong = default def.
func parseArgon2Var(name, s string, def, limit uint64) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v > limit {
		return 0, ValidationError{name, fmt.Sprintf("harus bilangan bulat desimal tanpa tanda, paling besar %d", limit)}
	}
	return v, nil
}

// loadArgon2 mengisi c.Argon2 dari ERP_ARGON2_* (kosong = default OWASP) dan memvalidasinya. Nilai di luar batas wajar
// ditolak (bukan dipotong): parameter terlalu kecil melemahkan hash, terlalu besar membuat login rentan DoS memori/CPU.
func loadArgon2(c *Config, rawMemory, rawTime, rawParallel string) error {
	def := password.DefaultParams()
	m, err := parseArgon2Var("ERP_ARGON2_MEMORY_KIB", rawMemory, uint64(def.MemoryKiB), math.MaxUint32)
	if err != nil {
		return err
	}
	t, err := parseArgon2Var("ERP_ARGON2_TIME", rawTime, uint64(def.Time), math.MaxUint32)
	if err != nil {
		return err
	}
	p, err := parseArgon2Var("ERP_ARGON2_PARALLELISM", rawParallel, uint64(def.Parallelism), math.MaxUint8)
	if err != nil {
		return err
	}
	// Nilai di atas batas atas ditolak sebelum konversi (cegah pelingkaran uint8/uint32 menjadi nilai sah).
	if m > password.MaxMemoryKiB {
		return ValidationError{"ERP_ARGON2_MEMORY_KIB", fmt.Sprintf("maksimum %d KiB", password.MaxMemoryKiB)}
	}
	if t > password.MaxTime {
		return ValidationError{"ERP_ARGON2_TIME", fmt.Sprintf("maksimum %d", password.MaxTime)}
	}
	if p > password.MaxParallelism {
		return ValidationError{"ERP_ARGON2_PARALLELISM", fmt.Sprintf("maksimum %d", password.MaxParallelism)}
	}
	params := password.Params{MemoryKiB: uint32(m), Time: uint32(t), Parallelism: uint8(p)}
	if err := params.ValidateConfig(); err != nil {
		var pe *password.ParamError
		if errors.As(err, &pe) {
			v := map[string]string{"memory": "ERP_ARGON2_MEMORY_KIB", "time": "ERP_ARGON2_TIME", "parallelism": "ERP_ARGON2_PARALLELISM", "combination": "ERP_ARGON2_TIME"}[pe.Field]
			return ValidationError{v, pe.Reason}
		}
		return ValidationError{"ERP_ARGON2_MEMORY_KIB", err.Error()}
	}
	c.Argon2 = params
	return nil
}

// maxArgon2MaxConcurrent adalah batas atas ERP_ARGON2_MAX_CONCURRENT: di atas ini memori terburuk (batas x memori per
// hash) tidak lagi masuk akal untuk satu proses.
const maxArgon2MaxConcurrent = 64

// DefaultArgon2MaxConcurrent adalah batas hash serentak bawaan: min(GOMAXPROCS, 4), minimal 1. Argon2id terikat CPU dan
// memori; lebih dari jumlah inti tidak mempercepat apa pun dan 4 hash serentak pada parameter OWASP minimum (19 MiB)
// hanya memakai sekitar 76 MiB.
func DefaultArgon2MaxConcurrent() int { return min(max(runtime.GOMAXPROCS(0), 1), 4) }

// loadArgon2MaxConcurrent mengisi c.Argon2MaxConcurrent dari ERP_ARGON2_MAX_CONCURRENT (kosong = default).
func loadArgon2MaxConcurrent(c *Config, rawVal string) error {
	rawVal = strings.TrimSpace(rawVal)
	if rawVal == "" {
		c.Argon2MaxConcurrent = DefaultArgon2MaxConcurrent()
		return nil
	}
	v, err := strconv.ParseUint(rawVal, 10, 64)
	if err != nil || v < 1 || v > maxArgon2MaxConcurrent {
		return ValidationError{"ERP_ARGON2_MAX_CONCURRENT", fmt.Sprintf("harus bilangan bulat desimal 1..%d", maxArgon2MaxConcurrent)}
	}
	c.Argon2MaxConcurrent = int(v)
	return nil
}
