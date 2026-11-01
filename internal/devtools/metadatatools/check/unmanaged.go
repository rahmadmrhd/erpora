package check

import (
	"fmt"

	"erpora/internal/devtools/metadatatools/naming"
)

// checkUnmanaged memeriksa unmanagedColumns (Requirement 2.9) dan larangan bersama generate.crud.
//
// Catatan: konsep unmanagedColumns adalah usulan perancang yang menunggu konfirmasi pemilik produk (tasks.md
// Overview). Pemeriksa tidak dapat membuktikan bahwa sebuah kolom di luar metadata berklasifikasi
// highly_confidential; yang dapat ditegakkan secara statis hanya: nama sah, tidak ganda, tidak bentrok dengan
// kolom yang dikelola metadata, dan tidak bersama generate.crud. Seluruh logika ini ada di berkas ini.
func (x *rctx) checkUnmanaged() {
	if len(x.r.UnmanagedColumns) == 0 {
		return
	}
	managed := map[string]string{}
	for _, f := range x.r.Fields {
		if s, err := naming.ToSnake(f.Name); err == nil {
			managed[s] = "field " + f.Name
		}
	}
	for _, rel := range x.r.Relations {
		typ, _ := rel.Attrs["type"].(string)
		for _, col := range relationColumns(rel.Name, typ) {
			if s, err := naming.ToSnake(col); err == nil {
				managed[s] = "kolom relasi " + rel.Name
			}
		}
	}
	seen := map[string]bool{}
	for i, uc := range x.r.UnmanagedColumns {
		ptr := fmt.Sprintf("/unmanagedColumns/%d", i)
		name, _ := uc["name"].(string)
		for _, v := range naming.Validate(name) {
			x.atDoc(SeverityError, ptr, nameCode(v.Code), "kolom unmanaged: %s", v.Error())
		}
		s, err := naming.ToSnake(name)
		if err != nil {
			continue
		}
		if seen[s] {
			x.atDoc(SeverityError, ptr, CodeUnmanagedColumnDuplicate, "kolom unmanaged %q dideklarasikan lebih dari sekali", name)
		}
		seen[s] = true
		if what, clash := managed[s]; clash {
			x.atDoc(SeverityError, ptr, CodeUnmanagedColumnManaged,
				"kolom %q sudah dikelola metadata (%s); unmanagedColumns hanya untuk kolom highly_confidential di luar metadata", s, what)
		}
	}
	if crud, _ := x.r.Generate["crud"].(bool); crud {
		x.atDoc(SeverityError, "/generate", CodeCrudWithUnmanaged,
			"generate.crud tidak boleh dipakai bersama unmanagedColumns: kolom tulis tangan tidak dapat dibangkitkan CRUD (Requirement 2.9)")
	}
}
