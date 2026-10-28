package export

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// SanitizeCell memberi awalan ' pada sel yang diawali =, +, -, @, tab, atau CR agar tidak ditafsirkan
// sebagai formula oleh spreadsheet (security-guidelines §3.2). String desimal murni (^-?[0-9]+(\.[0-9]+)?$,
// mis. "-1250.00") dikecualikan karena tidak dapat menjadi formula dan uang/kuantitas di sistem ini berupa
// string desimal.
func SanitizeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		if isPlainDecimal(s) {
			return s
		}
		return "'" + s
	}
	return s
}

// isPlainDecimal mencocokkan ^-?[0-9]+(\.[0-9]+)?$ tanpa regexp.
func isPlainDecimal(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	digits := func() bool {
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return i > start
	}
	if !digits() {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if !digits() {
			return false
		}
	}
	return i == len(s)
}

// formatCell mengubah nilai skalar menjadi teks sel. Nilai bersarang (map/slice/array/struct) ditolak
// tanpa menyebut isinya agar field bersarang tidak bocor di bawah klasifikasi kolom yang lebih rendah.
func formatCell(key string, v any) (string, error) {
	// Pointer nil dicek lebih dulu: *T nil tetap memenuhi fmt.Stringer bila T punya method nilai (mis. *time.Time),
	// dan memanggil String() padanya panic.
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return "", nil
	}
	switch x := v.(type) {
	case nil:
		return "", nil
	case *time.Time:
		return x.UTC().Format(time.RFC3339), nil
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case json.Number:
		return x.String(), nil
	case time.Time:
		return x.UTC().Format(time.RFC3339), nil
	case fmt.Stringer:
		return x.String(), nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return "", nil
		}
		return formatCell(key, rv.Elem().Interface())
	case reflect.String:
		return rv.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 64), nil
	}
	return "", fmt.Errorf("%w: kolom %q", ErrNonScalarValue, key)
}
