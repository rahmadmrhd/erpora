package dbtools

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestConfirm_Table(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"nama cocok", "erp\n", false},
		{"nama cocok CRLF", "erp\r\n", false},
		{"nama cocok tanpa newline akhir", "erp", false},
		{"huruf besar beda", "ERP\n", true},
		{"nama lain", "salah\n", true},
		{"kosong", "\n", true},
		{"EOF", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Confirm(strings.NewReader(tc.input), &out, "erp")
			if tc.wantErr && !errors.Is(err, ErrAborted) {
				t.Fatalf("err = %v, want ErrAborted", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if !strings.Contains(out.String(), "erp") {
				t.Errorf("prompt harus menyebut nama database: %q", out.String())
			}
		})
	}
}
