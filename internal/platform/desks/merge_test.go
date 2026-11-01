package desks

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	cases := map[string]Config{
		`{}`:                                 {},
		`{"hidden":[]}`:                      {Hidden: []string{}},
		`{"hidden":["a"],"order":["b","c"]}`: {Hidden: []string{"a"}, Order: []string{"b", "c"}},
		`{"order":["a-b","c1"]}`:             {Order: []string{"a-b", "c1"}},
		` {"hidden":["a"]} `:                 {Hidden: []string{"a"}},
	}
	for in, want := range cases {
		got, err := ParseConfig([]byte(in))
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", in, got, want)
		}
	}
}

func TestParseConfig_Invalid(t *testing.T) {
	long := make([]string, maxConfigIDs+1)
	for i := range long {
		long[i] = `"m` + strconv.Itoa(i) + `"`
	}
	cases := map[string]string{
		"kosong":                "",
		"null":                  `null`,
		"bukan objek":           `[]`,
		"angka":                 `1`,
		"hidden bukan array":    `{"hidden":"a"}`,
		"order bukan array":     `{"order":{"a":1}}`,
		"elemen bukan string":   `{"hidden":[1]}`,
		"properti tak dikenal":  `{"hidden":["a"],"extra":true}`,
		"id berspasi":           `{"hidden":["a b"]}`,
		"id huruf besar":        `{"hidden":["A"]}`,
		"id diawali angka":      `{"hidden":["1a"]}`,
		"id kosong":             `{"hidden":[""]}`,
		"id terlalu panjang":    `{"hidden":["` + strings.Repeat("a", 41) + `"]}`,
		"terlalu banyak id":     `{"hidden":[` + strings.Join(long, ",") + `]}`,
		"data setelah objek":    `{} {}`,
		"json rusak":            `{"hidden":`,
		"kunci ganda":           `{"hidden":["a"],"hidden":["b"]}`,
		"lebih dari batas byte": `{"hidden":["a"],"pad":"` + strings.Repeat("x", maxConfigBytes) + `"}`,
	}
	for name, in := range cases {
		if _, err := ParseConfig([]byte(in)); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
	}
}
