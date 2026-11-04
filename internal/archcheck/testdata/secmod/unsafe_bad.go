package secfixture

import "unsafe"

// Fixture: tepat 1 pelanggaran IDENT (impor unsafe di kode non-test modul dilarang).
var unsafeSize = unsafe.Sizeof(int64(0))
