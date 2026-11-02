// Package dbmodel menurunkan model skema database dari metadata resource yang sudah diselesaikan (spec
// resource-metadata Requirement 10; design Komponen 7). Model ini adalah sisi "diharapkan" mesin selisih
// (paket diff) dan masukan pembangkit SQL (paket sqlgen): satu model untuk pemeriksa dan pembangkit.
//
// Paket ini tidak menghasilkan SQL. Model hanya memuat nilai terstruktur (tipe, batas, daftar nilai enum, AST
// kondisi dan ekspresi); seluruh teks SQL, termasuk kutipan identifier dan escape literal, dibuat oleh
// sqlgen dari model ini (Requirement 19.7, 19.8). Tidak ada teks berkas metadata yang disalin apa adanya:
// nama lolos paket naming, nilai enum lolos pola Requirement 2.11, angka lolos parser desimal.
//
// Aturan pemetaan yang ditetapkan paket ini (keputusan pelaksana, task 10.3):
//   - string dan text menjadi text; minLength/maxLength menjadi satu CHECK length per kolom (sejalan dengan
//     migrasi tulis tangan yang ada, mis. example_notes.title).
//   - integer menjadi integer bila minimum dan maximum keduanya ada dan muat 32-bit, selain itu bigint;
//     minimum/maximum menjadi satu CHECK rentang.
//   - decimal(p,s) menjadi numeric(p,s) dan wajib punya precision dan scale; enum menjadi text + satu CHECK
//     IN (nilai); datetime menjadi timestamptz.
//   - pattern dan format tidak masuk database (RE2 dan PostgreSQL berbeda semantik); ditegakkan aplikasi.
//   - tidak ada kolom tenant_id (ADR-0004); id uuid kunci utama wajib berasal dari sdk:audit.
package dbmodel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"erpora/sdk/metadata"
)

// Jenis tabel.
const (
	KindResource = "resource"
	KindPivot    = "pivot"
)

// Nilai onDelete (sama dengan kunci metadata).
const (
	OnDeleteRestrict = "restrict"
	OnDeleteCascade  = "cascade"
	OnDeleteSetNull  = "setNull"
)

// SQLType adalah tipe kolom dalam bentuk kanonik PostgreSQL (sama dengan keluaran format_type): "text",
// "bigint", "numeric(18,2)", "timestamp with time zone". String() sekaligus bentuk yang dipakai di DDL.
type SQLType struct {
	Name      string
	Precision int // hanya numeric
	Scale     int // hanya numeric
}

// Tipe dasar yang dipakai model.
var (
	TypeText      = SQLType{Name: "text"}
	TypeBigint    = SQLType{Name: "bigint"}
	TypeInteger   = SQLType{Name: "integer"}
	TypeBoolean   = SQLType{Name: "boolean"}
	TypeDate      = SQLType{Name: "date"}
	TypeUUID      = SQLType{Name: "uuid"}
	TypeTimestamp = SQLType{Name: "timestamp with time zone"}
)

// Numeric membuat tipe numeric(p,s).
func Numeric(precision, scale int) SQLType {
	return SQLType{Name: "numeric", Precision: precision, Scale: scale}
}

func (t SQLType) String() string {
	if t.Name == "numeric" {
		return fmt.Sprintf("numeric(%d,%d)", t.Precision, t.Scale)
	}
	return t.Name
}

// Default adalah nilai bawaan kolom sebagai literal tertipe. Value sudah divalidasi menurut Type (desimal
// lolos parser desimal, boolean true atau false, enum lolos pola, tanggal lolos parser) dan hanya boleh
// dijadikan SQL lewat sqlgen.
type Default struct {
	Type  metadata.FieldType
	Value string
	// Now menyatakan DEFAULT now() (hanya datetime; Value kosong). Hanya dipakai kolom bawaan sdk:audit
	// (K22, pemilik produk 2 Nov 2026); metadata tidak punya kunci default dinamis.
	Now bool
}

// Column adalah satu kolom model.
type Column struct {
	Name    string
	Type    SQLType
	NotNull bool
	Default *Default
	// Generated adalah ekspresi kolom turunan (GENERATED ALWAYS AS (...) STORED); nil bila kolom biasa.
	Generated *metadata.Expr
	// Field adalah nama field metadata (camelCase); kosong untuk kolom relasi dan pivot.
	Field string
	// Relation adalah nama relasi pemilik kolom (kolom FK atau morph); kosong untuk kolom field.
	Relation string
	// MigrateFrom adalah nama kolom asal (snake_case) untuk field migrateFrom (Requirement 12.2).
	MigrateFrom string
	// FieldType adalah tipe metadata (untuk penerjemah ekspresi).
	FieldType metadata.FieldType

	camel string // nama camelCase untuk penamaan objek turunan
}

// CheckKind adalah jenis CHECK yang dikenal model.
type CheckKind string

