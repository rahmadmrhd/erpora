package export

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSanitizeCell_Table(t *testing.T) {
	cases := map[string]string{
		"=1+1":      "'=1+1",
		"+1":        "'+1",
		"-1+2":      "'-1+2",
		"@SUM(A1)":  "'@SUM(A1)",
		"\t=1":      "'\t=1",
		"\r=1":      "'\r=1",
		"-1250.00":  "-1250.00",
		"-5":        "-5",
		"-5.":       "'-5.",
		"1+1":       "1+1",
		"":          "",
		"Kopi":      "Kopi",
		"-":         "'-",
		"-.5":       "'-.5",
		"-1e5":      "'-1e5",
		"- 5":       "'- 5",
		"0.5":       "0.5",
		"=cmd|' /C": "'=cmd|' /C",
	}
	for in, want := range cases {
		if got := SanitizeCell(in); got != want {
			t.Errorf("SanitizeCell(%q) = %q, want %q", in, got, want)
		}
	}
}

type dec struct{ s string }

func (d dec) String() string { return d.s }

type status string

func TestFormatCell(t *testing.T) {
	ts := time.Date(2026, 10, 27, 7, 0, 0, 0, time.FixedZone("x", 7*3600))
	n := 5
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""}, {"a", "a"}, {true, "true"}, {42, "42"}, {int64(-7), "-7"}, {uint8(3), "3"},
		{1.5, "1.5"}, {float32(2), "2"}, {json.Number("12.30"), "12.30"}, {ts, "2026-10-27T00:00:00Z"},
		{dec{"1250.00"}, "1250.00"}, {status("open"), "open"}, {&n, "5"}, {(*int)(nil), ""},
	}
	for _, tc := range cases {
		got, err := formatCell("k", tc.in)
		if err != nil || got != tc.want {
			t.Errorf("formatCell(%#v) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []any{map[string]any{"a": 1}, []string{"a"}, [2]int{1, 2}, struct{ A int }{1}, []byte("x")} {
		if _, err := formatCell("k", bad); !errors.Is(err, ErrNonScalarValue) {
			t.Errorf("formatCell(%#v) err = %v, want ErrNonScalarValue", bad, err)
		}
	}
}
