package export

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"

	"erpora/sdk/authz"
	"erpora/sdk/authz/authztest"
	"erpora/sdk/module"
)

// utf8BOM ditulis sebagai byte mentah agar tes tidak bergantung pada konstanta paket.
var utf8BOM = string([]byte{0xEF, 0xBB, 0xBF})

type recAuditor struct {
	mu     sync.Mutex
	events []Event
}

func (a *recAuditor) RecordExport(_ context.Context, e Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
	return nil
}

type dropMasker struct{ drop string }

func (m dropMasker) MaskFields(_ context.Context, _ string, fields map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range fields {
		if k != m.drop {
			out[k] = v
		}
	}
	return out
}

func testCtx(a Auditor) context.Context {
	ctx := authz.WithFieldMasker(context.Background(), authztest.AllowAllFields)
	if a != nil {
		ctx = WithAuditor(ctx, a)
	}
	return ctx
}

func seq(rows ...Row) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		for _, r := range rows {
			if !yield(r, nil) {
				return
			}
		}
	}
}

func internalCols() []Column {
	return []Column{
		{Key: "id", Header: "ID", Classification: module.ClassInternal},
		{Key: "title", Header: "Judul", Classification: module.ClassInternal},
		{Key: "amount", Header: "Jumlah", Classification: module.ClassInternal},
	}
}

func TestWrite_CSV_Golden(t *testing.T) {
	aud := &recAuditor{}
	var buf bytes.Buffer
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols()}
	n, err := Write(testCtx(aud), &buf, spec, seq(
		Row{"id": "1", "title": "=1+1", "amount": "-10.50"},
		Row{"id": "2", "title": "Kopi, \"susu\"", "amount": nil},
	))
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	want := utf8BOM + "ID,Judul,Jumlah\r\n1,'=1+1,-10.50\r\n2,\"Kopi, \"\"susu\"\"\",\r\n"
	if buf.String() != want {
		t.Errorf("csv = %q\nwant  %q", buf.String(), want)
	}
	if len(aud.events) != 1 {
		t.Fatalf("events = %+v", aud.events)
	}
	e := aud.events[0]
	if e.Resource != "x.note" || e.Format != CSV || e.Filename != "notes" || strings.Join(e.Columns, ",") != "id,title,amount" ||
		e.Rows != 2 || e.Classification != module.ClassInternal || e.Err != nil {
		t.Errorf("event = %+v", e)
	}
}

func TestWrite_MaskedColumnsEmpty(t *testing.T) {
	aud := &recAuditor{}
	ctx := WithAuditor(authz.WithFieldMasker(context.Background(), dropMasker{drop: "cost"}), aud)
	var buf bytes.Buffer
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: []Column{
		{Key: "id", Header: "ID", Classification: module.ClassInternal},
		{Key: "cost", Header: "Biaya", Classification: module.ClassConfidential},
	}}
	if _, err := Write(ctx, &buf, spec, seq(Row{"id": "1", "cost": "9"})); err != nil {
		t.Fatal(err)
	}
	if buf.String() != utf8BOM+"ID,Biaya\r\n1,\r\n" {
		t.Errorf("csv = %q", buf.String())
	}
}

func TestWrite_NestedValueRejected(t *testing.T) {
	aud := &recAuditor{}
	var buf bytes.Buffer
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols()}
	_, err := Write(testCtx(aud), &buf, spec, seq(Row{"id": "1", "title": map[string]any{"secret": "x"}}))
	if !errors.Is(err, ErrNonScalarValue) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(buf.String(), "secret") || strings.Contains(err.Error(), "secret") {
		t.Errorf("nilai bersarang bocor: %q / %v", buf.String(), err)
	}
	if len(aud.events) != 1 || aud.events[0].Err == nil {
		t.Errorf("events = %+v", aud.events)
	}
}

func TestValidate_HighlyConfidentialColumnRejected(t *testing.T) {
	aud := &recAuditor{}
	var buf bytes.Buffer
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: []Column{
		{Key: "nik", Header: "NIK", Classification: module.ClassHighlyConfidential},
	}}
	_, err := Write(testCtx(aud), &buf, spec, seq(Row{"nik": "1"}))
	if !errors.Is(err, ErrForbiddenClassification) {
		t.Fatalf("err = %v", err)
	}
	if buf.Len() != 0 || len(aud.events) != 0 {
		t.Errorf("buf=%q events=%+v", buf.String(), aud.events)
	}
}

