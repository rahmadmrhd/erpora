package metadata

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

var propTypes = Types{
	"a": TypeInteger, "b": TypeDecimal, "s": TypeString, "d": TypeDate, "t": TypeDateTime,
	"e": TypeEnum, "u": TypeUUID, "f": TypeBoolean, "x": TypeText,
}

var propOps = []string{"eq", "ne", "gt", "gte", "lt", "lte", "in", "contains", "isnull", "bogus", ""}

func randScalar(r *rand.Rand) any {
	switch r.Intn(14) {
	case 0:
		return nil
	case 1:
		return r.Intn(2) == 0
	case 2:
		return json.Number(fmt.Sprint(r.Intn(2000) - 1000))
	case 3:
		return json.Number("9007199254740993")
	case 4:
		return json.Number("1.5")
	case 5:
		return fmt.Sprintf("%d.%02d", r.Intn(1000), r.Intn(100))
	case 6:
		return "2026-0" + fmt.Sprint(1+r.Intn(9)) + "-1" + fmt.Sprint(r.Intn(10))
	case 7:
		return "2026-01-01T10:00:00Z"
	case 8:
		return "123e4567-e89b-12d3-a456-426614174000"
	case 9:
		return strings.Repeat("abé\U0001F600", r.Intn(4))
	case 10:
		return "\xff\xfe"
	case 11:
		return []any{"a", json.Number("1")}
	case 12:
		return map[string]any{"k": "v"}
	}
	return fmt.Sprintf("v%d", r.Intn(5))
}

func randLeaf(r *rand.Rand) map[string]any {
	fields := []string{"a", "b", "s", "d", "t", "e", "u", "f", "x", "zz"}
	m := map[string]any{"field": fields[r.Intn(len(fields))], "op": propOps[r.Intn(len(propOps))]}
	switch r.Intn(4) {
	case 0:
		m["ref"] = fields[r.Intn(len(fields))]
	case 1:
		list := make([]any, 1+r.Intn(3))
		for i := range list {
			list[i] = randScalar(r)
		}
		m["value"] = list
	default:
		m["value"] = randScalar(r)
	}
	return m
}

func randCond(r *rand.Rand, depth int) any {
	if depth <= 0 || r.Intn(3) == 0 {
		return randLeaf(r)
	}
	n := 1 + r.Intn(3)
	kids := make([]any, n)
	for i := range kids {
		kids[i] = randCond(r, depth-1)
	}
	switch r.Intn(4) {
	case 0:
		return map[string]any{"all": kids}
	case 1:
		return map[string]any{"any": kids}
	case 2:
		return map[string]any{"not": kids[0]}
	}
	// simpul rusak: kunci campur, nilai sembarang
	return map[string]any{"all": randScalar(r), "extra": kids}
}

func randRecord(r *rand.Rand) Record {
	rec := Record{}
	for k := range propTypes {
		switch r.Intn(4) {
		case 0: // tidak tersedia
		default:
			rec[k] = randScalar(r)
		}
	}
	return rec
}

// Property: evaluator tidak panik atau galat untuk input apa pun (Requirement 4.5, 19.1).
func TestPropertyEvaluateIsTotal(t *testing.T) {
	prop := func(seed int64) (ok bool) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("panik untuk seed %d: %v", seed, p)
				ok = false
			}
		}()
		r := rand.New(rand.NewSource(seed))
		raw := randCond(r, 1+r.Intn(6))
		rec := randRecord(r)
		res := Evaluate(raw, propTypes, rec)
		if res.Value && len(res.Reasons) > 0 {
			t.Errorf("seed %d: hasil true tidak boleh membawa alasan", seed)
			return false
		}
		for _, rs := range res.Reasons {
			if rs.Code == "" || !strings.HasPrefix(rs.Path, "$") && !strings.HasPrefix(rs.Path, "{") {
				t.Errorf("seed %d: alasan tidak wajar %q", seed, rs)
				return false
			}
		}
		_ = EvaluateExpr(map[string]any{"concat": []any{raw, map[string]any{"field": "a"}}}, propTypes, rec)
		_ = EvaluateExpr(map[string]any{"if": raw, "then": map[string]any{"lit": "y"}, "else": map[string]any{"field": "b"}}, propTypes, rec)
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
}

