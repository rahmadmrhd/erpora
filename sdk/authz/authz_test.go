package authz

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"erpora/sdk/identity"
)

func principalCtx() context.Context {
	return identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", Kind: identity.KindHuman})
}

func TestNoEngineFailsClosed(t *testing.T) {
	cases := map[string]context.Context{
		"background":         context.Background(),
		"principal":          principalCtx(),
		"system tanpa mesin": AsSystem(context.Background(), "test.reason"),
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Require(ctx, "x.note.read"); err == nil {
				t.Error("Require harus galat")
			}
			if err := Check(ctx, "x.note.read", map[string]any{"a": 1}); err == nil {
				t.Error("Check harus galat")
			}
			if err := CheckWrite(ctx, "x.note", []string{"title"}); err == nil {
				t.Error("CheckWrite harus galat")
			} else {
				var ff *FieldForbiddenError
				if !errors.As(err, &ff) || !reflect.DeepEqual(ff.Fields, []string{"title"}) {
					t.Errorf("CheckWrite err = %v", err)
				}
			}
			fa := Fields(ctx, "x.note")
			if fa == nil || fa.Field("title") != AccessNone || fa.Group("general") != AccessNone {
				t.Error("Fields harus semua none")
			}
			if err := fa.Readable("title"); err == nil {
				t.Error("Readable harus galat")
			}
			if got, ok := MaskFields(ctx, "x.note", map[string]any{"title": "a"}).(map[string]any); !ok || len(got) != 0 {
				t.Errorf("MaskFields = %#v", got)
			}
			sql, _ := renderFilter(t, Filter(ctx, "x.note", "read"))
			if !hasFalse(sql) {
				t.Errorf("Filter harus WHERE false, got %q", sql)
			}
		})
	}
}

func TestRequireWithoutActorIsUnauthenticated(t *testing.T) {
	if err := Require(context.Background(), "x.note.read"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("tanpa engine: %v", err)
	}
	e := &stubEngine{require: func(context.Context, string) error { return nil }}
	ctx := WithEngine(context.Background(), e)
	if err := Require(ctx, "x.note.read"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("dengan engine tanpa principal: %v", err)
	}
	if e.calls != 0 {
		t.Error("engine tidak boleh dipanggil tanpa principal/hak sistem")
	}
	if err := Require(principalCtx(), "x.note.read"); !errors.Is(err, ErrForbidden) {
		t.Errorf("principal tanpa engine: %v", err)
	}
}

// openAccess adalah FieldAccess yang mengizinkan semuanya (hanya untuk uji).
type openAccess struct{}

func (openAccess) Group(string) Access      { return AccessWrite }
func (openAccess) Field(string) Access      { return AccessWrite }
func (openAccess) Readable(...string) error { return nil }

func TestNoPrincipalDenies(t *testing.T) {
	allow := &stubEngine{
		require:    func(context.Context, string) error { return nil },
		check:      func(context.Context, string, Record) error { return nil },
		checkWrite: func(context.Context, string, []string) error { return nil },
		filter: func(context.Context, string, string) (RowFilter, error) {
			return RowFilter{Branches: []RowBranch{{Conditions: []RowCondition{AllRows()}}}}, nil
		},
		fields:     func(context.Context, string) FieldAccess { return openAccess{} },
		maskFields: func(_ context.Context, _ string, f map[string]any) map[string]any { return f },
	}
	ctx := WithEngine(context.Background(), allow)
	if err := Check(ctx, "x.note.read", map[string]any{"a": 1}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Check tanpa principal harus ErrNotFound (FR-119), got %v", err)
	}
	if err := CheckWrite(ctx, "x.note", []string{"a"}); err == nil {
		t.Error("CheckWrite harus galat")
	}
	if fa := Fields(ctx, "x.note"); fa.Field("a") != AccessNone {
		t.Error("Fields harus none")
	}
	if got := MaskFields(ctx, "x.note", map[string]any{"a": 1}).(map[string]any); len(got) != 0 {
		t.Error("MaskFields harus kosong")
	}
	if sql, _ := renderFilter(t, Filter(ctx, "x.note", "read")); !hasFalse(sql) {
		t.Errorf("Filter: %q", sql)
	}
	if allow.calls != 0 {
		t.Errorf("engine terpanggil %d kali tanpa principal", allow.calls)
	}
}

func TestSystemReasonReachesEngineWithoutPrincipal(t *testing.T) {
	e := &stubEngine{require: func(ctx context.Context, p string) error {
		if r, ok := SystemReason(ctx); !ok || r != "test.reason" {
			t.Errorf("reason hilang: %q %v", r, ok)
		}
		return nil
	}}
	ctx := WithEngine(AsSystem(context.Background(), "test.reason"), e)
	if err := Require(ctx, "x.note.read"); err != nil {
		t.Fatalf("hak sistem harus diteruskan ke mesin: %v", err)
	}
}

