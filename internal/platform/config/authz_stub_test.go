package config

import (
	"os"
	"path/filepath"
	"testing"
)

func clearAuthzEnv(t *testing.T) {
	t.Helper()
	clearEnv(t)
	t.Setenv("ERP_AUTHZ_STUB", "")
	os.Unsetenv("ERP_AUTHZ_STUB")
}

func TestLoad_AuthzStubDefaults(t *testing.T) {
	clearAuthzEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	c, err := Load("")
	if err != nil || c.AuthzStub != "deny" {
		t.Fatalf("production: AuthzStub = %q err = %v", c.AuthzStub, err)
	}
	t.Setenv("ERP_ENV", "development")
	c, err = Load("")
	if err != nil || c.AuthzStub != "allow" {
		t.Fatalf("development: AuthzStub = %q err = %v", c.AuthzStub, err)
	}
}

func TestLoad_AuthzStubInvalid(t *testing.T) {
	clearAuthzEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_AUTHZ_STUB", "maybe")
	_, err := Load("")
	wantVar(t, err, "ERP_AUTHZ_STUB")
}

func TestLoad_AuthzStubProcessEnvOverridesFile(t *testing.T) {
	clearAuthzEnv(t)
	f := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(f, []byte("ERP_DATABASE_URL=postgres://u:p@h/db\nERP_AUTHZ_STUB=allow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ERP_AUTHZ_STUB", "deny")
	c, err := Load(f)
	if err != nil || c.AuthzStub != "deny" {
		t.Fatalf("AuthzStub = %q err = %v", c.AuthzStub, err)
	}
}

func TestLoad_AuthzStubFromFile(t *testing.T) {
	clearAuthzEnv(t)
	f := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(f, []byte("ERP_DATABASE_URL=postgres://u:p@h/db\nERP_AUTHZ_STUB=allow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(f)
	if err != nil || c.AuthzStub != "allow" {
		t.Fatalf("AuthzStub = %q err = %v", c.AuthzStub, err)
	}
}
