package config

import (
	"testing"
	"time"
)

func TestLoad_DBStatementTimeout(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_DB_STATEMENT_TIMEOUT_MS", "")
	c, err := Load("")
	if err != nil || c.DBStatementTimeout != 30*time.Second {
		t.Fatalf("default: %v, %v", c.DBStatementTimeout, err)
	}
	for in, want := range map[string]time.Duration{"1000": time.Second, "600000": 10 * time.Minute, "45000": 45 * time.Second} {
		t.Setenv("ERP_DB_STATEMENT_TIMEOUT_MS", in)
		if c, err = Load(""); err != nil || c.DBStatementTimeout != want {
			t.Errorf("%s: %v, %v", in, c.DBStatementTimeout, err)
		}
	}
	for _, bad := range []string{"999", "600001", "abc", "-1", "1.5", "0"} {
		t.Setenv("ERP_DB_STATEMENT_TIMEOUT_MS", bad)
		_, err := Load("")
		wantVar(t, err, "ERP_DB_STATEMENT_TIMEOUT_MS")
	}
}
