package sqlgen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

// Kode galat penerjemah.
const (
	ErrUnknownField    = "unknown_field"
	ErrGeneratedRef    = "generated_ref"
	ErrType            = "type_mismatch"
	ErrNotImmutable    = "not_immutable"
	ErrTooLarge        = "too_large"
	ErrLockUnsupported = "lock_unsupported"
	ErrLiteral         = "invalid_literal"
	ErrInvalid         = "invalid"
)

// MaxSQLBytes membatasi panjang ekspresi hasil terjemahan. Pembulatan selain half_up dan down menggandakan
// sub-ekspresi, sehingga pembulatan bertingkat dapat membesar secara eksponensial.
const MaxSQLBytes = 16 << 10

// TranslateError adalah galat penerjemah ekspresi atau kondisi.
type TranslateError struct {
	Code string
	Msg  string
}

func (e *TranslateError) Error() string { return "sqlgen: " + e.Code + ": " + e.Msg }

func terr(code, format string, args ...any) error {
	return &TranslateError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// FieldInfo menjelaskan satu field metadata yang dapat dirujuk ekspresi atau kondisi.
type FieldInfo struct {
	Column    string
	Type      metadata.FieldType
	Generated bool
}

// Env memetakan nama field metadata (camelCase) ke kolom.
type Env map[string]FieldInfo

// EnvForTable membangun Env dari kolom berasal-field milik tabel.
func EnvForTable(t *dbmodel.Table) Env {
	env := Env{}
	for _, c := range t.Columns {
		if c.Field == "" {
			continue
		}
		env[c.Field] = FieldInfo{Column: c.Name, Type: c.FieldType, Generated: c.Generated != nil}
	}
	return env
}

type family int

const (
	famNull family = iota
	famText
	famUUID
	famInt
	famNum
	famBool
	famTime
)

func familyOf(ft metadata.FieldType) family {
	switch ft {
	case metadata.TypeString, metadata.TypeText, metadata.TypeEnum:
		return famText
	case metadata.TypeUUID:
		return famUUID
	case metadata.TypeInteger:
		return famInt
	case metadata.TypeDecimal:
		return famNum
	case metadata.TypeBoolean:
		return famBool
	case metadata.TypeDate, metadata.TypeDateTime:
		return famTime
	}
	return famNull
}

func numeric(f family) bool { return f == famInt || f == famNum }

func compatible(a, b family) bool {
	return a == b || a == famNull || b == famNull || (numeric(a) && numeric(b))
}

func quoteCol(info FieldInfo) (string, error) { return QuoteIdent(info.Column) }

// ---- kondisi ----

// CondOptions mengatur penerjemahan kondisi.
type CondOptions struct {
	// Qualifier adalah awalan kolom (mis. "OLD"); kosong = tanpa awalan.
	Qualifier string
	// Restricted membatasi ke yang dapat dipakai pemicu: operator eq, ne, in, isnull atas kolom milik sendiri
	// (tanpa ref), design D-11 dan D-26.
	Restricted bool
}

const (
	upperAlpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerAlpha = "abcdefghijklmnopqrstuvwxyz"
)

// Condition menerjemahkan kondisi ke ekspresi boolean SQL. Semantik mengikuti evaluator sdk/metadata: eq dan
// ne memperlakukan null sebagai nilai (IS NOT DISTINCT FROM), perbandingan urut dan contains dengan null
// bernilai false, teks dibandingkan menurut urutan byte (COLLATE "C"), contains tidak peka huruf besar-kecil
// hanya untuk huruf ASCII.
func Condition(c *metadata.Condition, env Env, o CondOptions) (string, error) {
	s, err := condition(c, env, o)
	if err != nil {
		return "", err
	}
	if len(s) > MaxSQLBytes {
		return "", terr(ErrTooLarge, "kondisi %d byte melebihi batas %d", len(s), MaxSQLBytes)
	}
	return s, nil
}

func condition(c *metadata.Condition, env Env, o CondOptions) (string, error) {
	switch c.Kind {
	case metadata.CondAll, metadata.CondAny:
		sep := " AND "
		if c.Kind == metadata.CondAny {
			sep = " OR "
		}
		parts := make([]string, 0, len(c.Children))
		for _, ch := range c.Children {
			s, err := condition(ch, env, o)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "(" + strings.Join(parts, sep) + ")", nil
	case metadata.CondNot:
		s, err := condition(c.Children[0], env, o)
		if err != nil {
			return "", err
		}
		return "NOT (" + s + ")", nil
	}
	return leafCondition(c, env, o)
}

func (o CondOptions) col(info FieldInfo) (string, error) {
	q, err := quoteCol(info)
	if err != nil {
		return "", err
	}
	if o.Qualifier != "" {
		return o.Qualifier + "." + q, nil
	}
	return q, nil
}

func lookup(env Env, name string) (FieldInfo, error) {
	info, ok := env[name]
	if !ok {
		return FieldInfo{}, terr(ErrUnknownField, "field %q tidak dikenal", truncate(name))
	}
	if info.Generated {
		return FieldInfo{}, terr(ErrGeneratedRef, "field %q adalah kolom turunan dan tidak boleh dirujuk di sini", name)
	}
	return info, nil
}

func leafCondition(c *metadata.Condition, env Env, o CondOptions) (string, error) {
	info, err := lookup(env, c.Field)
	if err != nil {
		return "", err
	}
	col, err := o.col(info)
	if err != nil {
		return "", err
	}
	if o.Restricted {
		switch c.Op {
		case metadata.OpEq, metadata.OpNe, metadata.OpIn, metadata.OpIsNull:
		default:
			return "", terr(ErrLockUnsupported, "operator %q tidak dapat diterjemahkan untuk pemicu (hanya eq, ne, in, isnull)", c.Op)
		}
		if c.Ref != "" {
			return "", terr(ErrLockUnsupported, "ref tidak dapat diterjemahkan untuk pemicu")
		}
	}
	fam := familyOf(info.Type)
	if c.Op == metadata.OpIsNull {
		want := true
		if b, ok := c.Value.(bool); ok {
			want = b
		}
		if want {
			return col + " IS NULL", nil
		}
		return col + " IS NOT NULL", nil
	}
	// Operator urut dan contains hanya untuk tipe yang didukung evaluator.
	switch c.Op {
	case metadata.OpGt, metadata.OpGte, metadata.OpLt, metadata.OpLte:
		if (fam != famText || info.Type == metadata.TypeEnum) && !numeric(fam) && fam != famTime {
			return "", terr(ErrType, "operator %q tidak berlaku untuk tipe %s", c.Op, info.Type)
		}
	case metadata.OpContains:
		if info.Type != metadata.TypeString && info.Type != metadata.TypeText {
			return "", terr(ErrType, "contains hanya untuk string dan text, bukan %s", info.Type)
		}
	}

	if c.Op == metadata.OpIn {
		list, _ := c.Value.([]any)
		lits := make([]string, 0, len(list))
		for _, v := range list {
			l, err := condLiteral(info.Type, v)
			if err != nil {
				return "", err
			}
			lits = append(lits, l)
		}
		if len(lits) == 0 {
			return "false", nil
		}
		return "COALESCE(" + col + " IN (" + strings.Join(lits, ", ") + "), false)", nil
	}

	var rhs string
	textCmp := fam == famText && info.Type != metadata.TypeEnum
	if c.Ref != "" {
		ref, err := lookup(env, c.Ref)
		if err != nil {
			return "", err
		}
		if familyOf(ref.Type) != fam && (!numeric(fam) || !numeric(familyOf(ref.Type))) {
			return "", terr(ErrType, "ref %q bertipe %s tidak sebanding dengan %s", c.Ref, ref.Type, info.Type)
		}
		if rhs, err = o.col(ref); err != nil {
			return "", err
		}
		if textCmp && isOrdering(c.Op) {
			rhs += ` COLLATE "C"`
		}
	} else if c.Value == nil {
		switch c.Op {
		case metadata.OpEq:
			return col + " IS NULL", nil
		case metadata.OpNe:
			return col + " IS NOT NULL", nil
		}
		return "", terr(ErrLiteral, "pembanding null hanya untuk eq dan ne")
	} else if rhs, err = condLiteral(info.Type, c.Value); err != nil {
		return "", err
	}

	switch c.Op {
	case metadata.OpEq:
		return col + " IS NOT DISTINCT FROM " + rhs, nil
	case metadata.OpNe:
		return col + " IS DISTINCT FROM " + rhs, nil
	case metadata.OpContains:
		fold := func(x string) string {
			return "translate(" + x + ", '" + upperAlpha + "', '" + lowerAlpha + "')"
		}
		return "COALESCE(strpos(" + fold(col) + ", " + fold(rhs) + ") > 0, false)", nil
	}
	lhs := col
	if textCmp {
		lhs += ` COLLATE "C"`
	}
	op := map[metadata.Op]string{metadata.OpGt: ">", metadata.OpGte: ">=", metadata.OpLt: "<", metadata.OpLte: "<="}[c.Op]
	return "COALESCE(" + lhs + " " + op + " " + rhs + ", false)", nil
}

func isOrdering(op metadata.Op) bool {
	return op == metadata.OpGt || op == metadata.OpGte || op == metadata.OpLt || op == metadata.OpLte
}

var (
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	intRe  = regexp.MustCompile(`^-?[0-9]{1,18}$`)
)

// condLiteral mengubah nilai pembanding menjadi literal SQL menurut tipe field; nilai yang tidak cocok tipe
// ditolak (evaluator menganggapnya tak tentu; SQL tidak punya "tak tentu").
func condLiteral(ft metadata.FieldType, v any) (string, error) {
	bad := func() (string, error) {
		return "", terr(ErrLiteral, "nilai %T tidak sah untuk tipe %s", v, ft)
	}
	switch ft {
	case metadata.TypeString, metadata.TypeText, metadata.TypeEnum:
		s, ok := v.(string)
		if !ok {
			return bad()
		}
		return Literal(s)
	case metadata.TypeUUID:
		s, ok := v.(string)
		if !ok || !uuidRe.MatchString(s) {
			return bad()
		}
		return Literal(s)
	case metadata.TypeDate:
		s, ok := v.(string)
		if !ok {
			return bad()
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return bad()
		}
		return Literal(s)
	case metadata.TypeDateTime:
		s, ok := v.(string)
		if !ok || !strings.HasSuffix(s, "Z") {
			return bad()
		}
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			return bad()
		}
		return Literal(s)
	case metadata.TypeBoolean:
		b, ok := v.(bool)
		if !ok {
			return bad()
		}
		return strconv.FormatBool(b), nil
	case metadata.TypeInteger:
		s, ok := numberText(v)
		if !ok || !intRe.MatchString(s) {
			return bad()
		}
		return s, nil
	case metadata.TypeDecimal:
		s, ok := numberText(v)
		if !ok {
			if str, isStr := v.(string); isStr {
				s, ok = str, true
			}
		}
		if !ok {
			return bad()
		}
		lit, err := NumericLiteral(s)
		if err != nil {
			return bad()
		}
		return lit, nil
	}
	return bad()
}

func numberText(v any) (string, bool) {
	switch x := v.(type) {
	case json.Number:
		return x.String(), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), true
		}
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	return "", false
}

