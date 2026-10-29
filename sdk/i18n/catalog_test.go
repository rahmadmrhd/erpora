package i18n

import (
	"fmt"
	"strings"
	"testing"
)

func parseStr(s string, lim Limits) (Flat, []Issue) {
	return ParseCatalog(strings.NewReader(s), lim)
}

func codes(is []Issue) []string {
	out := make([]string, len(is))
	for i, x := range is {
		out[i] = x.Code
	}
	return out
}

// wantIssue memastikan ada tepat satu Issue dengan kode dan (bila diberikan) Detail memuat potongan.
func wantIssue(t *testing.T, is []Issue, code string, detailHas ...string) Issue {
	t.Helper()
	if len(is) == 0 {
		t.Fatalf("tidak ada Issue, want %s", code)
	}
	for _, x := range is {
		if x.Code != code {
			continue
		}
		for _, d := range detailHas {
			if !strings.Contains(x.Detail, d) && !strings.Contains(x.Key, d) {
				t.Errorf("Issue %s: Detail/Key %q/%q harus memuat %q", code, x.Detail, x.Key, d)
			}
		}
		return x
	}
	t.Fatalf("tidak ada Issue berkode %s; ada %v", code, codes(is))
	return Issue{}
}

func TestParseCatalogFlattensTree(t *testing.T) {
	got, issues := parseStr(`{
		"note": {"title": "Catatan", "status": {"draft": "Draf"}},
		"fields": {"note": {"title": "Judul"}},
		"errors": {"not_found": "Catatan tidak ditemukan"}
	}`, DefaultLimits())
	if len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
	want := Flat{
		"note.title":        "Catatan",
		"note.status.draft": "Draf",
		"fields.note.title": "Judul",
		"errors.not_found":  "Catatan tidak ditemukan",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseCatalogDottedSegments(t *testing.T) {
	got, issues := parseStr(`{"a.b": {"c": "x"}, "order": {"status.draft": "Draf"}}`, DefaultLimits())
	if len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
	if got["a.b.c"] != "x" || got["order.status.draft"] != "Draf" {
		t.Errorf("got = %v", got)
	}
}

func TestParseCatalogEmptyObjectAndBOM(t *testing.T) {
	for name, in := range map[string]string{"kosong": `{}`, "bom": string(rune(0xFEFF)) + `{"a":"x"}`} {
		got, issues := parseStr(in, DefaultLimits())
		if len(issues) != 0 {
			t.Errorf("%s: issues = %v", name, issues)
		}
		if name == "bom" && got["a"] != "x" {
			t.Errorf("bom: got = %v", got)
		}
	}
}

func TestParseCatalogFlattenCollisionNamesBothPaths(t *testing.T) {
	got, issues := parseStr(`{"a.b": {"c": "x"}, "a": {"b.c": "y"}}`, DefaultLimits())
	is := wantIssue(t, issues, CodeFlattenCollision, "a.b.c", "/a.b/c", "/a/b.c")
	if is.Key != "a.b.c" {
		t.Errorf("Key = %q, want a.b.c", is.Key)
	}
	if got != nil {
		t.Errorf("Flat harus nil bila ada Issue, got %v", got)
	}
}

// Tabrakan tiga jalur atau lebih: SATU Issue per kunci lengkap yang memuat SEMUA jalur, urut deterministik
// (Requirement 1.6; keputusan G23).
func TestParseCatalogFlattenCollisionThreePathsOneIssue(t *testing.T) {
	const in = `{"b": {"c": {"b.c": "1"}}, "b.c": {"b": {"c": "2"}, "b.c": "3"}}`
	var first string
	for i := 0; i < 20; i++ { // urutan pelaporan tidak bergantung pada iterasi map
		got, issues := parseStr(in, DefaultLimits())
		if got != nil {
			t.Fatal("Flat harus nil bila ada tabrakan")
		}
		if len(issues) != 1 {
			t.Fatalf("issues = %v, want tepat satu Issue untuk satu kunci", issues)
		}
		is := issues[0]
		if is.Code != CodeFlattenCollision || is.Key != "b.c.b.c" {
			t.Fatalf("Issue = %+v", is)
		}
		for _, p := range []string{"/b/c/b.c", "/b.c/b/c", "/b.c/b.c"} {
			if !strings.Contains(is.Detail, p) {
				t.Errorf("Detail %q tidak menyebut jalur %s", is.Detail, p)
			}
		}
		if first == "" {
			first = is.Detail
		} else if is.Detail != first {
			t.Fatalf("Detail tidak deterministik: %q vs %q", is.Detail, first)
		}
	}
	// jalur terurut leksikografis
	if i, j, k := strings.Index(first, "/b.c/b.c"), strings.Index(first, "/b.c/b/c"), strings.Index(first, "/b/c/b.c"); !(i < j && j < k) {
		t.Errorf("jalur harus terurut: %q", first)
	}
}

// Dua kunci berbeda yang masing-masing bertabrakan menghasilkan dua Issue (satu per kunci), terurut menurut kunci.
func TestParseCatalogFlattenCollisionOneIssuePerKey(t *testing.T) {
	_, issues := parseStr(`{"a.b": {"c": "1"}, "a": {"b.c": "2"}, "x.y": {"z": "3"}, "x": {"y.z": "4"}}`, DefaultLimits())
	if len(issues) != 2 || issues[0].Key != "a.b.c" || issues[1].Key != "x.y.z" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestParseCatalogDuplicateKeys(t *testing.T) {
	cases := map[string]string{
		"akar":      `{"a": "x", "a": "y"}`,
		"bersarang": `{"o": {"k": "1", "k": "2"}}`,
		"escape":    `{"a": "x", "a": "y"}`,
		"objek":     `{"a": {"b": "x"}, "a": {"c": "y"}}`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, issues := parseStr(in, DefaultLimits())
			wantIssue(t, issues, CodeDuplicateKey)
		})
	}
	_, issues := parseStr(`{"o": {"k": "1", "k": "2"}}`, DefaultLimits())
	wantIssue(t, issues, CodeDuplicateKey, "o.k")
}

func TestParseCatalogInvalidSegments(t *testing.T) {
	bad := []string{"", "a b", "a..b", ".a", "a.", "a::b", "é", "a/b", "a:b", "a\tb", "a{b}", "a'b"}
	for _, seg := range bad {
		t.Run(fmt.Sprintf("%q", seg), func(t *testing.T) {
			b, _ := jsonString(seg)
			_, issues := parseStr(`{`+b+`: "x"}`, DefaultLimits())
			wantIssue(t, issues, CodeInvalidSegment)
		})
	}
	// segmen sah (termasuk yang bertitik di tengah, garis bawah, hubung, angka)
	for _, seg := range []string{"a", "a.b", "A_b-c", "9", "status.draft", "a.b.c"} {
		b, _ := jsonString(seg)
		if _, issues := parseStr(`{`+b+`: "x"}`, DefaultLimits()); len(issues) != 0 {
			t.Errorf("segmen %q seharusnya sah: %v", seg, issues)
		}
	}
	// segmen tak sah di tingkat dalam menyebut jalur
	_, issues := parseStr(`{"a": {"b c": "x"}}`, DefaultLimits())
	wantIssue(t, issues, CodeInvalidSegment, "a")
}

func jsonString(s string) (string, error) {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case r < 0x20:
			fmt.Fprintf(&sb, `\u%04x`, r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String(), nil
}

func TestParseCatalogHTMLNotAllowed(t *testing.T) {
	bad := []string{
		`<script>alert(1)</script>`,
		`<img src=x onerror=alert(1)>`,
		`<a href="x">y</a>`,
		`<B>x</B>`,
		`<b class="x">y</b>`,
		`5 < 6`,
		`a<br/>b`,
		`<iframe>x</iframe>`,
		`x <style>p{}</style>`,
		`<my-tag>x</my-tag>`,
		`<b >x</b>`,
	}
	for _, msg := range bad {
		t.Run(msg, func(t *testing.T) {
			b, _ := jsonString(msg)
			_, issues := parseStr(`{"k": `+b+`}`, DefaultLimits())
			if len(issues) == 0 {
				t.Fatalf("pesan %q seharusnya ditolak", msg)
			}
			// pesan bersintaks ICU tak sah dilaporkan sebagai invalid_icu; selebihnya html_not_allowed
			is := issues[0]
			if is.Code != CodeHTMLNotAllowed && is.Code != CodeInvalidICU {
				t.Errorf("Code = %s, want html_not_allowed/invalid_icu", is.Code)
			}
			if is.Key != "k" {
				t.Errorf("Key = %q, want k", is.Key)
			}
		})
	}
	// yang tidak termasuk HTML: tag rich sederhana, kutip ICU, dan teks tanpa '<'
	for _, msg := range []string{`Klik <b>di sini</b>`, `<a>tautan</a>`, `a '<' b`, `a '<b>' c`, `x > y`, `<b><i>x</i></b>`, `<h1>judul</h1>`} {
		b, _ := jsonString(msg)
		if _, issues := parseStr(`{"k": `+b+`}`, DefaultLimits()); len(issues) != 0 {
			t.Errorf("pesan %q seharusnya sah: %v", msg, issues)
		}
	}
	// kode khusus html_not_allowed untuk kasus yang sintaks ICU-nya sah
	b, _ := jsonString(`<script>alert(1)</script>`)
	_, issues := parseStr(`{"k": `+b+`}`, DefaultLimits())
	wantIssue(t, issues, CodeHTMLNotAllowed)
	b, _ = jsonString(`5 < 6`)
	_, issues = parseStr(`{"k": `+b+`}`, DefaultLimits())
	wantIssue(t, issues, CodeHTMLNotAllowed)
}

func TestParseCatalogICUIssues(t *testing.T) {
	_, issues := parseStr(`{"a": {"b": "Halo {nama"}}`, DefaultLimits())
	is := wantIssue(t, issues, CodeInvalidICU)
	if is.Key != "a.b" {
		t.Errorf("Key = %q, want a.b", is.Key)
	}
	_, issues = parseStr(`{"k": "{n, selectordinal, one {#st} other {#th}}"}`, DefaultLimits())
	wantIssue(t, issues, CodeUnsupportedConstruct)
	_, issues = parseStr(`{"k": "{n, number}"}`, DefaultLimits())
	wantIssue(t, issues, CodeUnsupportedConstruct)
	_, issues = parseStr(`{"k": "`+strings.Repeat("a", 2001)+`"}`, DefaultLimits())
	wantIssue(t, issues, CodeTooLarge)
	// Detail tidak boleh memuat pesan utuh
	_, issues = parseStr(`{"k": "`+strings.Repeat("{", 500)+`"}`, DefaultLimits())
	if len(issues) == 0 || len(issues[0].Detail) > 300 {
		t.Errorf("Detail harus ada dan ringkas: %v", issues)
	}
}

func TestParseCatalogInvalidJSON(t *testing.T) {
	cases := map[string]string{
		"kosong":             ``,
		"kurung buka":        `{`,
		"array akar":         `[]`,
		"string akar":        `"x"`,
		"angka akar":         `1`,
		"nilai angka":        `{"a": 1}`,
		"nilai null":         `{"a": null}`,
		"nilai bool":         `{"a": true}`,
		"nilai array":        `{"a": ["x"]}`,
		"array di dalam":     `{"a": {"b": [1]}}`,
		"koma berlebih":      `{"a": "x",}`,
		"data sesudah akar":  `{"a": "x"} {"b": "y"}`,
		"kutip tunggal":      `{'a': 'x'}`,
		"string tak ditutup": `{"a": "x`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got, issues := parseStr(in, DefaultLimits())
			wantIssue(t, issues, CodeInvalidJSON)
			if got != nil {
				t.Errorf("Flat harus nil bila ada Issue")
			}
		})
	}
}

