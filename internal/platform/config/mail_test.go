package config

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// mailEnv menyiapkan environment minimal untuk Load dengan ERP_ENV tertentu dan variabel surat tambahan.
func mailEnv(t *testing.T, envName string, extra map[string]string) {
	t.Helper()
	clearAuthzEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ENV", envName)
	t.Setenv("ERP_AUTHZ_STUB", "deny")
	for k, v := range extra {
		t.Setenv(k, v)
	}
}

func smtpEnv(overrides map[string]string) map[string]string {
	m := map[string]string{
		"ERP_MAIL_DRIVER":   "smtp",
		"ERP_SMTP_HOST":     "smtp.example.com",
		"ERP_SMTP_FROM":     "noreply@example.com",
		"ERP_SMTP_USERNAME": "mailer",
		"ERP_SMTP_PASSWORD": "S3cr3t-sandi-smtp",
	}
	for k, v := range overrides {
		m[k] = v
	}
	return m
}

func TestLoad_MailDefaults(t *testing.T) {
	for _, envName := range []string{"development", "production"} {
		mailEnv(t, envName, nil)
		c, err := Load("")
		if err != nil {
			t.Fatalf("%s: %v", envName, err)
		}
		if c.MailDriver != MailDriverLog {
			t.Errorf("%s: MailDriver = %q, want log", envName, c.MailDriver)
		}
		if c.SMTP.TLSMode != SMTPTLSStartTLS {
			t.Errorf("%s: TLSMode default = %q, want starttls", envName, c.SMTP.TLSMode)
		}
	}
}

func TestLoad_SMTPValid(t *testing.T) {
	mailEnv(t, "production", smtpEnv(nil))
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.MailDriver != MailDriverSMTP || c.SMTP.Host != "smtp.example.com" || c.SMTP.From != "noreply@example.com" ||
		c.SMTP.Username != "mailer" || string(c.SMTP.Password) != "S3cr3t-sandi-smtp" || c.SMTP.TLSMode != SMTPTLSStartTLS || c.SMTP.Port != 587 {
		t.Fatalf("SMTP = %+v", c.SMTP)
	}
	// Port bawaan mengikuti mode TLS.
	for mode, port := range map[string]int{"tls": 465, "starttls": 587} {
		mailEnv(t, "production", smtpEnv(map[string]string{"ERP_SMTP_TLS_MODE": mode}))
		c, err := Load("")
		if err != nil || c.SMTP.Port != port {
			t.Errorf("mode %s: port = %d err = %v, want %d", mode, c.SMTP.Port, err, port)
		}
	}
	mailEnv(t, "production", smtpEnv(map[string]string{"ERP_SMTP_PORT": "2525"}))
	if c, err := Load(""); err != nil || c.SMTP.Port != 2525 {
		t.Errorf("port eksplisit: %d %v", c.SMTP.Port, err)
	}
}

func TestLoad_SMTPPasswordNotTrimmed(t *testing.T) {
	mailEnv(t, "production", smtpEnv(map[string]string{"ERP_SMTP_PASSWORD": "  sandi dengan spasi  "}))
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if string(c.SMTP.Password) != "  sandi dengan spasi  " {
		t.Errorf("sandi SMTP dipotong: %q", string(c.SMTP.Password))
	}
}

func TestLoad_SMTPInvalid(t *testing.T) {
	cases := []struct {
		name string
		env  string
		over map[string]string
		v    string
	}{
		{"driver tak dikenal", "production", map[string]string{"ERP_MAIL_DRIVER": "sendmail"}, "ERP_MAIL_DRIVER"},
		{"smtp tanpa host", "production", map[string]string{"ERP_SMTP_HOST": ""}, "ERP_SMTP_HOST"},
		{"host berisi spasi", "production", map[string]string{"ERP_SMTP_HOST": "smtp example.com"}, "ERP_SMTP_HOST"},
		{"host berisi skema", "production", map[string]string{"ERP_SMTP_HOST": "smtp://smtp.example.com"}, "ERP_SMTP_HOST"},
		{"host berisi CRLF", "production", map[string]string{"ERP_SMTP_HOST": "smtp.example.com\r\nX: y"}, "ERP_SMTP_HOST"},
		{"smtp tanpa from", "production", map[string]string{"ERP_SMTP_FROM": ""}, "ERP_SMTP_FROM"},
		{"from bukan alamat polos", "production", map[string]string{"ERP_SMTP_FROM": "ERPora <noreply@example.com>"}, "ERP_SMTP_FROM"},
		{"from injeksi header", "production", map[string]string{"ERP_SMTP_FROM": "a@example.com\r\nBcc: x@example.com"}, "ERP_SMTP_FROM"},
		{"port bukan angka", "production", map[string]string{"ERP_SMTP_PORT": "abc"}, "ERP_SMTP_PORT"},
		{"port nol", "production", map[string]string{"ERP_SMTP_PORT": "0"}, "ERP_SMTP_PORT"},
		{"port terlalu besar", "production", map[string]string{"ERP_SMTP_PORT": "70000"}, "ERP_SMTP_PORT"},
		{"mode tls tak dikenal", "production", map[string]string{"ERP_SMTP_TLS_MODE": "ssl"}, "ERP_SMTP_TLS_MODE"},
		{"mode none ditolak di production", "production", map[string]string{"ERP_SMTP_TLS_MODE": "none", "ERP_SMTP_USERNAME": "", "ERP_SMTP_PASSWORD": ""}, "ERP_SMTP_TLS_MODE"},
		{"mode none dengan username (auth tanpa TLS)", "development", map[string]string{"ERP_SMTP_TLS_MODE": "none"}, "ERP_SMTP_TLS_MODE"},
		{"username tanpa sandi", "production", map[string]string{"ERP_SMTP_PASSWORD": ""}, "ERP_SMTP_PASSWORD"},
		{"sandi tanpa username", "production", map[string]string{"ERP_SMTP_USERNAME": ""}, "ERP_SMTP_USERNAME"},
		{"username berisi CRLF", "production", map[string]string{"ERP_SMTP_USERNAME": "a\r\nb"}, "ERP_SMTP_USERNAME"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mailEnv(t, tc.env, smtpEnv(tc.over))
			_, err := Load("")
			if err == nil {
				t.Fatal("diterima, harus ditolak")
			}
			wantVar(t, err, tc.v)
			if strings.Contains(err.Error(), "S3cr3t-sandi-smtp") {
				t.Errorf("galat membocorkan sandi SMTP: %v", err)
			}
		})
	}
}

