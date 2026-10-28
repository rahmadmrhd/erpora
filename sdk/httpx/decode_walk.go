package httpx

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

const maxDetails = 20

var (
	jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	jsonNumberType      = reflect.TypeFor[json.Number]()
)

// walker menelusuri token JSON satu kali, dipandu reflect.Type DTO, untuk menegakkan batas kedalaman,
// panjang array, kunci ganda, kunci yang sama persis (huruf besar/kecil) dan kesesuaian tipe skalar.
type walker struct {
	dec      *json.Decoder
	maxDepth int
	maxArray int
	details  []Detail // temuan 422 (unknown_field, invalid_type)
}

func (w *walker) addDetail(path, code, msg string) {
	if len(w.details) < maxDetails {
		w.details = append(w.details, Detail{Field: path, Code: code, Message: msg})
	}
}

func malformed() *Error {
	return badRequest("malformed_json", "Body request bukan JSON yang valid.", "")
}

func (w *walker) root(t reflect.Type) error {
	tok, err := w.dec.Token()
	if err != nil {
		return malformed()
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return badRequest("expected_object", "Body request harus berupa objek JSON.", "")
	}
	if err := w.object(t, "", 1); err != nil {
		return err
	}
	if _, err := w.dec.Token(); !errors.Is(err, io.EOF) {
		return badRequest("trailing_data", "Ada data setelah objek JSON.", "")
	}
	return nil
}

// value membaca satu nilai; t nil berarti isi tidak diperiksa tipenya (any, Unmarshaler, nilai terlewati).
func (w *walker) value(t reflect.Type, quoted bool, path string, depth int) error {
	tok, err := w.dec.Token()
	if err != nil {
		return malformed()
	}
	if d, ok := tok.(json.Delim); ok {
		if d == '{' {
			return w.object(t, path, depth+1)
		}
		return w.array(t, path, depth+1)
	}
	w.scalar(tok, t, quoted, path)
	return nil
}

