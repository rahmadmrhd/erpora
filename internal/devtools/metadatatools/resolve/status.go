package resolve

import (
	"fmt"
	"slices"
)

// lifecycleTransitions adalah tabel transisi `status` dokumen baku (K23, Requirement 13.4, design D-38). Tabel
// ini milik metadata bawaan sdk.document: resource tidak dapat menambah atau mengubah nilai maupun transisi
// pada rilis pertama (penambahan kelak adalah perluasan aditif). Nilai-nilainya (urutan deklarasi) berada pada
// field status sdk/metadata/builtin/document.abstract.yaml; tes memastikan keduanya konsisten.
var lifecycleTransitions = []Transition{
	{"draft", "deleted"},
	{"draft", "accepted"},
	{"draft", "pending_approval"},
	{"pending_approval", "accepted"},
	{"pending_approval", "rejected"},
	{"accepted", "cancelled"},
	{"accepted", "done"},
}

// LifecycleTransitions mengembalikan salinan tabel transisi lifecycle bawaan.
func LifecycleTransitions() []Transition { return slices.Clone(lifecycleTransitions) }

func stringList(v any) []string {
	var out []string
	if list, ok := v.([]any); ok {
		for _, e := range list {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// statusAxes menyelesaikan statusAxes dokumen sendiri menjadi field enum readOnly bukan-nullable (nilai awal =
// default) dan entri StatusAxes berurutan menurut deklarasi (K24). Pemeriksaan semantik (initial, pasangan,
// resource dokumen) dilakukan metadata:check; di sini hanya bentrok nama dengan field lain yang ditolak.
func (b *builder) statusAxes() {
	raw, _ := b.doc().Value["statusAxes"].(map[string]any)
	for _, name := range b.orderedKeys(raw, "/statusAxes") {
		ptr := "/statusAxes/" + esc(name)
		ax, _ := raw[name].(map[string]any)
		if prev := b.index[name]; prev != nil {
			b.fail(ptr, CodeDuplicateField, fmt.Sprintf(
				"sumbu status %q bertabrakan dengan field %s yang sudah ada (dari %s, baris %d)", name, name, prev.Origin.Doc, prev.Origin.Line))
			continue
		}
		axis := &StatusAxis{Name: name, Values: stringList(ax["values"]), Origin: b.origin(ptr, OpDeclared)}
		axis.Initial, _ = ax["initial"].(string)
		if list, ok := ax["transitions"].([]any); ok {
			for _, e := range list {
				if m, ok := e.(map[string]any); ok {
					from, _ := m["from"].(string)
					to, _ := m["to"].(string)
					axis.Transitions = append(axis.Transitions, Transition{From: from, To: to})
				}
			}
		}
		vals := make([]any, len(axis.Values))
		for i, v := range axis.Values {
			vals[i] = v
		}
		f := &Field{
			Name: name, SourceName: name, IsStatusAxis: true,
			Attrs:  map[string]any{"type": "enum", "values": vals, "default": axis.Initial, "readOnly": true},
			Origin: axis.Origin,
			AttrOrigin: map[string]Origin{
				"type":     axis.Origin,
				"readOnly": axis.Origin,
				"values":   b.origin(ptr+"/values", OpDeclared),
				"default":  b.origin(ptr+"/initial", OpDeclared),
			},
		}
		b.fields = append(b.fields, f)
		b.index[name] = f
		b.res.StatusAxes = append(b.res.StatusAxes, axis)
	}
}

// lifecycle mengisi Resolved.Lifecycle bila status berasal dari sdk.document (dokumen).
func (b *builder) lifecycle() {
	st := b.index["status"]
	if st == nil || st.ProtectedBy != KeyDocument {
		return
	}
	b.res.Lifecycle = &Lifecycle{
		Values:      stringList(st.Attrs["values"]),
		Initial:     st.Str("default"),
		Transitions: LifecycleTransitions(),
		Origin:      st.AttrOrigin["values"],
	}
}
