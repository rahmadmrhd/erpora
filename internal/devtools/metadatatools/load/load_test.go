package load

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

const validResource = `schemaVersion: "1"
kind: resource
key: sales.invoice
include:
  - source: sdk:document
labelTemplate: "{number} · {customerName}"
fields:
  number: {type: string, maxLength: 32, readOnly: true, sortable: true, filterable: true, label: sales.invoice.number}
  issuedOn: {type: date, sortable: true, label: sales.invoice.issuedOn}
  dueOn: {type: date, label: sales.invoice.dueOn}
  total: {type: decimal, precision: 18, scale: 2, aggregatable: true, label: sales.invoice.total}
  customerName: {type: string, maxLength: 200, readOnly: true, fieldGroup: general, classification: internal}
  contactEmail: {type: sdk:email, label: sales.invoice.contactEmail}
  state: {type: enum, values: [open, paid], extensible: true}
  approvalNote:
    type: text
    nullable: true
    requiredWhen: {field: total, op: gte, value: "50000000.00"}
    visibleWhen:
      all:
        - {field: state, op: eq, value: open}
        - any:
            - {field: total, op: gt, value: "1.00"}
            - not: {field: approvalNote, op: isnull, value: true}
relations:
  customer:
    type: belongsTo
    to: crm.customer
    required: true
    filter: {field: status, op: eq, value: active}
    snapshot: [{from: name, to: customerName}]
  lines:
    type: hasMany
    of: sales.invoiceLine
    embedded: true
rules:
  - id: dueAfterIssue
    assert: {field: dueOn, op: gte, ref: issuedOn}
    field: dueOn
    message: sales.invoice.error.dueBeforeIssued
indexes:
  - fields: [issuedOn, dueOn]
generate:
  crud: false
`

const validAbstract = `schemaVersion: "1"
kind: abstract
key: crm.address
export: true
layout: {section: address, columns: 2, order: [street, city]}
fields:
  street: {type: string, maxLength: 200, label: crm.address.street}
  city: {type: string, maxLength: 100}
`

const validFieldType = `schemaVersion: "1"
kind: fieldType
key: crm.email
export: true
base: {type: string, maxLength: 254, format: email}
classification: confidential
widget: email
operators: [eq, contains, isnull]
`

func mustParse(t *testing.T, src string) *Document {
	t.Helper()
	doc, probs := Parse("x.resource.yaml", []byte(src))
	if len(probs) != 0 {
		t.Fatalf("seharusnya valid, temuan: %v", probs)
	}
	return doc
}

func codes(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Code
	}
	return out
}

func find(ps []Problem, code string) *Problem {
	for i := range ps {
		if ps[i].Code == code {
			return &ps[i]
		}
	}
	return nil
}

func TestParseValidDocuments(t *testing.T) {
	d := mustParse(t, validResource)
	if d.Kind != "resource" || d.Key != "sales.invoice" || d.SchemaVersion != "1" {
		t.Fatalf("header = %+v", d)
	}
	if p := d.Pos("/fields/total"); p.Line != 11 {
		t.Errorf("baris /fields/total = %d, ingin 11", p.Line)
	}
	if d := mustParse(t, validAbstract); d.Kind != "abstract" {
		t.Errorf("kind = %q", d.Kind)
	}
	if d := mustParse(t, validFieldType); d.Kind != "fieldType" {
		t.Errorf("kind = %q", d.Kind)
	}
	var out struct {
		Key    string                    `json:"key"`
		Fields map[string]map[string]any `json:"fields"`
	}
	if err := d.Decode(&out); err != nil || out.Key != "sales.invoice" || len(out.Fields) != 8 {
		t.Fatalf("Decode: %v %+v", err, out)
	}
}

