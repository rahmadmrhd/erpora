package compatibility

import "testing"

func TestSatisfies(t *testing.T) {
	cases := []struct {
		version, constraint string
		want                bool
		wantErr             bool
	}{
		{"1.2.3", "^1.0", true, false},
		{"2.0.0", "^1.0", false, false},
		{"1.3.0-rc.1", "^1.0", false, false},
		{"1.3.0-rc.1", ">=1.3.0-rc.0", true, false},
		{"1.2.3", "~1.2", true, false},
		{"1.5.0", ">=1.2 <2.0", true, false},
		{"3.0.0", "^1.0 || ^3.0", true, false},
		{"x", "^1", false, true},
		{"1.0.0", "^^1", false, true},
	}
	for _, tc := range cases {
		got, err := Satisfies(tc.version, tc.constraint)
		if (err != nil) != tc.wantErr {
			t.Errorf("Satisfies(%q, %q) err = %v, wantErr %v", tc.version, tc.constraint, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("Satisfies(%q, %q) = %v, want %v", tc.version, tc.constraint, got, tc.want)
		}
	}
}