const (
	// CheckLength membatasi length(kolom): Min dan Max (kosong = tanpa batas).
	CheckLength CheckKind = "length"
	// CheckRange membatasi nilai numerik: Min dan Max sebagai teks desimal (kosong = tanpa batas).
	CheckRange CheckKind = "range"
	// CheckEnum membatasi nilai teks ke Values.
	CheckEnum CheckKind = "enum"
)

// Check adalah satu CHECK per kolom (paling banyak satu per kolom).
type Check struct {
	Name     string
	Column   string
	Kind     CheckKind
	Min, Max string
	Values   []string
	// Extensible menandai enum yang boleh bertambah nilai secara aditif (field extensible: true).
	Extensible bool
}

// Unique adalah constraint UNIQUE (satu atau beberapa kolom).
type Unique struct {
	Name    string
	Columns []string
}

// Index adalah indeks btree biasa atas kolom (tanpa ekspresi, tanpa predikat).
type Index struct {
	Name    string
	Columns []string
}

// ForeignKey adalah foreign key relasi satu modul.
type ForeignKey struct {
	Name       string
	Columns    []string
	RefTable   string
	RefColumns []string
	// OnDelete adalah restrict, cascade, atau setNull.
	OnDelete string
	// Relation adalah nama relasi metadata pemiliknya (kosong untuk pivot).
	Relation string
}

// Trigger adalah pemicu bangkitan yang diharapkan ada (aktif) pada tabel. Isi fungsi dihasilkan sqlgen dari
// templat tetap; model hanya memuat parameter terstruktur.
type Trigger struct {
	Name     string
	Function string
	// Events adalah INSERT, UPDATE, DELETE, TRUNCATE (huruf besar, terurut).
	Events []string
	// Lock adalah parameter pemicu penguncian dokumen (BEFORE UPDATE OR DELETE pada tabel dokumen); Child adalah
	// parameter pemicu anak (BEFORE INSERT OR UPDATE OR DELETE pada tabel anak); Truncate adalah pemicu
	// statement-level BEFORE TRUNCATE pada tabel dokumen atau anak. Tepat satu terisi.
	Lock     *LockSpec
	Child    *ChildLock
	Truncate *TruncateGuard
}

// TruncateGuard adalah parameter pemicu statement-level BEFORE TRUNCATE pada tabel dokumen dan tabel anaknya:
// TRUNCATE tidak memicu pemicu baris sehingga tanpa pemicu ini ia melewati proteksi dokumen terkunci (keputusan
// pemilik produk, 2 Nov 2026; Requirement 13.2). Pada tabel dokumen TRUNCATE ditolak bila ada baris yang terkunci
// menurut Condition (predikat kunci pemicu baris); pada tabel anak ditolak bila ada baris anak yang induknya
// terkunci. Keduanya juga menolak TRUNCATE pada transaction_isolation selain READ COMMITTED (snapshot lama dapat
// tidak melihat baris yang baru terkunci).
type TruncateGuard struct {
	// Condition adalah lockWhen resource (kolom milik sendiri; eq, ne, in, isnull); diisi pada tabel dokumen.
	Condition *metadata.Condition
	// Child diisi pada tabel anak, pivot, atau seberang hasOne: TRUNCATE ditolak bila ada baris yang FK-nya menunjuk
	// induk terkunci menurut kondisi kunci induk (predikat yang sama dengan pemicu anak baris). Tepat satu dari
	// Condition dan Child terisi.
	Child *ChildLock
}

// ChildLock adalah parameter pemicu anak dokumen: INSERT, UPDATE, dan DELETE baris anak ditolak bila baris induk
// terkunci menurut kondisi kunci induk (Requirement 13.11). Anak adalah baris embedded, pivot belongsToMany,
// atau tabel seberang hasOne.
type ChildLock struct {
	// Parent adalah tabel dokumen induk (Parent.Lock adalah kondisi kunci, atas kolom Parent sendiri).
	Parent *Table
	// FKColumn adalah kolom pada tabel anak yang menunjuk id induk.
	FKColumn string
}

// LockSpec adalah parameter templat pemicu penguncian dokumen (Requirement 13.8, design D-19).
type LockSpec struct {
	// Condition dievaluasi pada OLD (kolom milik sendiri; eq, ne, in, isnull).
	Condition *metadata.Condition
	// StatusColumn, VersionColumn, UpdatedAtColumn, dan UpdatedByColumn adalah kolom yang boleh berubah
	// pada transisi sah.
	StatusColumn, VersionColumn, UpdatedAtColumn, UpdatedByColumn string
	// Transitions adalah pasangan (status lama, status baru) yang sah.
	Transitions [][2]string
	// Axes adalah sumbu status tambahan (statusAxes, K24): kolomnya boleh berubah pada baris terkunci hanya dengan
	// pasangan transisi sumbu yang sah (Requirement 13.8 a2).
	Axes []LockAxis
	// MigratePairs adalah pasangan kolom (tujuan, asal) migrateFrom yang boleh diisi dari NULL.
	MigratePairs [][2]string
}

// LockAxis adalah satu sumbu status beserta pasangan (nilai lama, nilai baru) yang sah.
type LockAxis struct {
	Column      string
	Transitions [][2]string
}