// ---- ekspresi stored ----

type sqlExpr struct {
	sql string
	fam family
	ft  metadata.FieldType
}

// StoredExpr menerjemahkan ekspresi accessor stored menjadi ekspresi SQL untuk kolom GENERATED ... STORED.
// Hanya konstruk dari tata bahasa tertutup yang dipakai, dan hanya fungsi PostgreSQL bertanda immutable
// (round, trunc, abs, mod, COALESCE, NULLIF, CASE, operator aritmetika dan penggabungan teks, cast ke text dari
// tipe non-tanggal). concat atas date atau datetime ditolak karena cast ke text bergantung DateStyle (stable).
func StoredExpr(e *metadata.Expr, env Env) (string, error) {
	x, err := expr(e, env)
	if err != nil {
		return "", err
	}
	if len(x.sql) > MaxSQLBytes {
		return "", terr(ErrTooLarge, "ekspresi %d byte melebihi batas %d", len(x.sql), MaxSQLBytes)
	}
	return x.sql, nil
}

// StoredExprType mengembalikan tipe hasil ekspresi (untuk memeriksa kecocokan dengan kolom).
func StoredExprType(e *metadata.Expr, env Env) (metadata.FieldType, error) {
	x, err := expr(e, env)
	if err != nil {
		return "", err
	}
	return x.ft, nil
}

