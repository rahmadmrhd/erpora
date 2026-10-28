package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustCompile(t *testing.T, src string) Message {
	t.Helper()
	m, err := Compile(src, DefaultLimits())
	if err != nil {
		t.Fatalf("Compile(%q): %v", src, err)
	}
	return m
}

func compileCode(t *testing.T, src string, lim Limits) string {
	t.Helper()
	_, err := Compile(src, lim)
	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("Compile(%q) err = %v, want *CompileError", src, err)
	}
	return ce.Code
}

func TestCompileErrorCodes(t *testing.T) {
	lim := DefaultLimits()
	cases := []struct {
		src  string
		code string
	}{
		{"Halo {nama", CodeInvalidICU},
		{"{n, plural, one {x}}", CodeInvalidICU},
		{"{n, foo, bar}", CodeInvalidICU},
		{"{n, plural, foo {x} other {y}}", CodeInvalidICU},
		{"\xff\xfe", CodeInvalidICU},
		{"{n, selectordinal, other {x}}", CodeUnsupportedConstruct},
		{"{n, number}", CodeUnsupportedConstruct},
		{"{n, date, short}", CodeUnsupportedConstruct},
		{"{n, time}", CodeUnsupportedConstruct},
		{"{n, plural, offset:1 other {x}}", CodeUnsupportedConstruct},
		{strings.Repeat("a", lim.MaxMessageLen+1), CodeTooLarge},
	}
	for _, c := range cases {
		if got := compileCode(t, c.src, lim); got != c.code {
			t.Errorf("Compile(%.30q) code = %s, want %s", c.src, got, c.code)
		}
	}
}

func TestCompileLimitsBoundaries(t *testing.T) {
	lim := Limits{MaxMessageLen: 200, MaxDepth: 2, MaxOptions: 2, MaxArgs: 2}
	ok := []string{
		"{a, select, other {{b, select, other {x}}}}",
		"<b><i>x</i></b>",
		"{a, select, x {1} other {2}}",
		"{a}{a}{a}{b}",
	}
	for _, s := range ok {
		if _, err := Compile(s, lim); err != nil {
			t.Errorf("Compile(%q) harus lolos: %v", s, err)
		}
	}
	bad := []string{
		"{a, select, other {{b, select, other {{c, select, other {x}}}}}}",
		"<b><i><u>x</u></i></b>",
		"{a, select, x {1} y {2} other {3}}",
		"{a}{b}{c}",
		"<b>{a, select, other {<i>x</i>}}</b>",
	}
	for _, s := range bad {
		if got := compileCode(t, s, lim); got != CodeTooLarge {
			t.Errorf("Compile(%q) code = %s, want too_large", s, got)
		}
	}
}

func TestCompileCountsLengthInRunes(t *testing.T) {
	lim := Limits{MaxMessageLen: 5}
	if _, err := Compile("éééé😀", lim); err != nil {
		t.Errorf("5 rune harus lolos: %v", err)
	}
	if _, err := Compile("éééé😀x", lim); err == nil {
		t.Error("6 rune harus ditolak")
	}
}

func TestParamsOrderKindsAndOptions(t *testing.T) {
	m := mustCompile(t, "<b>{name}</b> {n, plural, one {# a} other {# b}} {g, select, f {x} m {y} other {z}} {name} {n}")
	want := []Param{
		{Name: "b", Kind: KindTag},
		{Name: "name", Kind: KindArg},
		{Name: "n", Kind: KindPlural},
		{Name: "g", Kind: KindSelect, Options: []string{"f", "m", "other"}},
	}
	if got := m.Params(); !reflect.DeepEqual(got, want) {
		t.Errorf("Params() = %+v, want %+v", got, want)
	}
}

func TestParamsNestedOrderFollowsFirstAppearance(t *testing.T) {
	m := mustCompile(t, "{a, select, x {{b} {c, plural, other {#}}} other {}}")
	var names []string
	for _, p := range m.Params() {
		names = append(names, p.Name)
	}
	if !reflect.DeepEqual(names, []string{"a", "b", "c"}) {
		t.Errorf("urutan = %v", names)
	}
}

func TestParamsKindUpgradeAndTagNamespace(t *testing.T) {
	m := mustCompile(t, "{n} {n, plural, other {x}} <n>t</n>")
	got := m.Params()
	if len(got) != 2 || got[0].Kind != KindPlural || got[1].Kind != KindTag || got[1].Name != "n" {
		t.Errorf("Params() = %+v", got)
	}
}

func TestParamsReturnsCopy(t *testing.T) {
	m := mustCompile(t, "{g, select, a {x} other {y}}")
	p := m.Params()
	p[0].Name = "diubah"
	p[0].Options[0] = "diubah"
	q := m.Params()
	if q[0].Name != "g" || q[0].Options[0] != "a" {
		t.Errorf("Params harus mengembalikan salinan: %+v", q)
	}
}

