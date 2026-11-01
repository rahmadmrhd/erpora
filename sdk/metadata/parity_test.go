package metadata

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Paritas dua sisi (Requirement 4.4, 22.2): Go membangkitkan korpus acak berseed dan menghitung ekspektasi
// dengan evaluatornya; berkas hasilnya DIKOMIT di contracts/metadata-vectors/random/ dan dibaca evaluator
// TypeScript (parity.test.ts), yang harus menghasilkan hasil identik. Tes ini membangkitkan ulang berkas di
// memori dan gagal bila berbeda dari yang dikomit (drift); perbarui dengan `go test ./metadata -update` atau
// UPDATE_VECTORS=1. Ekspektasi dihitung dari data yang sudah melewati JSON (persis yang dilihat TypeScript).
var updateVectors = flag.Bool("update", false, "tulis ulang korpus acak contracts/metadata-vectors/random")

const (
	randomVectorsDir = "../../contracts/metadata-vectors/random"
	parityMasterSeed = 20261101
)

var typedFields = []string{"a", "b", "s", "d", "t", "e", "u", "f", "x"}

func fieldList(r *rand.Rand) string {
	if r.Intn(25) == 0 {
		return "zz"
	}
	return typedFields[r.Intn(len(typedFields))]
}

var alphabet = []string{"a", "b", "A", "B", "c", " ", "-", "é", "É", "\U0001F600", "￿", "z", "1"}

func randText(r *rand.Rand, max int) string {
	var sb strings.Builder
	for i, n := 0, r.Intn(max+1); i < n; i++ {
		sb.WriteString(alphabet[r.Intn(len(alphabet))])
	}
	return sb.String()
}

func randDateString(r *rand.Rand) string {
	return fmt.Sprintf("%04d-%02d-%02d", 1+r.Intn(9999), 1+r.Intn(12), 1+r.Intn(31)) // sebagian tidak sah (sengaja)
}

func randTyped(r *rand.Rand, ft FieldType) any {
	if r.Intn(8) == 0 {
		return randScalar(r) // sengaja tidak cocok
	}
	switch ft {
	case TypeInteger:
		return json.Number(fmt.Sprint(r.Intn(21) - 10))
	case TypeDecimal:
		return []string{"0", "1", "-1", "1.5", "1.50", "2.505", "-2.505", "10.00", "0.10", "0.1", randDecString(r)}[r.Intn(11)]
	case TypeString, TypeText:
		return randText(r, 4)
	case TypeDate:
		return []string{"2026-01-01", "2026-02-28", "2024-02-29", "2026-03-01", randDateString(r)}[r.Intn(5)]
	case TypeDateTime:
		return []string{"2026-01-01T10:00:00Z", "2026-01-01T10:00:00.5Z", "2026-01-01T10:00:00.500Z", "2026-01-02T00:00:00Z", "2026-01-01T09:59:59.999999999Z"}[r.Intn(5)]
	case TypeEnum:
		return []string{"draft", "submitted", "cancelled"}[r.Intn(3)]
	case TypeUUID:
		return []string{"123e4567-e89b-12d3-a456-426614174000", "223e4567-e89b-12d3-a456-426614174000"}[r.Intn(2)]
	case TypeBoolean:
		return r.Intn(2) == 0
	}
	return nil
}

func genRecord(r *rand.Rand) Record {
	rec := Record{}
	for _, f := range typedFields {
		switch r.Intn(12) {
		case 0: // tidak tersedia
		case 1:
			rec[f] = nil
		default:
			rec[f] = randTyped(r, propTypes[f])
		}
	}
	return rec
}

func genLeafCond(r *rand.Rand) map[string]any {
	f := fieldList(r)
	ft := propTypes[f]
	m := map[string]any{"field": f}
	op := []string{"eq", "eq", "ne", "gt", "gte", "lt", "lte", "in", "contains", "isnull"}[r.Intn(10)]
	m["op"] = op
	switch op {
	case "isnull":
		m["value"] = r.Intn(2) == 0
	case "in":
		list := make([]any, 1+r.Intn(4))
		for i := range list {
			list[i] = randTyped(r, ft)
			if list[i] == nil {
				list[i] = "x"
			}
		}
		m["value"] = list
	default:
		switch {
		case r.Intn(4) == 0:
			m["ref"] = fieldList(r)
		default:
			v := randTyped(r, ft)
			if v == nil && op != "eq" && op != "ne" {
				v = "x"
			}
			m["value"] = v
		}
	}
	return m
}

