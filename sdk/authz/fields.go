package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
)

type fieldMaskerKey struct{}

// FieldMasker diimplementasi platform: stub sebelum PRD-001, akses level field (FR-112) sesudahnya.
// fields adalah salinan yang boleh diubah; hasilnya hanya memuat field yang boleh dilihat principal.
type FieldMasker interface {
	MaskFields(ctx context.Context, resource string, fields map[string]any) map[string]any
}

// WithFieldMasker memasang masker di ctx; dipanggil platform (runtime.Mount, ModuleContext).
func WithFieldMasker(ctx context.Context, m FieldMasker) context.Context {
	return context.WithValue(ctx, fieldMaskerKey{}, m)
}

// HasFieldMasker melaporkan apakah ctx membawa masker.
func HasFieldMasker(ctx context.Context) bool {
	m, ok := ctx.Value(fieldMaskerKey{}).(FieldMasker)
	return ok && m != nil
}

// MaskFields menerapkan masking field resource pada dto (signature PRD-001 FR-115). dto berupa
// map[string]any (disalin) atau struct/pointer struct (dikonversi lewat JSON dengan UseNumber). Tanpa
// masker di ctx, atau dto bukan objek, hasilnya map[string]any{} (gagal tertutup).
func MaskFields(ctx context.Context, resource string, dto any) any {
	m, ok := ctx.Value(fieldMaskerKey{}).(FieldMasker)
	if !ok || m == nil {
		return map[string]any{}
	}
	fields, ok := toFieldMap(dto)
	if !ok {
		return map[string]any{}
	}
	out := m.MaskFields(ctx, resource, fields)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func toFieldMap(dto any) (map[string]any, bool) {
	if dto == nil {
		return nil, false
	}
	if mp, ok := dto.(map[string]any); ok {
		return maps.Clone(mp), true
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil || out == nil {
		return nil, false
	}
	return out, true
}
