package export

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/sdk/module"
)

func serveSpec(format Format, class module.Classification) Spec {
	return Spec{Resource: "x.note", Format: format, Filename: "notes", Columns: []Column{
		{Key: "id", Header: "ID", Classification: class},
	}}
}

func TestServe_Headers(t *testing.T) {
	req := httptest.NewRequest("GET", "/export", nil)
	req = req.WithContext(testCtx(&recAuditor{}))

	t.Run("csv internal", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if err := Serve(rec, req, serveSpec(CSV, module.ClassInternal), seq(Row{"id": "1"})); err != nil {
			t.Fatal(err)
		}
		h := rec.Header()
		if h.Get("Content-Type") != "text/csv; charset=utf-8" ||
			h.Get("Content-Disposition") != `attachment; filename="notes.csv"; filename*=UTF-8''notes.csv` ||
			h.Get("Cache-Control") != "" {
			t.Errorf("headers = %v", h)
		}
		if !strings.Contains(rec.Body.String(), "ID\r\n1\r\n") {
			t.Errorf("body = %q", rec.Body.String())
		}
	})
	t.Run("csv confidential no-store", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if err := Serve(rec, req, serveSpec(CSV, module.ClassConfidential), seq(Row{"id": "1"})); err != nil {
			t.Fatal(err)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
		}
	})
	t.Run("xlsx", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if err := Serve(rec, req, serveSpec(XLSX, module.ClassInternal), seq(Row{"id": "1"})); err != nil {
			t.Fatal(err)
		}
		if rec.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
			!strings.Contains(rec.Header().Get("Content-Disposition"), `filename="notes.xlsx"`) {
			t.Errorf("headers = %v", rec.Header())
		}
	})
	t.Run("invalid filename", func(t *testing.T) {
		rec := httptest.NewRecorder()
		spec := serveSpec(CSV, module.ClassInternal)
		spec.Filename = "a\r\nb"
		if err := Serve(rec, req, spec, seq(Row{"id": "1"})); !errors.Is(err, ErrInvalidSpec) {
			t.Fatalf("err = %v", err)
		}
		if rec.Body.Len() != 0 || rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("tidak boleh ada output: %q %v", rec.Body.String(), rec.Header())
		}
	})
}

func TestServe_MidStreamErrorAborts(t *testing.T) {
	aud := &recAuditor{}
	req := httptest.NewRequest("GET", "/export", nil).WithContext(testCtx(aud))
	rows := func(yield func(Row, error) bool) {
		if !yield(Row{"id": "1"}, nil) {
			return
		}
		yield(nil, errors.New("db putus"))
	}
	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Errorf("recover = %v, want http.ErrAbortHandler", r)
		}
		if len(aud.events) != 1 || aud.events[0].Err == nil {
			t.Errorf("events = %+v", aud.events)
		}
	}()
	_ = Serve(httptest.NewRecorder(), req, serveSpec(CSV, module.ClassInternal), rows)
	t.Error("Serve harus panic")
}
