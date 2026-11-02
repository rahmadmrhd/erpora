package check

import (
	"fmt"
	"strings"

	"erpora/internal/devtools/metadatatools/naming"
)

// checkUnmanaged memeriksa unmanagedColumns (Requirement 2.9; keputusan pemilik produk 2 Nov 2026): tiap butir
// berbentuk {name, classification, reason}; classification wajib highly_confidential dan reason wajib tidak
// kosong setelah trim. Pemeriksa tidak dapat membuktikan klasifikasi kolom di luar metadata; yang ditegakkan
// adalah pernyataan pengembang itu, ditambah: nama sah, tidak ganda, tidak bentrok dengan kolom yang dikelola
// metadata, dan tidak bersama generate.crud.
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
		if class, _ := uc["classification"].(string); class != "highly_confidential" {
			x.atDoc(SeverityError, ptr+"/classification", CodeUnmanagedColumnClassification,
				"kolom unmanaged %q berklasifikasi %q: unmanagedColumns hanya untuk kolom highly_confidential (Requirement 2.9, K17); kolom dengan klasifikasi lain dideklarasikan sebagai field metadata", name, class)
		}
		if reason, _ := uc["reason"].(string); strings.TrimSpace(reason) == "" {
			x.atDoc(SeverityError, ptr+"/reason", CodeUnmanagedColumnReason,
				"kolom unmanaged %q wajib beralasan tertulis (reason tidak boleh kosong)", name)
		}
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
