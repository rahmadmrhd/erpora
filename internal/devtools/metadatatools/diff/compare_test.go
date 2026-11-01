package diff

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
)

func baseTable() *Table {
	return &Table{
		Name: "m_things",
		Columns: []Column{
			{Name: "id", Type: "uuid", NotNull: true},
			{Name: "title", Type: "text", NotNull: true},
			{Name: "note", Type: "text"},
		},
		Constraints: []Constraint{
			{Name: "m_things_pkey", Kind: "p", Def: "PRIMARY KEY (id)", Columns: []string{"id"}},
			{Name: "m_things_title_key", Kind: "u", Def: "UNIQUE (title)", Columns: []string{"title"}},
			{Name: "m_things_title_check", Kind: "c", Def: "CHECK ((length(title) <= 10))", Columns: []string{"title"}},
		},
		Indexes: []Index{{Name: "m_things_note_idx", Key: "btree:false:note:", Columns: []string{"note"}}},
	}
}

func cat(ts ...*Table) *Catalog {
	c := &Catalog{Schema: "s", Tables: map[string]*Table{}}
	for _, t := range ts {
		c.Tables[t.Name] = t
	}
	return c
}

func kinds(r *Report) string {
	var ks []string
	for _, f := range r.Findings {
		ks = append(ks, f.Kind)
	}
	return strings.Join(ks, ",")
}

func TestCompare_Equal(t *testing.T) {
	r := Compare(cat(baseTable()), cat(baseTable()), nil, CompareOptions{})
	if !r.Empty() || len(r.Notes) != 0 {
		t.Fatalf("laporan = %s", r)
	}
}

func TestCompare_NamesOfConstraintsAndIndexesAreIgnored(t *testing.T) {
	a := baseTable()
	a.Constraints[1].Name = "handwritten_unique"
	a.Constraints[2].Name = "handwritten_check"
	a.Indexes[0].Name = "handwritten_idx"
	if r := Compare(cat(a), cat(baseTable()), nil, CompareOptions{}); !r.Empty() {
		t.Fatalf("nama tidak boleh menjadi selisih: %s", r)
	}
}

func TestCompare_Differences(t *testing.T) {
	cases := []struct {
		name string
		mut  func(a *Table)
		want string
	}{
		{"kolom hilang", func(a *Table) { a.Columns = a.Columns[:2] }, "column_missing"},
		{"kolom berlebih", func(a *Table) { a.Columns = append(a.Columns, Column{Name: "extra", Type: "text"}) }, "column_extra"},
		{"tipe beda", func(a *Table) { a.Columns[1].Type = "character varying(10)" }, "column_type"},
		{"nullability", func(a *Table) { a.Columns[2].NotNull = true }, "column_nullability"},
		{"default", func(a *Table) { a.Columns[2].Default = "now()" }, "column_default"},
		{"turunan", func(a *Table) { a.Columns[2].Generated = "(title)" }, "column_generated"},
		{"unique berlebih", func(a *Table) {
			a.Constraints = append(a.Constraints, Constraint{Name: "x", Kind: "u", Def: "UNIQUE (note)", Columns: []string{"note"}})
		}, "unique_extra"},
		{"unique hilang", func(a *Table) { a.Constraints = append(a.Constraints[:1], a.Constraints[2:]...) }, "unique_missing"},
		{"check hilang", func(a *Table) { a.Constraints = a.Constraints[:2] }, "check_missing"},
		{"check berlebih", func(a *Table) {
			a.Constraints = append(a.Constraints, Constraint{Name: "x", Kind: "c", Def: "CHECK ((note <> ''::text))", Columns: []string{"note"}})
		}, "check_extra"},
		{"fk berlebih", func(a *Table) {
			a.Constraints = append(a.Constraints, Constraint{Name: "x", Kind: "f", Def: "FOREIGN KEY (note) REFERENCES o(id)", Columns: []string{"note"}})
		}, "foreign_key_extra"},
		{"kunci utama beda", func(a *Table) { a.Constraints[0].Def = "PRIMARY KEY (title)" }, "primary_key"},
		{"indeks tak dideklarasikan", func(a *Table) {
			a.Indexes = append(a.Indexes, Index{Name: "y", Key: "btree:false:title:", Columns: []string{"title"}})
		}, "index_extra"},
		{"indeks hilang", func(a *Table) { a.Indexes = nil }, "index_missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := baseTable()
			c.mut(a)
			r := Compare(cat(a), cat(baseTable()), nil, CompareOptions{})
			if kinds(r) != c.want {
				t.Fatalf("kinds = %q, want %q\n%s", kinds(r), c.want, r)
			}
			if r.Empty() {
				t.Error("Empty() harus false")
			}
		})
	}
}

