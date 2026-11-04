package authztest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"erpora/sdk/authz"
)

var noteResource = Resource{
	Key:         "x.note",
	ScopeFields: map[string]string{"warehouse": "warehouse_id"},
	Groups: []Group{
		{Key: "general", Default: true, Fields: []string{"title"}},
		{Key: "profit", Fields: []string{"cost_price", "margin"}},
		{Key: "detail", Fields: []string{"note_text"}},
	},
}

func newEngine() *Engine { return NewEngine([]Resource{noteResource}) }

func TestEngineRequireAndUnknownUser(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: All}}})
	ctx := e.Context(context.Background(), "u1")
	if err := authz.Require(ctx, "x.note.read"); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := authz.Require(ctx, "x.note.update"); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update: %v", err)
	}
	if err := authz.Require(e.Context(context.Background(), "ghost"), "x.note.read"); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("pengguna tak terdaftar: %v", err)
	}
	if err := authz.Require(context.Background(), "x.note.read"); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("tanpa principal: %v", err)
	}
}

func TestUnionDoesNotCrossCombineScopes(t *testing.T) {
	// peran A: read + all; peran B: update + owner. update atas record milik orang lain tidak boleh lolos.
	e := newEngine().AddUser(User{ID: "u1", Grants: []Grant{
		{Resource: "x.note", Actions: []string{"read"}, Scope: All},
		{Resource: "x.note", Actions: []string{"update"}, Scope: []string{"owner"}},
	}})
	ctx := e.Context(context.Background(), "u1")
	other := map[string]any{"createdBy": "u2"}
	mine := map[string]any{"createdBy": "u1"}
	if err := authz.Check(ctx, "x.note.read", other); err != nil {
		t.Errorf("read all: %v", err)
	}
	if err := authz.Check(ctx, "x.note.update", other); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("update milik orang lain harus ErrNotFound, got %v", err)
	}
	if err := authz.Check(ctx, "x.note.update", mine); err != nil {
		t.Errorf("update milik sendiri: %v", err)
	}
	if err := authz.Check(ctx, "x.note.delete", mine); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("aksi tanpa baris: %v", err)
	}
}

func TestOwnerNullNeverMatches(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"owner"}}}})
	ctx := e.Context(context.Background(), "u1")
	for name, rec := range map[string]map[string]any{
		"null":    {"createdBy": nil},
		"hilang":  {"title": "x"},
		"berbeda": {"createdBy": "u2"},
	} {
		if err := authz.Check(ctx, "x.note.read", rec); !errors.Is(err, authz.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestScopeEmptyAssignmentIsFalse(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"assigned:warehouse"}}}})
	ctx := e.Context(context.Background(), "u1")
	if err := authz.Check(ctx, "x.note.read", map[string]any{"warehouseId": "w1"}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("tanpa penugasan: %v", err)
	}
	rf, err := e.Filter(ctx, "x.note", "read")
	if err != nil || len(rf.Branches) != 1 || rf.Branches[0].Conditions[0].Kind != authz.CondAssigned || len(rf.Branches[0].Conditions[0].Values) != 0 {
		t.Errorf("RowFilter harus memuat assigned kosong (FALSE): %#v %v", rf, err)
	}
	e.AddUser(User{ID: "u1", Grants: e.users["u1"].Grants, Assignments: map[string][]string{"warehouse": {"w1"}}})
	if err := authz.Check(ctx, "x.note.read", map[string]any{"warehouseId": "w1"}); err != nil {
		t.Errorf("dengan penugasan: %v", err)
	}
	if err := authz.Check(ctx, "x.note.read", map[string]any{"warehouseId": "w2"}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("gudang lain: %v", err)
	}
}

func TestCheckRecordMissingScopeFieldDenies(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1",
		Grants:      []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"assigned:warehouse"}}},
		Assignments: map[string][]string{"warehouse": {"w1"}}})
	ctx := e.Context(context.Background(), "u1")
	if err := authz.Check(ctx, "x.note.read", map[string]any{"title": "x"}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("record tanpa field scope: %v", err)
	}
}

