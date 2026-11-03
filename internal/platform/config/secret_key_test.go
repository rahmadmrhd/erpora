package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/platform/crypto"
)

// skKey membuat kunci uji 32 byte acak: (base64 standar, byte mentah).
func skKey(t *testing.T) (string, []byte) {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(k), k
}

// skEnv menyiapkan environment minimal untuk Load dengan ERP_ENV dan ERP_SECRET_KEY tertentu.
func skEnv(t *testing.T, envName, key string) {
	t.Helper()
	clearAuthzEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ENV", envName)
	t.Setenv("ERP_AUTHZ_STUB", "deny")
	t.Setenv("ERP_SECRET_KEY", key)
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", "")
	os.Unsetenv("ERP_SECRET_KEY_PREVIOUS")
}

// assertKeyHint: petunjuk pembuatan kunci harus bisa dijalankan di binary release (command secret:generate hanya ada di
// build pengembang), jadi wajib menyebut alternatif openssl dan PowerShell CSPRNG, dan rujukan dokumen (keputusan
// pemilik produk 3 Nov 2026, ADR-0013 §3).
func assertKeyHint(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	for _, want := range []string{
		"erpora secret:generate", "hanya build pengembang", "openssl rand -base64 32",
		// Bentuk yang jalan di Windows PowerShell 5.1 DAN 7+ (GetBytes(32) statis baru ada di .NET 6, gagal di 5.1).
		"$b = New-Object byte[] 32; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)",
		"docs/operations/deployment.md",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("petunjuk harus memuat %q: %v", want, err)
		}
	}
	if strings.Contains(msg, "]::GetBytes(32)") {
		t.Errorf("petunjuk tidak boleh memakai GetBytes(32) statis (tidak ada di Windows PowerShell 5.1): %v", err)
	}
	// Get-Random bukan CSPRNG: hanya boleh muncul sebagai larangan, bukan sebagai resep.
	if strings.Contains(msg, "Get-Random -Max") || strings.Contains(msg, "| % {") {
		t.Errorf("petunjuk tidak boleh menyarankan Get-Random sebagai sumber kunci: %v", err)
	}
}

func TestConfigSecretKeyValid(t *testing.T) {
	b64, raw := skKey(t)
	for _, envName := range []string{"production", "development"} {
		t.Run(envName, func(t *testing.T) {
			skEnv(t, envName, b64)
			c, err := Load("")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(c.SecretKey, raw) {
				t.Error("kunci tidak terurai benar")
			}
		})
	}
}

func TestConfigSecretKeyTrimsWhitespace(t *testing.T) {
	b64, raw := skKey(t)
	skEnv(t, "production", "  "+b64+"\n")
	c, err := Load("")
	if err != nil || !bytes.Equal(c.SecretKey, raw) {
		t.Fatalf("len = %d err = %v", len(c.SecretKey), err)
	}
}

// Terisi tetapi tidak valid menggagalkan Load di semua mode (format selalu divalidasi), tanpa mencetak nilainya.
func TestConfigSecretKeyInvalidFailsStartup(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	long := base64.StdEncoding.EncodeToString(make([]byte, 33))
	std := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32)) // memuat '+' dan '/'
	vals := map[string]string{
		"bukan base64":    "bukan-base64!!!-rahasia",
		"terlalu pendek":  short,
		"terlalu panjang": long,
		"url-safe":        base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32)),
		"tanpa padding":   strings.TrimRight(std, "="),
		"spasi di tengah": std[:10] + " " + std[10:],
	}
	for _, envName := range []string{"production", "development"} {
		for _, varName := range []string{"ERP_SECRET_KEY", "ERP_SECRET_KEY_PREVIOUS"} {
			for name, val := range vals {
				t.Run(envName+"/"+varName+"/"+name, func(t *testing.T) {
					b64, _ := skKey(t)
					skEnv(t, envName, b64)
					t.Setenv(varName, val)
					_, err := Load("")
					wantVar(t, err, varName)
					if strings.Contains(err.Error(), val) {
						t.Errorf("pesan galat memuat nilai kunci: %v", err)
					}
					if varName == "ERP_SECRET_KEY" {
						assertKeyHint(t, err)
						return
					}
					// ERP_SECRET_KEY_PREVIOUS: pesan menjelaskan rotasi, bukan cara membuat kunci aktif.
					if !strings.Contains(err.Error(), "rotasi") || strings.Contains(err.Error(), "lalu isi ERP_SECRET_KEY") || strings.Contains(err.Error(), "secret:generate") {
						t.Errorf("pesan PREVIOUS harus menjelaskan rotasi tanpa petunjuk membuat kunci aktif: %v", err)
					}
				})
			}
		}
	}
}

