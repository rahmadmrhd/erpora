package config

import (
	"runtime"
	"testing"
)

func TestLoad_Argon2MaxConcurrentDefault(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	want := min(max(runtime.GOMAXPROCS(0), 1), 4)
	if c.Argon2MaxConcurrent != want || c.Argon2MaxConcurrent != DefaultArgon2MaxConcurrent() {
		t.Fatalf("Argon2MaxConcurrent = %d, want min(GOMAXPROCS,4) = %d", c.Argon2MaxConcurrent, want)
	}
	if c.Argon2MaxConcurrent < 1 || c.Argon2MaxConcurrent > 4 {
		t.Fatalf("default di luar 1..4: %d", c.Argon2MaxConcurrent)
	}
}

func TestLoad_Argon2MaxConcurrentOverride(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want int
	}{{"1", 1}, {"3", 3}, {" 8 ", 8}, {"64", 64}} {
		clearEnv(t)
		t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
		t.Setenv("ERP_ARGON2_MAX_CONCURRENT", tc.val)
		c, err := Load("")
		if err != nil {
			t.Fatalf("%q: %v", tc.val, err)
		}
		if c.Argon2MaxConcurrent != tc.want {
			t.Errorf("%q: Argon2MaxConcurrent = %d, want %d", tc.val, c.Argon2MaxConcurrent, tc.want)
		}
	}
}

func TestLoad_Argon2MaxConcurrentInvalid(t *testing.T) {
	for _, val := range []string{"0", "-1", "abc", "1.5", "65", "99999999999999999999", "+2", "0x4"} {
		clearEnv(t)
		t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
		t.Setenv("ERP_ARGON2_MAX_CONCURRENT", val)
		_, err := Load("")
		if err == nil {
			t.Fatalf("%q diterima, harus ditolak", val)
		}
		wantVar(t, err, "ERP_ARGON2_MAX_CONCURRENT")
	}
}
