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
	"time"

	"erpora/modules/example/internal/domain"
)

type fakeLister struct {
	items []domain.Note
	total int64
	err   error
	page  int
	size  int
}

func (f *fakeLister) ListNotes(_ context.Context, page, size int) ([]domain.Note, int64, error) {
	f.page, f.size = page, size
	return f.items, f.total, f.err
}

func serve(h http.HandlerFunc, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct{ Error struct{ Code string } }
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b.Error.Code
}

func TestNotes_InvalidParams(t *testing.T) {
	for _, q := range []string{"page=0", "page=abc", "pageSize=201", "pageSize=0", "page=-1"} {
		rec := serve(New(&fakeLister{}).ListNotes, "/notes?"+q)
		if rec.Code != 400 || errCode(t, rec) != "bad_request" {
			t.Errorf("%s: %d %s", q, rec.Code, rec.Body.String())
		}
	}
}

func TestNotes_DefaultsAndEnvelope(t *testing.T) {
	ts := time.Date(2026, 10, 22, 7, 0, 0, 0, time.FixedZone("x", 7*3600))
	lister := &fakeLister{items: []domain.Note{{ID: "0193a6c2-0000-7000-8000-000000000001", Title: "Halo", Version: 3, CreatedAt: ts, UpdatedAt: ts.Add(time.Hour)}}, total: 60}
	rec := serve(New(lister).ListNotes, "/notes")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if lister.page != 1 || lister.size != 25 {
		t.Errorf("default page=%d size=%d", lister.page, lister.size)
	}
	var body struct {
		Data []map[string]any `json:"data"`
		Page map[string]any   `json:"page"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0]["title"] != "Halo" || body.Data[0]["createdAt"] != "2026-10-22T00:00:00Z" ||
		body.Data[0]["updatedAt"] != "2026-10-22T01:00:00Z" || body.Data[0]["version"] != float64(3) {
		t.Errorf("data = %+v", body.Data)
	}
	if body.Page["page"] != float64(1) || body.Page["pageSize"] != float64(25) || body.Page["total"] != float64(60) || body.Page["hasMore"] != true {
		t.Errorf("page = %+v", body.Page)
	}
}

func TestNotes_EmptyListIsArray(t *testing.T) {
	rec := serve(New(&fakeLister{}).ListNotes, "/notes")
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if string(raw["data"]) != "[]" {
		t.Errorf("data = %s", raw["data"])
	}
}

func TestNotes_InternalErrorHidesDetail(t *testing.T) {
	rec := serve(New(&fakeLister{err: errors.New("SELECT rahasia")}).ListNotes, "/notes")
	if rec.Code != 500 || errCode(t, rec) != "internal_error" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("detail internal bocor: %s", rec.Body.String())
	}
}

func TestPing(t *testing.T) {
	rec := serve(New(&fakeLister{}).Ping, "/ping")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"data":{"pong":true}}` {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

// api-guidelines §8: detail 5xx ada di log (bukan di respons).
func TestNotes_InternalErrorIsLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	rec := serve(New(&fakeLister{err: errors.New("SELECT rahasia")}).ListNotes, "/notes")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "rahasia") {
		t.Fatalf("respons harus generik: %d %s", rec.Code, rec.Body.String())
	}
	if out := buf.String(); !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "SELECT rahasia") {
		t.Fatalf("error tidak tercatat di log: %q", out)
	}
}

func TestListNotes_BadPageEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-Id", "req-1")
	New(&fakeLister{}).ListNotes(rec, httptest.NewRequest(http.MethodGet, "/notes?page=0", nil))
	var b struct {
		Error struct {
			Code      string
			RequestID string `json:"requestId"`
			Details   []struct{ Field, Code string }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 400 || b.Error.Code != "bad_request" || b.Error.RequestID != "req-1" ||
		len(b.Error.Details) != 1 || b.Error.Details[0].Field != "page" || b.Error.Details[0].Code != "invalid" {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestPing_BodyUnchanged(t *testing.T) {
	rec := serve(New(&fakeLister{}).Ping, "/ping")
	if rec.Body.String() != `{"data":{"pong":true}}`+"\n" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestListNotes_RepoErrorIs500Generic(t *testing.T) {
	rec := serve(New(&fakeLister{err: errors.New("SELECT rahasia")}).ListNotes, "/notes")
	if rec.Code != 500 || errCode(t, rec) != "internal_error" || strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