// Kosong TIDAK menggagalkan Load (migrate, inspect, doctor, dll. tidak butuh kunci); hanya RequireSecretKey yang menolak.
func TestConfigSecretKeyEmptyLoadsButRequireFails(t *testing.T) {
	for _, envName := range []string{"production", "development"} {
		t.Run(envName, func(t *testing.T) {
			skEnv(t, envName, "")
			c, err := Load("")
			if err != nil {
				t.Fatalf("Load tanpa kunci harus lolos: %v", err)
			}
			if len(c.SecretKey) != 0 || len(c.SecretKeyPrevious) != 0 {
				t.Errorf("tidak boleh ada kunci otomatis: %d/%d", len(c.SecretKey), len(c.SecretKeyPrevious))
			}
			err = c.RequireSecretKey()
			wantVar(t, err, "ERP_SECRET_KEY")
			assertKeyHint(t, err)
			if !strings.Contains(err.Error(), "serve") {
				t.Errorf("pesan harus menyebut serve: %v", err)
			}
		})
	}
}

func TestConfigRequireSecretKeyPassesWhenSet(t *testing.T) {
	b64, _ := skKey(t)
	skEnv(t, "development", b64)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RequireSecretKey(); err != nil {
		t.Errorf("RequireSecretKey: %v", err)
	}
}

// ERP_SECRET_KEY_PREVIOUS tanpa kunci aktif tidak bermakna: RequireSecretKey menolak.
func TestConfigPreviousWithoutActiveFailsRequire(t *testing.T) {
	prev, _ := skKey(t)
	skEnv(t, "production", "")
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", prev)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	wantVar(t, c.RequireSecretKey(), "ERP_SECRET_KEY")
}

func TestConfigSecretKeyPreviousParsed(t *testing.T) {
	active, activeRaw := skKey(t)
	prev, prevRaw := skKey(t)
	skEnv(t, "production", active)
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", " "+prev+" ")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c.SecretKey, activeRaw) || !bytes.Equal(c.SecretKeyPrevious, prevRaw) {
		t.Error("kunci aktif dan lama tidak terurai benar")
	}
}

// Kunci lama yang sama dengan kunci aktif adalah salah konfigurasi.
func TestConfigPreviousEqualToActiveRejected(t *testing.T) {
	b64, _ := skKey(t)
	skEnv(t, "production", b64)
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", b64)
	_, err := Load("")
	wantVar(t, err, "ERP_SECRET_KEY_PREVIOUS")
	if !strings.Contains(err.Error(), "rotasi") || strings.Contains(err.Error(), "lalu isi ERP_SECRET_KEY") {
		t.Errorf("pesan harus menjelaskan rotasi: %v", err)
	}
}

// Id kunci pada header segel = 1 byte hash material; dua kunci berbeda yang kebetulan berid sama (1/256) harus ditolak
// saat startup dengan pesan yang jelas, bukan baru saat Sealer dibangun.
func TestConfigPreviousWithCollidingKeyIDRejected(t *testing.T) {
	activeB64, active := skKey(t)
	var prevB64 string
	for i := 0; ; i++ {
		if i > 100000 {
			t.Fatal("tidak menemukan kunci dengan id yang sama")
		}
		b64, raw := skKey(t)
		if !bytes.Equal(raw, active) && crypto.KeyIDOf(raw) == crypto.KeyIDOf(active) {
			prevB64 = b64
			break
		}
	}
	skEnv(t, "production", activeB64)
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", prevB64)
	_, err := Load("")
	wantVar(t, err, "ERP_SECRET_KEY_PREVIOUS")
	if !strings.Contains(err.Error(), "openssl rand -base64 32") || !strings.Contains(err.Error(), "hanya build pengembang") {
		t.Errorf("saran kunci baru harus bisa dijalankan di binary release: %v", err)
	}
	if !strings.Contains(err.Error(), "rotasi") || !strings.Contains(err.Error(), "ganti ERP_SECRET_KEY") || strings.Contains(err.Error(), prevB64) || strings.Contains(err.Error(), activeB64) {
		t.Errorf("pesan harus menjelaskan rotasi dan menyarankan mengganti kunci aktif, tanpa memuat nilai: %v", err)
	}
}

