package metadata

import (
	"strconv"
	"strings"
)

// ExprKind membedakan simpul ekspresi (tata bahasa tertutup, Requirement 5.2).
type ExprKind uint8

const (
	// ExprLit: literal null, bool, teks, atau bilangan bulat aman (Lit).
	ExprLit ExprKind = iota
	// ExprDec: literal desimal (Dec).
	ExprDec
	// ExprField: nilai field (Field).
	ExprField
	// ExprConcat: gabung teks atas Args; null menjadi teks kosong.
	ExprConcat
	// ExprCoalesce: argumen pertama yang bukan null.
	ExprCoalesce
	// ExprAdd, ExprSub, ExprMul, ExprDiv: aritmetika desimal; hasil dibulatkan ke Scale dengan Round.
	ExprAdd
	ExprSub
	ExprMul
	ExprDiv
	// ExprIf: Cond ? Then : Else (hanya cabang terpilih yang dievaluasi).
	ExprIf
)

// Expr adalah AST ekspresi accessor. Nilai null merambat melalui aritmetika (null + 1 = null); field yang
// tidak tersedia membuat hasil tak tentu; pembagian nol membuat hasil tak tentu (division_by_zero).
type Expr struct {
	Kind  ExprKind
	Lit   any     // ExprLit: nil, bool, string, int64
	Dec   Decimal // ExprDec
	Field string  // ExprField
	Args  []*Expr // concat, coalesce, add, sub, mul, div
	Cond  *Condition
	Then  *Expr
	Else  *Expr
	Scale int       // aritmetika: 0..MaxScale
	Round RoundMode // aritmetika
}

// ExprResult adalah hasil ekspresi. Type TypeNull dengan Value nil berarti null; bila Reasons tidak kosong,
// hasil tak tentu (Type TypeNull). Value: nil, bool, string (desimal selalu string), atau int64 (integer).
type ExprResult struct {
	Type    FieldType
	Value   any
	Reasons []Reason
}

var exprForms = [...]string{"lit", "dec", "field", "concat", "coalesce", "add", "sub", "mul", "div", "if"}

func (k ExprKind) key() string {
	switch k {
	case ExprLit:
		return "lit"
	case ExprDec:
		return "dec"
	case ExprField:
		return "field"
	case ExprConcat:
		return "concat"
	case ExprCoalesce:
		return "coalesce"
	case ExprAdd:
		return "add"
	case ExprSub:
		return "sub"
	case ExprMul:
		return "mul"
	case ExprDiv:
		return "div"
	}
	return "if"
}

// checkExprNode memeriksa aturan lokal satu simpul (jumlah argumen, skala, mode, literal); nargs adalah
// jumlah argumen sebenarnya. Mengembalikan kode alasan ("" bila sah).
func checkExprNode(e *Expr, nargs int) string {
	switch e.Kind {
	case ExprLit:
		switch x := e.Lit.(type) {
		case nil, bool:
		case string:
			if tooLong(x) {
				return ReasonInvalidExpression
			}
		case int64:
			if _, ok := safeInt64(x); !ok {
				return ReasonInvalidExpression
			}
		default:
			return ReasonInvalidExpression
		}
	case ExprField:
		if !isIdentLen(e.Field) {
			return ReasonInvalidExpression
		}
	case ExprConcat, ExprCoalesce:
		if nargs < 1 || nargs > MaxConcatArgs {
			return ReasonInvalidExpression
		}
	case ExprAdd, ExprMul:
		if nargs < 2 || nargs > MaxArithArgs || !arithOK(e) {
			return ReasonInvalidExpression
		}
	case ExprSub, ExprDiv:
		if nargs != 2 || !arithOK(e) {
			return ReasonInvalidExpression
		}
	case ExprIf:
		if e.Cond == nil || e.Then == nil || e.Else == nil {
			return ReasonInvalidExpression
		}
	case ExprDec:
	default:
		return ReasonInvalidExpression
	}
	return ""
}

func arithOK(e *Expr) bool { return e.Scale >= 0 && e.Scale <= MaxScale && e.Round.Valid() }

// ParseExpr membangun ekspresi dari JSON hasil dekode. Bentuk simpul: {lit}, {dec}, {field}, {concat:[...]},
// {coalesce:[...]}, {add|sub|mul|div:[...], scale, round}, {if:{kondisi}, then:{...}, else:{...}}.
func ParseExpr(raw any) (*Expr, error) {
	e, r := parseExprNode(raw, "$", 1, &budget{max: MaxExprNodes})
	if r != nil {
		return nil, &Error{*r}
	}
	return e, nil
}

