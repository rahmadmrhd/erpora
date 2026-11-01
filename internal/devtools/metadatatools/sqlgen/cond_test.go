package sqlgen

import (
	"errors"
	"testing"

	"erpora/sdk/metadata"
)

func parseCond(t *testing.T, raw any) *metadata.Condition {
	t.Helper()
	c, err := metadata.ParseCondition(raw)
	if err != nil {
		t.Fatalf("ParseCondition: %v", err)
	}
	return c
}

func condEnv() Env {
	e := testEnv()
	e["n"] = FieldInfo{Column: "n", Type: metadata.TypeInteger}
	e["note"] = FieldInfo{Column: "note", Type: metadata.TypeText}
	e["total"] = FieldInfo{Column: "total", Type: metadata.TypeDecimal}
	e["ref"] = FieldInfo{Column: "ref_col", Type: metadata.TypeString}
	return e
}

func leaf(field, op string, value any) map[string]any {
	return m("field", field, "op", op, "value", value)
}

func TestCondition_Golden(t *testing.T) {
	cases := []struct {
		name string
		cond any
		opts CondOptions
		want string
	}{
		{"eq teks", leaf("status", "eq", "draft"), CondOptions{}, `"status" IS NOT DISTINCT FROM 'draft'`},
		{"ne teks lock", leaf("status", "ne", "draft"), CondOptions{Qualifier: "OLD", Restricted: true}, `OLD."status" IS DISTINCT FROM 'draft'`},
		{"eq null", m("field", "note", "op", "eq", "value", nil), CondOptions{}, `"note" IS NULL`},
		{"ne null", m("field", "note", "op", "ne", "value", nil), CondOptions{}, `"note" IS NOT NULL`},
		{"in lock", leaf("status", "in", []any{"a", "b"}), CondOptions{Qualifier: "OLD", Restricted: true}, `COALESCE(OLD."status" IN ('a', 'b'), false)`},
		{"isnull default true", m("field", "note", "op", "isnull", "value", true), CondOptions{Restricted: true}, `"note" IS NULL`},
		{"isnull false", m("field", "note", "op", "isnull", "value", false), CondOptions{Restricted: true}, `"note" IS NOT NULL`},
		{"gt integer", leaf("n", "gt", 5), CondOptions{}, `COALESCE("n" > 5, false)`},
		{"gte desimal", leaf("total", "gte", "50000000.00"), CondOptions{}, `COALESCE("total" >= 50000000.00, false)`},
		{"lt teks collation C", leaf("name", "lt", "m"), CondOptions{}, `COALESCE("name" COLLATE "C" < 'm', false)`},
		{"ref", m("field", "qty", "op", "gte", "ref", "price"), CondOptions{}, `COALESCE("quantity" >= "unit_price", false)`},
		{"ref teks", m("field", "name", "op", "gt", "ref", "ref"), CondOptions{}, `COALESCE("name" COLLATE "C" > "ref_col" COLLATE "C", false)`},
		{"contains", leaf("name", "contains", "Ab"), CondOptions{}, `COALESCE(strpos(translate("name", 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz'), translate('Ab', 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz')) > 0, false)`},
		{"boolean", leaf("ok", "eq", true), CondOptions{}, `"ok" IS NOT DISTINCT FROM true`},
		{"all any not", m("all", []any{
			leaf("status", "eq", "draft"),
			m("any", []any{leaf("n", "gt", 1), m("not", leaf("note", "isnull", true))}),
		}), CondOptions{}, `("status" IS NOT DISTINCT FROM 'draft' AND (COALESCE("n" > 1, false) OR NOT ("note" IS NULL)))`},
		{"literal berbahaya", leaf("status", "eq", "x'); DROP TABLE t; --"), CondOptions{}, `"status" IS NOT DISTINCT FROM 'x''); DROP TABLE t; --'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Condition(parseCond(t, c.cond), condEnv(), c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
			if n := len(splitTopLevel(t, "SELECT "+got)); n != 1 {
				t.Errorf("pernyataan terpecah (%d): %s", n, got)
			}
		})
	}
}

func TestCondition_Errors(t *testing.T) {
	cases := []struct {
		name string
		cond any
		opts CondOptions
		code string
	}{
		{"field tak dikenal", leaf("nope", "eq", "x"), CondOptions{}, ErrUnknownField},
		{"lock: gt tidak boleh", leaf("n", "gt", 1), CondOptions{Restricted: true}, ErrLockUnsupported},
		{"lock: contains tidak boleh", leaf("name", "contains", "x"), CondOptions{Restricted: true}, ErrLockUnsupported},
		{"lock: ref tidak boleh", m("field", "name", "op", "eq", "ref", "ref"), CondOptions{Restricted: true}, ErrLockUnsupported},
		{"lock: kolom turunan tidak boleh", leaf("gen", "eq", "1"), CondOptions{Restricted: true}, ErrGeneratedRef},
		{"literal tipe salah (teks untuk integer)", leaf("n", "eq", "abc"), CondOptions{}, ErrLiteral},
		{"literal tipe salah (angka untuk teks)", leaf("name", "eq", 5), CondOptions{}, ErrLiteral},
		{"desimal tak sah", leaf("total", "eq", "1e3"), CondOptions{}, ErrLiteral},
		{"ref beda keluarga", m("field", "name", "op", "eq", "ref", "n"), CondOptions{}, ErrType},
		{"gt pada boolean", leaf("ok", "gt", true), CondOptions{}, ErrType},
		{"contains pada angka", leaf("n", "contains", "1"), CondOptions{}, ErrType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Condition(parseCond(t, c.cond), condEnv(), c.opts)
			var te *TranslateError
			if !errors.As(err, &te) || te.Code != c.code {
				t.Fatalf("err = %v, want kode %s", err, c.code)
			}
		})
	}
}