func TestEnginePanicFailsClosed(t *testing.T) {
	boom := func() { panic("boom") }
	e := &stubEngine{
		require:    func(context.Context, string) error { boom(); return nil },
		filter:     func(context.Context, string, string) (RowFilter, error) { boom(); return RowFilter{}, nil },
		check:      func(context.Context, string, Record) error { boom(); return nil },
		fields:     func(context.Context, string) FieldAccess { boom(); return nil },
		maskFields: func(context.Context, string, map[string]any) map[string]any { boom(); return nil },
		checkWrite: func(context.Context, string, []string) error { boom(); return nil },
	}
	ctx := asUser(e)
	if err := Require(ctx, "x.note.read"); !errors.Is(err, ErrForbidden) {
		t.Errorf("Require: %v", err)
	}
	if err := Check(ctx, "x.note.read", map[string]any{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Check: %v", err)
	}
	if err := CheckWrite(ctx, "x.note", []string{"a"}); err == nil {
		t.Error("CheckWrite")
	}
	if Fields(ctx, "x.note").Field("a") != AccessNone {
		t.Error("Fields")
	}
	if got := MaskFields(ctx, "x.note", map[string]any{"a": 1}).(map[string]any); len(got) != 0 {
		t.Error("MaskFields")
	}
	if sql, _ := renderFilter(t, Filter(ctx, "x.note", "read")); !hasFalse(sql) {
		t.Errorf("Filter: %q", sql)
	}
}

func TestEngineNilResultsFailClosed(t *testing.T) {
	ctx := asUser(&stubEngine{}) // Fields/MaskFields bawaan stub mengembalikan nil
	if fa := Fields(ctx, "x.note"); fa == nil || fa.Field("a") != AccessNone {
		t.Error("Fields nil dari mesin harus menjadi semua none")
	}
	if got := MaskFields(ctx, "x.note", map[string]any{"a": 1}).(map[string]any); len(got) != 0 {
		t.Errorf("MaskFields nil: %#v", got)
	}
}

func TestEngineErrorsStayErrors(t *testing.T) {
	ctx := asUser(&stubEngine{require: func(context.Context, string) error { return ErrForbidden }})
	if err := Require(ctx, "x.note.read"); !errors.Is(err, ErrForbidden) {
		t.Errorf("%v", err)
	}
	// galat tak dikenal dari mesin tetap penolakan, bukan lolos
	ctx = asUser(&stubEngine{
		require: func(context.Context, string) error { return errors.New("db down") },
		check:   func(context.Context, string, Record) error { return errors.New("db down") },
	})
	if err := Require(ctx, "x.note.read"); err == nil {
		t.Error("galat mesin harus tetap galat")
	}
	if err := Check(ctx, "x.note.read", map[string]any{}); err == nil {
		t.Error("galat mesin harus tetap galat")
	}
}

type fakeRecord struct{ v map[string]any }

func (f fakeRecord) AuthzValue(field string) (any, bool) { v, ok := f.v[field]; return v, ok }

func TestCheckRejectsUnsupportedRecordType(t *testing.T) {
	e := &stubEngine{check: func(context.Context, string, Record) error { return nil }}
	ctx := asUser(e)
	type model struct{ ID string }
	for name, rec := range map[string]any{"struct": model{"1"}, "ptr": &model{"1"}, "nil": nil, "string": "x", "nil record": (*fakeRecord)(nil)} {
		err := Check(ctx, "x.note.read", rec)
		if !errors.Is(err, ErrUnsupportedRecord) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if e.calls != 0 {
		t.Error("mesin tidak boleh dipanggil untuk tipe tak didukung")
	}
	// Record dan map diterima dan diteruskan sebagai Record
	var got Record
	e.check = func(_ context.Context, _ string, rec Record) error { got = rec; return nil }
	if err := Check(ctx, "x.note.read", fakeRecord{map[string]any{"a": 1}}); err != nil || got == nil {
		t.Errorf("Record: %v", err)
	}
	if err := Check(ctx, "x.note.read", map[string]any{"createdBy": "u1"}); err != nil {
		t.Fatalf("map: %v", err)
	}
	if v, ok := got.AuthzValue("created_by"); !ok || v != "u1" {
		t.Errorf("map harus dikanonikalkan: %v %v", v, ok)
	}
}

func TestWithEngineBoxedAndNilSafe(t *testing.T) {
	ctx := WithEngine(context.Background(), nil)
	if _, ok := engineFrom(ctx); ok {
		t.Error("engine nil tidak boleh dianggap ada")
	}
	var typedNil *stubEngine
	if _, ok := engineFrom(WithEngine(context.Background(), typedNil)); ok {
		t.Error("nil bertipe tidak boleh dianggap ada")
	}
	// nilai mentah di bawah kunci tiruan tidak diterima: kotak tak diekspor
	if _, ok := engineFrom(context.WithValue(context.Background(), engineKey{}, &stubEngine{})); ok {
		t.Error("nilai mentah tanpa kotak harus ditolak")
	}
	if e, ok := engineFrom(WithEngine(context.Background(), &stubEngine{})); !ok || e == nil {
		t.Error("engine sah harus terbaca")
	}
}

func TestAdminBoxedAndNilSafe(t *testing.T) {
	if _, ok := AdminFrom(context.Background()); ok {
		t.Error("tanpa admin")
	}
	if _, ok := AdminFrom(WithAdmin(context.Background(), nil)); ok {
		t.Error("admin nil")
	}
	type fakeAdmin struct{}
	if a, ok := AdminFrom(WithAdmin(context.Background(), &fakeAdmin{})); !ok || a == nil {
		t.Error("admin sah harus terbaca")
	}
	var typedNil *fakeAdmin
	if _, ok := AdminFrom(WithAdmin(context.Background(), typedNil)); ok {
		t.Error("nil bertipe")
	}
}

func TestMaskFieldsPrefersEngineOverLegacyMasker(t *testing.T) {
	e := &stubEngine{maskFields: func(_ context.Context, _ string, f map[string]any) map[string]any {
		delete(f, "secret")
		return f
	}}
	ctx := WithFieldMasker(asUser(e), &dropMasker{drop: "title"})
	got := MaskFields(ctx, "x.note", map[string]any{"title": "a", "secret": "s"})
	if !reflect.DeepEqual(got, map[string]any{"title": "a"}) {
		t.Errorf("mesin harus diutamakan: %#v", got)
	}
}