func TestValidate_ConfidentialRequiresAuditor(t *testing.T) {
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: []Column{
		{Key: "cost", Header: "Biaya", Classification: module.ClassConfidential},
	}}
	var buf bytes.Buffer
	if _, err := Write(testCtx(nil), &buf, spec, seq(Row{"cost": "9"})); !errors.Is(err, ErrNoAuditor) {
		t.Fatalf("err = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("buffer harus kosong: %q", buf.String())
	}
	aud := &recAuditor{}
	if _, err := Write(testCtx(aud), &buf, spec, seq(Row{"cost": "9"})); err != nil {
		t.Fatal(err)
	}
	if len(aud.events) != 1 || aud.events[0].Classification != module.ClassConfidential {
		t.Errorf("events = %+v", aud.events)
	}
}

func TestValidate_NoMaskerFailsClosed(t *testing.T) {
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols()}
	if err := Validate(WithAuditor(context.Background(), &recAuditor{}), spec); !errors.Is(err, ErrNoFieldMasker) {
		t.Errorf("err = %v", err)
	}
}

func TestValidate_InvalidSpec(t *testing.T) {
	base := func() Spec {
		return Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols()}
	}
	cases := map[string]func(*Spec){
		"filename crlf":      func(s *Spec) { s.Filename = "a\r\nb" },
		"filename traversal": func(s *Spec) { s.Filename = "../x" },
		"filename empty":     func(s *Spec) { s.Filename = "" },
		"filename dot":       func(s *Spec) { s.Filename = ".x" },
		"filename long":      func(s *Spec) { s.Filename = strings.Repeat("a", 101) },
		"duplicate column":   func(s *Spec) { s.Columns = append(s.Columns, s.Columns[0]) },
		"empty key":          func(s *Spec) { s.Columns[0].Key = "" },
		"no classification":  func(s *Spec) { s.Columns[0].Classification = "" },
		"bad format":         func(s *Spec) { s.Format = "pdf" },
		"no columns":         func(s *Spec) { s.Columns = nil },
		"no resource":        func(s *Spec) { s.Resource = "" },
		"negative max rows":  func(s *Spec) { s.MaxRows = -1 },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			s := base()
			mut(&s)
			if err := Validate(testCtx(&recAuditor{}), s); !errors.Is(err, ErrInvalidSpec) {
				t.Errorf("err = %v, want ErrInvalidSpec", err)
			}
		})
	}
	if err := Validate(testCtx(&recAuditor{}), base()); err != nil {
		t.Errorf("spec valid ditolak: %v", err)
	}
}

func TestWrite_MaxRows(t *testing.T) {
	aud := &recAuditor{}
	var buf bytes.Buffer
	spec := Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols(), MaxRows: 2}
	n, err := Write(testCtx(aud), &buf, spec, seq(Row{"id": "1"}, Row{"id": "2"}, Row{"id": "3"}))
	if !errors.Is(err, ErrTooManyRows) || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(aud.events) != 1 || aud.events[0].Rows != 2 || !errors.Is(aud.events[0].Err, ErrTooManyRows) {
		t.Errorf("events = %+v", aud.events)
	}
}

// I-1/M-5: panic pada iterator tetap teraudit (defer + recover + re-panic) dengan jumlah baris yang sudah tertulis.
func TestWrite_IteratorPanicStillAuditedAndRepanics(t *testing.T) {
	aud := &recAuditor{}
	rows := func(yield func(Row, error) bool) {
		if !yield(Row{"id": "1"}, nil) {
			return
		}
		panic("iterator meledak")
	}
	var buf bytes.Buffer
	func() {
		defer func() {
			if p := recover(); p != "iterator meledak" {
				t.Errorf("panic harus diteruskan apa adanya, got %v", p)
			}
		}()
		_, _ = Write(testCtx(aud), &buf, Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols()}, rows)
	}()
	if len(aud.events) != 1 || aud.events[0].Err == nil || aud.events[0].Rows != 1 {
		t.Fatalf("panic harus teraudit dengan Rows=1 dan Err: %+v", aud.events)
	}
}

func TestWrite_IteratorErrorRecorded(t *testing.T) {
	aud := &recAuditor{}
	boom := errors.New("boom")
	rows := func(yield func(Row, error) bool) {
		if !yield(Row{"id": "1"}, nil) {
			return
		}
		yield(nil, boom)
	}
	var buf bytes.Buffer
	_, err := Write(testCtx(aud), &buf, Spec{Resource: "x.note", Format: CSV, Filename: "notes", Columns: internalCols()}, rows)
	if !errors.Is(err, boom) || len(aud.events) != 1 || !errors.Is(aud.events[0].Err, boom) {
		t.Errorf("err=%v events=%+v", err, aud.events)
	}
}
