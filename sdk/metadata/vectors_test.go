package metadata_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"erpora/sdk/metadata"
)

// Korpus vektor bersama (Requirement 4.4): berkas yang sama dijalankan evaluator TypeScript
// (packages/platform-web/src/metadata). Format dijelaskan di field "description" tiap berkas.
const vectorsDir = "../../contracts/metadata-vectors"

func loadVectors(t *testing.T, rel string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vectorsDir, rel))
	if err != nil {
		t.Fatalf("baca korpus %s: %v", rel, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var doc struct {
		Cases []map[string]any `json:"cases"`
	}
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("parse korpus %s: %v", rel, err)
	}
	if len(doc.Cases) == 0 {
		t.Fatalf("korpus %s kosong", rel)
	}
	return doc.Cases
}

func typesOf(t *testing.T, c map[string]any) metadata.Types {
	t.Helper()
	raw, _ := c["types"].(map[string]any)
	out := metadata.Types{}
	for k, v := range raw {
		s, _ := v.(string)
		out[k] = metadata.FieldType(s)
	}
	return out
}

func recordOf(c map[string]any) metadata.Record {
	raw, _ := c["record"].(map[string]any)
	return metadata.Record(raw)
}

func reasonStrings(rs []metadata.Reason) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.String())
	}
	return out
}