func countNodes(c *Condition) int {
	n := 1
	for _, ch := range c.Children {
		n += countNodes(ch)
	}
	return n
}

func randValidCond(r *rand.Rand, depth int, budget *int) *Condition {
	fields := []string{"a", "b", "s", "d", "t", "e", "u", "f", "x", "zz"}
	leaf := func() *Condition {
		f := fields[r.Intn(len(fields))]
		switch r.Intn(5) {
		case 0:
			return IsNull(f, r.Intn(2) == 0)
		case 1:
			return CmpRef(f, []Op{OpEq, OpNe, OpGt, OpLte, OpContains}[r.Intn(5)], fields[r.Intn(len(fields))])
		case 2:
			return Cmp(f, OpIn, []any{randScalar(r), "z"})
		}
		return Cmp(f, []Op{OpEq, OpNe, OpGt, OpGte, OpLt, OpLte, OpContains}[r.Intn(7)], randScalar(r))
	}
	*budget--
	if depth <= 1 || *budget < 3 || r.Intn(3) == 0 {
		return leaf()
	}
	n := 1 + r.Intn(3)
	kids := make([]*Condition, 0, n)
	for i := 0; i < n && *budget > 0; i++ {
		kids = append(kids, randValidCond(r, depth-1, budget))
	}
	switch r.Intn(3) {
	case 0:
		return All(kids...)
	case 1:
		return Any(kids...)
	}
	return Not(kids[0])
}

// Property: jumlah simpul yang dikunjungi sama dengan jumlah simpul (linear, tanpa pengulangan kerja).
func TestPropertyEvalIsLinear(t *testing.T) {
	checked, total := 0, 0
	prop := func(seed int64) bool {
		total++
		r := rand.New(rand.NewSource(seed))
		budget := MaxConditionNodes
		c := randValidCond(r, MaxConditionDepth, &budget)
		leafOnly := c.Kind == CondLeaf && validateLeaf(c) != ""
		if leafOnly {
			return true // daun acak bisa tidak sah (mis. nilai tak sesuai operator); itu diuji di tempat lain
		}
		if err := c.Validate(); err != nil {
			return true
		}
		checked++
		ev := &evaluator{types: propTypes, rec: randRecord(r)}
		ev.cond(c, "$")
		if ev.steps != countNodes(c) {
			t.Errorf("seed %d: steps=%d nodes=%d", seed, ev.steps, countNodes(c))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 3000}); err != nil {
		t.Fatal(err)
	}
	if checked < total/2 {
		t.Fatalf("hanya %d dari %d pohon acak yang valid; pembangkit terlalu longgar", checked, total)
	}
	t.Logf("%d dari %d pohon diperiksa", checked, total)
}

