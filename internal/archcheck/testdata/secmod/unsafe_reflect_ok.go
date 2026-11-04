package secfixture

import "reflect"

// Fixture: pemakaian refleksi yang sah, 0 pelanggaran IDENT (tidak ada ekspresi bertipe unsafe.Pointer).
func reflectOK(v any) (reflect.Kind, int) {
	rv := reflect.ValueOf(v)
	return rv.Kind(), rv.NumField()
}