func expr(e *metadata.Expr, env Env) (sqlExpr, error) {
	switch e.Kind {
	case metadata.ExprLit:
		switch v := e.Lit.(type) {
		case nil:
			return sqlExpr{"NULL", famNull, metadata.TypeNull}, nil
		case bool:
			return sqlExpr{strconv.FormatBool(v), famBool, metadata.TypeBoolean}, nil
		case string:
			l, err := Literal(v)
			if err != nil {
				return sqlExpr{}, err
			}
			return sqlExpr{l + "::text", famText, metadata.TypeString}, nil
		case int64:
			return sqlExpr{wrapNeg(strconv.FormatInt(v, 10)), famInt, metadata.TypeInteger}, nil
		}
		return sqlExpr{}, terr(ErrInvalid, "literal %T tidak didukung", e.Lit)
	case metadata.ExprDec:
		l, err := NumericLiteral(e.Dec.String())
		if err != nil {
			return sqlExpr{}, err
		}
		return sqlExpr{wrapNeg(l), famNum, metadata.TypeDecimal}, nil
	case metadata.ExprField:
		info, err := lookup(env, e.Field)
		if err != nil {
			return sqlExpr{}, err
		}
		q, err := quoteCol(info)
		if err != nil {
			return sqlExpr{}, err
		}
		return sqlExpr{q, familyOf(info.Type), info.Type}, nil
	case metadata.ExprConcat:
		return concatExpr(e, env)
	case metadata.ExprCoalesce:
		return coalesceExpr(e, env)
	case metadata.ExprAdd, metadata.ExprSub, metadata.ExprMul, metadata.ExprDiv:
		return arithExpr(e, env)
	case metadata.ExprIf:
		return ifExpr(e, env)
	}
	return sqlExpr{}, terr(ErrInvalid, "simpul ekspresi tidak dikenal")
}