// Struktur melampaui batas ditolak tanpa dieksekusi: pohon raksasa tidak membuat parser bekerja lebih dari
// batas (satu anak dipakai ulang jutaan kali; kedalaman 100.000 tingkat).
func TestOversizeIsRejectedEarly(t *testing.T) {
	leaf := map[string]any{"field": "a", "op": "eq", "value": json.Number("1")}
	wide := make([]any, 2_000_000)
	for i := range wide {
		wide[i] = leaf
	}
	var deep any = leaf
	for i := 0; i < 100_000; i++ {
		deep = map[string]any{"not": deep}
	}
	start := time.Now()
	for name, raw := range map[string]any{"wide": map[string]any{"all": wide}, "deep": deep} {
		res := Evaluate(raw, propTypes, Record{"a": json.Number("1")})
		if res.Value || len(res.Reasons) != 1 {
			t.Fatalf("%s: hasil = %+v", name, res)
		}
		if c := res.Reasons[0].Code; c != ReasonTooManyNodes && c != ReasonTooDeep {
			t.Fatalf("%s: alasan = %v", name, res.Reasons[0])
		}
		if r := EvaluateExpr(map[string]any{"concat": []any{raw}}, propTypes, nil); len(r.Reasons) != 1 {
			t.Fatalf("%s expr: %+v", name, r)
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("penolakan terlalu lambat: %v", d)
	}
	// AST bangunan sendiri yang siklik tidak membuat Eval berputar tanpa henti.
	cyc := &Condition{Kind: CondNot}
	cyc.Children = []*Condition{cyc}
	if res := cyc.Eval(propTypes, nil); res.Value || len(res.Reasons) != 1 || res.Reasons[0].Code != ReasonTooDeep {
		t.Fatalf("siklus: %+v", res)
	}
	var nilCond *Condition
	if res := nilCond.Eval(propTypes, nil); res.Value || len(res.Reasons) != 1 {
		t.Fatalf("nil: %+v", res)
	}
	var nilExpr *Expr
	if res := nilExpr.Eval(propTypes, nil); res.Type != TypeNull || len(res.Reasons) != 1 {
		t.Fatalf("nil expr: %+v", res)
	}
}

func FuzzEvaluate(f *testing.F) {
	f.Add([]byte(`{"field":"a","op":"gt","value":5}`), []byte(`{"a":6}`))
	f.Add([]byte(`{"all":[{"field":"b","op":"lte","ref":"b"},{"not":{"field":"s","op":"contains","value":"x"}}]}`), []byte(`{"b":"1.50","s":"AxA"}`))
	f.Add([]byte(`{"add":[{"field":"b"},{"dec":"1"}],"scale":2,"round":"half_even"}`), []byte(`{"b":"2.345"}`))
	f.Add([]byte(`{"div":[{"lit":1},{"field":"a"}],"scale":3,"round":"floor"}`), []byte(`{"a":0}`))
	f.Fuzz(func(t *testing.T, condJSON, recJSON []byte) {
		decode := func(b []byte) (any, bool) {
			dec := json.NewDecoder(strings.NewReader(string(b)))
			dec.UseNumber()
			var v any
			return v, dec.Decode(&v) == nil
		}
		raw, ok := decode(condJSON)
		if !ok {
			return
		}
		recAny, _ := decode(recJSON)
		rec, _ := recAny.(map[string]any)
		_ = Evaluate(raw, propTypes, Record(rec))
		_ = EvaluateExpr(raw, propTypes, Record(rec))
	})
}

// ---- desimal terhadap oracle big.Rat ----

func randDecString(r *rand.Rand) string {
	var sb strings.Builder
	if r.Intn(2) == 0 {
		sb.WriteByte('-')
	}
	if r.Intn(4) == 0 {
		sb.WriteByte('0')
	} else {
		sb.WriteByte(byte('1' + r.Intn(9)))
		for i, n := 0, r.Intn(20); i < n; i++ {
			sb.WriteByte(byte('0' + r.Intn(10)))
		}
	}
	if scale := r.Intn(8); scale > 0 {
		sb.WriteByte('.')
		for i := 0; i < scale; i++ {
			sb.WriteByte(byte('0' + r.Intn(10)))
		}
	}
	return sb.String()
}

func ratOf(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("Rat.SetString(%q)", s)
	}
	return r
}

func ratDecimal(t *testing.T, d Decimal) *big.Rat { t.Helper(); return ratOf(t, d.String()) }

// refRound membulatkan x ke skala dengan formulasi lantai/pecahan (berbeda dari roundQuotient).
func refRound(x *big.Rat, scale int, mode RoundMode) *big.Rat {
	p := new(big.Rat).SetInt(pow10(scale))
	y := new(big.Rat).Mul(x, p)
	fl := new(big.Int).Div(y.Num(), y.Denom()) // Euclid: lantai karena penyebut > 0
	frac := new(big.Rat).Sub(y, new(big.Rat).SetInt(fl))
	ce := new(big.Int).Set(fl)
	if frac.Sign() > 0 {
		ce.Add(ce, bigOne)
	}
	neg := y.Sign() < 0
	away, toward := ce, fl
	if neg {
		away, toward = fl, ce
	}
	var res *big.Int
	switch mode {
	case RoundFloor:
		res = fl
	case RoundCeiling:
		res = ce
	case RoundDown:
		res = toward
	case RoundUp:
		res = away
	default:
		switch frac.Cmp(big.NewRat(1, 2)) {
		case 1:
			res = ce
		case -1:
			res = fl
		default:
			switch mode {
			case RoundHalfUp:
				res = away
			case RoundHalfDown:
				res = toward
			default:
				if fl.Bit(0) == 0 {
					res = fl
				} else {
					res = ce
				}
			}
		}
	}
	return new(big.Rat).SetFrac(res, pow10(scale))
}