func TestLoad_SMTPNoneAllowedInDevelopmentWithoutAuth(t *testing.T) {
	mailEnv(t, "development", smtpEnv(map[string]string{"ERP_SMTP_TLS_MODE": "none", "ERP_SMTP_USERNAME": "", "ERP_SMTP_PASSWORD": ""}))
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.SMTP.TLSMode != SMTPTLSNone || c.SMTP.Port != 25 {
		t.Errorf("SMTP = %+v", c.SMTP)
	}
}

// Driver log tidak mewajibkan variabel SMTP (migrate, inspect, doctor tetap jalan), tetapi format yang terisi divalidasi.
func TestLoad_LogDriverDoesNotRequireSMTP(t *testing.T) {
	mailEnv(t, "production", map[string]string{"ERP_MAIL_DRIVER": "log"})
	if _, err := Load(""); err != nil {
		t.Fatal(err)
	}
	mailEnv(t, "production", map[string]string{"ERP_MAIL_DRIVER": "log", "ERP_SMTP_PORT": "abc"})
	_, err := Load("")
	wantVar(t, err, "ERP_SMTP_PORT")
}

func TestSMTPPasswordRedactedEverywhere(t *testing.T) {
	const pw = "S3cr3t-sandi-smtp"
	mailEnv(t, "production", smtpEnv(nil))
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%+v", c.SMTP),
		fmt.Sprintf("%#v", c.SMTP), fmt.Sprintf("%v", c.SMTP.Password),
	} {
		if strings.Contains(s, pw) {
			t.Fatalf("sandi SMTP bocor: %s", s)
		}
	}
	var sb strings.Builder
	l := slog.New(slog.NewJSONHandler(&sb, nil))
	l.Info("cfg", "smtp", c.SMTP, "password", c.SMTP.Password)
	if strings.Contains(sb.String(), pw) {
		t.Fatalf("sandi SMTP bocor di slog: %s", sb.String())
	}
	if got := RedactSecret(c.SMTP.Password); got != "****" {
		t.Errorf("RedactSecret = %q", got)
	}
}

func TestRequireMailDriver(t *testing.T) {
	// development: log tetap sah (diisi eksplisit maupun tidak).
	for _, extra := range []map[string]string{nil, {"ERP_MAIL_DRIVER": "log"}} {
		mailEnv(t, "development", extra)
		c, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.RequireMailDriver(); err != nil {
			t.Errorf("development %v: %v", extra, err)
		}
	}
	// production: tidak diisi -> serve gagal start (fail-closed); Load sendiri lolos (migrate/inspect/doctor tetap jalan).
	mailEnv(t, "production", nil)
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load production tanpa surat harus lolos: %v", err)
	}
	if c.MailDriverExplicit {
		t.Error("MailDriverExplicit true padahal ERP_MAIL_DRIVER tidak diisi")
	}
	err = c.RequireMailDriver()
	if err == nil {
		t.Fatal("production tanpa ERP_MAIL_DRIVER harus gagal di RequireMailDriver")
	}
	wantVar(t, err, "ERP_MAIL_DRIVER")
	if !strings.Contains(err.Error(), "smtp") {
		t.Errorf("pesan harus menyebut smtp: %v", err)
	}
	// production: log diisi eksplisit oleh operator -> sah.
	mailEnv(t, "production", map[string]string{"ERP_MAIL_DRIVER": "log"})
	c, err = Load("")
	if err != nil || !c.MailDriverExplicit {
		t.Fatalf("Load: %v explicit=%v", err, c.MailDriverExplicit)
	}
	if err := c.RequireMailDriver(); err != nil {
		t.Errorf("production dengan log eksplisit: %v", err)
	}
	// production: smtp sah.
	mailEnv(t, "production", smtpEnv(nil))
	c, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RequireMailDriver(); err != nil {
		t.Errorf("production smtp: %v", err)
	}
	// Nilai hanya spasi dihitung tidak diisi.
	mailEnv(t, "production", map[string]string{"ERP_MAIL_DRIVER": "  "})
	c, _ = Load("")
	if err := c.RequireMailDriver(); err == nil {
		t.Error("ERP_MAIL_DRIVER berisi spasi harus dihitung tidak diisi")
	}
}
