package sqlgen

import (
	"math/rand"
	"strings"
	"testing"
)

func TestQuoteIdent(t *testing.T) {
	ok := map[string]string{
		"a":                   `"a"`,
		"sales_invoices":      `"sales_invoices"`,
		`we"ird`:              `"we""ird"`,
		`"; DROP TABLE x; --`: `"""; DROP TABLE x; --"`,
		"user":                `"user"`,
	}
	for in, want := range ok {
		got, err := QuoteIdent(in)
		if err != nil || got != want {
			t.Errorf("QuoteIdent(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "a\x00b", strings.Repeat("a", 64), "a\xffb"} {
		if got, err := QuoteIdent(bad); err == nil {
			t.Errorf("QuoteIdent(%q) = %q, want galat", bad, got)
		}
	}
	if _, err := QuoteIdent(strings.Repeat("a", 63)); err != nil {
		t.Errorf("63 byte harus sah: %v", err)
	}
}

func TestLiteral(t *testing.T) {
	cases := map[string]string{
		"abc":                  `'abc'`,
		"it's":                 `'it''s'`,
		`a\b`:                  `E'a\\b'`,
		`a'\b`:                 `E'a''\\b'`,
		"$$":                   `'$$'`,
		"*/ -- ;":              `'*/ -- ;'`,
		"x\ny":                 "'x\ny'",
		"":                     `''`,
		"日本語":                  `'日本語'`,
		`'); DROP TABLE t; --`: `'''); DROP TABLE t; --'`,
	}
	for in, want := range cases {
		got, err := Literal(in)
		if err != nil || got != want {
			t.Errorf("Literal(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"a\x00b", "a\xffb"} {
		if got, err := Literal(bad); err == nil {
			t.Errorf("Literal(%q) = %q, want galat (NUL atau UTF-8 tak sah)", bad, got)
		}
	}
}

// Properti: untuk string acak (alfabet penuh karakter berbahaya), literal terbaca kembali persis dan tetap
// satu "pernyataan" utuh.
func TestLiteral_RoundTripProperty(t *testing.T) {
	alphabet := []string{"'", "\\", "$", "\"", ";", "-", "/", "*", "\n", "\t", "a", "Z", "0", "é", "日", "$$", "$erpora$", "--", "/*", "*/", " ", "E", "`"}
	rng := rand.New(rand.NewSource(20261101))
	for i := 0; i < 2000; i++ {
		var sb strings.Builder
		for n := rng.Intn(12); n >= 0; n-- {
			sb.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		in := sb.String()
		lit, err := Literal(in)
		if err != nil {
			t.Fatalf("Literal(%q): %v", in, err)
		}
		if got := decodeLiteral(t, lit); got != in {
			t.Fatalf("Literal(%q) = %q terbaca %q", in, lit, got)
		}
		if stmts := splitTopLevel(t, "SELECT "+lit); len(stmts) != 1 {
			t.Fatalf("Literal(%q) = %q memecah pernyataan: %q", in, lit, stmts)
		}
	}
}

func TestDollarQuote(t *testing.T) {
	if got := DollarQuote("BEGIN RETURN 1; END"); got != "$erpora$BEGIN RETURN 1; END$erpora$" {
		t.Errorf("got %q", got)
	}
	body := "x $erpora$ y"
	got := DollarQuote(body)
	if got != "$erpora1$x $erpora$ y$erpora1$" {
		t.Errorf("got %q", got)
	}
	// Tag tidak boleh muncul di badan (termasuk sebagai awalan yang dapat menyatu).
	body2 := "a $erpora1 b $erpora$ c"
	g2 := DollarQuote(body2)
	tag := g2[:strings.Index(g2[1:], "$")+2]
	if strings.Contains(body2, tag) || tag == "$erpora$" {
		t.Errorf("tag %q berbenturan dengan badan %q", tag, body2)
	}
	if stmts := splitTopLevel(t, "CREATE FUNCTION f() RETURNS int LANGUAGE sql AS "+g2); len(stmts) != 1 {
		t.Errorf("pernyataan terpecah: %q", stmts)
	}
}

func TestNumericLiteral(t *testing.T) {
	good := map[string]string{"12.50": "12.50", "-3": "-3", "0": "0", "0.001": "0.001", "100": "100"}
	for in, want := range good {
		got, err := NumericLiteral(in)
		if err != nil || got != want {
			t.Errorf("NumericLiteral(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "1e5", "+1", "01", "1.", ".5", "1; DROP TABLE x", "1 ", "NaN", "0x10", "1'2", strings.Repeat("9", 70)} {
		if got, err := NumericLiteral(bad); err == nil {
			t.Errorf("NumericLiteral(%q) = %q, want galat", bad, got)
		}
	}
}
