package authz

// Access adalah tingkat akses sebuah grup field bagi principal (PRD-001 FR-112).
type Access uint8

const (
	// AccessNone: grup tidak boleh dilihat (field dihapus dari respons) maupun ditulis. Nilai nol sengaja
	// none agar nilai yang tidak terisi gagal tertutup.
	AccessNone Access = iota
	// AccessRead: boleh dibaca, tidak boleh ditulis.
	AccessRead
	// AccessWrite: boleh dibaca dan ditulis.
	AccessWrite
)

// String mengembalikan "none", "read", atau "write" (nilai tak dikenal: "none").
func (a Access) String() string {
	switch a {
	case AccessRead:
		return "read"
	case AccessWrite:
		return "write"
	default:
		return "none"
	}
}

// FieldAccess adalah akses level field principal pada satu resource.
type FieldAccess interface {
	// Group mengembalikan akses grup field (kunci manifest); grup tak dikenal: AccessNone.
	Group(key string) Access
	// Field mengembalikan akses field. name boleh camelCase (DTO) atau snake_case (manifest); dinormalisasi
	// lewat CanonicalField. Nama yang tidak dapat dinormalisasi: AccessNone. Field yang tidak tercantum di
	// grup mana pun mengikuti grup default.
	Field(name string) Access
	// Readable memeriksa field yang dipakai untuk filter, sort, group, atau fields=: galat
	// *FieldForbiddenError (403 field_forbidden) memuat semua field yang akses-nya none (FR-112).
	Readable(fields ...string) error
}

// FieldAccessSpec adalah masukan NewFieldAccess; dipakai mesin platform dan authztest agar logika
// pencocokan nama field (camelCase terhadap snake_case manifest) hanya ada di satu tempat.
type FieldAccessSpec struct {
	Resource string
	// Groups: akses principal per grup field (kunci grup manifest). Grup yang tidak ada = none.
	Groups map[string]Access
	// FieldGroups: nama field manifest (snake_case) ke kunci grup.
	FieldGroups map[string]string
	// DefaultGroup: kunci grup default; field yang tidak tercantum di FieldGroups mengikutinya. Kosong
	// atau tidak ada di Groups berarti field tak tercantum ditolak (none).
	DefaultGroup string
}

type fieldAccess struct {
	resource     string
	groups       map[string]Access
	fieldGroups  map[string]string
	defaultGroup string
}

// NewFieldAccess membangun FieldAccess dari spec. Peta pada spec disalin.
func NewFieldAccess(spec FieldAccessSpec) FieldAccess {
	fa := &fieldAccess{
		resource:     spec.Resource,
		groups:       make(map[string]Access, len(spec.Groups)),
		fieldGroups:  make(map[string]string, len(spec.FieldGroups)),
		defaultGroup: spec.DefaultGroup,
	}
	for k, v := range spec.Groups {
		fa.groups[k] = v
	}
	for k, v := range spec.FieldGroups {
		fa.fieldGroups[k] = v
	}
	return fa
}

func (f *fieldAccess) Group(key string) Access { return f.groups[key] }

func (f *fieldAccess) Field(name string) Access {
	canon := CanonicalField(name)
	if canon == "" {
		return AccessNone
	}
	group, ok := f.fieldGroups[canon]
	if !ok {
		group = f.defaultGroup
	}
	if group == "" {
		return AccessNone
	}
	return f.groups[group]
}

func (f *fieldAccess) Readable(fields ...string) error {
	var denied []string
	for _, name := range fields {
		if f.Field(name) < AccessRead {
			denied = append(denied, name)
		}
	}
	if len(denied) == 0 {
		return nil
	}
	return newFieldForbidden(f.resource, denied)
}

// denyAllFields adalah FieldAccess untuk keadaan gagal tertutup: semua none.
type denyAllFields struct{ resource string }

func (denyAllFields) Group(string) Access { return AccessNone }
func (denyAllFields) Field(string) Access { return AccessNone }
func (d denyAllFields) Readable(fields ...string) error {
	if len(fields) == 0 {
		return nil
	}
	return newFieldForbidden(d.resource, fields)
}

// ApplyMask mengembalikan salinan fields yang hanya memuat field berakses minimal read menurut fa
// (field none dihapus, bukan dikosongkan; api-guidelines §4.4). Kunci dicocokkan lewat fa.Field sehingga
// camelCase DTO cocok dengan grup snake_case manifest. fa nil: hasil kosong. fields tidak diubah.
func ApplyMask(fa FieldAccess, fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	if fa == nil {
		return out
	}
	for k, v := range fields {
		if fa.Field(k) >= AccessRead {
			out[k] = v
		}
	}
	return out
}

// CheckWriteAccess memeriksa bahwa semua changedFields berakses write menurut fa; selain itu galat
// *FieldForbiddenError berisi seluruh field yang ditolak (bukan hanya yang pertama; FR-112). fa nil
// menolak semuanya.
func CheckWriteAccess(fa FieldAccess, resource string, changedFields []string) error {
	var denied []string
	for _, name := range changedFields {
		if fa == nil || fa.Field(name) < AccessWrite {
			denied = append(denied, name)
		}
	}
	if len(denied) == 0 {
		return nil
	}
	return newFieldForbidden(resource, denied)
}
