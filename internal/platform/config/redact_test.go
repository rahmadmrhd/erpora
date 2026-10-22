package config

import (
	"strings"
	"testing"
)

func TestRedactDSN_Forms(t *testing.T) {
	cases := []struct {
		name, in string
		want     string // substring wajib ada ("" = tidak dicek)
		secret   string // tidak boleh muncul
	}{
		{"url userinfo", "postgres://erp:S3cret@db:5432/erp?sslmode=disable", "erp:****@db:5432/erp", "S3cret"},
		{"postgresql scheme", "postgresql://erp:S3cret@db/erp", "erp:****@db/erp", "S3cret"},
		{"url no password", "postgres://erp@db/erp", "erp@db/erp", ""},
		{"query password", "postgres://erp@db/erp?password=S3cret&sslmode=disable", "password=****", "S3cret"},
		{"query sslpassword", "postgres://erp@db/erp?sslpassword=S3cret", "sslpassword=****", "S3cret"},
		{"query password uppercase", "postgres://erp@db/erp?PASSWORD=S3cret", "****", "S3cret"},
		{"userinfo and query", "postgres://erp:A1b2@db/erp?password=C3d4", "erp:****@db", "C3d4"},
		{"keyword value", "host=db user=erp password=S3cret dbname=erp", "password=****", "S3cret"},
		{"keyword quoted", "host=db user=erp password='S3 cret' dbname=erp", "password=****", "S3 cret"},
		{"keyword quoted escaped", `host=db password='it\'s S3cret' user=erp`, "password=****", "S3cret"},
		{"keyword sslpassword", "host=db sslpassword=S3cret", "sslpassword=****", "S3cret"},
		{"keyword uppercase key", "host=db PASSWORD=S3cret", "****", "S3cret"},
		{"keyword password first", "password=S3cret host=db", "password=****", "S3cret"},
		{"keyword short password", "host=db password=a user=erp", "password=****", ""},
		{"url special chars", "postgres://erp:p%40ss%3Aw%2Frd%25@db.local:5432/erp?sslmode=disable", "erp:****@db.local:5432/erp", "p%40ss"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := RedactDSN(c.in)
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Errorf("RedactDSN(%q) = %q, want substring %q", c.in, out, c.want)
			}
			if c.secret != "" && strings.Contains(out, c.secret) {
				t.Errorf("RedactDSN(%q) = %q leaks %q", c.in, out, c.secret)
			}
		})
	}
}

func TestRedactDSN_UnparseableNeverEchoed(t *testing.T) {
	for _, in := range []string{
		"postgres://%zz:S3cret@db",
		"just-a-string-with-S3cret",
		"host=db password='unterminated S3cret",
		"host=db S3cret",
		"= S3cret",
	} {
		out := RedactDSN(in)
		if strings.Contains(out, "S3cret") {
			t.Errorf("RedactDSN(%q) = %q leaks secret", in, out)
		}
	}
}

func TestDSNParts(t *testing.T) {
	host, secrets := DSNParts("postgres://erp:S3cret@db.local:5432/erp?password=Q9&sslpassword=Z8")
	if host != "db.local:5432" {
		t.Errorf("host = %q", host)
	}
	if len(secrets) != 3 {
		t.Errorf("secrets = %v", secrets)
	}
	host, secrets = DSNParts("host=db.internal port=6543 user=erp password='S3 cret'")
	if host != "db.internal:6543" || len(secrets) != 1 || secrets[0] != "S3 cret" {
		t.Errorf("kv: host=%q secrets=%v", host, secrets)
	}
	if host, secrets = DSNParts("garbage"); host != "?" || len(secrets) != 0 {
		t.Errorf("garbage: %q %v", host, secrets)
	}
}
