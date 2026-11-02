// Package resolve menyelesaikan penyertaan (include), timpaan, metadata abstrak, tipe domain, dan blok menjadi
// metadata datar dengan jejak asal per atribut (spec resource-metadata Requirement 3; design D-4, D-5, D-25).
//
// Penyelesaian terjadi saat pembangkitan dan pemeriksaan; tidak ada jalur eksekusi saat runtime. Kondisi
// diperlakukan sebagai data peta generik (validasi semantik kondisi dilakukan pemeriksa, task 5.x).
//
// Semantik yang ditetapkan paket ini:
//   - Nama field berurutan menurut deklarasi: include berurutan, lalu field milik dokumen itu sendiri; di dalam
//     satu berkas menurut baris sumber.
//   - Nama field ganda dari include atau field sendiri ditolak; satu-satunya cara mengubah field yang disertakan
//     adalah set pada item include itu (tidak ada penimpaan implisit).
//   - Di dalam item include, kunci set/omit/add dan rujukan field memakai nama sumber (tanpa prefix); rujukan
//     yang tidak cocok dengan field yang disumbang item itu dianggap rujukan ke field resource penyertaan
//     (mis. useShipping) dan tidak diubah. Prefix ditambahkan ke semua nama yang disumbang item itu: field,
//     id aturan, kunci seksi tata letak, dan rujukan di dalamnya.
package resolve

import (
	"encoding/json"
	"fmt"
)

// SDKSlug adalah slug modul semu untuk metadata bawaan SDK (slug dicadangkan platform, sehingga tidak dapat
// bentrok dengan modul nyata).
const SDKSlug = "sdk"

// Kunci metadata bawaan yang field-nya dilindungi dari omit dan set (Requirement 3.6, 13.1).
const (
	KeyAudit    = "sdk.audit"
	KeyDocument = "sdk.document"
)

// Operasi asal nilai.
const (
	OpDeclared   = "declared"
	OpAdd        = "add"
	OpSet        = "set"
	OpDomainType = "domainType"
)

// Origin menunjuk asal sebuah nilai: berkas dan baris sumber, dokumen yang menetapkannya, dan jalur include.
type Origin struct {
	File   string
	Line   int
	Column int
	// Doc adalah kunci dokumen tempat nilai ditetapkan (untuk tipe domain: kunci tipe).
	Doc string
	// Op adalah declared, add, set, atau domainType.
	Op string
	// Chain adalah jalur include dari dokumen yang diselesaikan hingga Doc, tanpa dokumen yang diselesaikan itu
	// sendiri; kosong bila nilai ditetapkan langsung di dokumen yang diselesaikan.
	Chain []string
}

func (o Origin) withChain(parent string) Origin {
	c := make([]string, 0, len(o.Chain)+1)
	c = append(c, parent)
	c = append(c, o.Chain...)
	o.Chain = c
	return o
}

// Change mencatat satu perubahan atribut setelah field diperkenalkan (timpaan set atau tipe domain).
type Change struct {
	Attr   string
	Old    any
	New    any
	Origin Origin
}

// Field adalah satu field hasil resolusi. Attrs memuat atribut persis seperti nama kunci di berkas metadata
// (type, maxLength, classification, visibleWhen, ...), sudah digabung dengan dasar tipe domain; atribut yang
// tidak dinyatakan tidak ada (bawaan seperti nullable=false dan classification=internal diterapkan oleh
// accessor di bawah dan oleh pemeriksa).
type Field struct {
	Name string
	// SourceName adalah nama di dokumen yang disertakan sebelum prefix ditambahkan.
	SourceName string
	Attrs      map[string]any
	// Origin adalah asal field diperkenalkan.
	Origin Origin
	// AttrOrigin adalah asal nilai terakhir tiap atribut.
	AttrOrigin map[string]Origin
	// History memuat semua perubahan atribut berurutan (jejak asal lengkap).
	History []Change
	// DomainType adalah kunci tipe domain (mis. sdk.email); kosong bila tipe bawaan.
	DomainType string
	// Widget, Display, dan Operators berasal dari tipe domain.
	Widget    string
	Display   string
	Operators []string
	// ProtectedBy berisi kunci sdk.audit atau sdk.document bila field bawaan yang tidak dapat di-omit/set.
	ProtectedBy string
	// DefaultNow menandai kolom yang di database memakai DEFAULT now() secara implisit (createdAt dan updatedAt dari
	// sdk:audit; K22): bukan nilai bawaan di berkas metadata, hanya penanda untuk model skema turunan.
	DefaultNow bool
	// IsStatusAxis menandai field yang dibangkitkan dari statusAxes (K24); tidak diwarisi lewat include.
	IsStatusAxis bool
}