func TestParseCatalogLimits(t *testing.T) {
	t.Run("berkas melebihi MaxFileBytes", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxFileBytes = 64
		in := `{"a": "` + strings.Repeat("x", 100) + `"}`
		_, issues := parseStr(in, lim)
		wantIssue(t, issues, CodeTooLarge)
	})
	t.Run("berkas tepat di batas lolos", func(t *testing.T) {
		in := `{"a":"x"}`
		lim := DefaultLimits()
		lim.MaxFileBytes = len(in)
		if _, issues := parseStr(in, lim); len(issues) != 0 {
			t.Errorf("issues = %v", issues)
		}
	})
	t.Run("kunci melebihi MaxKeys", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxKeys = 3
		_, issues := parseStr(`{"a":"1","b":"2","c":"3","d":"4"}`, lim)
		wantIssue(t, issues, CodeTooLarge)
		if _, issues = parseStr(`{"a":"1","b":"2","c":"3"}`, lim); len(issues) != 0 {
			t.Errorf("tepat di batas harus lolos: %v", issues)
		}
	})
	t.Run("kunci lengkap melebihi MaxKeyLen", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxKeyLen = 10
		_, issues := parseStr(`{"abcdef": {"ghijkl": "x"}}`, lim)
		wantIssue(t, issues, CodeTooLarge)
		if _, issues = parseStr(`{"abcde": {"fghi": "x"}}`, lim); len(issues) != 0 {
			t.Errorf("tepat 10 karakter harus lolos: %v", issues)
		}
	})
	t.Run("sarang sangat dalam tidak menghabiskan sumber daya", func(t *testing.T) {
		in := strings.Repeat(`{"a":`, 100000) + `"x"` + strings.Repeat(`}`, 100000)
		_, issues := parseStr(in, Limits{MaxFileBytes: 1 << 20})
		if len(issues) == 0 {
			t.Fatal("harus ada Issue")
		}
	})
	t.Run("jumlah Issue dibatasi", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("{")
		for i := 0; i < 2000; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `"bad key %d": "x"`, i)
		}
		sb.WriteString("}")
		_, issues := parseStr(sb.String(), DefaultLimits())
		if len(issues) == 0 || len(issues) > 150 {
			t.Errorf("len(issues) = %d, harus 1..150", len(issues))
		}
	})
}

func TestParseCatalogReadError(t *testing.T) {
	_, issues := ParseCatalog(errReader{}, DefaultLimits())
	wantIssue(t, issues, CodeInvalidJSON)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("disk gagal") }