// Tabel pelanggaran skema: tiap kasus menunjuk baris yang benar.
func TestSchemaViolations(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\n"
	tests := []struct {
		name    string
		src     string
		pointer string
		line    int
		substr  string
	}{
		{"kind tak dikenal", "schemaVersion: \"1\"\nkind: table\nkey: sales.invoice\n", "/kind", 2, ""},
		{"kind hilang", "schemaVersion: \"1\"\nkey: sales.invoice\n", "", 0, "kind"},
		{"schemaVersion salah", "schemaVersion: \"2\"\nkind: resource\nkey: sales.invoice\n", "/schemaVersion", 1, ""},
		{"schemaVersion bukan teks", "schemaVersion: 1\nkind: resource\nkey: sales.invoice\n", "/schemaVersion", 1, ""},
		{"key tanpa slug", "schemaVersion: \"1\"\nkind: resource\nkey: invoice\n", "/key", 3, ""},
		{"key nama bukan camelCase", "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice_line\n", "/key", 3, ""},
		{"kunci tingkat atas tak dikenal", head + "bogus: 1\n", "/bogus", 4, "tidak dikenal"},
		{"kunci tak dikenal pada field", head + "fields:\n  a:\n    type: text\n    sql: x\n", "/fields/a/sql", 7, "tidak dikenal"},
		{"snake_case pada kunci atribut", head + "fields:\n  a:\n    type: string\n    max_length: 3\n", "/fields/a/max_length", 7, "tidak dikenal"},
		{"tipe tak sah", head + "fields:\n  a: {type: float}\n", "/fields/a/type", 5, ""},
		{"tipe hilang", head + "fields:\n  a: {maxLength: 3}\n", "/fields/a", 5, "type"},
		{"nama field bukan camelCase", head + "fields:\n  Customer_Name: {type: text}\n", "/fields/Customer_Name", 5, ""},
		{"nama field kapital berurutan", head + "fields:\n  taxID: {type: text}\n", "/fields/taxID", 5, ""},
		{"klasifikasi tak sah", head + "fields:\n  a: {type: text, classification: secret}\n", "/fields/a/classification", 5, ""},
		{"operator tak sah", head + "lockWhen: {field: status, op: like, value: x}\n", "/lockWhen/op", 4, ""},
		{"daun tanpa value atau ref", head + "lockWhen: {field: status, op: eq}\n", "/lockWhen", 4, ""},
		{"daun dengan value dan ref", head + "lockWhen: {field: a, op: eq, value: x, ref: b}\n", "/lockWhen", 4, ""},
		{"relasi tipe tak sah", head + "relations:\n  r: {type: owns, to: a.b}\n", "/relations/r/type", 5, ""},
		{"relasi belongsTo tanpa to", head + "relations:\n  r: {type: belongsTo}\n", "/relations/r", 5, "to"},
		{"embedded pada belongsTo", head + "relations:\n  r: {type: belongsTo, to: a.b, embedded: true}\n", "/relations/r/type", 5, ""},
		{"include source berupa jalur", head + "include:\n  - source: ../secret\n", "/include/0/source", 5, ""},
		{"include tanpa source", head + "include:\n  - prefix: billing\n", "/include/0", 5, "source"},
		{"enum nilai bukan snake_case", head + "fields:\n  a: {type: enum, values: [Open]}\n", "/fields/a/values/0", 5, ""},
		{"deprecated tanpa since", head + "fields:\n  a: {type: text, deprecated: {replacedBy: b}}\n", "/fields/a/deprecated", 5, "since"},
		{"clientVisible false tanpa alasan", head + "fields:\n  a: {type: text, clientVisible: false}\n", "/fields/a", 5, "clientVisibleReason"},
		{"unmanagedColumns tanpa alasan", head + "unmanagedColumns:\n  - name: secretCol\n", "/unmanagedColumns/0", 5, "reason"},
		{"abstract tidak boleh generate", "schemaVersion: \"1\"\nkind: abstract\nkey: a.b\ngenerate: {crud: true}\n", "/generate", 4, "tidak dikenal"},
		{"label bukan kunci i18n", head + "fields:\n  a: {type: text, label: \"Nama Pelanggan\"}\n", "/fields/a/label", 5, ""},
		{"labelTemplate memuat kata", head + "labelTemplate: \"No. {number}\"\n", "/labelTemplate", 4, ""},
		{"labelTemplate kurung tak berpasangan", head + "labelTemplate: \"{number\"\n", "/labelTemplate", 4, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ps := Parse("v.resource.yaml", []byte(tc.src))
			if len(ps) == 0 {
				t.Fatal("seharusnya ada temuan")
			}
			var hit *Problem
			for i := range ps {
				p := &ps[i]
				if p.Code != CodeSchema {
					continue
				}
				if (tc.pointer == "" || p.Pointer == tc.pointer) && strings.Contains(p.Message, tc.substr) {
					hit = p
					break
				}
			}
			if hit == nil {
				t.Fatalf("tidak ada temuan schema di %q memuat %q; temuan: %v", tc.pointer, tc.substr, ps)
			}
			if tc.line != 0 && hit.Line != tc.line {
				t.Errorf("baris = %d, ingin %d (%v)", hit.Line, tc.line, hit)
			}
			if hit.File != "v.resource.yaml" || hit.Column <= 0 && tc.line != 0 {
				t.Errorf("berkas/kolom tidak terisi: %+v", hit)
			}
		})
	}
}