func deref(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// unchecked: isi nilai tidak diperiksa tipenya (nil, any, atau tipe dengan Unmarshaler sendiri).
func unchecked(t reflect.Type) bool {
	base := deref(t)
	if base == nil || base.Kind() == reflect.Interface {
		return true
	}
	pt := reflect.PointerTo(base)
	return pt.Implements(jsonUnmarshalerType) || pt.Implements(textUnmarshalerType)
}

func joinKey(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// object dipanggil setelah token '{' dibaca; depth sudah termasuk objek ini.
func (w *walker) object(t reflect.Type, path string, depth int) error {
	if depth > w.maxDepth {
		return badRequest("too_deep", "Struktur JSON terlalu dalam.", path)
	}
	var fields map[string]fieldInfo
	var elem reflect.Type
	structMode := false
	if !unchecked(t) {
		switch bt := deref(t); bt.Kind() {
		case reflect.Struct:
			fields, structMode = fieldsOf(bt), true
		case reflect.Map:
			elem = bt.Elem()
		default:
			w.addDetail(path, "invalid_type", "Tipe nilai tidak sesuai.")
		}
	}
	seen := map[string]struct{}{}
	for w.dec.More() {
		kt, err := w.dec.Token()
		if err != nil {
			return malformed()
		}
		key, _ := kt.(string)
		kp := joinKey(path, key)
		if _, dup := seen[key]; dup {
			return badRequest("duplicate_key", "Kunci ganda dalam satu objek.", kp)
		}
		seen[key] = struct{}{}
		var ft reflect.Type
		quoted := false
		switch {
		case structMode:
			if fi, ok := fields[key]; ok {
				ft, quoted = fi.typ, fi.quoted
			} else {
				w.addDetail(kp, "unknown_field", "Field tidak dikenal.")
			}
		case elem != nil:
			ft = elem
		}
		if err := w.value(ft, quoted, kp, depth); err != nil {
			return err
		}
	}
	if _, err := w.dec.Token(); err != nil { // '}'
		return malformed()
	}
	return nil
}

// array dipanggil setelah token '[' dibaca; depth sudah termasuk array ini.
func (w *walker) array(t reflect.Type, path string, depth int) error {
	if depth > w.maxDepth {
		return badRequest("too_deep", "Struktur JSON terlalu dalam.", path)
	}
	var elem reflect.Type
	if !unchecked(t) {
		switch bt := deref(t); bt.Kind() {
		case reflect.Slice, reflect.Array:
			elem = bt.Elem()
		default:
			w.addDetail(path, "invalid_type", "Tipe nilai tidak sesuai.")
		}
	}
	i := 0
	for w.dec.More() {
		if i >= w.maxArray {
			return badRequest("array_too_long", "Array melebihi batas panjang.", path)
		}
		if err := w.value(elem, false, path+"["+strconv.Itoa(i)+"]", depth); err != nil {
			return err
		}
		i++
	}
	if _, err := w.dec.Token(); err != nil { // ']'
		return malformed()
	}
	return nil
}

// scalar memeriksa kesesuaian jenis token skalar terhadap kind tipe target.
func (w *walker) scalar(tok json.Token, t reflect.Type, quoted bool, path string) {
	if tok == nil || unchecked(t) { // null selalu diterima encoding/json
		return
	}
	bt := deref(t)
	ok := false
	switch v := tok.(type) {
	case string:
		ok = bt.Kind() == reflect.String
		if !ok && isByteSlice(bt) {
			// encoding/json mendekode string base64 standar ke []byte (dan named byte slice).
			_, derr := base64.StdEncoding.DecodeString(v)
			ok = derr == nil
		}
		if !ok && quoted {
			switch bt.Kind() {
			case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
				ok = true // nilainya dicek lapis kedua
			}
		}
	case bool:
		ok = bt.Kind() == reflect.Bool
	case json.Number:
		s := v.String()
		switch bt.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			_, err := strconv.ParseInt(s, 10, bt.Bits())
			ok = err == nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			_, err := strconv.ParseUint(s, 10, bt.Bits())
			ok = err == nil
		case reflect.Float32, reflect.Float64:
			_, err := strconv.ParseFloat(s, bt.Bits())
			ok = err == nil
		case reflect.String:
			ok = bt == jsonNumberType
		}
	}
	if !ok {
		w.addDetail(path, "invalid_type", "Tipe nilai tidak sesuai.")
	}
}

// --- pemetaan field struct menurut aturan encoding/json ---

type fieldInfo struct {
	typ    reflect.Type
	quoted bool
}

type fieldCand struct {
	info      fieldInfo
	depth     int
	tagged    bool
	ambiguous bool
}

var fieldCache sync.Map // reflect.Type -> map[string]fieldInfo

func fieldsOf(t reflect.Type) map[string]fieldInfo {
	if c, ok := fieldCache.Load(t); ok {
		return c.(map[string]fieldInfo)
	}
	cands := map[string]*fieldCand{}
	collectFields(t, 0, map[reflect.Type]bool{}, cands)
	out := make(map[string]fieldInfo, len(cands))
	for name, c := range cands {
		if !c.ambiguous {
			out[name] = c.info
		}
	}
	fieldCache.Store(t, out)
	return out
}

func collectFields(t reflect.Type, depth int, stack map[reflect.Type]bool, out map[string]*fieldCand) {
	if stack[t] {
		return
	}
	stack[t] = true
	defer delete(stack, t)
	for i := range t.NumField() {
		f := t.Field(i)
		ft := f.Type
		if f.Anonymous {
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if !f.IsExported() && ft.Kind() != reflect.Struct {
				continue
			}
		} else if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			collectFields(ft, depth+1, stack, out)
			continue
		}
		tagged := name != ""
		if name == "" {
			name = f.Name
		}
		cand := &fieldCand{
			info:   fieldInfo{typ: f.Type, quoted: hasOption(opts, "string")},
			depth:  depth,
			tagged: tagged,
		}
		switch prev := out[name]; {
		case prev == nil, cand.depth < prev.depth:
			out[name] = cand
		case cand.depth == prev.depth:
			switch {
			case cand.tagged && !prev.tagged:
				out[name] = cand
			case cand.tagged == prev.tagged:
				prev.ambiguous = true
			}
		}
	}
}

func hasOption(opts, want string) bool {
	for opts != "" {
		var o string
		o, opts, _ = strings.Cut(opts, ",")
		if o == want {
			return true
		}
	}
	return false
}

// isByteSlice meniru aturan encoding/json: slice dengan elemen berkind uint8 yang elemennya tidak mengimplementasikan
// json.Unmarshaler/TextUnmarshaler didekode dari string base64.
func isByteSlice(t reflect.Type) bool {
	if t.Kind() != reflect.Slice || t.Elem().Kind() != reflect.Uint8 {
		return false
	}
	pe := reflect.PointerTo(t.Elem())
	return !pe.Implements(jsonUnmarshalerType) && !pe.Implements(textUnmarshalerType)
}