func parseExprNode(raw any, path string, depth int, b *budget) (*Expr, *Reason) {
	if depth > MaxExprDepth {
		return nil, &Reason{ReasonTooDeep, path}
	}
	if b.nodes++; b.nodes > b.max {
		return nil, &Reason{ReasonTooManyNodes, path}
	}
	bad := func() (*Expr, *Reason) { return nil, &Reason{ReasonInvalidExpression, path} }
	m, ok := raw.(map[string]any)
	if !ok {
		return bad()
	}
	form, forms := "", 0
	for _, f := range exprForms {
		if _, has := m[f]; has {
			form = f
			forms++
		}
	}
	if forms != 1 {
		return bad()
	}
	want := 1
	switch form {
	case "add", "sub", "mul", "div", "if":
		want = 3
	}
	if len(m) != want {
		return bad()
	}
	e := &Expr{}
	val := m[form]

	// args mengurai larik argumen; nargs sudah diperiksa sebelum anak diurai (pre-order).
	args := func() *Reason {
		list, ok := val.([]any)
		if !ok {
			return &Reason{ReasonInvalidExpression, path}
		}
		if code := checkExprNode(e, len(list)); code != "" {
			return &Reason{code, path}
		}
		for i, el := range list {
			child, r := parseExprNode(el, path+"."+form+"["+strconv.Itoa(i)+"]", depth+1, b)
			if r != nil {
				return r
			}
			e.Args = append(e.Args, child)
		}
		return nil
	}

	switch form {
	case "lit":
		e.Kind = ExprLit
		switch x := val.(type) {
		case nil, bool, string:
			e.Lit = x
		default:
			n, ok := toSafeInt(x)
			if !ok {
				return bad()
			}
			e.Lit = n
		}
	case "dec":
		s, ok := val.(string)
		if !ok {
			return bad()
		}
		d, ok := ParseDecimal(s)
		if !ok {
			return bad()
		}
		e.Kind, e.Dec = ExprDec, d
	case "field":
		s, ok := val.(string)
		if !ok {
			return bad()
		}
		e.Kind, e.Field = ExprField, s
	case "concat", "coalesce":
		e.Kind = ExprConcat
		if form == "coalesce" {
			e.Kind = ExprCoalesce
		}
		if r := args(); r != nil {
			return nil, r
		}
		return e, nil
	case "add", "sub", "mul", "div":
		e.Kind = map[string]ExprKind{"add": ExprAdd, "sub": ExprSub, "mul": ExprMul, "div": ExprDiv}[form]
		scale, ok := toSafeInt(m["scale"])
		round, ok2 := m["round"].(string)
		if !ok || !ok2 || scale < 0 || scale > MaxScale {
			return bad()
		}
		e.Scale, e.Round = int(scale), RoundMode(round)
		if r := args(); r != nil {
			return nil, r
		}
		return e, nil
	default: // if
		e.Kind = ExprIf
		thenRaw, hasThen := m["then"]
		elseRaw, hasElse := m["else"]
		if !hasThen || !hasElse {
			return bad()
		}
		cond, r := parseCondNode(val, path+".if", 1, b)
		if r != nil {
			return nil, r
		}
		e.Cond = cond
		if e.Then, r = parseExprNode(thenRaw, path+".then", depth+1, b); r != nil {
			return nil, r
		}
		if e.Else, r = parseExprNode(elseRaw, path+".else", depth+1, b); r != nil {
			return nil, r
		}
		return e, nil
	}
	if code := checkExprNode(e, 0); code != "" {
		return nil, &Reason{code, path}
	}
	return e, nil
}

func validateExpr(e *Expr, path string, depth int, b *budget) *Reason {
	if depth > MaxExprDepth {
		return &Reason{ReasonTooDeep, path}
	}
	if b.nodes++; b.nodes > b.max {
		return &Reason{ReasonTooManyNodes, path}
	}
	if e == nil {
		return &Reason{ReasonInvalidExpression, path}
	}
	if code := checkExprNode(e, len(e.Args)); code != "" {
		return &Reason{code, path}
	}
	switch e.Kind {
	case ExprConcat, ExprCoalesce, ExprAdd, ExprSub, ExprMul, ExprDiv:
		for i, a := range e.Args {
			if r := validateExpr(a, path+"."+e.Kind.key()+"["+strconv.Itoa(i)+"]", depth+1, b); r != nil {
				return r
			}
		}
	case ExprIf:
		if r := validateNode(e.Cond, path+".if", 1, b); r != nil {
			return r
		}
		if r := validateExpr(e.Then, path+".then", depth+1, b); r != nil {
			return r
		}
		return validateExpr(e.Else, path+".else", depth+1, b)
	default:
		if len(e.Args) != 0 {
			return &Reason{ReasonInvalidExpression, path}
		}
	}
	return nil
}