func TestKindErrorDoesNotCascade(t *testing.T) {
	_, ps := Parse("v.yaml", []byte("schemaVersion: \"1\"\nkind: table\nkey: a.b\nfields:\n  a: {type: text}\n"))
	if len(ps) != 1 || ps[0].Pointer != "/kind" {
		t.Fatalf("kind tak sah hanya boleh menghasilkan satu temuan: %v", ps)
	}
}

func TestAllViolationsReportedAtOnce(t *testing.T) {
	src := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\nbogus: 1\nfields:\n  a: {type: float}\n  Bad_Name: {type: text}\nrelations:\n  r: {type: belongsTo}\n"
	_, ps := Parse("v.resource.yaml", []byte(src))
	pointers := map[string]bool{}
	for _, p := range ps {
		pointers[p.Pointer] = true
		if p.Line == 0 || p.File == "" {
			t.Errorf("temuan tanpa lokasi: %+v", p)
		}
	}
	for _, want := range []string{"/bogus", "/fields/a/type", "/fields/Bad_Name", "/relations/r"} {
		if !pointers[want] {
			t.Errorf("tidak dilaporkan: %s (ada %v)", want, pointers)
		}
	}
}

func repeatFields(n int) string {
	var sb strings.Builder
	sb.WriteString("schemaVersion: \"1\"\nkind: resource\nkey: sales.big\nfields:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "  f%s: {type: text}\n", letters(i))
	}
	return sb.String()
}

// letters menghasilkan nama huruf kecil unik (a, b, ..., z, aa, ab, ...).
func letters(i int) string {
	s := ""
	for {
		s = string(rune('a'+i%26)) + s
		i = i/26 - 1
		if i < 0 {
			return s
		}
	}
}