// Table adalah satu tabel model (resource atau pivot).
type Table struct {
	Name string
	// Resource adalah kunci resource metadata pemilik (pivot: kunci resource pemilik relasi).
	Resource string
	Module   string
	Kind     string
	// Relation adalah nama relasi pemilik pivot.
	Relation    string
	Columns     []*Column
	PrimaryKey  []string
	Uniques     []*Unique
	Indexes     []*Index
	ForeignKeys []*ForeignKey
	Checks      []*Check
	Triggers    []*Trigger
	// Lock adalah lockWhen resource (kondisi atas kolom milik sendiri); nil bila resource tidak terkunci.
	Lock *metadata.Condition
	// Unmanaged adalah nama kolom (snake_case) yang dikelola kode tulis tangan (unmanagedColumns); dikecualikan
	// dari perbandingan (Requirement 2.9).
	Unmanaged []string
	Large     bool

	byName map[string]*Column
}

// Column mengembalikan kolom menurut nama SQL, nil bila tidak ada.
func (t *Table) Column(name string) *Column { return t.byName[name] }

func (t *Table) index() {
	t.byName = make(map[string]*Column, len(t.Columns))
	for _, c := range t.Columns {
		t.byName[c.Name] = c
	}
}

// Schema adalah kumpulan tabel hasil turunan, terurut menurut nama tabel.
type Schema struct {
	Tables []*Table
	byName map[string]*Table
}

// Table mengembalikan tabel menurut nama SQL, nil bila tidak ada.
func (s *Schema) Table(name string) *Table {
	if s.byName == nil {
		for _, t := range s.Tables {
			if t.Name == name {
				return t
			}
		}
		return nil
	}
	return s.byName[name]
}

// ForModule mengembalikan model yang hanya memuat tabel milik slug modul.
func (s *Schema) ForModule(slug string) *Schema {
	out := &Schema{}
	for _, t := range s.Tables {
		if t.Module == slug {
			out.Tables = append(out.Tables, t)
		}
	}
	out.index()
	return out
}

func (s *Schema) index() {
	sort.Slice(s.Tables, func(i, j int) bool { return s.Tables[i].Name < s.Tables[j].Name })
	s.byName = make(map[string]*Table, len(s.Tables))
	for _, t := range s.Tables {
		t.index()
		s.byName[t.Name] = t
	}
}

// Summary adalah ringkasan teks deterministik seluruh model (untuk tes dan galat).
func (s *Schema) Summary() string {
	var sb strings.Builder
	for _, t := range s.Tables {
		fmt.Fprintf(&sb, "table %s (%s %s)\n", t.Name, t.Kind, t.Resource)
		for _, c := range t.Columns {
			fmt.Fprintf(&sb, "  col %s %s", c.Name, c.Type)
			if c.NotNull {
				sb.WriteString(" not null")
			}
			if c.Default != nil {
				if c.Default.Now {
					sb.WriteString(" default now()")
				} else {
					fmt.Fprintf(&sb, " default %s", strconv.Quote(c.Default.Value))
				}
			}
			if c.Generated != nil {
				sb.WriteString(" generated")
			}
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "  pk %s\n", strings.Join(t.PrimaryKey, ","))
		for _, u := range t.Uniques {
			fmt.Fprintf(&sb, "  unique %s (%s)\n", u.Name, strings.Join(u.Columns, ","))
		}
		for _, i := range t.Indexes {
			fmt.Fprintf(&sb, "  index %s (%s)\n", i.Name, strings.Join(i.Columns, ","))
		}
		for _, f := range t.ForeignKeys {
			fmt.Fprintf(&sb, "  fk %s (%s) -> %s(%s) %s\n", f.Name, strings.Join(f.Columns, ","), f.RefTable, strings.Join(f.RefColumns, ","), f.OnDelete)
		}
		for _, c := range t.Checks {
			fmt.Fprintf(&sb, "  check %s %s %s min=%q max=%q values=%s\n", c.Name, c.Column, c.Kind, c.Min, c.Max, strings.Join(c.Values, "|"))
		}
		for _, tr := range t.Triggers {
			fmt.Fprintf(&sb, "  trigger %s %s %s\n", tr.Name, tr.Function, strings.Join(tr.Events, ","))
		}
	}
	return sb.String()
}

// Kode temuan penurunan model.
const (
	CodeNoID        = "db_no_id"
	CodeType        = "db_type"
	CodeExpr        = "db_expr"
	CodeDefault     = "db_default"
	CodeRelation    = "db_relation"
	CodeUnsupported = "db_unsupported"
	CodeCollision   = "db_collision"
	CodeIndexField  = "db_index_field"
	CodeEnumValue   = "db_enum_value"
	CodeName        = "db_name"
	CodeLock        = "db_lock"
)

// NewSchema membangun Schema dari daftar tabel (diurutkan menurut nama dan diindeks).
func NewSchema(tables []*Table) *Schema {
	s := &Schema{Tables: append([]*Table(nil), tables...)}
	s.index()
	return s
}