func TestPropertyDecimalMatchesRat(t *testing.T) {
	modes := []RoundMode{RoundHalfUp, RoundHalfDown, RoundHalfEven, RoundUp, RoundDown, RoundFloor, RoundCeiling}
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		as, bs := randDecString(r), randDecString(r)
		a, ok1 := ParseDecimal(as)
		b, ok2 := ParseDecimal(bs)
		if !ok1 || !ok2 {
			t.Errorf("seed %d: pembangkit menghasilkan %q %q tidak sah", seed, as, bs)
			return false
		}
		ra, rb := ratOf(t, as), ratOf(t, bs)
		if got, want := a.Cmp(b), ra.Cmp(rb); got != want {
			t.Errorf("Cmp(%s,%s)=%d want %d", as, bs, got, want)
			return false
		}
		if ratDecimal(t, a.Add(b)).Cmp(new(big.Rat).Add(ra, rb)) != 0 ||
			ratDecimal(t, a.Sub(b)).Cmp(new(big.Rat).Sub(ra, rb)) != 0 ||
			ratDecimal(t, a.Mul(b)).Cmp(new(big.Rat).Mul(ra, rb)) != 0 {
			t.Errorf("aritmetika eksak menyimpang untuk %s, %s", as, bs)
			return false
		}
		if back, ok := ParseDecimal(a.String()); !ok || back.Cmp(a) != 0 || back.String() != a.String() {
			t.Errorf("roundtrip %s -> %s", as, a.String())
			return false
		}
		scale := r.Intn(6)
		mode := modes[r.Intn(len(modes))]
		if got, want := mustRound(t, a, scale, mode), refRound(ra, scale, mode); ratDecimal(t, got).Cmp(want) != 0 || got.Scale() != scale {
			t.Errorf("Round(%s,%d,%s)=%s want %s", as, scale, mode, got, want.FloatString(scale))
			return false
		}
		if rb.Sign() != 0 {
			q, ok := a.Div(b, scale, mode)
			want := refRound(new(big.Rat).Quo(ra, rb), scale, mode)
			if !ok || ratDecimal(t, q).Cmp(want) != 0 || q.Scale() != scale {
				t.Errorf("Div(%s,%s,%d,%s)=%s want %s", as, bs, scale, mode, q, want.FloatString(scale))
				return false
			}
		} else if _, ok := a.Div(b, scale, mode); ok {
			t.Errorf("Div by zero harus ditolak")
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 4000}); err != nil {
		t.Fatal(err)
	}
}

// Property: pola acak tidak membuat validator panik, dan pola yang diterima selalu dapat dikompilasi RE2.
func TestPropertyValidatePatternIsTotal(t *testing.T) {
	atoms := []string{"a", "b", ".", "\\d", "\\w", "[a-c]", "[^x]", "(", ")", "|", "*", "+", "?", "{2}", "{1,3}", "{", "}", "[", "]", "\\", "^", "$", "(?:", "(?=", "\\b", "-", "é", "\U0001F600"}
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		var sb strings.Builder
		for i, n := 0, r.Intn(14); i < n; i++ {
			sb.WriteString(atoms[r.Intn(len(atoms))])
		}
		p := sb.String()
		defer func() {
			if x := recover(); x != nil {
				t.Errorf("panik untuk %q: %v", p, x)
			}
		}()
		if err := ValidatePattern(p); err == nil {
			pat, cerr := CompilePattern(p)
			if cerr != nil {
				t.Errorf("pola %q lolos validasi tetapi gagal kompilasi: %v", p, cerr)
				return false
			}
			_ = pat.Match("ab12é\U0001F600-")
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 20000}); err != nil {
		t.Fatal(err)
	}
}

func mustRound(t *testing.T, a Decimal, scale int, mode RoundMode) Decimal {
	t.Helper()
	r, ok := a.Round(scale, mode)
	if !ok {
		t.Fatalf("Round(%s, %d, %s) ditolak", a, scale, mode)
	}
	return r
}
