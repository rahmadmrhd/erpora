package apicontract

import (
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateBody(t *testing.T, s *Set, op Operation, status int, body string) error {
	t.Helper()
	sch, err := s.ResponseSchema(op, status)
	if err != nil {
		t.Fatalf("ResponseSchema(%s, %d): %v", op.OperationID, status, err)
	}
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(body))
	if err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	return sch.Validate(v)
}

const (
	goodNote = `{"id":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","title":"a","version":1,"createdAt":"2026-10-21T07:15:00Z"}`
	goodPage = `{"page":1,"pageSize":25,"total":1,"hasMore":false}`
)

func TestResponseSchema_Validates(t *testing.T) {
	s := loadSet(t, "common", "notes")
	list := opOf(t, s, "notes", "listNotes")

	if err := validateBody(t, s, list, 200, `{"data":[`+goodNote+`],"page":`+goodPage+`}`); err != nil {
		t.Errorf("body valid ditolak: %v", err)
	}
	bad := map[string]string{
		"version string":   `{"data":[{"id":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","title":"a","version":"1","createdAt":"2026-10-21T07:15:00Z"}],"page":` + goodPage + `}`,
		"page hilang":      `{"data":[` + goodNote + `]}`,
		"id bukan uuid":    `{"data":[{"id":"abc","title":"a","version":1,"createdAt":"2026-10-21T07:15:00Z"}],"page":` + goodPage + `}`,
		"createdAt buruk":  `{"data":[{"id":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","title":"a","version":1,"createdAt":"kemarin"}],"page":` + goodPage + `}`,
		"data bukan array": `{"data":{},"page":` + goodPage + `}`,
	}
	for name, body := range bad {
		if err := validateBody(t, s, list, 200, body); err == nil {
			t.Errorf("%s: body salah harus ditolak", name)
		}
	}

	// $ref respons komponen (400) dan fallback default (418 -> default).
	errBody := `{"error":{"code":"bad_request","message":"x"}}`
	if err := validateBody(t, s, list, 400, errBody); err != nil {
		t.Errorf("400: %v", err)
	}
	if err := validateBody(t, s, list, 418, errBody); err != nil {
		t.Errorf("418 jatuh ke default: %v", err)
	}
	if err := validateBody(t, s, list, 400, `{"data":[]}`); err == nil {
		t.Error("400 tanpa error harus ditolak")
	}
}

func TestResponseSchema_UnknownStatusAndNoBody(t *testing.T) {
	s := loadSet(t, "common", "notes")
	plain := opOf(t, s, "notes", "getPlain")
	if _, err := s.ResponseSchema(plain, 418); err == nil {
		t.Error("status 418 tanpa default harus error")
	}
	empty := opOf(t, s, "notes", "getEmpty")
	if _, err := s.ResponseSchema(empty, 204); err == nil {
		t.Error("respons tanpa body harus error")
	}
}
