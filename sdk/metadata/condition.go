package metadata

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Op adalah operator daun kondisi (selaras api-guidelines §5.1).
type Op string

const (
	OpEq       Op = "eq"
	OpNe       Op = "ne"
	OpGt       Op = "gt"
	OpGte      Op = "gte"
	OpLt       Op = "lt"
	OpLte      Op = "lte"
	OpIn       Op = "in"
	OpContains Op = "contains"
	OpIsNull   Op = "isnull"
)

func (o Op) valid() bool {
	switch o {
	case OpEq, OpNe, OpGt, OpGte, OpLt, OpLte, OpIn, OpContains, OpIsNull:
		return true
	}
	return false
}

// CondKind membedakan simpul kondisi.
type CondKind uint8

const (
	// CondLeaf: perbandingan {Field, Op, Value|Ref}.
	CondLeaf CondKind = iota
	// CondAll: AND atas Children (minimal satu).
	CondAll
	// CondAny: OR atas Children (minimal satu).
	CondAny
	// CondNot: negasi atas tepat satu Child.
	CondNot
)

// Condition adalah AST kondisi (Requirement 4.1). Kode bangkitan menanamnya sebagai data bertipe memakai
// All, Any, Not, Cmp, CmpRef, dan IsNull; ParseCondition membangunnya dari JSON hasil dekode.
type Condition struct {
	Kind     CondKind
	Children []*Condition
	Field    string
	Op       Op
	// Value adalah literal pembanding: nil, bool, string, bilangan (json.Number atau tipe numerik Go), atau
	// []any untuk OpIn. nil pada eq/ne berarti pembanding null. Untuk OpIsNull: bool (nil berarti true).
	Value any
	// Ref, bila tidak kosong, membandingkan Field dengan field lain pada resource yang sama (Value harus nil).
	Ref string
}

// All membuat simpul AND.
func All(cs ...*Condition) *Condition { return &Condition{Kind: CondAll, Children: cs} }

// Any membuat simpul OR.
func Any(cs ...*Condition) *Condition { return &Condition{Kind: CondAny, Children: cs} }

// Not membuat simpul negasi.
func Not(c *Condition) *Condition { return &Condition{Kind: CondNot, Children: []*Condition{c}} }

// Cmp membuat daun field op literal.
func Cmp(field string, op Op, value any) *Condition {
	return &Condition{Kind: CondLeaf, Field: field, Op: op, Value: value}
}

// CmpRef membuat daun field op ref (bandingkan dengan field lain).
func CmpRef(field string, op Op, ref string) *Condition {
	return &Condition{Kind: CondLeaf, Field: field, Op: op, Ref: ref}
}

// IsNull membuat daun isnull; null=true menguji kosong, false menguji terisi.
func IsNull(field string, null bool) *Condition {
	return &Condition{Kind: CondLeaf, Field: field, Op: OpIsNull, Value: null}
}

// Result adalah hasil evaluasi kondisi. Value false disertai Reasons berarti hasil tak tentu (tipe tak cocok,
// field tidak tersedia, dan sebagainya) atau struktur tidak sah; Reasons kosong berarti hasil pasti.
type Result struct {
	Value   bool
	Reasons []Reason
}

// budget membatasi jumlah simpul satu pemuatan struktur.
type budget struct {
	nodes, max int
}

func isIdentLen(s string) bool { return s != "" && len(s) <= MaxIdentifierLen }

// isScalarLiteral: nil, bool, string, atau bilangan berhingga.
func isScalarLiteral(v any) bool {
	switch x := v.(type) {
	case nil, bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return !math.IsNaN(x) && !math.IsInf(x, 0)
	case float32:
		return !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0)
	case json.Number:
		_, ok := parseJSONNumber(x)
		return ok
	}
	return false
}

