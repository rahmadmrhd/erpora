package export

import (
	"encoding/csv"
	"io"
)

// bom adalah penanda urutan byte UTF-8 agar Excel membaca CSV sebagai UTF-8.
const bom = string(rune(0xFEFF))

// csvSink menulis CSV: BOM UTF-8, akhir baris CRLF, setiap sel melewati SanitizeCell.
type csvSink struct {
	cw *csv.Writer
}

func newCSVSink(w io.Writer) (*csvSink, error) {
	if _, err := io.WriteString(w, bom); err != nil {
		return nil, err
	}
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	return &csvSink{cw: cw}, nil
}

func (s *csvSink) header(cells []string) error { return s.row(cells) }

func (s *csvSink) row(cells []string) error {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = SanitizeCell(c)
	}
	return s.cw.Write(out)
}

func (s *csvSink) flush() error {
	s.cw.Flush()
	return s.cw.Error()
}

func (s *csvSink) close() error { return s.flush() }
