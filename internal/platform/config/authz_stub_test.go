package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if err := os.WriteFile(f, []byte("ERP_DATABASE_URL=postgres://u:p@h/db\nERP_ENV=development\nERP_AUTHZ_STUB=allow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(f)
	if err != nil || c.AuthzStub != "allow" {
		t.Fatalf("AuthzStub = %q err = %v", c.AuthzStub, err)
	}
}

// M8 (opsi A): allow dilarang di production sampai authN/authz nyata (PRD-001).
func TestLoad_AuthzStubAllowForbiddenInProduction(t *testing.T) {
	clearAuthzEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ENV", "production")
	t.Setenv("ERP_AUTHZ_STUB", "allow")
	_, err := Load("")
	wantVar(t, err, "ERP_AUTHZ_STUB")
	if !strings.Contains(err.Error(), "PRD-001") || !strings.Contains(err.Error(), "production") {
		t.Errorf("pesan harus menjelaskan larangan: %v", err)
	}
}

func TestLoad_AuthzStubMatrix(t *testing.T) {
	cases := []struct{ env, stub, want string }{
		{"development", "allow", "allow"},
		{"development", "deny", "deny"},
		{"production", "deny", "deny"},
		{"production", "", "deny"},
		{"development", "", "allow"},
	}
	for _, tc := range cases {
		t.Run(tc.env+"/"+tc.stub, func(t *testing.T) {
			clearAuthzEnv(t)
			t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
			t.Setenv("ERP_ENV", tc.env)
			if tc.stub != "" {
				t.Setenv("ERP_AUTHZ_STUB", tc.stub)
			}
			c, err := Load("")
			if err != nil || c.AuthzStub != tc.want {
				t.Fatalf("AuthzStub = %q err = %v, want %q", c.AuthzStub, err, tc.want)
			}
		})
	}
}