func TestSameParams(t *testing.T) {
	p := func(s string) []Param { return mustCompile(t, s).Params() }
	if !SameParams(p("{a} {b}"), p("{b} {a}")) {
		t.Error("urutan tidak boleh berpengaruh")
	}
	if !SameParams(p("{n} item"), p("{n, plural, one {# item} other {# items}}")) {
		t.Error("arg dan plural harus kompatibel")
	}
	if SameParams(p("{a}"), p("{a} {b}")) {
		t.Error("jumlah berbeda")
	}
	if SameParams(p("{a}"), p("{b}")) {
		t.Error("nama berbeda")
	}
	if SameParams(p("{g}"), p("{g, select, a {x} other {y}}")) {
		t.Error("arg vs select tidak kompatibel")
	}
	if SameParams(p("{b}"), p("<b>x</b>")) {
		t.Error("arg vs tag tidak kompatibel")
	}
	if !SameParams(p("{g, select, a {x} other {y}}"), p("{g, select, b {x} other {y}}")) {
		t.Error("opsi select tidak dibandingkan")
	}
}

func TestValidateMessageHTMLRules(t *testing.T) {
	ok := []string{"Klik <b>di sini</b>", "<a>x</a>", "<h1>x</h1>", "a '<' b", "a > b", "tanpa markup"}
	for _, s := range ok {
		if _, err := ValidateMessage(s, DefaultLimits()); err != nil {
			t.Errorf("ValidateMessage(%q) harus lolos: %v", s, err)
		}
	}
	bad := []string{
		"<script>x</script>", "<IMG>x</IMG>", "a < b", "a<br/>b", "<b >x</b>", "<b>x</b >", "<my-tag>x</my-tag>",
		"<iframe>x</iframe>", "<form>x</form>",
	}
	for _, s := range bad {
		_, err := ValidateMessage(s, DefaultLimits())
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Code != CodeHTMLNotAllowed {
			t.Errorf("ValidateMessage(%q) err = %v, want html_not_allowed", s, err)
		}
	}
	// Compile sendiri menerima markup (paritas FormatJS)
	if _, err := Compile("<script>x</script> a < b", DefaultLimits()); err != nil {
		t.Errorf("Compile harus menerima: %v", err)
	}
	// galat sintaks tetap mendahului aturan HTML
	_, err := ValidateMessage("<a href=\"x\">y</a>", DefaultLimits())
	var ce *CompileError
	if !errors.As(err, &ce) || ce.Code != CodeInvalidICU {
		t.Errorf("err = %v, want invalid_icu", err)
	}
}

func TestCompileErrorIssueDoesNotQuoteWholeMessage(t *testing.T) {
	_, err := Compile(strings.Repeat("{", 400), DefaultLimits())
	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatal("harus galat")
	}
	if is := ce.Issue(); is.Code != CodeInvalidICU || len(is.Detail) > 200 {
		t.Errorf("Issue = %+v", is)
	}
}

type stringer struct{}

func (stringer) String() string { return "stringer" }

func TestFormatArgTypes(t *testing.T) {
	cases := []struct {
		name string
		arg  any
		want string
	}{
		{"int", 42, "42"},
		{"int8", int8(-3), "-3"},
		{"uint64", uint64(1) << 63, "9223372036854775808"},
		{"float32", float32(0.1), "0.1"},
		{"float64", 3.5, "3.5"},
		{"json.Number", json.Number("007"), "007"},
		{"bool", true, "true"},
		{"Stringer", stringer{}, "stringer"},
		{"string", "x", "x"},
	}
	for _, c := range cases {
		m := mustCompile(t, "[{v}]")
		got, err := m.Format("en", Args{"v": c.arg})
		if err != nil || got != "["+c.want+"]" {
			t.Errorf("%s: got %q, %v; want [%s]", c.name, got, err, c.want)
		}
	}
	m := mustCompile(t, "{v}")
	if _, err := m.Format("en", Args{"v": []int{1}}); !errors.Is(err, ErrUnsupportedArg) {
		t.Errorf("err = %v, want ErrUnsupportedArg", err)
	}
	if _, err := m.Format("en", Args{"v": map[string]int{}}); !errors.Is(err, ErrUnsupportedArg) {
		t.Errorf("err = %v, want ErrUnsupportedArg", err)
	}
}

func TestFormatPluralWithNumericTypes(t *testing.T) {
	m := mustCompile(t, "{n, plural, one {satu} other {lain #}}")
	for _, v := range []any{1, int64(1), uint8(1), float32(1), 1.0, "1", json.Number("1"), true} {
		if got, _ := m.Format("en", Args{"n": v}); got != "satu" {
			t.Errorf("n=%#v: got %q", v, got)
		}
	}
	if got, _ := m.Format("en", Args{"n": 1234}); got != "lain 1,234" {
		t.Errorf("got %q", got)
	}
	if got, _ := m.Format("id", Args{"n": 1234}); got != "lain 1.234" {
		t.Errorf("got %q", got)
	}
}

