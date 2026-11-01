// Package diff adalah mesin selisih database (spec resource-metadata Requirement 10; design Komponen 7):
// menjalankan semua migrasi goose modul pada database sementara dengan role non-superuser, membaca katalog
// PostgreSQL, dan membandingkannya dengan skema turunan metadata (paket dbmodel).
//
// Pembanding bekerja pada dua katalog hasil introspeksi yang SAMA: katalog "aktual" (hasil migrasi) dan
// katalog "diharapkan" (DDL bangkitan sqlgen dari model, diterapkan ke skema sementara pada database yang
// sama). Dengan begitu normalisasi tipe, nilai bawaan, ekspresi CHECK, dan ekspresi kolom turunan dilakukan
// PostgreSQL sendiri (pg_get_expr, pg_get_constraintdef, format_type); pemeriksa dan pembangkit memakai
// mesin yang sama dan perbedaan penulisan semata (mis. BETWEEN versus >=/<=) tidak menjadi selisih.
//
// Perbandingan mengabaikan NAMA constraint dan indeks (migrasi tulis tangan sering bernama lain) tetapi
// membandingkan definisinya; pemicu dibandingkan menurut nama, status aktif, jenis, dan badan fungsi.
//
// Paket ini hanya untuk lingkungan pengembang dan CI (build devtools); tidak pernah di binary rilis atau
// terhadap database produksi.
package diff

import (
	"fmt"
	"sort"
	"strings"
)

// Column adalah satu kolom hasil introspeksi (semua teks sudah dinormalkan PostgreSQL).
type Column struct {
	Name string
	// Type adalah keluaran format_type: "text", "numeric(18,2)", "timestamp with time zone".
	Type    string
	NotNull bool
	// Default adalah ekspresi nilai bawaan (pg_get_expr); kosong bila tidak ada atau kolom turunan.
	Default string
	// Generated adalah ekspresi kolom turunan stored; kosong bila kolom biasa.
	Generated string
}

// Constraint adalah primary key (Kind "p"), unique ("u"), foreign key ("f"), atau CHECK ("c").
type Constraint struct {
	Name string
	Kind string
	// Def adalah keluaran pg_get_constraintdef.
	Def     string
	Columns []string
}

// Index adalah indeks yang tidak mendukung constraint (constraint dilaporkan sebagai Constraint).
type Index struct {
	Name string
	// Key menggabungkan metode, keunikan, kolom atau ekspresi, dan predikat; dasar perbandingan.
	Key     string
	Columns []string
}

// Trigger adalah pemicu non-internal.
type Trigger struct {
	Name string
	// Enabled bernilai true bila pemicu aktif untuk sesi biasa (tgenabled O).
	Enabled  bool
	Type     int
	Function string
	// Body adalah teks fungsi pemicu (prosrc).
	Body string
}

// Table adalah satu tabel hasil introspeksi.
type Table struct {
	Name        string
	Columns     []Column
	Constraints []Constraint
	Indexes     []Index
	Triggers    []Trigger
}

// Column mengembalikan kolom menurut nama.
func (t *Table) Column(name string) *Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// Catalog adalah hasil introspeksi satu skema.
type Catalog struct {
	Schema string
	Tables map[string]*Table
}

// Names mengembalikan nama tabel terurut.
func (c *Catalog) Names() []string {
	out := make([]string, 0, len(c.Tables))
	for n := range c.Tables {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Jenis temuan.
const (
	KindTableMissing      = "table_missing"
	KindTableExtra        = "table_extra"
	KindColumnMissing     = "column_missing"
	KindColumnExtra       = "column_extra"
	KindColumnType        = "column_type"
	KindColumnNullability = "column_nullability"
	KindColumnDefault     = "column_default"
	KindColumnGenerated   = "column_generated"
	KindPrimaryKey        = "primary_key"
	KindUniqueMissing     = "unique_missing"
	KindUniqueExtra       = "unique_extra"
	KindCheckMissing      = "check_missing"
	KindCheckExtra        = "check_extra"
	KindForeignKeyMissing = "foreign_key_missing"
	KindForeignKeyExtra   = "foreign_key_extra"
	KindIndexMissing      = "index_missing"
	KindIndexExtra        = "index_extra"
	KindTriggerMissing    = "trigger_missing"
	KindTriggerDisabled   = "trigger_disabled"
	KindTriggerChanged    = "trigger_changed"
	KindTriggerExtra      = "trigger_extra"
)

// Finding adalah satu selisih (atau catatan). Expected dan Actual memuat definisi hasil normalisasi
// PostgreSQL. Objek model terkait diisi bila ada, untuk pembangkit migrasi.
type Finding struct {
	Kind     string
	Table    string
	Object   string
	Message  string
	Expected string
	Actual   string
}

func (f Finding) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] %s", f.Kind, f.Table)
	if f.Object != "" {
		sb.WriteString("." + f.Object)
	}
	if f.Message != "" {
		sb.WriteString(": " + f.Message)
	}
	if f.Expected != "" || f.Actual != "" {
		fmt.Fprintf(&sb, " (diharapkan %q, aktual %q)", f.Expected, f.Actual)
	}
	return sb.String()
}

// Report adalah hasil perbandingan. Findings adalah selisih (gerbang CI: harus kosong); Notes hanya
// informasi (tabel atau pemicu di luar model).
type Report struct {
	// Role adalah nama role migrasi sementara yang dipakai Run (informasi; sudah dihapus saat Run kembali).
	Role     string
	Findings []Finding
	Notes    []Finding
}

// Empty melaporkan apakah tidak ada selisih (catatan tidak dihitung).
func (r *Report) Empty() bool { return len(r.Findings) == 0 }

func (r *Report) String() string {
	var sb strings.Builder
	if r.Empty() {
		sb.WriteString("selisih kosong\n")
	}
	for _, f := range r.Findings {
		sb.WriteString(f.String() + "\n")
	}
	for _, f := range r.Notes {
		sb.WriteString("catatan " + f.String() + "\n")
	}
	return sb.String()
}

func (r *Report) sort() {
	less := func(a, b Finding) bool {
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Object != b.Object {
			return a.Object < b.Object
		}
		return a.Expected+a.Actual < b.Expected+b.Actual
	}
	sort.SliceStable(r.Findings, func(i, j int) bool { return less(r.Findings[i], r.Findings[j]) })
	sort.SliceStable(r.Notes, func(i, j int) bool { return less(r.Notes[i], r.Notes[j]) })
}
