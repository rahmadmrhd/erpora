package authz

import "reflect"

// Record adalah satu record yang diperiksa lapis record (Check). AuthzValue mengembalikan nilai field
// untuk evaluasi scope; field adalah nama manifest (snake_case). ok false berarti nilai tidak tersedia
// dan scope yang membutuhkannya ditolak (gagal tertutup; tidak pernah dianggap cocok).
type Record interface {
	AuthzValue(field string) (value any, ok bool)
}

// mapRecord membungkus map[string]any. Kunci dikanonikalkan (camelCase DTO menjadi snake_case manifest);
// dua kunci yang runtuh ke nama kanonik yang sama dianggap ambigu dan tidak tersedia.
type mapRecord struct {
	values    map[string]any
	ambiguous map[string]bool
}

func recordFromMap(m map[string]any) Record {
	r := &mapRecord{values: make(map[string]any, len(m))}
	for k, v := range m {
		canon := CanonicalField(k)
		if canon == "" {
			continue
		}
		if _, dup := r.values[canon]; dup {
			if r.ambiguous == nil {
				r.ambiguous = map[string]bool{}
			}
			r.ambiguous[canon] = true
			continue
		}
		r.values[canon] = v
	}
	return r
}

func (r *mapRecord) AuthzValue(field string) (any, bool) {
	canon := CanonicalField(field)
	if canon == "" || r.ambiguous[canon] {
		return nil, false
	}
	v, ok := r.values[canon]
	return v, ok
}

// asRecord mengubah argumen Check menjadi Record: Record atau map[string]any; selain itu (termasuk nil
// dan nil bertipe) ditolak.
func asRecord(v any) (Record, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case Record:
		if isNilValue(x) {
			return nil, false
		}
		return x, true
	case map[string]any:
		return recordFromMap(x), true
	}
	return nil, false
}

// isNilValue melaporkan nil biasa maupun nil bertipe pada tipe yang dapat nil.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Chan, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}
