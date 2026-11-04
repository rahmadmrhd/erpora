package metadata

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestProgrammaticConditionAndTypedRecord(t *testing.T) {
	types := Types{"total": TypeDecimal, "qty": TypeInteger, "status": TypeEnum, "note": TypeText, "dueOn": TypeDate, "issuedOn": TypeDate}
	dec, _ := ParseDecimal("50000000.00")
	cond := All(
		Cmp("total", OpGte, "50000000.00"),
		Any(Cmp("status", OpIn, []any{"draft", "submitted"}), IsNull("note", true)),
		Not(Cmp("qty", OpLt, 1)),
		CmpRef("dueOn", OpGte, "issuedOn"),
	)
	rec := Record{"total": dec, "qty": int64(3), "status": "draft", "note": nil, "dueOn": "2026-02-01", "issuedOn": "2026-01-01"}
	if res := cond.Eval(types, rec); !res.Value || len(res.Reasons) != 0 {
		t.Fatalf("Eval = %+v", res)
	}
	rec["qty"] = 0
	if res := cond.Eval(types, rec); res.Value || len(res.Reasons) != 0 {
		t.Fatalf("qty 0 harus false pasti: %+v", res)
	}
}

func TestIntegerAcceptsOnlySafeIntegers(t *testing.T) {
	types := Types{"i": TypeInteger}
	cases := []struct {
		raw  any
		want bool
	}{
		{5, true}, {int8(-5), true}, {uint8(5), true}, {int64(maxSafeInt), true}, {int64(maxSafeInt) + 1, false},
		{uint64(maxSafeInt) + 1, false}, {float64(5), true}, {5.5, false}, {math.NaN(), false}, {math.Inf(1), false},
		{float32(7), true}, {json.Number("5"), true}, {json.Number("5.0"), true}, {json.Number("1e2"), true},
		{json.Number("9007199254740993"), false}, {json.Number("x"), false}, {"5", false}, {true, false},
	}
	for _, c := range cases {
		res := Cmp("i", OpIsNull, false).Eval(types, Record{"i": c.raw})
		if got := res.Value && len(res.Reasons) == 0; got != c.want {
			t.Errorf("integer %#v diterima=%v, want %v (%v)", c.raw, got, c.want, res.Reasons)
		}
	}
}

func TestStringLengthCountsUTF16(t *testing.T) {
	types := Types{"s": TypeString}
	check := func(s string) []Reason { return Cmp("s", OpEq, "").Eval(types, Record{"s": s}).Reasons }
	if r := check(strings.Repeat("\U0001F600", MaxStringLen/2)); len(r) != 0 {
		t.Errorf("65536 satuan UTF-16 harus diterima: %v", r)
	}
	if r := check(strings.Repeat("\U0001F600", MaxStringLen/2+1)); len(r) != 1 || r[0].Code != ReasonValueTooLong {
		t.Errorf("65538 satuan UTF-16 harus ditolak: %v", r)
	}
	if r := check(strings.Repeat("é", MaxStringLen+1)); len(r) != 1 {
		t.Errorf("65537 karakter BMP harus ditolak: %v", r)
	}
}

func TestInvalidUTF8IsNormalized(t *testing.T) {
	types := Types{"s": TypeString}
	// byte tidak sah dibaca sebagai U+FFFD per byte, sama seperti surrogat tunggal di klien.
	if res := Cmp("s", OpEq, "��").Eval(types, Record{"s": "\xff\xfe"}); !res.Value {
		t.Fatalf("byte tidak sah harus setara U+FFFD: %+v", res)
	}
	if res := Cmp("s", OpLt, "￾").Eval(types, Record{"s": "\xff"}); !res.Value {
		t.Fatalf("urutan setelah normalisasi: %+v", res)
	}
}

func TestContainsFoldsOnlyASCII(t *testing.T) {
	types := Types{"s": TypeString}
	for _, c := range []struct {
		hay, needle string
		want        bool
	}{
		{"ABC", "bc", true}, {"abc", "BC", true}, {"É", "é", false}, {"K", "k", false}, {"İ", "i", false}, {"x", "", true},
	} {
		if res := Cmp("s", OpContains, c.needle).Eval(types, Record{"s": c.hay}); res.Value != c.want {
			t.Errorf("contains(%q,%q) = %v, want %v", c.hay, c.needle, res.Value, c.want)
		}
	}
}

func TestExprProgrammatic(t *testing.T) {
	types := Types{"price": TypeDecimal, "qty": TypeInteger}
	e := &Expr{Kind: ExprMul, Scale: 2, Round: RoundHalfUp, Args: []*Expr{{Kind: ExprField, Field: "price"}, {Kind: ExprField, Field: "qty"}}}
	res := e.Eval(types, Record{"price": "19.995", "qty": 3})
	if res.Type != TypeDecimal || res.Value != "59.99" { // 59.985 -> 59.99
		t.Fatalf("Eval = %+v", res)
	}
	if res := (&Expr{Kind: ExprAdd, Scale: 40, Round: RoundHalfUp, Args: []*Expr{{Kind: ExprLit, Lit: int64(1)}, {Kind: ExprLit, Lit: int64(1)}}}).Eval(types, nil); res.Type != TypeNull || len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonInvalidExpression {
		t.Fatalf("skala 40 harus ditolak: %+v", res)
	}
}

