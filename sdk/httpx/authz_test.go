package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"erpora/sdk/authz"
)

type authzBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func writeAuthz(t *testing.T, err error) (handled bool, status int, body authzBody, raw string) {
	t.Helper()
	rec := httptest.NewRecorder()
	handled = WriteAuthzError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), err)
	raw = rec.Body.String()
	if raw != "" {
		if jerr := json.Unmarshal([]byte(raw), &body); jerr != nil {
			t.Fatalf("body bukan JSON: %v (%s)", jerr, raw)
		}
	}
	return handled, rec.Code, body, raw
}

func TestWriteAuthzErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", authz.ErrNotFound, http.StatusNotFound, "not_found"},
		{"forbidden", authz.ErrForbidden, http.StatusForbidden, "permission_denied"},
		{"unauthenticated", authz.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated"},
		{"wrapped not found", fmt.Errorf("memuat catatan: %w", authz.ErrNotFound), http.StatusNotFound, "not_found"},
		{"wrapped forbidden", fmt.Errorf("x: %w", authz.ErrForbidden), http.StatusForbidden, "permission_denied"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			handled, status, body, _ := writeAuthz(t, c.err)
			if !handled || status != c.status || body.Error.Code != c.code || body.Error.Message == "" {
				t.Errorf("handled=%v status=%d body=%+v", handled, status, body)
			}
		})
	}
}

func TestWriteAuthzErrorFieldForbiddenDetails(t *testing.T) {
	err := fmt.Errorf("update: %w", &authz.FieldForbiddenError{Resource: "x.item", Fields: []string{"costPrice", "margin"}})
	handled, status, body, _ := writeAuthz(t, err)
	if !handled || status != http.StatusForbidden || body.Error.Code != "field_forbidden" {
		t.Fatalf("handled=%v status=%d body=%+v", handled, status, body)
	}
	if len(body.Error.Details) != 2 || body.Error.Details[0].Field != "costPrice" || body.Error.Details[1].Field != "margin" {
		t.Fatalf("details = %+v", body.Error.Details)
	}
	for _, d := range body.Error.Details {
		if d.Code != "field_forbidden" || d.Message == "" {
			t.Errorf("detail = %+v", d)
		}
	}
}

func TestWriteAuthzErrorNotFoundIdenticalForMissingAndOutOfScope(t *testing.T) {
	_, _, _, a := writeAuthz(t, authz.ErrNotFound)
	_, _, _, b := writeAuthz(t, fmt.Errorf("record 42 di luar scope: %w", authz.ErrNotFound))
	if a != b {
		t.Errorf("badan 404 tidak boleh membedakan sebab:\n%s\n%s", a, b)
	}
}

func TestWriteAuthzErrorIgnoresOtherErrors(t *testing.T) {
	for name, err := range map[string]error{
		"nil":                 nil,
		"lain":                errors.New("boom"),
		"record tak didukung": authz.ErrUnsupportedRecord,
		"httpx.Error":         NewError(http.StatusTeapot, "teapot", "x"),
	} {
		rec := httptest.NewRecorder()
		if WriteAuthzError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), err) {
			t.Errorf("%s: tidak boleh ditangani", name)
		}
		if rec.Body.Len() != 0 || rec.Code != http.StatusOK {
			t.Errorf("%s: tidak boleh menulis respons (code=%d body=%q)", name, rec.Code, rec.Body.String())
		}
	}
}
