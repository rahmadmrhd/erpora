package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type lineDTO struct {
	ItemID   string `json:"itemId"`
	Quantity string `json:"quantity"`
}

type createDTO struct {
	Title    string            `json:"title"`
	Tags     []string          `json:"tags"`
	Lines    []lineDTO         `json:"lines"`
	Meta     map[string]string `json:"meta"`
	Any      map[string]any    `json:"any"`
	Internal string            `json:"-"`
}

func newReq(contentType, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return r
}

func TestDecodeJSON_Table(t *testing.T) {
	const js = "application/json"
	tags := func(n int) string {
		return `{"tags":[` + strings.TrimSuffix(strings.Repeat(`"a",`, n), ",") + `]}`
	}
	nested := func(n int) string {
		return `{"any":` + strings.Repeat(`{"a":`, n) + `1` + strings.Repeat("}", n) + `}`
	}
	pad := func(base string, size int) string {
		return base + strings.Repeat(" ", size-len(base))
	}

	cases := []struct {
		name, ct, body string
		ctxLimit       int64
		status         int
		code, detail   string
		field          string
	}{
		{name: "ok", ct: js, body: `{"title":"a","lines":[{"itemId":"x","quantity":"1.5"}]}`},
		{name: "ok_charset", ct: "application/json; charset=UTF-8", body: `{"title":"a"}`},
		{name: "ok_trailing_ws", ct: js, body: `{"title":"a"}` + "\n\t "},
		{name: "ct_text", ct: "text/plain", body: `{"title":"a"}`, status: 400, code: "bad_request", detail: "unsupported_media_type"},
		{name: "ct_missing", ct: "", body: `{"title":"a"}`, status: 400, code: "bad_request", detail: "unsupported_media_type"},
		{name: "ct_latin1", ct: "application/json; charset=iso-8859-1", body: `{"title":"a"}`, status: 400, code: "bad_request", detail: "unsupported_media_type"},
		{name: "empty", ct: js, body: ``, status: 400, code: "bad_request", detail: "body_required"},
		{name: "empty_ws", ct: js, body: `  `, status: 400, code: "bad_request", detail: "body_required"},
		{name: "malformed", ct: js, body: `{"title":`, status: 400, code: "bad_request", detail: "malformed_json"},
		{name: "not_object_array", ct: js, body: `[1]`, status: 400, code: "bad_request", detail: "expected_object"},
		{name: "not_object_null", ct: js, body: `null`, status: 400, code: "bad_request", detail: "expected_object"},
		{name: "trailing_object", ct: js, body: `{"title":"a"} {"title":"b"}`, status: 400, code: "bad_request", detail: "trailing_data"},
		{name: "trailing_garbage", ct: js, body: `{"title":"a"}x`, status: 400, code: "bad_request", detail: "trailing_data"},
		{name: "unknown", ct: js, body: `{"title":"a","admin":true}`, status: 422, code: "validation_failed", detail: "unknown_field", field: "admin"},
		{name: "case_variant", ct: js, body: `{"Title":"a"}`, status: 422, code: "validation_failed", detail: "unknown_field", field: "Title"},
		{name: "dash_tag", ct: js, body: `{"Internal":"x"}`, status: 422, code: "validation_failed", detail: "unknown_field", field: "Internal"},
		{name: "nested_unknown", ct: js, body: `{"lines":[{"itemId":"x"},{"itemId":"y","price":"1"}]}`, status: 422, code: "validation_failed", detail: "unknown_field", field: "lines[1].price"},
		{name: "dup", ct: js, body: `{"title":"a","title":"b"}`, status: 400, code: "bad_request", detail: "duplicate_key", field: "title"},
		{name: "dup_map", ct: js, body: `{"meta":{"k":"1","k":"2"}}`, status: 400, code: "bad_request", detail: "duplicate_key", field: "meta.k"},
		{name: "type", ct: js, body: `{"title":1}`, status: 422, code: "validation_failed", detail: "invalid_type", field: "title"},
		{name: "type_nested", ct: js, body: `{"lines":[{"quantity":1.5}]}`, status: 422, code: "validation_failed", detail: "invalid_type", field: "lines[0].quantity"},
		{name: "depth_ok", ct: js, body: nested(31)},
		{name: "depth_bad", ct: js, body: nested(32), status: 400, code: "bad_request", detail: "too_deep"},
		{name: "array_ok", ct: js, body: tags(1000)},
		{name: "array_bad", ct: js, body: tags(1001), status: 400, code: "bad_request", detail: "array_too_long", field: "tags"},
		{name: "too_large_ctx", ct: js, body: pad(`{"title":"a"}`, 65), ctxLimit: 64, status: 400, code: "bad_request", detail: "body_too_large"},
		{name: "too_large_default", ct: js, body: `{"title":"` + strings.Repeat("a", 1<<20) + `"}`, status: 400, code: "bad_request", detail: "body_too_large"},
		{name: "limit_exact", ct: js, body: pad(`{"title":"a"}`, 1<<20)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newReq(tc.ct, tc.body)
			if tc.ctxLimit > 0 {
				r = r.WithContext(WithBodyLimit(r.Context(), tc.ctxLimit))
			}
			_, err := DecodeJSON[createDTO](httptest.NewRecorder(), r)
			if tc.status == 0 {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			var he *Error
			if !errors.As(err, &he) {
				t.Fatalf("err = %v, want *Error", err)
			}
			if he.Status != tc.status || he.Code != tc.code {
				t.Errorf("status/code = %d %s, want %d %s", he.Status, he.Code, tc.status, tc.code)
			}
			if len(he.Details) == 0 {
				t.Fatalf("details kosong")
			}
			if he.Details[0].Code != tc.detail {
				t.Errorf("details[0].code = %q, want %q", he.Details[0].Code, tc.detail)
			}
			if tc.field != "" && he.Details[0].Field != tc.field {
				t.Errorf("details[0].field = %q, want %q", he.Details[0].Field, tc.field)
			}
		})
	}
}

