package sqlgen

import (
	"errors"
	"strings"
	"testing"

	"erpora/sdk/metadata"
)

func testEnv() Env {
	return Env{
		"qty":    {Column: "quantity", Type: metadata.TypeDecimal},
		"price":  {Column: "unit_price", Type: metadata.TypeDecimal},
		"count":  {Column: "count", Type: metadata.TypeInteger},
		"name":   {Column: "name", Type: metadata.TypeString},
		"status": {Column: "status", Type: metadata.TypeEnum},
		"when":   {Column: "when_at", Type: metadata.TypeDate},
		"ok":     {Column: "ok", Type: metadata.TypeBoolean},
		"gen":    {Column: "gen", Type: metadata.TypeDecimal, Generated: true},
	}
}

func parseExpr(t *testing.T, raw any) *metadata.Expr {
	t.Helper()
	e, err := metadata.ParseExpr(raw)
	if err != nil {
		t.Fatalf("ParseExpr: %v", err)
	}
	return e
}

func m(kv ...any) map[string]any {
	out := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

func fld(n string) any { return m("field", n) }

func TestStoredExpr_Golden(t *testing.T) {
	cases := []struct {
		name string
		expr any
		want string
	}{
		{"mul half_up", m("mul", []any{fld("qty"), fld("price")}, "scale", 2, "round", "half_up"),
			`round(("quantity" * "unit_price"), 2)`},
		{"add tiga argumen down", m("add", []any{fld("qty"), fld("price"), fld("count")}, "scale", 0, "round", "down"),
			`trunc(("quantity" + "unit_price" + "count"), 0)`},
		{"sub", m("sub", []any{fld("qty"), fld("price")}, "scale", 2, "round", "half_up"),
			`round(("quantity" - "unit_price"), 2)`},
		{"div tanpa pembagian nol", m("div", []any{fld("qty"), fld("price")}, "scale", 4, "round", "half_up"),
			`round(("quantity" / NULLIF("unit_price", 0)), 4)`},
		{"concat", m("concat", []any{fld("name"), m("lit", " / "), fld("qty"), fld("count"), fld("ok")}),
			`(COALESCE("name", '') || ' / '::text || COALESCE("quantity"::text, '') || COALESCE("count"::text, '') || COALESCE("ok"::text, ''))`},
		{"coalesce", m("coalesce", []any{fld("qty"), m("dec", "0.00")}), `COALESCE("quantity", 0.00)`},
		{"if", m("if", m("field", "status", "op", "eq", "value", "draft"), "then", m("lit", "a"), "else", m("lit", "b")),
			`CASE WHEN "status" IS NOT DISTINCT FROM 'draft' THEN 'a'::text ELSE 'b'::text END`},
		{"literal bilangan", m("add", []any{m("lit", 5), fld("count")}, "scale", 0, "round", "half_up"),
			`round((5 + "count"), 0)`},
		{"literal tanda kutip", m("concat", []any{m("lit", "it's"), fld("name")}), `('it''s'::text || COALESCE("name", ''))`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := StoredExpr(parseExpr(t, c.expr), testEnv())
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestStoredExpr_AllRoundModesTranslate(t *testing.T) {
	for _, mode := range []string{"half_up", "half_down", "half_even", "up", "down", "floor", "ceiling"} {
		got, err := StoredExpr(parseExpr(t, m("sub", []any{fld("qty"), fld("price")}, "scale", 3, "round", mode)), testEnv())
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if len(splitTopLevel(t, "SELECT "+got)) != 1 {
			t.Errorf("%s: pernyataan terpecah: %s", mode, got)
		}
		switch mode {
		case "half_up":
			if !strings.HasPrefix(got, "round(") {
				t.Errorf("half_up = %s", got)
			}
		case "down":
			if !strings.HasPrefix(got, "trunc(") {
				t.Errorf("down = %s", got)
			}
		default:
			if !strings.Contains(got, "CASE") {
				t.Errorf("%s = %s", mode, got)
			}
		}
	}
}

func TestStoredExpr_Errors(t *testing.T) {
	cases := []struct {
		name string
		expr any
		code string
	}{
		{"field tak dikenal", m("add", []any{fld("nope"), fld("qty")}, "scale", 2, "round", "half_up"), ErrUnknownField},
		{"rujukan kolom turunan", m("add", []any{fld("gen"), fld("qty")}, "scale", 2, "round", "half_up"), ErrGeneratedRef},
		{"aritmetika pada teks", m("add", []any{fld("name"), fld("qty")}, "scale", 2, "round", "half_up"), ErrType},
		{"concat tanggal tidak immutable", m("concat", []any{fld("when"), m("lit", "x")}), ErrNotImmutable},
		{"coalesce tipe campur", m("coalesce", []any{fld("name"), fld("qty")}), ErrType},
		{"if cabang beda tipe", m("if", m("field", "ok", "op", "eq", "value", true), "then", m("lit", "a"), "else", fld("qty")), ErrType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := StoredExpr(parseExpr(t, c.expr), testEnv())
			var te *TranslateError
			if !errors.As(err, &te) || te.Code != c.code {
				t.Fatalf("err = %v, want kode %s", err, c.code)
			}
		})
	}
}

func TestStoredExpr_TooLarge(t *testing.T) {
	// Pembulatan bertingkat menggandakan sub-ekspresi; hasil dibatasi agar tidak meledak.
	e := fld("qty")
	for i := 0; i < 7; i++ {
		e = m("sub", []any{e, fld("price")}, "scale", 2, "round", "half_even")
	}
	_, err := StoredExpr(parseExpr(t, e), testEnv())
	var te *TranslateError
	if !errors.As(err, &te) || te.Code != ErrTooLarge {
		t.Fatalf("err = %v, want too_large", err)
	}
}

func TestStoredExpr_ResultType(t *testing.T) {
	typ, err := StoredExprType(parseExpr(t, m("concat", []any{fld("name")})), testEnv())
	if err != nil || typ != metadata.TypeString {
		t.Errorf("concat -> %v, %v", typ, err)
	}
	typ, err = StoredExprType(parseExpr(t, m("mul", []any{fld("qty"), fld("price")}, "scale", 2, "round", "half_up")), testEnv())
	if err != nil || typ != metadata.TypeDecimal {
		t.Errorf("mul -> %v, %v", typ, err)
	}
}
