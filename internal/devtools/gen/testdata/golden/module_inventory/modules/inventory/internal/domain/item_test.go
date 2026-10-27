package domain

import (
	"strings"
	"testing"
)

func TestNewItem(t *testing.T) {
	cases := []struct {
		name    string
		title   string
		wantErr bool
	}{
		{"kosong", "", true},
		{"hanya spasi", "   ", true},
		{"201 karakter", strings.Repeat("a", 201), true},
		{"1 karakter", "a", false},
		{"200 karakter", strings.Repeat("a", 200), false},
		{"200 rune multibyte", strings.Repeat("é", 200), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := NewItem(tc.title)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && n.Title != tc.title {
				t.Errorf("Title = %q", n.Title)
			}
		})
	}
}