func wrapNeg(s string) string {
	if strings.HasPrefix(s, "-") {
		return "(" + s + ")"
	}
	return s
}

func isQuotedIdent(s string) bool {
	return len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' && !strings.Contains(s[1:len(s)-1], `"`)
}

func concatExpr(e *metadata.Expr, env Env) (sqlExpr, error) {
	parts := make([]string, 0, len(e.Args))
	for _, a := range e.Args {
		x, err := expr(a, env)
		if err != nil {
			return sqlExpr{}, err
		}
		switch {
		case a.Kind == metadata.ExprLit && x.fam == famText:
			parts = append(parts, x.sql)
		case x.fam == famText:
			parts = append(parts, "COALESCE("+x.sql+", '')")
		case x.fam == famNull:
			parts = append(parts, "''")
		case x.fam == famTime:
			return sqlExpr{}, terr(ErrNotImmutable, "concat atas date atau datetime tidak dapat dijadikan kolom turunan (cast ke text bukan immutable)")
		default:
			cast := "(" + x.sql + ")::text"
			if isQuotedIdent(x.sql) || a.Kind == metadata.ExprLit {
				cast = x.sql + "::text"
			}
			parts = append(parts, "COALESCE("+cast+", '')")
		}
	}
	return sqlExpr{"(" + strings.Join(parts, " || ") + ")", famText, metadata.TypeString}, nil
}