func genCond(r *rand.Rand, depth int) any {
	if depth <= 0 || r.Intn(3) == 0 {
		return genLeafCond(r)
	}
	n := 1 + r.Intn(3)
	kids := make([]any, n)
	for i := range kids {
		kids[i] = genCond(r, depth-1)
	}
	switch r.Intn(10) {
	case 0, 1, 2, 3:
		return map[string]any{"all": kids}
	case 4, 5, 6:
		return map[string]any{"any": kids}
	case 7, 8:
		return map[string]any{"not": kids[0]}
	}
	return map[string]any{"all": randScalar(r), "extra": kids} // simpul rusak
}

func genExpr(r *rand.Rand, depth int) any {
	leaf := func() any {
		switch r.Intn(7) {
		case 0:
			return map[string]any{"lit": nil}
		case 1:
			return map[string]any{"lit": randText(r, 3)}
		case 2:
			return map[string]any{"lit": json.Number(fmt.Sprint(r.Intn(21) - 10))}
		case 3:
			return map[string]any{"dec": randDecString(r)}
		case 4:
			return map[string]any{"lit": r.Intn(2) == 0}
		}
		return map[string]any{"field": fieldList(r)}
	}
	if depth <= 0 {
		return leaf()
	}
	args := func(n int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = genExpr(r, depth-1)
		}
		return out
	}
	modes := []string{"half_up", "half_down", "half_even", "up", "down", "floor", "ceiling"}
	switch r.Intn(8) {
	case 0:
		return map[string]any{"concat": args(1 + r.Intn(3))}
	case 1:
		return map[string]any{"coalesce": args(1 + r.Intn(3))}
	case 2, 3, 4:
		op := []string{"add", "sub", "mul", "div"}[r.Intn(4)]
		n := 2
		if op == "add" || op == "mul" {
			n = 2 + r.Intn(2)
		}
		return map[string]any{op: args(n), "scale": json.Number(fmt.Sprint(r.Intn(7))), "round": modes[r.Intn(len(modes))]}
	case 5:
		return map[string]any{"if": genCond(r, 2), "then": genExpr(r, depth-1), "else": genExpr(r, depth-1)}
	}
	return leaf()
}

func genTemplate(r *rand.Rand) string {
	var sb strings.Builder
	for i, n := 0, r.Intn(5); i < n; i++ {
		if r.Intn(2) == 0 {
			sb.WriteString([]string{"A-", " · ", ", ", "[", "]", "\U0001F600", "<b>"}[r.Intn(7)])
		} else {
			sb.WriteString("{" + fieldList(r) + "}")
		}
	}
	if r.Intn(40) == 0 {
		sb.WriteString("{")
	}
	return sb.String()
}

// roundTrip meneruskan v lewat JSON (UseNumber) agar yang dievaluasi persis yang dibaca TypeScript.
func roundTrip(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func reasonList(rs []Reason) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.String())
	}
	return out
}

func typesMap() map[string]string {
	m := map[string]string{}
	for k, v := range propTypes {
		m[k] = string(v)
	}
	return m
}

func asTypes(m map[string]string) Types {
	out := Types{}
	for k, v := range m {
		out[k] = FieldType(v)
	}
	return out
}

func asRecord(v any) Record {
	m, _ := v.(map[string]any)
	return Record(m)
}

type randomFile struct {
	name        string
	description string
	cases       []any
}

