package pgtest

import (
	"regexp"
	"strings"
	"testing"
)

func TestWithDatabase(t *testing.T) {
	cases := []struct {
		name, in, db, want string
	}{
		{"url dengan query", "postgres://erp:erp@127.0.0.1:15432/postgres?sslmode=disable", "erpora_test_ab", "postgres://erp:erp@127.0.0.1:15432/erpora_test_ab?sslmode=disable"},
		{"url tanpa path", "postgres://erp:erp@host:5432?sslmode=disable", "x1", "postgres://erp:erp@host:5432/x1?sslmode=disable"},
		{"skema postgresql", "postgresql://u:p@h/postgres", "x1", "postgresql://u:p@h/x1"},
	}
	for _, c := range cases {
		got, err := withDatabase(c.in, c.db)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestWithDatabase_Invalid(t *testing.T) {
	for _, in := range []string{"", "host=localhost dbname=postgres", "://bad"} {
		if _, err := withDatabase(in, "x"); err == nil {
			t.Errorf("withDatabase(%q) seharusnya error", in)
		}
	}
}

func TestUniqueDatabaseName(t *testing.T) {
	re := regexp.MustCompile(`^erpora_test_[0-9a-f]{16}$`)
	a, b := uniqueDatabaseName(), uniqueDatabaseName()
	if !re.MatchString(a) || !re.MatchString(b) {
		t.Fatalf("format salah: %q %q", a, b)
	}
	if a == b {
		t.Fatal("nama harus unik")
	}
	if strings.ContainsAny(a, `"' ;`) {
		t.Fatal("nama harus aman sebagai identifier")
	}
}
