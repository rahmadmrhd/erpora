package httprt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/sdk/httpx"
)

type body struct {
	Name *string `json:"name"`
	Qty  *int64  `json:"qty"`
}

func req(payload string) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), r
}

func asHTTP(t *testing.T, err error) *httpx.Error {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) {
		t.Fatalf("bukan *httpx.Error: %v", err)
	}
	return he
}

func detailMap(he *httpx.Error) map[string]string {
	m := map[string]string{}
	for _, d := range he.Details {
		m[d.Field] = d.Code
	}
	return m
}

func TestDecodePresence(t *testing.T) {
	w, r := req(`{"name":null,"qty":3}`)
	v, present, err := decodeBody[body](w, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !present["name"] || !present["qty"] || len(present) != 2 || v.Name != nil || *v.Qty != 3 {
		t.Errorf("v=%+v present=%v", v, present)
	}
	w, r = req(`{}`)
	_, present, err = decodeBody[body](w, r, nil)
	if err != nil || len(present) != 0 {
		t.Errorf("err=%v present=%v", err, present)
	}
}

func TestDecodeUnknownAndReadOnly(t *testing.T) {
	w, r := req(`{"name":"a","id":"x","bogus":1}`)
	_, _, err := decodeBody[body](w, r, map[string]struct{}{"id": {}})
	he := asHTTP(t, err)
	d := detailMap(he)
	if he.Status != 422 || he.Code != "validation_failed" || d["id"] != "read_only" || d["bogus"] != "unknown_field" {
		t.Errorf("status=%d code=%s details=%v", he.Status, he.Code, d)
	}
}

func TestDecodeStructuralErrors(t *testing.T) {
	for payload, want := range map[string]string{
		`{"qty":"x"}`:             "invalid_type",
		`{"name":"a","name":"b"}`: "duplicate_key",
		`[1]`:                     "expected_object",
		`{"name":`:                "malformed_json",
	} {
		w, r := req(payload)
		_, _, err := decodeBody[body](w, r, nil)
		he := asHTTP(t, err)
		found := false
		for _, d := range he.Details {
			found = found || d.Code == want
		}
		if !found {
			t.Errorf("%s: details %v tidak memuat %s", payload, he.Details, want)
		}
	}
}

func TestDecodeBodyTooLarge(t *testing.T) {
	w, r := req(`{"name":"` + strings.Repeat("a", 64) + `"}`)
	r = r.WithContext(httpx.WithBodyLimit(r.Context(), 16))
	_, _, err := decodeBody[body](w, r, nil)
	he := asHTTP(t, err)
	if he.Status != 400 || detailMap(he)[""] != "body_too_large" {
		t.Errorf("status=%d details=%v", he.Status, he.Details)
	}
}

func TestViolationsError(t *testing.T) {
	if violationsError[violation](context.Background(), nil) != nil {
		t.Error("tanpa pelanggaran harus nil")
	}
	he := asHTTP(t, violationsError(context.Background(), []violation{
		{Field: "a", Code: "required"}, {Field: "b", Code: "rule_x", MessageKey: "m.k", Business: true},
	}))
	d := detailMap(he)
	if he.Status != 422 || he.Code != "validation_failed" || len(d) != 1 || d["a"] != "required" {
		t.Errorf("struktur dulu: %d %s %v", he.Status, he.Code, d)
	}
	he = asHTTP(t, violationsError(context.Background(), []violation{{Field: "b", Code: "rule_x", MessageKey: "m.k", Business: true}}))
	d = detailMap(he)
	if he.Status != 422 || he.Code != "business_rule_violated" || d["b"] != "rule_x" {
		t.Errorf("bisnis: %d %s %v", he.Status, he.Code, d)
	}
	var many []violation
	for range 100 {
		many = append(many, violation{Field: "f", Code: "required"})
	}
	he = asHTTP(t, violationsError(context.Background(), many))
	if len(he.Details) != maxViolations {
		t.Errorf("details = %d", len(he.Details))
	}
}

func TestMessagesAreStaticAndEnvelopeSerializes(t *testing.T) {
	for _, c := range []string{"required", "invalid_type", "read_only", "too_long", "decimal_scale_exceeded", "zzz"} {
		if structuralMessage(c) == "" {
			t.Errorf("pesan kosong untuk %s", c)
		}
	}
	err := violationsError(context.Background(), []violation{{Field: "a", Code: "required"}})
	rec := httptest.NewRecorder()
	httpx.RespondError(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
	var env struct{ Error struct{ Code string } }
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil || env.Error.Code != "validation_failed" || rec.Code != 422 {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestVal(t *testing.T) {
	s := "x"
	if val[string](nil) != nil || val(&s) != "x" {
		t.Error("val")
	}
}

func TestClone(t *testing.T) {
	if clone[string](nil) != nil {
		t.Error("nil tetap nil")
	}
	s := "x"
	c := clone(&s)
	*c = "y"
	if s != "x" {
		t.Error("salinan harus independen")
	}
}