func coalesceExpr(e *metadata.Expr, env Env) (sqlExpr, error) {
	var parts []string
	fam := famNull
	ft := metadata.TypeNull
	for _, a := range e.Args {
		x, err := expr(a, env)
		if err != nil {
			return sqlExpr{}, err
		}
		if !compatible(fam, x.fam) {
			return sqlExpr{}, terr(ErrType, "argumen coalesce bertipe campur")
		}
		if fam == famNull {
			fam, ft = x.fam, x.ft
		} else if fam == famInt && x.fam == famNum {
			fam, ft = famNum, x.ft
		}
		parts = append(parts, x.sql)
	}
	return sqlExpr{"COALESCE(" + strings.Join(parts, ", ") + ")", fam, ft}, nil
}

func ifExpr(e *metadata.Expr, env Env) (sqlExpr, error) {
	cond, err := condition(e.Cond, env, CondOptions{})
	if err != nil {
		return sqlExpr{}, err
	}
	th, err := expr(e.Then, env)
	if err != nil {
		return sqlExpr{}, err
	}
	el, err := expr(e.Else, env)
	if err != nil {
		return sqlExpr{}, err
	}
	if !compatible(th.fam, el.fam) {
		return sqlExpr{}, terr(ErrType, "cabang then dan else bertipe berbeda")
	}
	fam, ft := th.fam, th.ft
	if fam == famNull {
		fam, ft = el.fam, el.ft
	} else if fam == famInt && el.fam == famNum {
		fam, ft = famNum, el.ft
	}
	return sqlExpr{"CASE WHEN " + cond + " THEN " + th.sql + " ELSE " + el.sql + " END", fam, ft}, nil
}

func arithExpr(e *metadata.Expr, env Env) (sqlExpr, error) {
	parts := make([]string, 0, len(e.Args))
	for _, a := range e.Args {
		x, err := expr(a, env)
		if err != nil {
			return sqlExpr{}, err
		}
		if !numeric(x.fam) && x.fam != famNull {
			return sqlExpr{}, terr(ErrType, "aritmetika membutuhkan integer atau decimal, bukan %s", x.ft)
		}
		parts = append(parts, x.sql)
	}
	if e.Kind == metadata.ExprDiv {
		r, err := divExact(parts[0], parts[1], e.Scale, e.Round)
		if err != nil {
			return sqlExpr{}, err
		}
		return sqlExpr{r, famNum, metadata.TypeDecimal}, nil
	}
	op := map[metadata.ExprKind]string{metadata.ExprAdd: " + ", metadata.ExprSub: " - ", metadata.ExprMul: " * "}[e.Kind]
	raw := "(" + strings.Join(parts, op) + ")"
	r, err := roundSQL(raw, e.Scale, e.Round)
	if err != nil {
		return sqlExpr{}, err
	}
	return sqlExpr{r, famNum, metadata.TypeDecimal}, nil
}

func zeros(n int) string { return strings.Repeat("0", n) }

// roundSQL membulatkan x ke scale digit pecahan menurut mode, dengan fungsi immutable PostgreSQL saja.
// round(numeric, int) membulatkan setengah menjauhi nol (sama dengan half_up evaluator); mode lain disusun
// dari trunc, perbandingan, dan CASE. Bila x NULL, hasilnya NULL pada semua mode.
func roundSQL(x string, scale int, mode metadata.RoundMode) (string, error) {
	if scale < 0 || scale > metadata.MaxScale || !mode.Valid() {
		return "", terr(ErrInvalid, "skala atau mode pembulatan tidak sah")
	}
	if len(x)*9 > MaxSQLBytes {
		return "", terr(ErrTooLarge, "ekspresi pembulatan bertingkat terlalu besar")
	}
	s := strconv.Itoa(scale)
	unit := "1"
	if scale > 0 {
		unit = "0." + zeros(scale-1) + "1"
	}
	half := "0." + zeros(scale) + "5"
	k := "1" + zeros(scale)
	tr := "trunc(" + x + ", " + s + ")"
	rd := "round(" + x + ", " + s + ")"
	switch mode {
	case metadata.RoundHalfUp:
		return rd, nil
	case metadata.RoundDown:
		return tr, nil
	case metadata.RoundFloor:
		return "CASE WHEN " + x + " >= 0 OR " + tr + " = " + x + " THEN " + tr + " ELSE " + tr + " - " + unit + " END", nil
	case metadata.RoundCeiling:
		return "CASE WHEN " + x + " <= 0 OR " + tr + " = " + x + " THEN " + tr + " ELSE " + tr + " + " + unit + " END", nil
	case metadata.RoundUp:
		return "CASE WHEN " + tr + " = " + x + " THEN " + tr + " WHEN " + x + " > 0 THEN " + tr + " + " + unit + " ELSE " + tr + " - " + unit + " END", nil
	case metadata.RoundHalfDown:
		return "CASE WHEN abs(" + x + " - " + tr + ") = " + half + " THEN " + tr + " ELSE " + rd + " END", nil
	}
	// half_even
	return "CASE WHEN abs(" + x + " - " + tr + ") = " + half + " THEN CASE WHEN mod(trunc(" + x + " * " + k + ", 0), 2) = 0 THEN " +
		tr + " WHEN " + x + " > 0 THEN " + tr + " + " + unit + " ELSE " + tr + " - " + unit + " END ELSE " + rd + " END", nil
}

