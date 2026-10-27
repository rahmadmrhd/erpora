package export

import (
	"archive/zip"
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestWrite_XLSX(t *testing.T) {
	var buf bytes.Buffer
	spec := Spec{Resource: "x.note", Format: XLSX, Filename: "notes", Columns: internalCols()}
	n, err := Write(testCtx(&recAuditor{}), &buf, spec, seq(
		Row{"id": "1", "title": "=1+1", "amount": "-10.50"},
		Row{"id": "2", "title": "a<b&c", "amount": nil},
	))
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	files := map[string]string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
	}
	slices.Sort(names)
	want := []string{"[Content_Types].xml", "_rels/.rels", "xl/_rels/workbook.xml.rels", "xl/workbook.xml", "xl/worksheets/sheet1.xml"}
	if !slices.Equal(names, want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	sheet := files["xl/worksheets/sheet1.xml"]
	for _, must := range []string{`t="inlineStr"`, `<t xml:space="preserve">=1+1</t>`, `a&lt;b&amp;c`, `<t xml:space="preserve">-10.50</t>`, `r="C3"`} {
		if !strings.Contains(sheet, must) {
			t.Errorf("sheet1.xml tidak memuat %q:\n%s", must, sheet)
		}
	}
	if strings.Contains(sheet, "<f>") || strings.Contains(sheet, "<f ") || strings.Contains(sheet, "'=1+1") {
		t.Errorf("sheet1.xml tidak boleh memuat formula atau awalan kutip:\n%s", sheet)
	}
}

func TestXMLText_DropsInvalidChars(t *testing.T) {
	if got := xmlText("a\x00b\x0bc\ufffe" + "d\u2028e"); got != "abcd\u2028e" {
		t.Errorf("xmlText = %q", got)
	}
}

func TestColName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if got := colName(i); got != want {
			t.Errorf("colName(%d) = %q, want %q", i, got, want)
		}
	}
}
