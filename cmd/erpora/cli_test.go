package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/testsupport/secretkey"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := newRootCmd(&out, &errOut)
	root.SetArgs(args)
	err := root.Execute()
	// Banner flavor dev (stderr) bukan bagian dari keluaran yang dibandingkan test.
	return out.String() + stripBanner(errOut.String()), err
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ERP_DATABASE_URL", "ERP_ENV", "ERP_HTTP_ADDR", "ERP_LOG_FORMAT",
		"ERP_TENANCY_MODE", "ERP_DEFAULT_TENANT_ID", "ERP_DB_SLOW_QUERY_MS", "ERP_SECRET_KEY", "ERP_SECRET_KEY_PREVIOUS",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestVersion_PrintsAllFields(t *testing.T) {
	out, err := run(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"platform:", "sdk: 0.1.0", "commit:", "built:", "flavor: " + buildinfo.Flavor()} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestInspectConfig_RedactsPassword(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://erp:secret@127.0.0.1:1/db")
	out, err := run(t, "inspect", "config")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ERP_DATABASE_URL=postgres://erp:****@127.0.0.1:1/db") {
		t.Errorf("DSN not redacted:\n%s", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("password leaked:\n%s", out)
	}
	for _, k := range []string{"ERP_ENV=", "ERP_HTTP_ADDR=", "ERP_LOG_FORMAT=", "ERP_TENANCY_MODE=", "ERP_DEFAULT_TENANT_ID=", "ERP_DB_SLOW_QUERY_MS=", "ERP_AUTHZ_STUB=", "flavor="} {
		if !strings.Contains(out, k) {
			t.Errorf("missing %q:\n%s", k, out)
		}
	}
}

func TestInspectConfig_MissingDatabaseURL_ExitCode2(t *testing.T) {
	clearEnv(t)
	_, err := run(t, "inspect", "config")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCode(err) != 2 {
		t.Errorf("exitCode = %d", exitCode(err))
	}
	if !strings.Contains(err.Error(), "ERP_DATABASE_URL") {
		t.Errorf("message: %v", err)
	}
}

func TestInspectConfig_ShowsEffectiveAuthzStub(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_AUTHZ_STUB", "")
	os.Unsetenv("ERP_AUTHZ_STUB")
	t.Setenv("ERP_DATABASE_URL", "postgres://erp:secret@127.0.0.1:1/db")
	out, err := run(t, "inspect", "config")
	if err != nil || !strings.Contains(out, "ERP_AUTHZ_STUB=deny\n") {
		t.Fatalf("production default: %v\n%s", err, out)
	}
	t.Setenv("ERP_ENV", "development")
	out, err = run(t, "inspect", "config")
	if err != nil || !strings.Contains(out, "ERP_AUTHZ_STUB=allow\n") {
		t.Fatalf("development default: %v\n%s", err, out)
	}
}

func TestServe_MissingDatabaseURL_ExitCode2(t *testing.T) {
	clearEnv(t)
	_, err := run(t, "serve")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCode(err) != 2 {
		t.Errorf("exitCode = %d", exitCode(err))
	}
	if !strings.Contains(err.Error(), "ERP_DATABASE_URL") {
		t.Errorf("message: %v", err)
	}
}

func TestExitCode(t *testing.T) {
	if exitCode(nil) != 0 || exitCode(errors.New("x")) != 1 ||
		exitCode(config.ValidationError{Var: "A", Reason: "b"}) != 2 {
		t.Fatal("unexpected exit codes")
	}
}

func TestList_ShowsCommands(t *testing.T) {
	out, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version", "inspect"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestCompletion_PowerShell(t *testing.T) {
	out, err := run(t, "completion", "powershell")
	if err != nil {
		t.Fatal(err)
	}
	if exitCode(err) != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("empty completion output")
	}
}

// `serve` butuh ERP_SECRET_KEY di semua mode (tanpa kunci sementara): exit 2 sebelum database disentuh.
func TestServe_MissingSecretKey_ExitCode2(t *testing.T) {
	for _, envName := range []string{"production", "development"} {
		t.Run(envName, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("ERP_DATABASE_URL", "postgres://erp:secret@127.0.0.1:1/db")
			t.Setenv("ERP_ENV", envName)
			t.Setenv("ERP_AUTHZ_STUB", "deny")
			_, err := run(t, "serve")
			if err == nil || exitCode(err) != 2 {
				t.Fatalf("exit = %d err = %v, want 2", exitCode(err), err)
			}
			for _, want := range []string{"ERP_SECRET_KEY", "secret:generate"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("pesan harus memuat %q: %v", want, err)
				}
			}
		})
	}
}

// Command selain serve tidak butuh kunci; `inspect config` menampilkan status tanpa nilai kunci.
func TestInspectConfig_SecretKeyOptionalAndRedacted(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://erp:secret@127.0.0.1:1/db")
	out, err := run(t, "inspect", "config")
	if err != nil {
		t.Fatalf("tanpa kunci harus lolos: %v", err)
	}
	if !strings.Contains(out, "ERP_SECRET_KEY=\n") {
		t.Errorf("kunci tak diset harus tampil kosong:\n%s", out)
	}
	secretkey.Setenv(t)
	t.Setenv("ERP_SECRET_KEY_PREVIOUS", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	out, err = run(t, "inspect", "config")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ERP_SECRET_KEY=****\n") || !strings.Contains(out, "ERP_SECRET_KEY_PREVIOUS=****\n") {
		t.Errorf("kunci harus tersamarkan:\n%s", out)
	}
	if strings.Contains(out, secretkey.B64()) || strings.Contains(out, os.Getenv("ERP_SECRET_KEY_PREVIOUS")) {
		t.Errorf("nilai kunci bocor:\n%s", out)
	}
}