// divExact membagi a dengan b secara eksak lalu membulatkan ke scale digit dengan mode yang sama dengan
// evaluator (sdk/metadata Decimal.Div: hasil dibulatkan sekali dari hasil bagi sejati). Operator "/" numeric
// PostgreSQL hanya menghitung ±16 digit signifikan sehingga membulatkan dua kali; di sini bagi eksak atas
// bilangan bulat: N = a * 10^scale, q = div(N, b) (dipotong ke nol), r = mod(N, b), lalu arah pembulatan
// dipilih dari sisa r dan pembagi (2*|r| dibanding |b|); hasil dikalikan 10^-scale (perkalian eksak, bukan
// pembagian). Pembagi nol menjadi NULL (evaluator: tak tentu).
func divExact(a, b string, scale int, mode metadata.RoundMode) (string, error) {
	if scale < 0 || scale > metadata.MaxScale || !mode.Valid() {
		return "", terr(ErrInvalid, "skala atau mode pembulatan tidak sah")
	}
	if len(a)*6+len(b)*8 > MaxSQLBytes {
		return "", terr(ErrTooLarge, "ekspresi pembagian terlalu besar")
	}
	k := "1" + zeros(scale)
	unit := "1"
	if scale > 0 {
		unit = "0." + zeros(scale-1) + "1"
	}
	n := "((" + a + ")::numeric * " + k + ")"
	d := "NULLIF((" + b + ")::numeric, 0)"
	q := "div(" + n + ", " + d + ")"
	r := "mod(" + n + ", " + d + ")"
	sg := "(sign(" + n + ") * sign(" + d + "))"
	twice := "2 * abs(" + r + ")"
	ad := "abs(" + d + ")"
	var cand string
	switch mode {
	case metadata.RoundDown:
		cand = q
	case metadata.RoundUp:
		cand = "CASE WHEN " + r + " = 0 THEN " + q + " ELSE " + q + " + " + sg + " END"
	case metadata.RoundFloor:
		cand = "CASE WHEN " + r + " = 0 OR " + sg + " > 0 THEN " + q + " ELSE " + q + " - 1 END"
	case metadata.RoundCeiling:
		cand = "CASE WHEN " + r + " = 0 OR " + sg + " < 0 THEN " + q + " ELSE " + q + " + 1 END"
	case metadata.RoundHalfUp:
		cand = "CASE WHEN " + twice + " >= " + ad + " THEN " + q + " + " + sg + " ELSE " + q + " END"
	case metadata.RoundHalfDown:
		cand = "CASE WHEN " + twice + " > " + ad + " THEN " + q + " + " + sg + " ELSE " + q + " END"
	default: // half_even
		cand = "CASE WHEN " + twice + " > " + ad + " OR (" + twice + " = " + ad + " AND mod(" + q + ", 2) <> 0) THEN " + q + " + " + sg + " ELSE " + q + " END"
	}
	out := "((" + cand + ") * " + unit + ")"
	if len(out) > MaxSQLBytes {
		return "", terr(ErrTooLarge, "ekspresi pembagian terlalu besar")
	}
	return out, nil
}
