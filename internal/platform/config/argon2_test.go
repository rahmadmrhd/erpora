package config

import (
	"testing"

	"erpora/internal/platform/identity/password"
)

func TestLoad_Argon2Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Argon2 != password.DefaultParams() {
		t.Fatalf("Argon2 = %+v, want default OWASP %+v", c.Argon2, password.DefaultParams())
	}
}

func TestLoad_Argon2Overrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ARGON2_MEMORY_KIB", "65536")
	t.Setenv("ERP_ARGON2_TIME", "3")
	t.Setenv("ERP_ARGON2_PARALLELISM", "4")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (password.Params{MemoryKiB: 65536, Time: 3, Parallelism: 4}); c.Argon2 != want {
		t.Fatalf("Argon2 = %+v, want %+v", c.Argon2, want)
	}
	// Satu variabel saja: sisanya tetap default.
	clearEnv(t)
	t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("ERP_ARGON2_TIME", "4")
	c, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (password.Params{MemoryKiB: 19456, Time: 4, Parallelism: 1}); c.Argon2 != want {
		t.Fatalf("Argon2 = %+v, want %+v", c.Argon2, want)
	}
}

func TestLoad_Argon2Invalid(t *testing.T) {
	cases := []struct {
		name, v, val, want string
	}{
		{"memori bukan angka", "ERP_ARGON2_MEMORY_KIB", "abc", "ERP_ARGON2_MEMORY_KIB"},
		{"memori negatif", "ERP_ARGON2_MEMORY_KIB", "-1", "ERP_ARGON2_MEMORY_KIB"},
		{"memori pecahan", "ERP_ARGON2_MEMORY_KIB", "1.5", "ERP_ARGON2_MEMORY_KIB"},
		{"memori di bawah OWASP", "ERP_ARGON2_MEMORY_KIB", "19455", "ERP_ARGON2_MEMORY_KIB"},
		{"memori nol", "ERP_ARGON2_MEMORY_KIB", "0", "ERP_ARGON2_MEMORY_KIB"},
		{"memori di atas batas", "ERP_ARGON2_MEMORY_KIB", "262145", "ERP_ARGON2_MEMORY_KIB"},
		{"memori overflow", "ERP_ARGON2_MEMORY_KIB", "99999999999999999999", "ERP_ARGON2_MEMORY_KIB"},
		{"waktu nol", "ERP_ARGON2_TIME", "0", "ERP_ARGON2_TIME"},
		{"waktu di atas batas", "ERP_ARGON2_TIME", "11", "ERP_ARGON2_TIME"},
		{"waktu bukan angka", "ERP_ARGON2_TIME", "dua", "ERP_ARGON2_TIME"},
		{"waktu 1 dengan memori default", "ERP_ARGON2_TIME", "1", "ERP_ARGON2_TIME"},
		{"paralelisme nol", "ERP_ARGON2_PARALLELISM", "0", "ERP_ARGON2_PARALLELISM"},
		{"paralelisme di atas batas", "ERP_ARGON2_PARALLELISM", "17", "ERP_ARGON2_PARALLELISM"},
		{"paralelisme melingkar uint8", "ERP_ARGON2_PARALLELISM", "257", "ERP_ARGON2_PARALLELISM"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
			t.Setenv(c.v, c.val)
			_, err := Load("")
			wantVar(t, err, c.want)
		})
	}
	t.Run("waktu 1 diterima bila memori 46 MiB", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("ERP_DATABASE_URL", "postgres://u:p@h/db")
		t.Setenv("ERP_ARGON2_TIME", "1")
		t.Setenv("ERP_ARGON2_MEMORY_KIB", "47104")
		if _, err := Load(""); err != nil {
			t.Fatal(err)
		}
	})
}
