package secfixture

import (
	"reflect"

	"erpora/sdk/identity"
)

// Fixture: tepat 2 pelanggaran IDENT TANPA `import "unsafe"`. Nilai bertipe unsafe.Pointer didapat lewat inferensi
// tipe dari reflect (Value.UnsafePointer) lalu diberikan ke reflect.NewAt: jalur pemalsuan "kotak tertutup" context
// di sdk/identity yang dibuktikan reviewer. Aturan menandai setiap ekspresi bertipe unsafe.Pointer.
func forgeViaReflect(p *identity.Principal) reflect.Value {
	ptr := reflect.ValueOf(p).UnsafePointer()     // 1
	return reflect.NewAt(reflect.TypeOf(*p), ptr) // 2
}
