package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeNotes struct {
	n   int64
	err error
}

func (f fakeNotes) Count(context.Context) (int64, error) { return f.n, f.err }

func TestSummary_OK(t *testing.T) {
	rec := httptest.NewRecorder()
	New(fakeNotes{n: 3}).Summary(rec, httptest.NewRequest(http.MethodGet, "/summary", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Data struct{ NoteCount int64 }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Data.NoteCount != 3 {
		t.Fatalf("body = %s err = %v", rec.Body.String(), err)
	}
	if !strings.Contains(rec.Body.String(), `"noteCount":3`) {
		t.Errorf("nama field: %s", rec.Body.String())
	}
}

func TestSummary_ContractErrorIs500WithoutDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	New(fakeNotes{err: errors.New("SELECT rahasia")}).Summary(rec, httptest.NewRequest(http.MethodGet, "/summary", nil))
	var body struct{ Error struct{ Code string } }
	if rec.Code != 500 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error.Code != "internal_error" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("detail bocor: %s", rec.Body.String())
	}
}

// api-guidelines §8: detail 5xx ada di log (bukan di respons).
func TestSummary_ContractErrorIsLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	rec := httptest.NewRecorder()
	New(fakeNotes{err: errors.New("SELECT rahasia")}).Summary(rec, httptest.NewRequest(http.MethodGet, "/summary", nil))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "rahasia") {
		t.Fatalf("respons harus generik: %d %s", rec.Code, rec.Body.String())
	}
	if out := buf.String(); !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "SELECT rahasia") {
		t.Fatalf("error tidak tercatat di log: %q", out)
	}
}
