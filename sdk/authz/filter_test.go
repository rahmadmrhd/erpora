package authz

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

type noteRow struct {
	ID        string
	CreatedBy string
	Warehouse string
}

func (noteRow) TableName() string { return "notes" }

func dryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db
}

// renderFilter menerapkan scope ke query daftar notes dan mengembalikan SQL beserta variabel terikat.
func renderFilter(t *testing.T, scope func(*gorm.DB) *gorm.DB, pre ...func(*gorm.DB) *gorm.DB) (string, []any) {
	t.Helper()
	db := dryDB(t).Model(&noteRow{})
	for _, p := range pre {
		db = p(db)
	}
	var out []noteRow
	tx := db.Scopes(scope).Find(&out)
	if tx.Error != nil {
		t.Fatalf("query: %v", tx.Error)
	}
	return tx.Statement.SQL.String(), tx.Statement.Vars
}

var falseRE = regexp.MustCompile(`(?i)\bwhere\b.*\bfalse\b|\band\b.*\bfalse\b`)

func hasFalse(sql string) bool { return falseRE.MatchString(sql) }

func norm(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, "`", "")), " ") }

func engineWithFilter(rf RowFilter, err error) context.Context {
	return asUser(&stubEngine{filter: func(context.Context, string, string) (RowFilter, error) { return rf, err }})
}

func branch(cs ...RowCondition) RowBranch { return RowBranch{Conditions: cs} }

func TestFilterOwnerParameterized(t *testing.T) {
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(OwnerIs("created_by", "u1"))}}, nil)
	sql, vars := renderFilter(t, Filter(ctx, "x.note", "read"))
	if got := norm(sql); !strings.Contains(got, "WHERE notes.created_by = ?") {
		t.Errorf("sql = %q", got)
	}
	if len(vars) != 1 || vars[0] != "u1" {
		t.Errorf("vars = %#v", vars)
	}
}

func TestFilterAllAddsNoClause(t *testing.T) {
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(AllRows())}}, nil)
	sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"))
	if strings.Contains(strings.ToLower(sql), "where") {
		t.Errorf("scope all tidak boleh menambah WHERE: %q", sql)
	}
	// dan tetap mempertahankan kondisi pemanggil
	sql, vars := renderFilter(t, Filter(ctx, "x.note", "read"), func(db *gorm.DB) *gorm.DB { return db.Where("warehouse = ?", "w1") })
	if got := norm(sql); !strings.Contains(got, "WHERE warehouse = ?") || strings.Contains(got, "OR") || len(vars) != 1 {
		t.Errorf("sql = %q vars = %v", got, vars)
	}
}

func TestFilterBranchesAreGroupedAndAndedWithCallerConditions(t *testing.T) {
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{
		branch(OwnerIs("created_by", "u1")),
		branch(AssignedIn("warehouse", []string{"w1", "w2"})),
	}}, nil)
	pre := func(db *gorm.DB) *gorm.DB { return db.Where("status = ?", "open") }
	sql, vars := renderFilter(t, Filter(ctx, "x.note", "read"), pre)
	want := "WHERE status = ? AND (notes.created_by = ? OR notes.warehouse IN (?,?))"
	if got := norm(sql); !strings.Contains(got, want) {
		t.Errorf("sql = %q\nwant substring %q", got, want)
	}
	if len(vars) != 4 || vars[0] != "open" || vars[1] != "u1" {
		t.Errorf("vars = %#v", vars)
	}
}

func TestFilterConditionsInBranchAreAnded(t *testing.T) {
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{
		branch(OwnerIs("created_by", "u1"), AssignedIn("warehouse", []string{"w1"})),
		branch(OwnerIs("created_by", "u2")),
	}}, nil)
	sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"))
	want := "WHERE ((notes.created_by = ? AND notes.warehouse = ?) OR notes.created_by = ?)"
	if got := norm(sql); !strings.Contains(got, want) {
		t.Errorf("sql = %q\nwant substring %q", got, want)
	}
}