func TestCompare_TableMissingAndExtra(t *testing.T) {
	r := Compare(cat(), cat(baseTable()), nil, CompareOptions{})
	if kinds(r) != "table_missing" {
		t.Fatalf("kinds = %q", kinds(r))
	}
	other := &Table{Name: "m_other", Columns: []Column{{Name: "id", Type: "uuid"}}}
	r = Compare(cat(baseTable(), other, &Table{Name: "goose_module_m"}, &Table{Name: "z_unrelated"}), cat(baseTable()), nil, CompareOptions{NotePrefixes: []string{"m_"}})
	if !r.Empty() {
		t.Fatalf("tabel berlebih bukan selisih: %s", r)
	}
	if len(r.Notes) != 1 || r.Notes[0].Kind != KindTableExtra || r.Notes[0].Table != "m_other" {
		t.Errorf("notes = %+v", r.Notes)
	}
}

func TestCompare_UnmanagedColumnsExcluded(t *testing.T) {
	a := baseTable()
	a.Columns = append(a.Columns, Column{Name: "secret_token", Type: "bytea"})
	a.Constraints = append(a.Constraints,
		Constraint{Name: "u2", Kind: "u", Def: "UNIQUE (secret_token)", Columns: []string{"secret_token"}},
		Constraint{Name: "c2", Kind: "c", Def: "CHECK ((octet_length(secret_token) > 0))"})
	a.Indexes = append(a.Indexes, Index{Name: "i2", Key: "btree:false:secret_token:", Columns: []string{"secret_token"}})
	model := &dbmodel.Schema{Tables: []*dbmodel.Table{{Name: "m_things", Unmanaged: []string{"secret_token"}}}}
	if r := Compare(cat(a), cat(baseTable()), model, CompareOptions{}); !r.Empty() {
		t.Fatalf("kolom tak terkelola dan objek yang menyentuhnya harus dikecualikan: %s", r)
	}
	// Tanpa pernyataan unmanaged, semuanya selisih.
	if r := Compare(cat(a), cat(baseTable()), nil, CompareOptions{}); r.Empty() {
		t.Fatal("tanpa unmanaged harus ada selisih")
	}
}

func TestCompare_Triggers(t *testing.T) {
	trg := Trigger{Name: "m_things_lock_trg", Enabled: true, Type: 27, Function: "m_things_lock_fn", Body: "BEGIN END"}
	exp := baseTable()
	exp.Triggers = []Trigger{trg}
	mk := func(mut func(*Trigger)) *Table {
		a := baseTable()
		x := trg
		if mut != nil {
			mut(&x)
			a.Triggers = []Trigger{x}
		}
		return a
	}
	if r := Compare(cat(mk(func(*Trigger) {})), cat(exp), nil, CompareOptions{}); !r.Empty() {
		t.Fatalf("identik harus kosong: %s", r)
	}
	if r := Compare(cat(mk(nil)), cat(exp), nil, CompareOptions{}); kinds(r) != "trigger_missing" {
		t.Errorf("pemicu hilang: %q", kinds(r))
	}
	if r := Compare(cat(mk(func(x *Trigger) { x.Enabled = false })), cat(exp), nil, CompareOptions{}); kinds(r) != "trigger_disabled" {
		t.Errorf("pemicu nonaktif: %q", kinds(r))
	}
	if r := Compare(cat(mk(func(x *Trigger) { x.Body = "BEGIN RETURN NEW; END" })), cat(exp), nil, CompareOptions{}); kinds(r) != "trigger_changed" {
		t.Errorf("badan berubah: %q", kinds(r))
	}
	if r := Compare(cat(mk(func(x *Trigger) { x.Type = 29 })), cat(exp), nil, CompareOptions{}); kinds(r) != "trigger_changed" {
		t.Errorf("jenis pemicu berubah: %q", kinds(r))
	}
	// Pemicu di luar model hanya catatan.
	a := baseTable()
	a.Triggers = []Trigger{{Name: "handwritten_audit_trg", Enabled: true}}
	r := Compare(cat(a), cat(baseTable()), nil, CompareOptions{})
	if !r.Empty() || len(r.Notes) != 1 || r.Notes[0].Kind != KindTriggerExtra {
		t.Errorf("pemicu tak dikenal = %s", r)
	}
}

func TestReport_DeterministicAndPrintable(t *testing.T) {
	a := baseTable()
	a.Columns = append(a.Columns, Column{Name: "zz", Type: "text"}, Column{Name: "aa", Type: "text"})
	r1 := Compare(cat(a), cat(baseTable()), nil, CompareOptions{})
	r2 := Compare(cat(a), cat(baseTable()), nil, CompareOptions{})
	if r1.String() != r2.String() || !strings.Contains(r1.String(), "column_extra") || !strings.Contains(r1.String(), "m_things") {
		t.Errorf("laporan = %s", r1)
	}
	if strings.Index(r1.String(), "aa") > strings.Index(r1.String(), "zz") {
		t.Error("temuan harus terurut")
	}
}