// Type mengembalikan tipe dasar (builtin) field.
func (f *Field) Type() string { return f.Str("type") }

// Str mengembalikan atribut teks, atau "" bila tidak ada atau bukan teks.
func (f *Field) Str(attr string) string {
	s, _ := f.Attrs[attr].(string)
	return s
}

// Bool mengembalikan atribut boolean, false bila tidak ada.
func (f *Field) Bool(attr string) bool {
	b, _ := f.Attrs[attr].(bool)
	return b
}

// Num mengembalikan atribut numerik sebagai teks desimal asli ("" bila tidak ada).
func (f *Field) Num(attr string) string {
	switch v := f.Attrs[attr].(type) {
	case json.Number:
		return v.String()
	case string:
		return v
	}
	return ""
}

// Classification mengembalikan klasifikasi efektif (bawaan internal).
func (f *Field) Classification() string {
	if c := f.Str("classification"); c != "" {
		return c
	}
	return "internal"
}

// FieldGroup mengembalikan grup field yang dinyatakan, atau DefaultGroupName bila tidak dinyatakan. Untuk
// grup default sebenarnya (kunci bertanda default di manifest) pakai Resolved.GroupOf.
func (f *Field) FieldGroup() string {
	if g := f.Str("fieldGroup"); g != "" {
		return g
	}
	return DefaultGroupName
}

// Format mengembalikan atribut format (email atau url), "" bila tidak ada.
func (f *Field) Format() string { return f.Str("format") }

// Relation adalah satu relasi milik resource (abstrak tidak punya relasi).
type Relation struct {
	Name   string
	Attrs  map[string]any
	Origin Origin
}

// Rule adalah satu aturan lintas field (Requirement 4.3).
type Rule struct {
	ID     string
	Attrs  map[string]any // assert, field, message, scope
	Origin Origin
}

// Section adalah seksi tata letak dari blok (Requirement 3.10).
type Section struct {
	Key     string
	Columns int
	Order   []string // nama field akhir
	Origin  Origin
}

// Use adalah dokumen metadata yang dipakai (langsung atau transitif) oleh hasil resolusi; dasar pelaporan dampak
// perubahan bersama (Requirement 3.12).
type Use struct {
	Key    string
	Module string
	// Kind adalah include (abstrak atau resource) atau fieldType.
	Kind   string
	Direct bool
	// Via adalah jalur dari dokumen yang diselesaikan ke Key (Key di akhir).
	Via []string
}

// Resolved adalah metadata datar satu resource atau abstrak.
type Resolved struct {
	Key         string
	Module      string
	Kind        string // resource atau abstract
	Export      bool
	Description string
	Source      Origin
	// DefaultGroup adalah kunci grup default modul pemilik (lihat Module.DefaultGroup).
	DefaultGroup string

	Fields    []*Field
	Relations []*Relation
	Rules     []*Rule
	Sections  []Section

	// LockWhen adalah kondisi penguncian hasil gabungan (OR) kondisi bawaan dan turunan; nil bila tidak ada.
	LockWhen        map[string]any
	LockWhenOrigins []Origin

	// Atribut tingkat resource (hanya dari dokumen itu sendiri; tidak diwarisi lewat include).
	LabelTemplate    string
	Indexes          []map[string]any
	Generate         map[string]any
	Large            bool
	OwnerField       string
	ScopeFields      map[string]string
	UnmanagedColumns []map[string]any

	Uses []Use

	// Lifecycle adalah siklus hidup status baku (K23) pada dokumen (menyertakan sdk:document); nil bila bukan
	// dokumen. StatusAxes adalah sumbu status tambahan (K24) dari dokumen itu sendiri, berurutan menurut
	// deklarasi; tidak diwarisi lewat include.
	Lifecycle  *Lifecycle
	StatusAxes []*StatusAxis

	byName map[string]*Field
}

// Transition adalah satu pasangan transisi status (dari, ke).
type Transition struct{ From, To string }