// validateLeaf memeriksa kebentukan daun; mengembalikan kode alasan ("" bila sah).
func validateLeaf(c *Condition) string {
	if !isIdentLen(c.Field) || !c.Op.valid() || len(c.Ref) > MaxIdentifierLen || len(c.Children) != 0 {
		return ReasonInvalidCondition
	}
	switch c.Op {
	case OpIsNull:
		if c.Ref != "" {
			return ReasonInvalidCondition
		}
		if _, ok := c.Value.(bool); !ok && c.Value != nil {
			return ReasonInvalidCondition
		}
	case OpIn:
		list, ok := c.Value.([]any)
		if c.Ref != "" || !ok || len(list) == 0 {
			return ReasonInvalidCondition
		}
		if len(list) > MaxInValues {
			return ReasonListTooLong
		}
		for _, e := range list {
			if e == nil || !isScalarLiteral(e) {
				return ReasonInvalidCondition
			}
		}
	case OpEq, OpNe:
		if c.Ref != "" && c.Value != nil {
			return ReasonInvalidCondition
		}
		if !isScalarLiteral(c.Value) {
			return ReasonInvalidCondition
		}
	default: // gt, gte, lt, lte, contains
		if c.Ref != "" {
			if c.Value != nil {
				return ReasonInvalidCondition
			}
		} else if c.Value == nil || !isScalarLiteral(c.Value) {
			return ReasonInvalidCondition
		}
	}
	return ""
}

// validateNode memeriksa batas dan bentuk secara pre-order; berhenti pada pelanggaran pertama.
func validateNode(c *Condition, path string, depth int, b *budget) *Reason {
	if depth > MaxConditionDepth {
		return &Reason{ReasonTooDeep, path}
	}
	if b.nodes++; b.nodes > b.max {
		return &Reason{ReasonTooManyNodes, path}
	}
	if c == nil {
		return &Reason{ReasonInvalidCondition, path}
	}
	switch c.Kind {
	case CondAll, CondAny:
		if len(c.Children) == 0 || c.Field != "" || c.Ref != "" || c.Value != nil || c.Op != "" {
			return &Reason{ReasonInvalidCondition, path}
		}
		key := "all"
		if c.Kind == CondAny {
			key = "any"
		}
		for i, ch := range c.Children {
			if r := validateNode(ch, path+"."+key+"["+strconv.Itoa(i)+"]", depth+1, b); r != nil {
				return r
			}
		}
	case CondNot:
		if len(c.Children) != 1 || c.Field != "" || c.Ref != "" || c.Value != nil || c.Op != "" {
			return &Reason{ReasonInvalidCondition, path}
		}
		return validateNode(c.Children[0], path+".not", depth+1, b)
	case CondLeaf:
		if code := validateLeaf(c); code != "" {
			return &Reason{code, path}
		}
	default:
		return &Reason{ReasonInvalidCondition, path}
	}
	return nil
}

// Validate memeriksa batas (Requirement 1.3, 19.1) dan kebentukan AST; hasilnya *Error atau nil.
func (c *Condition) Validate() error {
	if r := validateNode(c, "$", 1, &budget{max: MaxConditionNodes}); r != nil {
		return &Error{*r}
	}
	return nil
}

// ParseCondition membangun kondisi dari JSON hasil dekode (map[string]any, []any, string, bool, nil, bilangan).
// Bentuk: {all:[...]} | {any:[...]} | {not:{...}} | {field, op, value} | {field, op, ref}. Galat berupa *Error.
func ParseCondition(raw any) (*Condition, error) {
	c, r := parseCondNode(raw, "$", 1, &budget{max: MaxConditionNodes})
	if r != nil {
		return nil, &Error{*r}
	}
	return c, nil
}