func buildRandomVectors(t *testing.T) []randomFile {
	t.Helper()
	tm := typesMap()
	var conds, exprs, labels, decs []any

	r := rand.New(rand.NewSource(parityMasterSeed))
	for i := 0; i < 500; i++ {
		cond := roundTrip(t, genCond(r, 1+r.Intn(4)))
		rec := roundTrip(t, genRecord(r))
		res := Evaluate(cond, asTypes(tm), asRecord(rec))
		conds = append(conds, map[string]any{
			"name": fmt.Sprintf("random condition %d", i), "types": tm, "record": rec, "condition": cond,
			"expect": map[string]any{"result": res.Value, "reasons": reasonList(res.Reasons)},
		})
	}
	for i := 0; i < 400; i++ {
		expr := roundTrip(t, genExpr(r, 1+r.Intn(3)))
		rec := roundTrip(t, genRecord(r))
		res := EvaluateExpr(expr, asTypes(tm), asRecord(rec))
		exprs = append(exprs, map[string]any{
			"name": fmt.Sprintf("random expression %d", i), "types": tm, "record": rec, "expr": expr,
			"expect": map[string]any{"type": string(res.Type), "value": res.Value, "reasons": reasonList(res.Reasons)},
		})
	}
	for i := 0; i < 80; i++ {
		tpl := genTemplate(r)
		rec := roundTrip(t, genRecord(r))
		c := map[string]any{"name": fmt.Sprintf("random label %d", i), "types": tm, "record": rec, "template": tpl}
		p, err := ParseLabelTemplate(tpl)
		if err != nil {
			c["expect"] = map[string]any{"error": err.(*Error).Reason.Code}
		} else {
			res := p.Render(asTypes(tm), asRecord(rec))
			c["expect"] = map[string]any{"text": res.Text, "reasons": reasonList(res.Reasons)}
		}
		labels = append(labels, c)
	}
	modes := []RoundMode{RoundHalfUp, RoundHalfDown, RoundHalfEven, RoundUp, RoundDown, RoundFloor, RoundCeiling}
	for i := 0; i < 400; i++ {
		as, bs := randDecString(r), randDecString(r)
		a, _ := ParseDecimal(as)
		b, _ := ParseDecimal(bs)
		scale, mode := r.Intn(8), modes[r.Intn(len(modes))]
		var c map[string]any
		switch r.Intn(6) {
		case 0:
			c = map[string]any{"op": "cmp", "a": as, "b": bs, "out": json.Number(fmt.Sprint(a.Cmp(b)))}
		case 1:
			c = map[string]any{"op": "add", "a": as, "b": bs, "out": a.Add(b).String()}
		case 2:
			c = map[string]any{"op": "sub", "a": as, "b": bs, "out": a.Sub(b).String()}
		case 3:
			c = map[string]any{"op": "mul", "a": as, "b": bs, "out": a.Mul(b).String()}
		case 4:
			c = map[string]any{"op": "round", "a": as, "scale": scale, "mode": string(mode), "out": a.Round(scale, mode).String()}
		default:
			var out any
			if q, ok := a.Div(b, scale, mode); ok {
				out = q.String()
			}
			c = map[string]any{"op": "div", "a": as, "b": bs, "scale": scale, "mode": string(mode), "out": out}
		}
		c["name"] = fmt.Sprintf("random decimal %d %s", i, c["op"])
		decs = append(decs, c)
	}
	return []randomFile{
		{"conditions.json", "Korpus acak berseed (Go membangkitkan dan menghitung ekspektasi; TypeScript harus identik). Format sama dengan ../conditions.json.", conds},
		{"expressions.json", "Korpus acak berseed. Format sama dengan ../expressions.json.", exprs},
		{"labels.json", "Korpus acak berseed. Format sama dengan ../labels.json.", labels},
		{"decimals.json", "Korpus acak berseed. Format sama dengan ../decimals.json.", decs},
	}
}

func renderRandomFile(t *testing.T, f randomFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	desc, _ := json.Marshal(f.description)
	fmt.Fprintf(&buf, "{\n  \"vectorVersion\": 1,\n  \"seed\": %d,\n  \"description\": %s,\n  \"cases\": [\n", parityMasterSeed, desc)
	for i, c := range f.cases {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		buf.WriteString("    ")
		buf.Write(b)
		if i < len(f.cases)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("  ]\n}\n")
	return buf.Bytes()
}

func TestRandomVectorsAreCurrent(t *testing.T) {
	update := *updateVectors || os.Getenv("UPDATE_VECTORS") == "1"
	if update {
		if err := os.MkdirAll(randomVectorsDir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range buildRandomVectors(t) {
		want := renderRandomFile(t, f)
		path := filepath.Join(randomVectorsDir, f.name)
		if update {
			if err := os.WriteFile(path, want, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("baca %s: %v (jalankan: go test ./metadata -update)", path, err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s berbeda dari hasil pembangkitan; jalankan `go test ./metadata -update` (atau UPDATE_VECTORS=1) dan komit", path)
		}
	}
}

// Sebaran hasil harus beragam agar paritas bermakna (bukan semua false).
func TestRandomVectorsAreDiverse(t *testing.T) {
	files := buildRandomVectors(t)
	count := func(f randomFile, pred func(map[string]any) bool) int {
		n := 0
		for _, c := range f.cases {
			if pred(c.(map[string]any)["expect"].(map[string]any)) {
				n++
			}
		}
		return n
	}
	conds := files[0]
	if tr := count(conds, func(e map[string]any) bool { return e["result"] == true }); tr < 40 {
		t.Errorf("hanya %d kondisi bernilai true", tr)
	}
	if def := count(conds, func(e map[string]any) bool { return len(e["reasons"].([]string)) == 0 && e["result"] == false }); def < 40 {
		t.Errorf("hanya %d kondisi false pasti", def)
	}
	if unk := count(conds, func(e map[string]any) bool { return len(e["reasons"].([]string)) > 0 }); unk < 40 {
		t.Errorf("hanya %d kondisi tak tentu", unk)
	}
	if ok := count(files[1], func(e map[string]any) bool { return e["type"] == "decimal" }); ok < 15 {
		t.Errorf("hanya %d ekspresi desimal", ok)
	}
	if ok := count(files[1], func(e map[string]any) bool { return e["type"] == "string" }); ok < 15 {
		t.Errorf("hanya %d ekspresi teks", ok)
	}
}
