package commands

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/testsupport/secretkey"
)

func loadWith(t *testing.T, env map[string]string) (config.Config, error) {
	t.Helper()
	for _, k := range []string{"ERP_DATABASE_URL", "ERP_ENV", "ERP_SECRET_KEY", "ERP_AUTHZ_STUB"} {
		t.Setenv(k, "")
	}
	t.Setenv("ERP_AUTHZ_STUB", "deny")
	for k, v := range env {
		t.Setenv(k, v)
	}
	return config.Load("")
}

// Kunci tidak ada: DSN tetap diteruskan (postgres tetap diperiksa) dan kunci menjadi temuan sendiri.
func TestDoctorInputs_MissingSecretKeyKeepsDSN(t *testing.T) {
	const dsn = "postgres://u:p@h/db"
	cfg, err := loadWith(t, map[string]string{"ERP_DATABASE_URL": dsn, "ERP_ENV": "development"})
	in := doctorInputs(cfg, err)
	if in.DSN != dsn || !in.SecretKeyChecked || in.ConfigErr != "" {
		t.Fatalf("DSN harus tetap ada: %+v", in)
	}
	if !strings.Contains(in.SecretKeyErr, "ERP_SECRET_KEY") || !strings.Contains(in.SecretKeyErr, "secret:generate") {
		t.Errorf("SecretKeyErr = %q", in.SecretKeyErr)
	}
}

func TestDoctorInputs_InvalidSecretKeyDoesNotLeakValue(t *testing.T) {
	const bad = "bukan-base64-rahasia!!!"
	cfg, err := loadWith(t, map[string]string{"ERP_DATABASE_URL": "postgres://u:p@h/db", "ERP_SECRET_KEY": bad})
	in := doctorInputs(cfg, err)
	if in.SecretKeyErr == "" || strings.Contains(in.SecretKeyErr, bad) {
		t.Errorf("SecretKeyErr = %q", in.SecretKeyErr)
	}
}

func TestDoctorInputs_ValidAndOtherErrors(t *testing.T) {
	cfg, err := loadWith(t, map[string]string{"ERP_DATABASE_URL": "postgres://u:p@h/db", "ERP_SECRET_KEY": secretkey.B64()})
	if in := doctorInputs(cfg, err); in.DSN == "" || !in.SecretKeyChecked || in.SecretKeyErr != "" {
		t.Errorf("valid: %+v", in)
	}
	// DSN belum diatur: kunci belum dinilai.
	cfg, err = loadWith(t, map[string]string{"ERP_SECRET_KEY": secretkey.B64()})
	if in := doctorInputs(cfg, err); in.DSN != "" || in.SecretKeyChecked || in.ConfigErr != "" {
		t.Errorf("tanpa DSN: %+v", in)
	}
	// Galat lain ditampilkan apa adanya.
	cfg, err = loadWith(t, map[string]string{"ERP_DATABASE_URL": "postgres://u:p@h/db", "ERP_ENV": "staging"})
	if in := doctorInputs(cfg, err); !strings.Contains(in.ConfigErr, "ERP_ENV") {
		t.Errorf("galat lain: %+v", in)
	}
	if in := doctorInputs(config.Config{}, errors.New("boom")); in.ConfigErr != "boom" {
		t.Errorf("galat generik: %+v", in)
	}
}

// Galat ERP_SECRET_KEY_PREVIOUS (format, panjang, sama dengan aktif) membuat serve gagal: harus menjadi temuan FAIL,
// bukan ConfigErr (yang hanya warn), dan DSN tetap diteruskan.
func TestDoctorInputs_PreviousKeyErrorsAreSecretKeyFindings(t *testing.T) {
	active := secretkey.B64()
	cases := map[string]string{
		"bukan base64":      "bukan-base64!!!",
		"panjang salah":     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16)),
		"sama dengan aktif": active,
	}
	for name, prev := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadWith(t, map[string]string{"ERP_DATABASE_URL": "postgres://u:p@h/db", "ERP_SECRET_KEY": active, "ERP_SECRET_KEY_PREVIOUS": prev})
			in := doctorInputs(cfg, err)
			if in.ConfigErr != "" || !in.SecretKeyChecked || in.DSN == "" {
				t.Fatalf("harus temuan kunci dengan DSN utuh: %+v", in)
			}
			if !strings.Contains(in.SecretKeyErr, "ERP_SECRET_KEY_PREVIOUS") || strings.Contains(in.SecretKeyErr, prev) {
				t.Errorf("SecretKeyErr = %q", in.SecretKeyErr)
			}
		})
	}
}