func TestInvalidScopesNeverMeanAll(t *testing.T) {
	for name, scope := range map[string][]string{
		"kosong":            {},
		"token tak dikenal": {"everything"},
		"dimensi tak ada":   {"assigned:nope"},
		"custom tak ada":    {"custom:x.missing"},
	} {
		e := newEngine().AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: scope}}})
		ctx := e.Context(context.Background(), "u1")
		if err := authz.Check(ctx, "x.note.read", map[string]any{"createdBy": "u1", "warehouseId": "w1"}); !errors.Is(err, authz.ErrNotFound) {
			t.Errorf("%s: Check = %v", name, err)
		}
		rf, err := e.Filter(ctx, "x.note", "read")
		if err != nil || len(rf.Branches) != 0 {
			t.Errorf("%s: RowFilter = %#v %v", name, rf, err)
		}
	}
}

func TestCustomScopePanicInMatchDenies(t *testing.T) {
	p := authz.ScopeProvider{Key: "x.recent", Label: "l",
		Filter: func(_ context.Context, db *gorm.DB) *gorm.DB { return db },
		Match:  func(context.Context, authz.Record) (bool, error) { panic("boom") },
	}
	e := NewEngine([]Resource{noteResource}, p).AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"custom:x.recent"}}}})
	if err := authz.Check(e.Context(context.Background(), "u1"), "x.note.read", map[string]any{"a": 1}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("panic Match: %v", err)
	}
	p.Match = func(context.Context, authz.Record) (bool, error) { return true, errors.New("x") }
	e = NewEngine([]Resource{noteResource}, p).AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"custom:x.recent"}}}})
	if err := authz.Check(e.Context(context.Background(), "u1"), "x.note.read", map[string]any{"a": 1}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("galat Match: %v", err)
	}
}

func TestAndWithinRowOrAcrossRows(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1",
		Grants: []Grant{
			{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"owner", "assigned:warehouse"}},
			{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"assigned:warehouse"}},
		},
		Assignments: map[string][]string{"warehouse": {"w1"}}})
	ctx := e.Context(context.Background(), "u1")
	cases := []struct {
		rec  map[string]any
		want error
	}{
		{map[string]any{"createdBy": "u1", "warehouseId": "w1"}, nil},
		{map[string]any{"createdBy": "u2", "warehouseId": "w1"}, nil}, // baris kedua
		{map[string]any{"createdBy": "u1", "warehouseId": "w2"}, authz.ErrNotFound},
	}
	for i, c := range cases {
		if err := authz.Check(ctx, "x.note.read", c.rec); !errors.Is(err, c.want) && err != c.want {
			t.Errorf("kasus %d: %v", i, err)
		}
	}
	rf, _ := e.Filter(ctx, "x.note", "read")
	if len(rf.Branches) != 2 || len(rf.Branches[0].Conditions) != 2 {
		t.Errorf("RowFilter = %#v", rf)
	}
}

func TestMaskFieldsCamelCaseMatchesSnakeGroup(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1",
		Grants:      []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: All}},
		FieldAccess: map[string]map[string]authz.Access{"x.note": {"detail": authz.AccessRead}}})
	ctx := e.Context(context.Background(), "u1")
	dto := struct {
		Title     string `json:"title"`
		CostPrice string `json:"costPrice"`
		Margin    string `json:"margin"`
		NoteText  string `json:"noteText"`
		Extra     string `json:"extra"`
	}{"a", "9", "1", "n", "e"}
	got := authz.MaskFields(ctx, "x.note", dto)
	want := map[string]any{"title": "a", "noteText": "n", "extra": "e"} // costPrice/margin: grup profit none
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v want %#v", got, want)
	}
}