func TestTypedErrorsAreReturned(t *testing.T) {
	if _, err := ParseCondition(map[string]any{"all": []any{}}); err == nil {
		t.Fatal("kondisi tidak sah harus galat")
	} else if e, ok := err.(*Error); !ok || e.Reason.Code != ReasonInvalidCondition {
		t.Fatalf("galat = %#v", err)
	}
	if _, err := ParseExpr(map[string]any{}); err == nil {
		t.Fatal("ekspresi tidak sah harus galat")
	}
	if _, err := ParseLabelTemplate("{"); err == nil {
		t.Fatal("templat tidak sah harus galat")
	}
	if err := ValidatePattern("(?=a)"); err == nil {
		t.Fatal("lookahead harus ditolak")
	}
}

func TestCompilePatternMatches(t *testing.T) {
	p, err := CompilePattern(`^[A-Z]{2}\d{2}$`)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Match("AB12") || p.Match("ab12") {
		t.Fatal("pencocokan salah")
	}
}

func TestUnknownRoundModeIsRejected(t *testing.T) {
	d, _ := ParseDecimal("2.5")
	for _, m := range []RoundMode{"", "bankers", "HALF_UP", "half-down", "toString"} {
		if r, ok := d.Round(0, m); ok {
			t.Errorf("Round dengan mode %q harus ditolak, dapat %s", m, r)
		}
		if _, ok := d.Div(DecimalFromInt(3), 2, m); ok {
			t.Errorf("Div dengan mode %q harus ditolak", m)
		}
	}
	if _, ok := d.Round(-1, RoundHalfUp); ok {
		t.Error("skala negatif harus ditolak")
	}
	if _, ok := d.Div(DecimalFromInt(3), -1, RoundHalfUp); ok {
		t.Error("Div dengan skala negatif harus ditolak")
	}
	// mode sah tetap bekerja, termasuk skala yang sama dengan nilai
	if r, ok := d.Round(1, RoundHalfDown); !ok || r.String() != "2.5" {
		t.Errorf("Round skala sama = %v %v", r, ok)
	}
	// AST bangunan sendiri dengan mode tak dikenal ditolak oleh Validate, bukan dibulatkan diam-diam
	bad := &Expr{Kind: ExprAdd, Scale: 0, Round: "bankers", Args: []*Expr{{Kind: ExprDec, Dec: d}, {Kind: ExprDec, Dec: d}}}
	if res := bad.Eval(nil, nil); res.Type != TypeNull || len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonInvalidExpression {
		t.Errorf("mode tak dikenal pada ekspresi: %+v", res)
	}
}

func TestNonFiniteJSONNumbersAreInvalid(t *testing.T) {
	types := Types{"i": TypeInteger, "d": TypeDecimal}
	for _, n := range []json.Number{"NaN", "nan", "Infinity", "-Infinity", "inf", "+Inf", "1e999", "-1e999", "0x1p3", "1_0", "", "+1", ".5", "1.", "01", " 1", "--1"} {
		res := Evaluate(map[string]any{"field": "i", "op": "eq", "value": n}, types, Record{"i": json.Number("1")})
		if res.Value || len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonInvalidCondition {
			t.Errorf("literal %q: %+v, want invalid_condition", string(n), res)
		}
		res = Evaluate(map[string]any{"field": "i", "op": "in", "value": []any{n}}, types, Record{"i": json.Number("1")})
		if len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonInvalidCondition {
			t.Errorf("elemen in %q: %+v", string(n), res)
		}
		res = Cmp("i", OpIsNull, false).Eval(types, Record{"i": n})
		if res.Value || len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonTypeMismatch {
			t.Errorf("record %q: %+v, want type_mismatch", string(n), res)
		}
		if e := EvaluateExpr(map[string]any{"lit": n}, types, nil); len(e.Reasons) != 1 || e.Reasons[0].Code != ReasonInvalidExpression {
			t.Errorf("lit %q: %+v", string(n), e)
		}
	}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		res := Evaluate(map[string]any{"field": "i", "op": "eq", "value": f}, types, Record{"i": 1})
		if len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonInvalidCondition {
			t.Errorf("float %v: %+v", f, res)
		}
	}
	// angka JSON sah tetap diterima
	for _, n := range []json.Number{"0", "-0", "5", "5.0", "1e2", "-1.5E+3", "9007199254740991"} {
		if res := Evaluate(map[string]any{"field": "i", "op": "gte", "value": n}, Types{"i": TypeInteger}, Record{"i": 0}); len(res.Reasons) == 1 && res.Reasons[0].Code == ReasonInvalidCondition {
			t.Errorf("angka sah %q ditolak", string(n))
		}
	}
}
