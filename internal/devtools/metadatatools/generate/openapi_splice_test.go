package generate

import (
	"errors"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func miniSchema(name, desc string) namedSchema {
	s := yMap()
	yPut(s, "type", yStr("object"))
	yPut(s, "description", yStr(desc))
	yPut(s, generatedMarkerKey, yBool(true))
	return namedSchema{Name: name, Node: s}
}

func spliceOK(t *testing.T, src string, want ...namedSchema) string {
	t.Helper()
	out, err := SpliceSchemas([]byte(src), want)
	if err != nil {
		t.Fatal(err)
	}
	// hasil harus YAML sah dan idempoten
	var v any
	if err := yaml.Unmarshal(out, &v); err != nil {
		t.Fatalf("hasil bukan YAML sah: %v\n%s", err, out)
	}
	again, err := SpliceSchemas(out, want)
	if err != nil || string(again) != string(out) {
		t.Fatalf("tidak idempoten (err=%v):\n%s\n---\n%s", err, out, again)
	}
	return string(out)
}

// inOrder melaporkan apakah semua bagian muncul di s dengan urutan seperti diberikan.
func inOrder(s string, parts ...string) bool {
	pos := -1
	for _, p := range parts {
		i := strings.Index(s, p)
		if i <= pos {
			return false
		}
		pos = i
	}
	return true
}

const baseDoc = "openapi: 3.1.0\ninfo:\n  title: t\npaths: {}\n"

func TestSpliceCreatesComponents(t *testing.T) {
	out := spliceOK(t, baseDoc, miniSchema("A", "a"))
	if !strings.Contains(out, "components:\n  schemas:\n    A:\n      type: object") {
		t.Errorf("\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("harus diakhiri baris baru")
	}
}

func TestSpliceIntoEmptySchemasAndNull(t *testing.T) {
	for _, doc := range []string{baseDoc + "components:\n  schemas: {}\n", baseDoc + "components:\n  schemas:\n"} {
		out := spliceOK(t, doc, miniSchema("A", "a"))
		if !strings.Contains(out, "  schemas:\n    A:") {
			t.Errorf("\n%s", out)
		}
	}
}

func TestSpliceAddsSchemasWhenComponentsHasOtherKeys(t *testing.T) {
	doc := baseDoc + "components:\n  securitySchemes:\n    x: {type: http}\n"
	out := spliceOK(t, doc, miniSchema("A", "a"))
	if !strings.Contains(out, "securitySchemes:\n    x: {type: http}\n  schemas:\n    A:") {
		t.Errorf("\n%s", out)
	}
}

func TestSplicePreservesHandWrittenAndComments(t *testing.T) {
	doc := `openapi: 3.1.0
paths: {}
# komentar atas components
components:
  schemas:
    # komentar Hand
    Hand:
      type: object   # inline
      properties:
        x: { type: string }

    Other: { type: string }
  responses: {}
# komentar penutup
`
	out := spliceOK(t, doc, miniSchema("A", "a"), miniSchema("B", "b"))
	for _, want := range []string{"# komentar atas components", "# komentar Hand", "type: object   # inline", "Other: { type: string }", "  responses: {}", "# komentar penutup"} {
		if !strings.Contains(out, want) {
			t.Errorf("hilang %q:\n%s", want, out)
		}
	}
	if !inOrder(out, "Other:", "    A:", "    B:", "  responses:") {
		t.Errorf("urutan salah:\n%s", out)
	}
}

func TestSpliceReplacesGeneratedInPlace(t *testing.T) {
	doc := spliceOK(t, baseDoc, miniSchema("A", "lama"), miniSchema("B", "b"))
	doc = strings.Replace(doc, "    B:", "    Hand:\n      type: string\n    B:", 1) // sisip tulis tangan di tengah
	out := spliceOK(t, doc, miniSchema("A", "baru"), miniSchema("B", "b"))
	if strings.Contains(out, "lama") || !strings.Contains(out, "baru") {
		t.Errorf("\n%s", out)
	}
	if !inOrder(out, "    A:", "    Hand:", "    B:") {
		t.Errorf("posisi harus tetap:\n%s", out)
	}
}

func TestSpliceRemovesOrphanGeneratedOnly(t *testing.T) {
	doc := spliceOK(t, baseDoc, miniSchema("A", "a"), miniSchema("Gone", "g"))
	doc = strings.Replace(doc, "    Gone:", "    Hand:\n      type: string\n    Gone:", 1)
	out := spliceOK(t, doc, miniSchema("A", "a"))
	if strings.Contains(out, "Gone") || !strings.Contains(out, "Hand:") {
		t.Errorf("\n%s", out)
	}
	out = spliceOK(t, doc) // tanpa skema yang diinginkan: semua bangkitan dibuang
	if strings.Contains(out, "    A:") || !strings.Contains(out, "Hand:") {
		t.Errorf("\n%s", out)
	}
}

func TestSpliceConflictWithHandWritten(t *testing.T) {
	doc := baseDoc + "components:\n  schemas:\n    A:\n      type: string\n"
	before := doc
	_, err := SpliceSchemas([]byte(doc), []namedSchema{miniSchema("A", "a")})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if doc != before {
		t.Error("masukan tidak boleh berubah")
	}
}

func TestSpliceCRLFPreserved(t *testing.T) {
	doc := strings.ReplaceAll(baseDoc, "\n", "\r\n")
	out, err := SpliceSchemas([]byte(doc), []namedSchema{miniSchema("A", "a")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(string(out), "\r\n", ""), "\n") {
		t.Error("campuran akhir baris")
	}
}

func TestSpliceUnsupportedShapes(t *testing.T) {
	for _, doc := range []string{
		baseDoc + "components: {schemas: {X: {type: string}}}\n",
		baseDoc + "components:\n  schemas: {X: {type: string}}\n",
	} {
		if _, err := SpliceSchemas([]byte(doc), []namedSchema{miniSchema("A", "a")}); err == nil {
			t.Errorf("bentuk flow harus ditolak:\n%s", doc)
		}
	}
	if _, err := SpliceSchemas([]byte("- a\n- b\n"), []namedSchema{miniSchema("A", "a")}); err == nil {
		t.Error("dokumen bukan objek harus ditolak")
	}
}

func TestSpliceMultilineDescriptionRoundTrip(t *testing.T) {
	desc := "baris satu\nbaris dua: \"kutip\" # bukan komentar\n  menjorok\n- bukan list\n"
	out := spliceOK(t, baseDoc, miniSchema("A", desc))
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Description string `yaml:"description"`
			}
		}
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Components.Schemas["A"].Description != desc {
		t.Errorf("deskripsi berubah: %q", doc.Components.Schemas["A"].Description)
	}
}

func TestGeneratedSchemaNames(t *testing.T) {
	doc := spliceOK(t, baseDoc, miniSchema("B", "b"), miniSchema("A", "a"))
	names, err := GeneratedSchemaNames([]byte(doc))
	if err != nil || strings.Join(names, ",") != "A,B" {
		t.Errorf("names=%v err=%v", names, err)
	}
}