// Lifecycle adalah siklus hidup status baku dokumen: nilai (urutan deklarasi sdk:document), nilai awal, dan
// tabel transisi bawaan (K23). Resource tidak dapat menambah nilai atau transisi.
type Lifecycle struct {
	Values      []string
	Initial     string
	Transitions []Transition
	// Origin menunjuk asal nilai status pada metadata bawaan.
	Origin Origin
}

// StatusAxis adalah satu sumbu status tambahan: field enum readOnly bukan-nullable bernilai awal Initial, dengan
// pasangan transisi sendiri yang boleh mundur atau siklik (K24). Field sumbu ada di Resolved.Fields (IsStatusAxis).
type StatusAxis struct {
	Name        string
	Values      []string
	Initial     string
	Transitions []Transition
	Origin      Origin
}

func allows(ts []Transition, from, to string) bool {
	for _, t := range ts {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}

// Allows menyatakan pasangan (from, to) sah pada tabel transisi.
func (l *Lifecycle) Allows(from, to string) bool { return allows(l.Transitions, from, to) }

// Allows menyatakan pasangan (from, to) sah pada transisi sumbu.
func (a *StatusAxis) Allows(from, to string) bool { return allows(a.Transitions, from, to) }

// IsDocument menyatakan resource atau abstrak menyertakan sdk:document (punya Lifecycle).
func (r *Resolved) IsDocument() bool { return r.Lifecycle != nil }

// Field mengembalikan field menurut nama, nil bila tidak ada.
func (r *Resolved) Field(name string) *Field { return r.byName[name] }

// GroupOf mengembalikan grup field efektif: yang dinyatakan, atau grup default modul pemilik.
func (r *Resolved) GroupOf(f *Field) string {
	if g := f.Str("fieldGroup"); g != "" {
		return g
	}
	return r.DefaultGroup
}

// Generates menyatakan apakah dokumen menghasilkan tabel, kontrak, dan kode: hanya resource, tidak abstrak
// (Requirement 3.4).
func (r *Resolved) Generates() bool { return r.Kind == "resource" }

func (r *Resolved) index() {
	r.byName = make(map[string]*Field, len(r.Fields))
	for _, f := range r.Fields {
		r.byName[f.Name] = f
	}
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = deepCopy(x)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, x := range t {
			s[i] = deepCopy(x)
		}
		return s
	}
	return v
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return deepCopy(m).(map[string]any)
}

func (f *Field) clone(chainParent string) *Field {
	c := *f
	c.Attrs = copyMap(f.Attrs)
	c.Origin = f.Origin.withChain(chainParent)
	c.AttrOrigin = make(map[string]Origin, len(f.AttrOrigin))
	for k, o := range f.AttrOrigin {
		c.AttrOrigin[k] = o.withChain(chainParent)
	}
	c.History = make([]Change, len(f.History))
	for i, h := range f.History {
		c.History[i] = Change{Attr: h.Attr, Old: deepCopy(h.Old), New: deepCopy(h.New), Origin: h.Origin.withChain(chainParent)}
	}
	c.Operators = append([]string(nil), f.Operators...)
	return &c
}

// Kode temuan resolver.
const (
	CodeIncludeNotFound              = "include_not_found"
	CodeIncludeAccess                = "include_access"
	CodeIncludeCycle                 = "include_cycle"
	CodeIncludeKind                  = "include_kind"
	CodeDuplicateField               = "duplicate_field"
	CodeDuplicateRule                = "duplicate_rule"
	CodeOverrideMissing              = "override_missing"
	CodeOverrideProtected            = "override_protected"
	CodeOverrideLowersClassification = "override_lowers_classification"
	CodeOverrideLoosensGroup         = "override_loosens_group"
	CodeOverrideRevokesReadOnly      = "override_revokes_readonly"
	CodeOverrideTypeReferenced       = "override_type_referenced"
	CodeTypeNotFound                 = "type_not_found"
	CodeTypeAccess                   = "type_access"
	CodeTypeKind                     = "type_kind"
	CodeTypeLowersClassification     = "type_lowers_classification"
	CodePrefixInvalid                = "prefix_invalid"
	CodeLimit                        = "limit"
	CodeLockWhenLimit                = "lockwhen_limit"
	CodeDuplicateKey                 = "duplicate_key"
	CodeKeyModule                    = "key_module"
)

func (o Origin) String() string { return fmt.Sprintf("%s:%d (%s %s)", o.File, o.Line, o.Doc, o.Op) }