func parseCondNode(raw any, path string, depth int, b *budget) (*Condition, *Reason) {
	if depth > MaxConditionDepth {
		return nil, &Reason{ReasonTooDeep, path}
	}
	if b.nodes++; b.nodes > b.max {
		return nil, &Reason{ReasonTooManyNodes, path}
	}
	invalid := func() (*Condition, *Reason) { return nil, &Reason{ReasonInvalidCondition, path} }
	m, ok := raw.(map[string]any)
	if !ok {
		return invalid()
	}
	_, hasAll := m["all"]
	_, hasAny := m["any"]
	_, hasNot := m["not"]
	if hasAll || hasAny || hasNot {
		if len(m) != 1 {
			return invalid()
		}
		switch {
		case hasNot:
			child, r := parseCondNode(m["not"], path+".not", depth+1, b)
			if r != nil {
				return nil, r
			}
			return Not(child), nil
		default:
			key, kind := "all", CondAll
			if hasAny {
				key, kind = "any", CondAny
			}
			list, ok := m[key].([]any)
			if !ok || len(list) == 0 {
				return invalid()
			}
			c := &Condition{Kind: kind}
			for i, el := range list {
				child, r := parseCondNode(el, path+"."+key+"["+strconv.Itoa(i)+"]", depth+1, b)
				if r != nil {
					return nil, r
				}
				c.Children = append(c.Children, child)
			}
			return c, nil
		}
	}
	for k := range m {
		if k != "field" && k != "op" && k != "value" && k != "ref" {
			return invalid()
		}
	}
	field, ok1 := m["field"].(string)
	opStr, ok2 := m["op"].(string)
	if !ok1 || !ok2 {
		return invalid()
	}
	value, hasValue := m["value"]
	refRaw, hasRef := m["ref"]
	ref := ""
	if hasRef {
		s, ok := refRaw.(string)
		if !ok || s == "" {
			return invalid()
		}
		ref = s
	}
	op := Op(opStr)
	if hasRef && hasValue {
		return invalid()
	}
	if !hasRef && !hasValue && op != OpIsNull {
		return invalid()
	}
	c := &Condition{Kind: CondLeaf, Field: field, Op: op, Value: value, Ref: ref}
	if code := validateLeaf(c); code != "" {
		return nil, &Reason{code, path}
	}
	return c, nil
}

// Evaluate menafsirkan kondisi mentah (JSON hasil dekode) dan mengevaluasinya. Fungsi total: struktur tidak
// sah menghasilkan Value false dengan satu Reason.
func Evaluate(raw any, types Types, rec Record) Result {
	c, err := ParseCondition(raw)
	if err != nil {
		return Result{Reasons: []Reason{err.(*Error).Reason}}
	}
	return c.eval(types, rec)
}

// Eval mengevaluasi kondisi terhadap record. Fungsi total: AST tidak sah (nil, terlalu dalam, terlalu besar)
// menghasilkan Value false dengan satu Reason; tidak pernah panik.
func (c *Condition) Eval(types Types, rec Record) Result {
	if err := c.Validate(); err != nil {
		return Result{Reasons: []Reason{err.(*Error).Reason}}
	}
	return c.eval(types, rec)
}

func (c *Condition) eval(types Types, rec Record) Result {
	ev := &evaluator{types: types, rec: rec}
	t, rs := ev.cond(c, "$")
	if t == triUnknown {
		return Result{Reasons: rs}
	}
	return Result{Value: t == triTrue}
}

type tri uint8

const (
	triFalse tri = iota
	triTrue
	triUnknown
)

func boolTri(b bool) tri {
	if b {
		return triTrue
	}
	return triFalse
}

// evaluator memegang konteks satu evaluasi; steps menghitung simpul yang dikunjungi (untuk uji linear).
type evaluator struct {
	types Types
	rec   Record
	steps int
}

func (ev *evaluator) cond(c *Condition, path string) (tri, []Reason) {
	ev.steps++
	switch c.Kind {
	case CondAll, CondAny:
		key := "all"
		if c.Kind == CondAny {
			key = "any"
		}
		sawTrue, sawFalse := false, false
		var unk []Reason
		for i, ch := range c.Children {
			t, rs := ev.cond(ch, path+"."+key+"["+strconv.Itoa(i)+"]")
			switch t {
			case triTrue:
				sawTrue = true
			case triFalse:
				sawFalse = true
			default:
				unk = append(unk, rs...)
			}
		}
		if c.Kind == CondAll {
			switch {
			case sawFalse:
				return triFalse, nil
			case len(unk) > 0:
				return triUnknown, unk
			}
			return triTrue, nil
		}
		switch {
		case sawTrue:
			return triTrue, nil
		case len(unk) > 0:
			return triUnknown, unk
		}
		return triFalse, nil
	case CondNot:
		t, rs := ev.cond(c.Children[0], path+".not")
		switch t {
		case triTrue:
			return triFalse, nil
		case triFalse:
			return triTrue, nil
		}
		return triUnknown, rs
	}
	return ev.leaf(c, path)
}