func TestDefaultGroupFollowsAction(t *testing.T) {
	read := newEngine().AddUser(User{ID: "r", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: All}}})
	upd := newEngine().AddUser(User{ID: "w", Grants: []Grant{{Resource: "x.note", Actions: []string{"read", "update"}, Scope: All}}})
	none := newEngine().AddUser(User{ID: "n", Grants: []Grant{{Resource: "x.note", Actions: []string{"create"}, Scope: All}}})
	if got := authz.Fields(read.Context(context.Background(), "r"), "x.note").Group("general"); got != authz.AccessRead {
		t.Errorf("read: %v", got)
	}
	if got := authz.Fields(upd.Context(context.Background(), "w"), "x.note").Group("general"); got != authz.AccessWrite {
		t.Errorf("update: %v", got)
	}
	if got := authz.Fields(none.Context(context.Background(), "n"), "x.note").Group("general"); got != authz.AccessNone {
		t.Errorf("tanpa read/update: %v", got)
	}
}

func TestCheckWriteCamelCase(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1",
		Grants:      []Grant{{Resource: "x.note", Actions: []string{"read", "update"}, Scope: All}},
		FieldAccess: map[string]map[string]authz.Access{"x.note": {"profit": authz.AccessRead}}})
	ctx := e.Context(context.Background(), "u1")
	if err := authz.CheckWrite(ctx, "x.note", []string{"title"}); err != nil {
		t.Fatalf("title: %v", err)
	}
	err := authz.CheckWrite(ctx, "x.note", []string{"title", "costPrice", "noteText"})
	var ff *authz.FieldForbiddenError
	if !errors.As(err, &ff) || !reflect.DeepEqual(ff.Fields, []string{"costPrice", "noteText"}) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadableRejectsNoneForFilterAndSort(t *testing.T) {
	e := newEngine().AddUser(User{ID: "u1", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: All}}})
	fa := authz.Fields(e.Context(context.Background(), "u1"), "x.note")
	if err := fa.Readable("title", "createdAt"); err != nil {
		t.Fatalf("Readable: %v", err)
	}
	var ff *authz.FieldForbiddenError
	if err := fa.Readable("title", "costPrice"); !errors.As(err, &ff) || !reflect.DeepEqual(ff.Fields, []string{"costPrice"}) {
		t.Fatalf("err = %v", err)
	}
}

func TestAdministratorAndSystem(t *testing.T) {
	ctx := AsAdministrator(context.Background(), "adm")
	if err := authz.Require(ctx, "any.resource.delete"); err != nil {
		t.Fatalf("Require: %v", err)
	}
	if err := authz.Check(ctx, "any.resource.update", map[string]any{"createdBy": "other"}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if err := authz.CheckWrite(ctx, "any.resource", []string{"costPrice"}); err != nil {
		t.Fatalf("CheckWrite: %v", err)
	}
	sys := newEngine().Context(authz.AsSystem(context.Background(), "test.system"), "ghost")
	if err := authz.Require(sys, "x.note.read"); err != nil {
		t.Fatalf("sistem: %v", err)
	}
	// hak sistem tanpa principal
	sysOnly := authz.WithEngine(authz.AsSystem(context.Background(), "test.system"), newEngine())
	if err := authz.Require(sysOnly, "x.note.read"); err != nil {
		t.Fatalf("sistem tanpa principal: %v", err)
	}
}

func TestIDORTwoUsers(t *testing.T) {
	e := newEngine().
		AddUser(User{ID: "a", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"owner"}}}}).
		AddUser(User{ID: "b", Grants: []Grant{{Resource: "x.note", Actions: []string{"read"}, Scope: []string{"owner"}}}})
	noteOfA := map[string]any{"createdBy": "a"}
	if err := authz.Check(e.Context(context.Background(), "a"), "x.note.read", noteOfA); err != nil {
		t.Fatalf("A: %v", err)
	}
	if err := authz.Check(e.Context(context.Background(), "b"), "x.note.read", noteOfA); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("B harus 404: %v", err)
	}
}