func TestSizeLimits(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\n"
	t.Run("200 field lolos", func(t *testing.T) { mustParse(t, repeatFields(200)) })
	t.Run("201 field ditolak", func(t *testing.T) {
		_, ps := Parse("v.yaml", []byte(repeatFields(201)))
		if p := find(ps, CodeSchema); p == nil || p.Pointer != "/fields" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("16 relasi lolos, 17 ditolak", func(t *testing.T) {
		build := func(n int) string {
			var sb strings.Builder
			sb.WriteString(head + "relations:\n")
			for i := 0; i < n; i++ {
				fmt.Fprintf(&sb, "  r%s: {type: belongsTo, to: crm.customer}\n", letters(i))
			}
			return sb.String()
		}
		mustParse(t, build(16))
		_, ps := Parse("v.yaml", []byte(build(17)))
		if p := find(ps, CodeSchema); p == nil || p.Pointer != "/relations" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("100 nilai in lolos, 101 ditolak", func(t *testing.T) {
		build := func(n int) string {
			vals := make([]string, n)
			for i := range vals {
				vals[i] = fmt.Sprintf("v%d", i)
			}
			return head + "lockWhen: {field: status, op: in, value: [" + strings.Join(vals, ", ") + "]}\n"
		}
		mustParse(t, build(100))
		_, ps := Parse("v.yaml", []byte(build(101)))
		if p := find(ps, CodeSchema); p == nil || p.Pointer != "/lockWhen/value" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("100 nilai enum lolos, 101 ditolak", func(t *testing.T) {
		build := func(n int) string {
			vals := make([]string, n)
			for i := range vals {
				vals[i] = fmt.Sprintf("v%d", i)
			}
			return head + "fields:\n  s: {type: enum, values: [" + strings.Join(vals, ", ") + "]}\n"
		}
		mustParse(t, build(100))
		_, ps := Parse("v.yaml", []byte(build(101)))
		if p := find(ps, CodeSchema); p == nil || p.Pointer != "/fields/s/values" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("pattern 200 lolos, 201 ditolak", func(t *testing.T) {
		mustParse(t, head+"fields:\n  s: {type: string, pattern: \""+strings.Repeat("a", 200)+"\"}\n")
		_, ps := Parse("v.yaml", []byte(head+"fields:\n  s: {type: string, pattern: \""+strings.Repeat("a", 201)+"\"}\n"))
		if p := find(ps, CodeSchema); p == nil || p.Pointer != "/fields/s/pattern" {
			t.Fatalf("temuan = %v", ps)
		}
	})
}

// chain membangun kondisi tersarang: kedalaman d (daun = 1).
func chain(d int) string {
	if d == 1 {
		return "{field: a, op: eq, value: x}"
	}
	return "{not: " + chain(d-1) + "}"
}

func wideAll(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = "{field: a, op: eq, value: x}"
	}
	return "{all: [" + strings.Join(items, ", ") + "]}"
}

func TestConditionLimits(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\n"
	t.Run("kedalaman 4 lolos", func(t *testing.T) { mustParse(t, head+"lockWhen: "+chain(4)+"\n") })
	t.Run("kedalaman 5 ditolak", func(t *testing.T) {
		_, ps := Parse("v.yaml", []byte(head+"lockWhen: "+chain(5)+"\n"))
		p := find(ps, CodeConditionDepth)
		if p == nil || p.Pointer != "/lockWhen" || p.Line != 4 {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("32 node lolos", func(t *testing.T) {
		// all + 31 daun = 32 node.
		mustParse(t, head+"lockWhen: "+wideAll(31)+"\n")
	})
	t.Run("33 node ditolak", func(t *testing.T) {
		_, ps := Parse("v.yaml", []byte(head+"lockWhen: "+wideAll(32)+"\n"))
		if p := find(ps, CodeConditionNodes); p == nil || p.Pointer != "/lockWhen" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("batas berlaku pada rules, filter relasi, dan field", func(t *testing.T) {
		src := head + "fields:\n  a: {type: text, visibleWhen: " + wideAll(40) + "}\n" +
			"relations:\n  r: {type: belongsTo, to: a.b, filter: " + chain(6) + "}\n" +
			"rules:\n  - {id: x, assert: " + wideAll(40) + ", message: a.b.c}\n"
		_, ps := Parse("v.yaml", []byte(src))
		got := map[string]bool{}
		for _, p := range ps {
			if p.Code == CodeConditionNodes || p.Code == CodeConditionDepth {
				got[p.Pointer] = true
			}
		}
		for _, want := range []string{"/fields/a/visibleWhen", "/relations/r/filter", "/rules/0/assert"} {
			if !got[want] {
				t.Errorf("batas kondisi tidak diperiksa di %s: %v", want, ps)
			}
		}
	})
	t.Run("batas berlaku pada timpaan include", func(t *testing.T) {
		src := head + "include:\n  - source: sdk:document\n    set:\n      status: {readonlyWhen: " + wideAll(40) + "}\n"
		_, ps := Parse("v.yaml", []byte(src))
		if find(ps, CodeConditionNodes) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
}

func TestForbiddenContent(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\n"
	tests := []struct {
		name string
		src  string
		code string
	}{
		{"URL pada description", head + "description: \"lihat https://example.com/x\"\n", CodeForbiddenValue},
		{"URL pada default", head + "fields:\n  a: {type: string, default: \"http://evil.example\"}\n", CodeForbiddenValue},
		{"URL protokol lain", head + "fields:\n  a: {type: string, default: \"ftp://evil.example\"}\n", CodeForbiddenValue},
		{"tanda titik koma pada description", head + "description: \"x; DROP TABLE users\"\n", CodeSchema},
		{"backtick pada description", head + "description: \"`ls`\"\n", CodeSchema},
		{"karakter kontrol pada description", head + "description: \"a\\u0000b\"\n", CodeSchema},
		{"dolar ganda pada description", head + "description: \"$$ body $$\"\n", CodeSchema},
		{"kurung kurawal pada description", head + "description: \"${jndi}\"\n", CodeSchema},
		{"komentar blok pada description", head + "description: \"a <script>\"\n", CodeSchema},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ps := Parse("v.yaml", []byte(tc.src))
			if find(ps, tc.code) == nil {
				t.Fatalf("seharusnya %s; temuan = %v", tc.code, ps)
			}
		})
	}
	// Teks biasa dan regex pattern yang memuat // tidak ditolak.
	mustParse(t, head+"description: \"Nomor faktur, dibuat otomatis (lihat panduan).\"\nfields:\n  a: {type: string, pattern: \"^a//b$\"}\n")
}

func TestHostileYAML(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\n"
	t.Run("berkas lebih dari 256 KB", func(t *testing.T) {
		big := head + "description: \"" + strings.Repeat("a", 256*1024) + "\"\n"
		_, ps := Parse("big.yaml", []byte(big))
		if len(ps) != 1 || ps[0].Code != CodeFileTooLarge || ps[0].File != "big.yaml" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("tepat 256 KB tidak ditolak karena ukuran", func(t *testing.T) {
		pad := 256*1024 - len(head) - len("description: \"\"\n")
		_, ps := Parse("big.yaml", []byte(head+"description: \""+strings.Repeat("a", pad)+"\"\n"))
		if find(ps, CodeFileTooLarge) != nil {
			t.Fatalf("256 KB seharusnya lolos batas ukuran: %v", codes(ps))
		}
	})
	t.Run("alias bomb", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString(head + "x-bomb:\n  a: &a [lol, lol, lol, lol, lol, lol, lol, lol, lol]\n")
		prev := "a"
		for _, n := range []string{"b", "c", "d", "e", "f", "g", "h", "i"} {
			fmt.Fprintf(&sb, "  %s: &%s [*%s, *%s, *%s, *%s, *%s, *%s, *%s, *%s, *%s]\n", n, n, prev, prev, prev, prev, prev, prev, prev, prev, prev)
			prev = n
		}
		_, ps := Parse("bomb.yaml", []byte(sb.String()))
		if find(ps, CodeYAMLAlias) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("ekspansi alias besar dengan sedikit alias", func(t *testing.T) {
		vals := strings.TrimSuffix(strings.Repeat("x, ", 4000), ", ")
		src := head + "x-big:\n  a: &a [" + vals + "]\n  b: [" + strings.TrimSuffix(strings.Repeat("*a, ", 10), ", ") + "]\n"
		_, ps := Parse("exp.yaml", []byte(src))
		if find(ps, CodeYAMLAlias) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("alias melingkar", func(t *testing.T) {
		_, ps := Parse("cyc.yaml", []byte(head+"x-c: &a [*a]\n"))
		if len(ps) == 0 {
			t.Fatal("alias melingkar harus ditolak")
		}
	})
	t.Run("alias biasa dalam batas diterima sebagai YAML", func(t *testing.T) {
		src := head + "fields:\n  a: &t {type: text}\n  b: *t\n"
		doc, ps := Parse("ok.yaml", []byte(src))
		if len(ps) != 0 {
			t.Fatalf("temuan = %v", ps)
		}
		if _, ok := doc.Value["fields"].(map[string]any)["b"]; !ok {
			t.Fatal("alias harus diekspansi")
		}
	})
	t.Run("kedalaman berlebih", func(t *testing.T) {
		deep := head + "x-deep: " + strings.Repeat("[", 200) + strings.Repeat("]", 200) + "\n"
		_, ps := Parse("deep.yaml", []byte(deep))
		if p := find(ps, CodeYAMLDepth); p == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("kedalaman ekstrem tidak membuat panik", func(t *testing.T) {
		deep := head + "x-deep: " + strings.Repeat("[", 30000) + strings.Repeat("]", 30000) + "\n"
		_, ps := Parse("deep.yaml", []byte(deep))
		if len(ps) == 0 {
			t.Fatal("harus ditolak")
		}
	})
	t.Run("tag kustom", func(t *testing.T) {
		for _, src := range []string{
			head + "description: !custom abc\n",
			head + "description: !!binary YWJj\n",
			head + "x-t: !!python/object/apply:os.system [ls]\n",
			head + "x-s: !!set {a, b}\n",
			head + "fields: !Evil\n  a: {type: text}\n",
		} {
			_, ps := Parse("tag.yaml", []byte(src))
			p := find(ps, CodeYAMLTag)
			if p == nil || p.Line < 4 {
				t.Errorf("tag harus ditolak (%q): %v", src, ps)
			}
		}
	})
	t.Run("tag inti diizinkan", func(t *testing.T) {
		mustParse(t, head+"description: !!str abc\nfields:\n  a: {type: text, nullable: !!bool true}\n")
	})
	t.Run("merge key", func(t *testing.T) {
		_, ps := Parse("m.yaml", []byte(head+"fields:\n  base: &b {type: text}\n  a:\n    <<: *b\n"))
		if find(ps, CodeYAMLMerge) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("kunci ganda", func(t *testing.T) {
		_, ps := Parse("d.yaml", []byte(head+"fields:\n  a: {type: text}\n  a: {type: text}\n"))
		if p := find(ps, CodeYAMLDuplicateKey); p == nil || p.Line != 6 {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("kunci bukan teks", func(t *testing.T) {
		_, ps := Parse("k.yaml", []byte(head+"fields:\n  1: {type: text}\n"))
		if find(ps, CodeYAMLKey) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("lebih dari satu dokumen", func(t *testing.T) {
		_, ps := Parse("m.yaml", []byte(head+"---\nschemaVersion: \"1\"\n"))
		if find(ps, CodeYAMLMultiDoc) == nil {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("kosong dan bukan objek", func(t *testing.T) {
		for _, src := range []string{"", "   \n", "# komentar saja\n", "- a\n- b\n", "plain\n"} {
			_, ps := Parse("e.yaml", []byte(src))
			if len(ps) == 0 {
				t.Errorf("%q harus ditolak", src)
			}
		}
	})
	t.Run("sintaks YAML rusak membawa nomor baris", func(t *testing.T) {
		_, ps := Parse("s.yaml", []byte(head+"fields: [unclosed\n  a: 1\n"))
		p := find(ps, CodeYAMLSyntax)
		if p == nil || p.Line == 0 {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("tanggal tanpa kutip dibaca sebagai teks", func(t *testing.T) {
		d := mustParse(t, head+"fields:\n  a: {type: date, default: 2026-11-01}\n")
		def := d.Value["fields"].(map[string]any)["a"].(map[string]any)["default"]
		if def != "2026-11-01" {
			t.Fatalf("default = %#v", def)
		}
	})
}

func TestLimitsConfigurable(t *testing.T) {
	l := DefaultLimits()
	if l.MaxFileBytes != 256*1024 {
		t.Fatalf("MaxFileBytes = %d", l.MaxFileBytes)
	}
	l.MaxFileBytes = 50
	_, ps := ParseWithLimits("a.yaml", []byte(validResource), l)
	if len(ps) != 1 || ps[0].Code != CodeFileTooLarge {
		t.Fatalf("temuan = %v", ps)
	}
}

func TestLoadFS(t *testing.T) {
	fsys := fstest.MapFS{
		"metadata/invoice.resource.yaml":        {Data: []byte(validResource)},
		"metadata/blocks/address.abstract.yaml": {Data: []byte(validAbstract)},
		"metadata/types/email.type.yaml":        {Data: []byte(validFieldType)},
		"metadata/bad.resource.yaml":            {Data: []byte("schemaVersion: \"1\"\nkind: resource\nkey: sales.bad\nbogus: 1\n")},
		"metadata/mismatch.abstract.yaml":       {Data: []byte(validResource)},
		"metadata/plain.yaml":                   {Data: []byte(validResource)},
		"metadata/README.md":                    {Data: []byte("abaikan")},
	}
	docs, ps := LoadFS(fsys, "metadata")
	if len(docs) != 3 {
		t.Fatalf("dokumen = %d (%v)", len(docs), ps)
	}
	files := map[string]bool{}
	for _, p := range ps {
		files[p.File] = true
		if p.File == "" || p.Line == 0 && p.Code != CodeFileName && p.Code != CodeKindMismatch {
			t.Errorf("temuan tanpa lokasi: %+v", p)
		}
	}
	for _, want := range []string{"metadata/bad.resource.yaml", "metadata/mismatch.abstract.yaml", "metadata/plain.yaml"} {
		if !files[want] {
			t.Errorf("tidak ada temuan untuk %s: %v", want, ps)
		}
	}
	if find(ps, CodeKindMismatch) == nil || find(ps, CodeFileName) == nil {
		t.Errorf("harus ada temuan nama berkas dan ketidakcocokan kind: %v", codes(ps))
	}
	for _, d := range docs {
		if !strings.HasPrefix(d.File, "metadata/") {
			t.Errorf("File dokumen = %q", d.File)
		}
	}
	// Urutan deterministik.
	docs2, ps2 := LoadFS(fsys, "metadata")
	if len(docs2) != len(docs) || fmt.Sprint(ps2) != fmt.Sprint(ps) {
		t.Error("LoadFS tidak deterministik")
	}
	// Direktori tak ada bukan galat: modul tanpa metadata tidak terpengaruh.
	docs3, ps3 := LoadFS(fsys, "tidakAda")
	if len(docs3) != 0 || len(ps3) != 0 {
		t.Errorf("direktori tak ada: %v %v", docs3, ps3)
	}
}

func TestProblemString(t *testing.T) {
	p := Problem{File: "a/b.yaml", Line: 3, Column: 5, Pointer: "/fields/x", Code: CodeSchema, Message: "salah"}
	s := p.String()
	for _, want := range []string{"a/b.yaml:3:5", "/fields/x", "salah"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q tidak memuat %q", s, want)
		}
	}
}

func TestMessagesAreLocalizedAndHelpful(t *testing.T) {
	head := "schemaVersion: \"1\"\nkind: resource\nkey: sales.invoice\n"
	_, ps := Parse("v.yaml", []byte(head+"fields:\n  a: {type: float}\n  b: {type: text, maxLength: 0}\n"))
	var typeMsg, lenMsg string
	for _, p := range ps {
		switch p.Pointer {
		case "/fields/a/type":
			typeMsg = p.Message
		case "/fields/b/maxLength":
			lenMsg = p.Message
		}
	}
	if !strings.Contains(typeMsg, "tipe harus") || strings.Contains(typeMsg, "does not match") {
		t.Errorf("pesan tipe = %q", typeMsg)
	}
	if lenMsg == "" {
		t.Errorf("maxLength 0 harus ditolak: %v", ps)
	}
}

const kitchenSink = `schemaVersion: "1"
kind: resource
key: sales.invoice
description: Faktur penjualan, termasuk baris dan lampiran.
large: true
ownerField: createdBy
scopeFields:
  inventory.warehouse: warehouseId
include:
  - source: crm.address
    prefix: billing
    omit: [line2]
    set:
      postalCode: {requiredWhen: {field: useShipping, op: eq, value: true}}
    add:
      note: {type: text, nullable: true}
  - source: sdk:audit
labelTemplate: "{number} / {customerName}"
layout: {section: main, columns: 2, order: [number]}
fields:
  number: {type: string, minLength: 1, maxLength: 32, pattern: "^[A-Z0-9-]+$", readOnly: true, unique: true, index: true}
  total: {type: decimal, precision: 18, scale: 2, currency: IDR, minimum: "0.00", maximum: "99999999.99", aggregatable: true}
  qty: {type: integer, minimum: 0, maximum: 1000, default: 1}
  state: {type: enum, values: [draft, open], default: draft, extensible: true}
  phone: {type: string, maxLength: 32, deprecated: {since: "1.4.0", replacedBy: phoneE164}, removeAfter: "2.0.0"}
  phoneE164: {type: string, maxLength: 20, migrateFrom: phone, fieldGroup: contact, classification: confidential}
  secret: {type: text, clientVisible: false, clientVisibleReason: "nilai ambang bersifat internal"}
  marginPct:
    type: decimal
    precision: 9
    scale: 4
    accessor:
      kind: expr
      expr:
        round: {value: {div: [{field: profit}, {field: total}]}, scale: 4, mode: halfUp}
  fullName:
    type: string
    maxLength: 200
    accessor: {kind: func, func: fullNameOf}
  customerName: {type: string, maxLength: 200, snapshotSource: true, sortable: true, groupable: true, filterable: true}
relations:
  customer: {type: belongsTo, to: crm.customer, onDelete: restrict, required: true, snapshot: [{from: name, to: customerName}]}
  profile: {type: hasOne, to: sales.invoiceProfile}
  lines: {type: hasMany, of: sales.invoiceLine, embedded: true}
  tags: {type: belongsToMany, to: sales.tag, pivotFields: {note: {type: text, nullable: true}}}
  subject: {type: morphTo, types: [sales.order, sales.quote]}
  comments: {type: morphMany, of: sales.comment, as: subject}
  labels: {type: morphToMany, to: sales.label}
rules:
  - {id: totalPositive, assert: {field: total, op: gt, value: "0"}, message: sales.invoice.error.total, scope: both}
lockWhen: {any: [{field: state, op: in, value: [open, paid]}, {field: locked, op: eq, value: true}]}
indexes:
  - {fields: [number, state], unique: true}
unmanagedColumns:
  - {name: idNumberEnc, reason: dikelola kode tulis tangan}
generate: {crud: true}
`

func TestKitchenSinkIsValid(t *testing.T) {
	mustParse(t, kitchenSink)
}
