package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

const (
	xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	nsMain    = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRel     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsPkgRel  = "http://schemas.openxmlformats.org/package/2006/relationships"
)

// xlsxSink menulis XLSX minimal dengan stdlib: satu sheet "Data", semua sel t="inlineStr", elemen <f>
// tidak pernah ditulis sehingga formula injection mustahil secara konstruksi (karena itu awalan ' tidak
// dipakai di XLSX).
type xlsxSink struct {
	zw    *zip.Writer
	sheet io.Writer
	n     int
}

func newXLSXSink(w io.Writer) (*xlsxSink, error) {
	zw := zip.NewWriter(w)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xmlHeader + `<Relationships xmlns="` + nsPkgRel + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", xmlHeader + `<workbook xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">` +
			`<sheets><sheet name="Data" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", xmlHeader + `<Relationships xmlns="` + nsPkgRel + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`},
	}
	for _, p := range parts {
		fw, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(fw, p.body); err != nil {
			return nil, err
		}
	}
	sheet, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(sheet, xmlHeader+`<worksheet xmlns="`+nsMain+`"><sheetData>`); err != nil {
		return nil, err
	}
	return &xlsxSink{zw: zw, sheet: sheet}, nil
}

func (s *xlsxSink) header(cells []string) error { return s.writeRow(cells) }

func (s *xlsxSink) row(cells []string) error { return s.writeRow(cells) }

func (s *xlsxSink) writeRow(cells []string) error {
	s.n++
	var b strings.Builder
	b.WriteString(`<row r="` + strconv.Itoa(s.n) + `">`)
	for i, c := range cells {
		b.WriteString(`<c r="` + colName(i) + strconv.Itoa(s.n) + `" t="inlineStr"><is><t xml:space="preserve">`)
		b.WriteString(xmlText(c))
		b.WriteString(`</t></is></c>`)
	}
	b.WriteString(`</row>`)
	_, err := io.WriteString(s.sheet, b.String())
	return err
}

func (s *xlsxSink) flush() error { return s.zw.Flush() }

func (s *xlsxSink) close() error {
	if _, err := io.WriteString(s.sheet, `</sheetData></worksheet>`); err != nil {
		return err
	}
	return s.zw.Close()
}

// colName mengubah indeks kolom 0-based menjadi huruf kolom spreadsheet (A, B, ..., Z, AA, ...).
func colName(i int) string {
	var out []byte
	for i++; i > 0; i = (i - 1) / 26 {
		out = append([]byte{byte('A' + (i-1)%26)}, out...)
	}
	return string(out)
}

// xmlText membuang karakter yang tidak valid di XML 1.0 lalu meng-escape sisanya.
func xmlText(s string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == 0x9 || r == 0xA || r == 0xD,
			r >= 0x20 && r <= 0xD7FF,
			r >= 0xE000 && r <= 0xFFFD,
			r >= 0x10000 && r <= 0x10FFFF:
			return r
		}
		return -1
	}, s)
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(clean)) // menulis ke bytes.Buffer tidak pernah gagal
	return buf.String()
}
