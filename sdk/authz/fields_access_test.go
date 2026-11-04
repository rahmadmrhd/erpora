package authz

import (
	"errors"
	"reflect"
	"testing"
)

func testAccess() FieldAccess {
	return NewFieldAccess(FieldAccessSpec{
		Resource:     "x.item",
		DefaultGroup: "general",
		Groups:       map[string]Access{"general": AccessWrite, "profit": AccessNone, "detail": AccessRead},
		FieldGroups:  map[string]string{"cost_price": "profit", "margin": "profit", "note": "detail", "title": "general"},
	})
}

func TestMaskFieldsCamelCaseMatchesSnakeGroup(t *testing.T) {
	fa := testAccess()
	got := ApplyMask(fa, map[string]any{"title": "a", "costPrice": "9", "margin": "1", "note": "n", "extra": "e"})
	want := map[string]any{"title": "a", "note": "n", "extra": "e"} // extra: tak tercantum -> grup default (write)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestFieldAccessUnknownNameFailsClosed(t *testing.T) {
	fa := testAccess()
	for _, name := range []string{"costPrice", "cost_price", "margin"} {
		if fa.Field(name) != AccessNone {
			t.Errorf("Field(%q) harus none", name)
		}
	}
	// nama yang tidak dapat dikanonikalkan tidak boleh jatuh ke grup default
	for _, name := range []string{"", "taxID", "Margin", "x-y", "cost_Price"} {
		if got := fa.Field(name); got != AccessNone {
			t.Errorf("Field(%q) = %v, want none", name, got)
		}
	}
	if fa.Field("title") != AccessWrite || fa.Field("note") != AccessRead || fa.Field("unlisted") != AccessWrite {
		t.Error("grup default mengikuti akses grup default")
	}
	if fa.Group("profit") != AccessNone || fa.Group("detail") != AccessRead || fa.Group("nope") != AccessNone {
		t.Error("Group")
	}
}

func TestFieldAccessMissingDefaultGroupDeniesUnlisted(t *testing.T) {
	fa := NewFieldAccess(FieldAccessSpec{Resource: "x.item", Groups: map[string]Access{"a": AccessWrite}, FieldGroups: map[string]string{"t": "a"}})
	if fa.Field("t") != AccessWrite || fa.Field("other") != AccessNone {
		t.Error("tanpa DefaultGroup, field tak tercantum harus none")
	}
}

func TestReadableRejectsNoneForFilterAndSort(t *testing.T) {
	fa := testAccess()
	if err := fa.Readable("title", "note"); err != nil {
		t.Fatalf("Readable: %v", err)
	}
	if err := fa.Readable(); err != nil {
		t.Fatalf("Readable kosong: %v", err)
	}
	err := fa.Readable("title", "costPrice", "margin", "costPrice")
	var ff *FieldForbiddenError
	if !errors.As(err, &ff) {
		t.Fatalf("err = %v", err)
	}
	if ff.Resource != "x.item" || !reflect.DeepEqual(ff.Fields, []string{"costPrice", "margin"}) {
		t.Errorf("%#v", ff)
	}
}

func TestCheckWriteCamelCase(t *testing.T) {
	fa := testAccess()
	if err := CheckWriteAccess(fa, "x.item", []string{"title"}); err != nil {
		t.Fatalf("write ke grup write: %v", err)
	}
	if err := CheckWriteAccess(fa, "x.item", nil); err != nil {
		t.Fatalf("tanpa perubahan: %v", err)
	}
	err := CheckWriteAccess(fa, "x.item", []string{"title", "costPrice", "note", "costPrice"})
	var ff *FieldForbiddenError
	if !errors.As(err, &ff) {
		t.Fatalf("err = %v", err)
	}
	// read-only (note) dan none (costPrice) sama-sama ditolak; nama dipertahankan seperti diberikan
	if !reflect.DeepEqual(ff.Fields, []string{"costPrice", "note"}) || ff.Resource != "x.item" {
		t.Errorf("%#v", ff)
	}
}

func TestApplyMaskDoesNotMutateInput(t *testing.T) {
	in := map[string]any{"title": "a", "margin": "1"}
	_ = ApplyMask(testAccess(), in)
	if _, ok := in["margin"]; !ok {
		t.Error("input tidak boleh diubah")
	}
}

func TestApplyMaskNilAccessDeniesAll(t *testing.T) {
	if got := ApplyMask(nil, map[string]any{"a": 1}); len(got) != 0 {
		t.Errorf("got %#v", got)
	}
}

func TestAccessString(t *testing.T) {
	if AccessNone.String() != "none" || AccessRead.String() != "read" || AccessWrite.String() != "write" || Access(9).String() != "none" {
		t.Error("String")
	}
}