type innerDTO struct {
	X string `json:"x"`
}

type strTagDTO struct {
	N        int64    `json:"n,string"`
	Tagged   innerDTO `json:"nested"`
	innerDTO          // tanpa tag: field dipromosikan ke tingkat atas
}

// namedEmbedDTO: struct embedded BERTAG diperlakukan sebagai field bernama (tidak dipromosikan).
type namedEmbedDTO struct {
	innerDTO `json:"inner"`
}

// M-9: tag ",string" dan struct embedded bertag/tanpa tag.
func TestDecodeJSON_StringTagAndEmbedded(t *testing.T) {
	const js = "application/json"
	cases := []struct {
		name, body string
		status     int
		detail     string
	}{
		{name: "string_tag_quoted_ok", body: `{"n":"5"}`},
		{name: "string_tag_unquoted_number", body: `{"n":5}`, status: 422, detail: "invalid_type"},
		{name: "string_tag_non_numeric", body: `{"n":"abc"}`, status: 422, detail: "invalid_type"},
		{name: "embedded_untagged_promoted_ok", body: `{"x":"a"}`},
		{name: "tagged_struct_ok", body: `{"nested":{"x":"a"}}`},
		{name: "tagged_struct_unknown_nested", body: `{"nested":{"y":"a"}}`, status: 422, detail: "unknown_field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeJSON[strTagDTO](httptest.NewRecorder(), newReq(js, tc.body))
			assertDecodeResult(t, err, tc.status, tc.detail)
		})
	}
	t.Run("named_embedded_not_promoted", func(t *testing.T) {
		if _, err := DecodeJSON[namedEmbedDTO](httptest.NewRecorder(), newReq(js, `{"inner":{"x":"a"}}`)); err != nil {
			t.Fatalf("err = %v", err)
		}
		_, err := DecodeJSON[namedEmbedDTO](httptest.NewRecorder(), newReq(js, `{"x":"a"}`))
		assertDecodeResult(t, err, 422, "unknown_field")
	})
}

func assertDecodeResult(t *testing.T, err error, status int, detail string) {
	t.Helper()
	if status == 0 {
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		return
	}
	var he *Error
	if !errors.As(err, &he) || he.Status != status || len(he.Details) == 0 || he.Details[0].Code != detail {
		t.Fatalf("err = %v (%+v), want %d/%s", err, he, status, detail)
	}
}

func TestDecodeJSON_Values(t *testing.T) {
	got, err := DecodeJSON[createDTO](httptest.NewRecorder(),
		newReq("application/json", `{"title":"a","lines":[{"itemId":"x","quantity":"1.5"}]}`))
	if err != nil || got.Title != "a" || got.Lines[0].Quantity != "1.5" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestDecodeJSON_AllowEmptyBody(t *testing.T) {
	got, err := DecodeJSON[createDTO](httptest.NewRecorder(), newReq("", ""), AllowEmptyBody())
	if err != nil || got.Title != "" || got.Lines != nil {
		t.Errorf("got %+v err %v", got, err)
	}
	_, err = DecodeJSON[createDTO](httptest.NewRecorder(), newReq("", `{}`), AllowEmptyBody())
	var he *Error
	if !errors.As(err, &he) || he.Details[0].Code != "unsupported_media_type" {
		t.Errorf("err = %v", err)
	}
}

func TestDecodeJSON_NonStructPanics(t *testing.T) {
	defer func() {
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, "DTO struct") {
			t.Errorf("panic = %v", r)
		}
	}()
	_, _ = DecodeJSON[map[string]any](httptest.NewRecorder(), newReq("application/json", `{}`))
}