func TestFilterAllInBranchWithOtherConditionsKeepsOthers(t *testing.T) {
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(AllRows(), OwnerIs("created_by", "u1"))}}, nil)
	sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"))
	if got := norm(sql); !strings.Contains(got, "WHERE notes.created_by = ?") {
		t.Errorf("all AND owner harus tetap membatasi: %q", got)
	}
}

func TestFilterDenyShapesRenderFalse(t *testing.T) {
	tooLong := strings.Repeat("a", 70)
	cases := map[string]RowFilter{
		"kosong":                      {},
		"cabang tanpa kondisi":        {Branches: []RowBranch{{}}},
		"kind nol":                    {Branches: []RowBranch{branch(RowCondition{})}},
		"kind tak dikenal":            {Branches: []RowBranch{branch(RowCondition{Kind: ConditionKind(99)})}},
		"assigned nil":                {Branches: []RowBranch{branch(AssignedIn("warehouse", nil))}},
		"assigned kosong":             {Branches: []RowBranch{branch(AssignedIn("warehouse", []string{}))}},
		"owner userID kosong":         {Branches: []RowBranch{branch(OwnerIs("created_by", ""))}},
		"owner tanpa kolom":           {Branches: []RowBranch{branch(OwnerIs("", "u1"))}},
		"kolom injeksi":               {Branches: []RowBranch{branch(OwnerIs("created_by; DROP TABLE notes", "u1"))}},
		"kolom huruf besar":           {Branches: []RowBranch{branch(OwnerIs("CreatedBy", "u1"))}},
		"kolom terlalu panjang":       {Branches: []RowBranch{branch(OwnerIs(tooLong, "u1"))}},
		"custom tanpa filter":         {Branches: []RowBranch{branch(RowCondition{Kind: CondCustom})}},
		"all tapi kondisi lain rusak": {Branches: []RowBranch{branch(AllRows(), AssignedIn("warehouse", nil))}},
	}
	for name, rf := range cases {
		t.Run(name, func(t *testing.T) {
			sql, vars := renderFilter(t, Filter(engineWithFilter(rf, nil), "x.note", "read"))
			if !hasFalse(sql) {
				t.Errorf("harus WHERE false, got %q", sql)
			}
			if len(vars) != 0 {
				t.Errorf("tidak boleh ada variabel: %#v", vars)
			}
		})
	}
}

func TestFilterInvalidBranchDroppedButValidOneKept(t *testing.T) {
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{
		branch(AssignedIn("warehouse", nil)), // FALSE
		branch(OwnerIs("created_by", "u1")),
	}}, nil)
	sql, vars := renderFilter(t, Filter(ctx, "x.note", "read"))
	got := norm(sql)
	if !strings.Contains(got, "WHERE notes.created_by = ?") || hasFalse(sql) || len(vars) != 1 {
		t.Errorf("sql = %q vars = %v", got, vars)
	}
}

func TestFilterDeniedPreservesCallerConditions(t *testing.T) {
	sql, _ := renderFilter(t, Filter(engineWithFilter(RowFilter{}, nil), "x.note", "read"), func(db *gorm.DB) *gorm.DB { return db.Where("status = ?", "open") })
	if got := norm(sql); !strings.Contains(got, "WHERE status = ? AND false") {
		t.Errorf("sql = %q", got)
	}
}

func TestFilterSQLParameterized(t *testing.T) {
	evil := "x' OR 1=1 --"
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{
		branch(AssignedIn("warehouse", []string{evil})),
		branch(OwnerIs("created_by", evil)),
	}}, nil)
	sql, vars := renderFilter(t, Filter(ctx, "x.note", "read"))
	if strings.Contains(sql, "1=1") || strings.Contains(sql, "x'") {
		t.Errorf("nilai tidak boleh masuk ke SQL: %q", sql)
	}
	if len(vars) != 2 || vars[0] != evil || vars[1] != evil {
		t.Errorf("vars = %#v", vars)
	}
}