// Bila hanya kunci yang bermasalah, Config tetap terisi (DSN dll.) agar `erpora doctor` dapat memeriksa PostgreSQL.
func TestConfigSecretKeyErrorStillReturnsRestOfConfig(t *testing.T) {
	skEnv(t, "development", "bukan-base64!!!")
	c, err := Load("")
	wantVar(t, err, "ERP_SECRET_KEY")
	if c.DatabaseURL != "postgres://u:p@h/db" || c.Env != EnvDevelopment {
		t.Errorf("Config parsial tidak terisi: %+v", c)
	}
}

func TestConfigSecretKeyFromEnvFile(t *testing.T) {
	b64, raw := skKey(t)
	skEnv(t, "", "")
	os.Unsetenv("ERP_ENV")
	os.Unsetenv("ERP_SECRET_KEY")
	f := filepath.Join(t.TempDir(), ".env")
	body := "ERP_DATABASE_URL=postgres://u:p@h/db\nERP_AUTHZ_STUB=deny\nERP_SECRET_KEY=" + b64 + "\n"
	if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(f)
	if err != nil || !bytes.Equal(c.SecretKey, raw) {
		t.Fatalf("len = %d err = %v", len(c.SecretKey), err)
	}
}

// TestRedactHidesSecretKey: kunci tidak muncul lewat RedactSecret, fmt (kunci maupun Config utuh), JSON, maupun slog.
func TestRedactHidesSecretKey(t *testing.T) {
	b64, raw := skKey(t)
	prevB64, prevRaw := skKey(t)
	skEnv(t, "production", b64)
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", prevB64)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	js, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	jsKey, _ := json.Marshal(c.SecretKey)
	var tb, jb bytes.Buffer
	slog.New(slog.NewTextHandler(&tb, nil)).Info("cfg", "cfg", c, "key", c.SecretKey)
	slog.New(slog.NewJSONHandler(&jb, nil)).Info("cfg", "cfg", c, "key", c.SecretKey)
	outs := map[string]string{
		"key %v": fmt.Sprintf("%v", c.SecretKey), "key %+v": fmt.Sprintf("%+v", c.SecretKey),
		"key %#v": fmt.Sprintf("%#v", c.SecretKey), "key %s": fmt.Sprintf("%s", c.SecretKey),
		"key %x": fmt.Sprintf("%x", c.SecretKey), "key %d": fmt.Sprintf("%d", c.SecretKey),
		"cfg %v": fmt.Sprintf("%v", c), "cfg %+v": fmt.Sprintf("%+v", c), "cfg %#v": fmt.Sprintf("%#v", c),
		"RedactSecret": RedactSecret(c.SecretKey), "json cfg": string(js), "json key": string(jsKey),
		"slog text": tb.String(), "slog json": jb.String(),
	}
	leaks := []string{b64, string(raw), fmt.Sprintf("%x", raw), fmt.Sprintf("%v", raw), prevB64, string(prevRaw), fmt.Sprintf("%x", prevRaw)}
	for name, out := range outs {
		for _, leak := range leaks {
			if strings.Contains(out, leak) {
				t.Errorf("%s membocorkan kunci: %s", name, out)
			}
		}
	}
	if RedactSecret(c.SecretKey) != "****" || RedactSecret(nil) != "" {
		t.Errorf("RedactSecret = %q / %q", RedactSecret(c.SecretKey), RedactSecret(nil))
	}
}
