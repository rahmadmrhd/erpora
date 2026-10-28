package authz

import (
	"context"
	"maps"
	"reflect"
	"testing"
)

type dropMasker struct {
	drop     string
	resource string
}

func (m *dropMasker) MaskFields(_ context.Context, resource string, fields map[string]any) map[string]any {
	m.resource = resource
	out := maps.Clone(fields)
	delete(out, m.drop)
	return out
}

func TestMaskFields_NoMaskerFailsClosed(t *testing.T) {
	ctx := context.Background()
	got, ok := MaskFields(ctx, "x.note", map[string]any{"title": "a"}).(map[string]any)
	if !ok || len(got) != 0 {
		t.Errorf("got %#v, want map kosong", got)
	}
	if HasFieldMasker(ctx) {
		t.Error("HasFieldMasker harus false")
	}
}

func TestMaskFields_UsesMaskerOnCopy(t *testing.T) {
	m := &dropMasker{drop: "cost"}
	ctx := WithFieldMasker(context.Background(), m)
	in := map[string]any{"title": "a", "cost": "9"}
	got := MaskFields(ctx, "x.note", in)
	if !reflect.DeepEqual(got, map[string]any{"title": "a"}) {
		t.Errorf("got %#v", got)
	}
	if _, ok := in["cost"]; !ok {
		t.Error("input asli tidak boleh diubah")
	}
	if m.resource != "x.note" || !HasFieldMasker(ctx) {
		t.Errorf("resource = %q", m.resource)
	}
}

func TestMaskFields_StructDTO(t *testing.T) {
	type dto struct {
		Title string `json:"title"`
		Cost  string `json:"cost"`
	}
	ctx := WithFieldMasker(context.Background(), &dropMasker{drop: "cost"})
	for name, in := range map[string]any{"value": dto{"a", "9"}, "pointer": &dto{"a", "9"}} {
		got := MaskFields(ctx, "x.note", in)
		if !reflect.DeepEqual(got, map[string]any{"title": "a"}) {
			t.Errorf("%s: got %#v", name, got)
		}
	}
}

func TestMaskFields_NonObject(t *testing.T) {
	ctx := WithFieldMasker(context.Background(), &dropMasker{drop: "x"})
	for _, in := range []any{[]int{1}, "x", 3, nil} {
		got, ok := MaskFields(ctx, "x.note", in).(map[string]any)
		if !ok || len(got) != 0 {
			t.Errorf("%#v -> %#v, want map kosong", in, got)
		}
	}
}
