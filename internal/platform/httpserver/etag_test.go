package httpserver

import "testing"

// If-None-Match memakai perbandingan LEMAH (RFC 9110 section 8.8.3.2): awalan W/ diabaikan di kedua sisi.
func TestETagMatches_WeakComparison(t *testing.T) {
	cases := []struct {
		header []string
		etag   string
		want   bool
	}{
		{[]string{`"x"`}, `"x"`, true},
		{[]string{`W/"x"`}, `W/"x"`, true},
		{[]string{`"x"`}, `W/"x"`, true}, // klien/perantara membuang W/
		{[]string{`W/"x"`}, `"x"`, true}, // ETag kuat dibandingkan dengan tag lemah klien
		{[]string{`"y", "x"`}, `W/"x"`, true},
		{[]string{`W/"y", W/"x"`}, `"x"`, true},
		{[]string{`"y"`}, `W/"x"`, false},
		{[]string{`W/"y"`}, `"x"`, false},
		{[]string{`*`}, `"x"`, true},
		{[]string{``}, `"x"`, false},
		{[]string{`x`}, `"x"`, false},
		{nil, `"x"`, false},
		{[]string{`"y"`, `"x"`}, `W/"x"`, true}, // beberapa header
	}
	for _, c := range cases {
		if got := etagMatches(c.header, c.etag); got != c.want {
			t.Errorf("etagMatches(%q, %s) = %v, want %v", c.header, c.etag, got, c.want)
		}
	}
}