func TestFilterEngineErrorDeniesEvenWithPermissiveFilter(t *testing.T) {
	sql, _ := renderFilter(t, Filter(engineWithFilter(RowFilter{Branches: []RowBranch{branch(AllRows())}}, errors.New("db down")), "x.note", "read"))
	if !hasFalse(sql) {
		t.Errorf("galat mesin harus WHERE false (walau RowFilter longgar): %q", sql)
	}
}

func customCond(f func(ctx context.Context, db *gorm.DB) *gorm.DB) RowCondition {
	return CustomScope(ScopeProvider{Key: "x.recent", Label: "l", Filter: f, Match: func(context.Context, Record) (bool, error) { return true, nil }})
}

func TestFilterCustomScope(t *testing.T) {
	c := customCond(func(_ context.Context, db *gorm.DB) *gorm.DB { return db.Where("created_at > ?", "2026-01-01") })
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(c), branch(OwnerIs("created_by", "u1"))}}, nil)
	pre := func(db *gorm.DB) *gorm.DB { return db.Where("status = ?", "open") }
	sql, vars := renderFilter(t, Filter(ctx, "x.note", "read"), pre)
	want := "WHERE status = ? AND (created_at > ? OR notes.created_by = ?)"
	if got := norm(sql); !strings.Contains(got, want) {
		t.Errorf("sql = %q\nwant substring %q", got, want)
	}
	if len(vars) != 3 {
		t.Errorf("vars = %#v", vars)
	}
}

func TestFilterCustomScopeWithOrInsideStaysGrouped(t *testing.T) {
	c := customCond(func(_ context.Context, db *gorm.DB) *gorm.DB {
		return db.Where("a = ?", 1).Or("b = ?", 2)
	})
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(c, OwnerIs("created_by", "u1"))}}, nil)
	pre := func(db *gorm.DB) *gorm.DB { return db.Where("status = ?", "open") }
	sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"), pre)
	want := "WHERE status = ? AND ((a = ? OR b = ?) AND notes.created_by = ?)"
	if got := norm(sql); !strings.Contains(got, want) {
		t.Errorf("sql = %q\nwant substring %q", got, want)
	}
}

func TestCustomScopePanicDenies(t *testing.T) {
	c := customCond(func(context.Context, *gorm.DB) *gorm.DB { panic("boom") })
	ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(c)}}, nil)
	sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"))
	if !hasFalse(sql) {
		t.Errorf("provider panic harus WHERE false: %q", sql)
	}
	// cabang lain yang sah tetap berlaku; cabang panic tidak meminjamkan "semua"
	ctx = engineWithFilter(RowFilter{Branches: []RowBranch{branch(c), branch(OwnerIs("created_by", "u1"))}}, nil)
	sql, _ = renderFilter(t, Filter(ctx, "x.note", "read"))
	if got := norm(sql); !strings.Contains(got, "WHERE notes.created_by = ?") || hasFalse(sql) {
		t.Errorf("sql = %q", got)
	}
}

func TestCustomScopeWithoutConditionsOrNilDenies(t *testing.T) {
	noop := customCond(func(_ context.Context, db *gorm.DB) *gorm.DB { return db }) // tanpa WHERE: bukan "semua"
	nilRes := customCond(func(context.Context, *gorm.DB) *gorm.DB { return nil })
	for name, c := range map[string]RowCondition{"tanpa WHERE": noop, "nil": nilRes} {
		ctx := engineWithFilter(RowFilter{Branches: []RowBranch{branch(c)}}, nil)
		sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"))
		if !hasFalse(sql) {
			t.Errorf("%s: harus WHERE false: %q", name, sql)
		}
	}
}

func TestFilterNilDBReturnsNil(t *testing.T) {
	if got := Filter(context.Background(), "x.note", "read")(nil); got != nil {
		t.Error("db nil dikembalikan apa adanya")
	}
}