// fieldValue mengambil dan mengoersi nilai field; kode alasan ("" bila sah).
func (ev *evaluator) fieldValue(name string) (value, string) {
	ft, ok := ev.types[name]
	if !ok {
		return value{}, ReasonUnknownField
	}
	raw, present := ev.rec[name]
	if !present {
		return value{}, ReasonFieldUnavailable
	}
	return coerce(ft, raw)
}

func orderable(ft FieldType) bool {
	switch ft {
	case TypeString, TypeText, TypeInteger, TypeDecimal, TypeDate, TypeDateTime:
		return true
	}
	return false
}

func textual(ft FieldType) bool { return ft == TypeString || ft == TypeText }

func opSupported(op Op, ft FieldType) bool {
	switch op {
	case OpGt, OpGte, OpLt, OpLte:
		return orderable(ft)
	case OpContains:
		return textual(ft)
	}
	return true
}

func (ev *evaluator) leaf(c *Condition, path string) (tri, []Reason) {
	unk := func(code string) (tri, []Reason) { return triUnknown, []Reason{{code, path}} }
	ft, declared := ev.types[c.Field]
	if !declared {
		return unk(ReasonUnknownField)
	}
	a, code := ev.fieldValue(c.Field)
	if code != "" {
		return unk(code)
	}
	if c.Op == OpIsNull {
		want := true
		if b, ok := c.Value.(bool); ok {
			want = b
		}
		return boolTri((a.k == kNull) == want), nil
	}
	if !opSupported(c.Op, ft) {
		return unk(ReasonUnsupportedOp)
	}

	if c.Op == OpIn {
		list, _ := c.Value.([]any)
		vals := make([]value, 0, len(list))
		for _, e := range list {
			v, code := coerce(ft, e)
			if code != "" {
				return unk(code)
			}
			vals = append(vals, v)
		}
		if a.k == kNull {
			return triFalse, nil
		}
		for _, v := range vals {
			if cmpValues(a, v) == 0 {
				return triTrue, nil
			}
		}
		return triFalse, nil
	}

	var b value
	if c.Ref != "" {
		rt, ok := ev.types[c.Ref]
		if !ok {
			return unk(ReasonUnknownField)
		}
		var code string
		if b, code = ev.fieldValue(c.Ref); code != "" {
			return unk(code)
		}
		if !opSupported(c.Op, rt) {
			return unk(ReasonUnsupportedOp)
		}
		fa, _ := familyOf(ft)
		if fb, _ := familyOf(rt); fa != fb {
			return unk(ReasonTypeMismatch)
		}
	} else if c.Value == nil {
		b = nullValue() // literal null (hanya eq/ne yang lolos validasi)
	} else {
		var code string
		if b, code = coerce(ft, c.Value); code != "" {
			return unk(code)
		}
	}

	switch c.Op {
	case OpEq, OpNe:
		eq := (a.k == kNull && b.k == kNull) || (a.k != kNull && b.k != kNull && cmpValues(a, b) == 0)
		return boolTri(eq == (c.Op == OpEq)), nil
	case OpContains:
		if a.k == kNull || b.k == kNull {
			return triFalse, nil
		}
		return boolTri(strings.Contains(asciiFold(a.s), asciiFold(b.s))), nil
	}
	if a.k == kNull || b.k == kNull {
		return triFalse, nil
	}
	n := cmpValues(a, b)
	switch c.Op {
	case OpGt:
		return boolTri(n > 0), nil
	case OpGte:
		return boolTri(n >= 0), nil
	case OpLt:
		return boolTri(n < 0), nil
	}
	return boolTri(n <= 0), nil // OpLte
}