func wantReasons(t *testing.T, e map[string]any) []string {
	t.Helper()
	list, _ := e["reasons"].([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		out = append(out, x.(string))
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func runConditionCases(t *testing.T, cases []map[string]any) {
	t.Helper()
	for _, c := range cases {
		name, _ := c["name"].(string)
		t.Run(truncate(name), func(t *testing.T) {
			exp := c["expect"].(map[string]any)
			got := metadata.Evaluate(c["condition"], typesOf(t, c), recordOf(c))
			if got.Value != exp["result"].(bool) {
				t.Errorf("result = %v, want %v (reasons %v)", got.Value, exp["result"], reasonStrings(got.Reasons))
			}
			if want := wantReasons(t, exp); !sameStrings(reasonStrings(got.Reasons), want) {
				t.Errorf("reasons = %v, want %v", reasonStrings(got.Reasons), want)
			}
		})
	}
}

func TestVectorsConditions(t *testing.T) {
	runConditionCases(t, loadVectors(t, "conditions.json"))
}

// sameValue membandingkan nilai ekspektasi JSON dengan hasil evaluator.
func sameValue(want, got any) bool {
	switch w := want.(type) {
	case nil:
		return got == nil
	case bool:
		g, ok := got.(bool)
		return ok && g == w
	case string:
		g, ok := got.(string)
		return ok && g == w
	case json.Number:
		g, ok := got.(int64)
		return ok && strconv.FormatInt(g, 10) == w.String()
	}
	return false
}

func runExprCases(t *testing.T, cases []map[string]any) {
	t.Helper()
	for _, c := range cases {
		name, _ := c["name"].(string)
		t.Run(truncate(name), func(t *testing.T) {
			exp := c["expect"].(map[string]any)
			got := metadata.EvaluateExpr(c["expr"], typesOf(t, c), recordOf(c))
			if string(got.Type) != exp["type"].(string) {
				t.Errorf("type = %q, want %q (reasons %v)", got.Type, exp["type"], reasonStrings(got.Reasons))
			}
			if !sameValue(exp["value"], got.Value) {
				t.Errorf("value = %#v, want %#v", got.Value, exp["value"])
			}
			if want := wantReasons(t, exp); !sameStrings(reasonStrings(got.Reasons), want) {
				t.Errorf("reasons = %v, want %v", reasonStrings(got.Reasons), want)
			}
		})
	}
}

func TestVectorsExpressions(t *testing.T) {
	runExprCases(t, loadVectors(t, "expressions.json"))
}

func runLabelCases(t *testing.T, cases []map[string]any) {
	t.Helper()
	for _, c := range cases {
		name, _ := c["name"].(string)
		t.Run(truncate(name), func(t *testing.T) {
			exp := c["expect"].(map[string]any)
			tpl, err := metadata.ParseLabelTemplate(c["template"].(string))
			if code, bad := exp["error"].(string); bad {
				if err == nil {
					t.Fatalf("templat seharusnya ditolak (%s)", code)
				}
				var me *metadata.Error
				if !errors.As(err, &me) || me.Reason.Code != code {
					t.Fatalf("galat = %v, want code %s", err, code)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLabelTemplate: %v", err)
			}
			got := tpl.Render(typesOf(t, c), recordOf(c))
			if got.Text != exp["text"].(string) {
				t.Errorf("text = %q, want %q", truncate(got.Text), truncate(exp["text"].(string)))
			}
			if want := wantReasons(t, exp); !sameStrings(reasonStrings(got.Reasons), want) {
				t.Errorf("reasons = %v, want %v", reasonStrings(got.Reasons), want)
			}
		})
	}
}

func TestVectorsLabels(t *testing.T) {
	runLabelCases(t, loadVectors(t, "labels.json"))
}

func mustDecimal(t *testing.T, s string) metadata.Decimal {
	t.Helper()
	d, ok := metadata.ParseDecimal(s)
	if !ok {
		t.Fatalf("ParseDecimal(%q) ditolak", s)
	}
	return d
}

func TestVectorsDecimals(t *testing.T) {
	for _, c := range loadVectors(t, "decimals.json") {
		name, _ := c["name"].(string)
		t.Run(truncate(name), func(t *testing.T) {
			op := c["op"].(string)
			str := func(k string) string { s, _ := c[k].(string); return s }
			num := func(k string) int { n, _ := c[k].(json.Number); v, _ := n.Int64(); return int(v) }
			wantStr := func() (string, bool) { s, ok := c["out"].(string); return s, ok }
			switch op {
			case "parse":
				d, ok := metadata.ParseDecimal(str("in"))
				want, wantOK := wantStr()
				if ok != wantOK {
					t.Fatalf("parse ok = %v, want %v", ok, wantOK)
				}
				if ok && d.String() != want {
					t.Errorf("String = %q, want %q", d.String(), want)
				}
			case "cmp":
				got := mustDecimal(t, str("a")).Cmp(mustDecimal(t, str("b")))
				n := c["out"].(json.Number)
				if fmt.Sprint(got) != n.String() {
					t.Errorf("cmp = %d, want %s", got, n)
				}
			case "add", "sub", "mul":
				a, b := mustDecimal(t, str("a")), mustDecimal(t, str("b"))
				var got metadata.Decimal
				switch op {
				case "add":
					got = a.Add(b)
				case "sub":
					got = a.Sub(b)
				default:
					got = a.Mul(b)
				}
				if want, _ := wantStr(); got.String() != want {
					t.Errorf("%s = %q, want %q", op, got.String(), want)
				}
			case "round":
				got := mustDecimal(t, str("a")).Round(num("scale"), metadata.RoundMode(str("mode")))
				if want, _ := wantStr(); got.String() != want {
					t.Errorf("round = %q, want %q", got.String(), want)
				}
			case "div":
				got, ok := mustDecimal(t, str("a")).Div(mustDecimal(t, str("b")), num("scale"), metadata.RoundMode(str("mode")))
				want, wantOK := wantStr()
				if ok != wantOK {
					t.Fatalf("div ok = %v, want %v", ok, wantOK)
				}
				if ok && got.String() != want {
					t.Errorf("div = %q, want %q", got.String(), want)
				}
			default:
				t.Fatalf("op tak dikenal: %s", op)
			}
		})
	}
}

func TestVectorsPatterns(t *testing.T) {
	for _, c := range loadVectors(t, "patterns.json") {
		name, _ := c["name"].(string)
		if scope, _ := c["scope"].(string); scope == "client" {
			continue
		}
		t.Run(truncate(name), func(t *testing.T) {
			pattern := c["pattern"].(string)
			err := metadata.ValidatePattern(pattern)
			if valid := c["valid"].(bool); !valid {
				if err == nil {
					t.Fatalf("pola %q seharusnya ditolak (%s)", pattern, c["error"])
				}
				var pe *metadata.PatternError
				if !errors.As(err, &pe) || pe.Code != c["error"].(string) {
					t.Fatalf("galat = %v, want code %s", err, c["error"])
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePattern(%q): %v", pattern, err)
			}
			for _, in := range c["inputs"].([]any) {
				m := in.(map[string]any)
				got, err := metadata.MatchPattern(pattern, m["value"].(string))
				if err != nil {
					t.Fatalf("MatchPattern: %v", err)
				}
				if got != m["match"].(bool) {
					t.Errorf("MatchPattern(%q, %q) = %v, want %v", pattern, truncate(m["value"].(string)), got, m["match"])
				}
			}
		})
	}
}