// Validate memeriksa batas dan kebentukan AST; hasilnya *Error atau nil.
func (e *Expr) Validate() error {
	if r := validateExpr(e, "$", 1, &budget{max: MaxExprNodes}); r != nil {
		return &Error{*r}
	}
	return nil
}

// EvaluateExpr menafsirkan ekspresi mentah (JSON hasil dekode) dan mengevaluasinya. Fungsi total: struktur
// tidak sah menghasilkan Type TypeNull dengan satu Reason.
func EvaluateExpr(raw any, types Types, rec Record) ExprResult {
	e, err := ParseExpr(raw)
	if err != nil {
		return ExprResult{Type: TypeNull, Reasons: []Reason{err.(*Error).Reason}}
	}
	return e.eval(types, rec)
}

// Eval mengevaluasi ekspresi terhadap record. Fungsi total; tidak pernah panik.
func (e *Expr) Eval(types Types, rec Record) ExprResult {
	if err := e.Validate(); err != nil {
		return ExprResult{Type: TypeNull, Reasons: []Reason{err.(*Error).Reason}}
	}
	return e.eval(types, rec)
}

func (e *Expr) eval(types Types, rec Record) ExprResult {
	ev := &evaluator{types: types, rec: rec}
	v, rs := ev.expr(e, "$")
	if len(rs) > 0 {
		return ExprResult{Type: TypeNull, Reasons: rs}
	}
	switch v.k {
	case kNull:
		return ExprResult{Type: TypeNull}
	case kBool:
		return ExprResult{Type: TypeBoolean, Value: v.b}
	case kNum:
		if v.isInt {
			return ExprResult{Type: TypeInteger, Value: v.num.IntPart()}
		}
		return ExprResult{Type: TypeDecimal, Value: v.num.String()}
	}
	return ExprResult{Type: v.ft, Value: v.s}
}

func (ev *evaluator) expr(e *Expr, path string) (value, []Reason) {
	ev.steps++
	unk := func(code string) (value, []Reason) { return nullValue(), []Reason{{code, path}} }
	switch e.Kind {
	case ExprLit:
		switch x := e.Lit.(type) {
		case bool:
			return value{k: kBool, ft: TypeBoolean, b: x}, nil
		case string:
			return value{k: kText, ft: TypeString, s: normText(x)}, nil
		case int64:
			return value{k: kNum, ft: TypeInteger, num: DecimalFromInt(x), isInt: true}, nil
		}
		return nullValue(), nil
	case ExprDec:
		return value{k: kNum, ft: TypeDecimal, num: e.Dec}, nil
	case ExprField:
		v, code := ev.fieldValue(e.Field)
		if code != "" {
			return unk(code)
		}
		return v, nil
	case ExprIf:
		t, rs := ev.cond(e.Cond, path+".if")
		switch t {
		case triUnknown:
			return nullValue(), rs
		case triTrue:
			return ev.expr(e.Then, path+".then")
		}
		return ev.expr(e.Else, path+".else")
	}

	vals := make([]value, 0, len(e.Args))
	var unknown []Reason
	for i, a := range e.Args {
		v, rs := ev.expr(a, path+"."+e.Kind.key()+"["+strconv.Itoa(i)+"]")
		unknown = append(unknown, rs...)
		vals = append(vals, v)
	}
	if len(unknown) > 0 {
		return nullValue(), unknown
	}

	switch e.Kind {
	case ExprConcat:
		var sb strings.Builder
		for _, v := range vals {
			sb.WriteString(valueText(v))
		}
		s := sb.String()
		if tooLong(s) {
			return unk(ReasonValueTooLong)
		}
		return value{k: kText, ft: TypeString, s: s}, nil
	case ExprCoalesce:
		for _, v := range vals {
			if v.k != kNull {
				return v, nil
			}
		}
		return nullValue(), nil
	}

	// aritmetika desimal
	hasNull := false
	for _, v := range vals {
		switch v.k {
		case kNull:
			hasNull = true
		case kNum:
		default:
			return unk(ReasonTypeMismatch)
		}
	}
	if hasNull {
		return nullValue(), nil
	}
	var res Decimal
	switch e.Kind {
	case ExprAdd:
		res = vals[0].num
		for _, v := range vals[1:] {
			res = res.Add(v.num)
		}
	case ExprMul:
		res = vals[0].num
		for _, v := range vals[1:] {
			res = res.Mul(v.num)
		}
	case ExprSub:
		res = vals[0].num.Sub(vals[1].num)
	default: // ExprDiv
		q, ok := vals[0].num.Div(vals[1].num, e.Scale, e.Round)
		if !ok {
			return unk(ReasonDivisionByZero)
		}
		return value{k: kNum, ft: TypeDecimal, num: q}, nil
	}
	return value{k: kNum, ft: TypeDecimal, num: res.Round(e.Scale, e.Round)}, nil
}