func TestFormatNilAndMissingAreMissing(t *testing.T) {
	m := mustCompile(t, "{a}|{n, plural, other {x}}|{g, select, other {y}}")
	got, err := m.Format("id", Args{"a": nil})
	if err != nil || got != "{a}|{n}|{g}" {
		t.Errorf("got %q, %v", got, err)
	}
	got, err = m.Format("id", nil)
	if err != nil || got != "{a}|{n}|{g}" {
		t.Errorf("args nil: got %q, %v", got, err)
	}
}

func TestFormatUnknownLocaleFallsBackToOther(t *testing.T) {
	m := mustCompile(t, "{n, plural, one {satu} other {lain}}")
	for _, loc := range []Locale{"", "zz", "not a locale", "x-klingon"} {
		got, err := m.Format(loc, Args{"n": 1})
		if err != nil || got != "lain" && got != "satu" {
			t.Errorf("loc %q: got %q, %v", loc, got, err)
		}
	}
}

func TestFormatOutputLimit(t *testing.T) {
	m := mustCompile(t, strings.Repeat("{a}", 600))
	big := strings.Repeat("x", 4000) // 600 * 4000 > MaxFormatOutput
	if _, err := m.Format("id", Args{"a": big}); !errors.Is(err, ErrOutputTooLarge) {
		t.Errorf("err = %v, want ErrOutputTooLarge", err)
	}
	if got, err := m.Format("id", Args{"a": "x"}); err != nil || len(got) != 600 {
		t.Errorf("got len %d, %v", len(got), err)
	}
}

func TestFormatPanicBecomesError(t *testing.T) {
	var nilStringer *panicStringer
	m := mustCompile(t, "{v}")
	if _, err := m.Format("id", Args{"v": nilStringer}); err == nil {
		t.Error("Stringer yang panik harus menjadi galat, bukan panic")
	}
}

type panicStringer struct{ s *string }

func (p *panicStringer) String() string { return *p.s }

func TestFormatConcurrent(t *testing.T) {
	m := mustCompile(t, "{n, plural, one {# item} other {# items}} <b>{g, select, a {A} other {O}}</b>")
	done := make(chan string, 16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			s, _ := m.Format("id", Args{"n": 1000 + i, "g": "a"})
			done <- s
		}(i)
	}
	for i := 0; i < 16; i++ {
		if s := <-done; !strings.HasSuffix(s, " items A") {
			t.Errorf("hasil tak terduga: %q", s)
		}
	}
}

func TestJSNumberString(t *testing.T) {
	cases := map[float64]string{
		0: "0", 1: "1", -0.5: "-0.5", 1e21: "1e+21", 1e-7: "1e-7", 0.000001: "0.000001", 123456789012: "123456789012",
		1.5e300: "1.5e+300", 2.5e-10: "2.5e-10", 100: "100", 1e20: "100000000000000000000",
	}
	for in, want := range cases {
		if got := jsNumberString(in); got != want {
			t.Errorf("jsNumberString(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestDecimalRoundingHalfExpand(t *testing.T) {
	cases := []struct {
		in     float64
		ip, fp string
	}{
		{1.0004, "1", ""},
		{1.0005, "1", "001"},
		{0.9995, "1", ""},
		{999.9995, "1000", ""},
		{1234.5678, "1234", "568"},
		{2.5e-7, "0", ""},
		{12.3, "12", "3"},
	}
	for _, c := range cases {
		d := newDecimal(c.in)
		if d.ip != c.ip || d.fp != c.fp {
			t.Errorf("newDecimal(%v) = %s.%s, want %s.%s", c.in, d.ip, d.fp, c.ip, c.fp)
		}
	}
}

func TestJSToNumber(t *testing.T) {
	for s, want := range map[string]string{
		"1": "1", " 5 ": "5", "": "0", "1.0": "1", "1e3": "1000", ".5": "0.5", "abc": "NaN", "1.": "1",
		"1e": "NaN", "-": "NaN", "Infinity": "Infinity", "-Infinity": "-Infinity", "0x10": "NaN", "1_0": "NaN",
	} {
		if got := jsNumberString(jsToNumber(s)); got != want {
			t.Errorf("jsToNumber(%q) = %s, want %s", s, got, want)
		}
	}
}

func Example() {
	m, _ := Compile("{n, plural, =0 {Belum ada catatan} other {# catatan}}", DefaultLimits())
	for _, n := range []int{0, 1500} {
		s, _ := m.Format("id", Args{"n": n})
		fmt.Println(s)
	}
	// Output:
	// Belum ada catatan
	// 1.500 catatan
}
