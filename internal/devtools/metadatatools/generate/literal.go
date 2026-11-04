package generate

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"erpora/sdk/metadata"
)

// goString adalah SATU-SATUNYA fungsi pengutip literal teks untuk keluaran Go (Requirement 19.9, design D-32):
// strconv.Quote menghasilkan literal Go yang sah untuk teks apa pun (kutip, garis miring terbalik, baris baru,
// backtick, "*/", "$$", dan byte tidak sah) sehingga masukan tidak pernah keluar dari literalnya.
func goString(s string) string { return strconv.Quote(s) }

// emitter menyusun literal Go dan mencatat impor yang dibutuhkan.
type emitter struct {
	usesJSON bool
}

var opConst = map[metadata.Op]string{
	metadata.OpEq: "metadata.OpEq", metadata.OpNe: "metadata.OpNe", metadata.OpGt: "metadata.OpGt",
	metadata.OpGte: "metadata.OpGte", metadata.OpLt: "metadata.OpLt", metadata.OpLte: "metadata.OpLte",
	metadata.OpIn: "metadata.OpIn", metadata.OpContains: "metadata.OpContains", metadata.OpIsNull: "metadata.OpIsNull",
}

// value menanam nilai literal kondisi atau default.
func (e *emitter) value(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "nil", nil
	case bool:
		return strconv.FormatBool(t), nil
	case string:
		return goString(t), nil
	case json.Number:
		// Teks angka dikutip; json.Number tidak menafsirkannya, sehingga teks apa pun tetap data.
		e.usesJSON = true
		return "json.Number(" + goString(t.String()) + ")", nil
	case int:
		return "int64(" + strconv.Itoa(t) + ")", nil
	case int64:
		return "int64(" + strconv.FormatInt(t, 10) + ")", nil
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return "", inputErr("literal float tidak berhingga")
		}
		return "float64(" + strconv.FormatFloat(t, 'g', -1, 64) + ")", nil
	case []any:
		parts := make([]string, len(t))
		for i, x := range t {
			s, err := e.value(x)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[]any{" + strings.Join(parts, ", ") + "}", nil
	}
	return "", inputErr("literal bertipe %T tidak didukung", v)
}

// cond menanam AST kondisi lewat konstruktor sdk/metadata (design D-2): tidak ada berkas yang ditafsirkan saat
// runtime.
func (e *emitter) cond(c *metadata.Condition) (string, error) {
	switch c.Kind {
	case metadata.CondAll, metadata.CondAny, metadata.CondNot:
		parts := make([]string, len(c.Children))
		for i, ch := range c.Children {
			s, err := e.cond(ch)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		fn := map[metadata.CondKind]string{metadata.CondAll: "All", metadata.CondAny: "Any", metadata.CondNot: "Not"}[c.Kind]
		return "metadata." + fn + "(" + strings.Join(parts, ", ") + ")", nil
	case metadata.CondLeaf:
		op, ok := opConst[c.Op]
		if !ok {
			return "", inputErr("operator %q tidak dikenal", c.Op)
		}
		switch {
		case c.Op == metadata.OpIsNull:
			null := true
			if b, ok := c.Value.(bool); ok {
				null = b
			}
			return "metadata.IsNull(" + goString(c.Field) + ", " + strconv.FormatBool(null) + ")", nil
		case c.Ref != "":
			return "metadata.CmpRef(" + goString(c.Field) + ", " + op + ", " + goString(c.Ref) + ")", nil
		}
		val, err := e.value(c.Value)
		if err != nil {
			return "", err
		}
		return "metadata.Cmp(" + goString(c.Field) + ", " + op + ", " + val + ")", nil
	}
	return "", inputErr("jenis kondisi tidak dikenal")
}

var typeConst = map[string]string{
	"string": "metadata.TypeString", "text": "metadata.TypeText", "integer": "metadata.TypeInteger",
	"decimal": "metadata.TypeDecimal", "boolean": "metadata.TypeBoolean", "date": "metadata.TypeDate",
	"datetime": "metadata.TypeDateTime", "enum": "metadata.TypeEnum", "uuid": "metadata.TypeUUID",
}

// fieldSpec menyusun literal FieldSpec satu field.
func (e *emitter) fieldSpec(f *fld) (string, error) {
	var b strings.Builder
	b.WriteString("{\n")
	kv := func(k, v string) { b.WriteString(k + ": " + v + ",\n") }
	kv("Name", goString(f.Name))
	kv("Type", typeConst[f.Type])
	flags := []struct {
		name string
		on   bool
	}{{"Nullable", f.Nullable}, {"ReadOnly", f.ReadOnly}, {"Computed", f.Computed}, {"Required", f.Required}}
	for _, fl := range flags {
		if fl.on {
			kv(fl.name, "true")
		}
	}
	if f.HasDefault {
		d, err := e.value(f.Default)
		if err != nil {
			return "", err
		}
		kv("HasDefault", "true")
		kv("Default", d)
	}
	if f.MinLength > 0 {
		kv("MinLength", strconv.Itoa(f.MinLength))
	}
	if f.MaxLength > 0 {
		kv("MaxLength", strconv.Itoa(f.MaxLength))
	}
	if f.Pattern != "" {
		kv("Pattern", goString(f.Pattern))
	}
	if f.Format != "" {
		kv("Format", goString(f.Format))
	}
	if len(f.Enum) > 0 {
		parts := make([]string, len(f.Enum))
		for i, v := range f.Enum {
			parts[i] = goString(v)
		}
		kv("Enum", "[]string{"+strings.Join(parts, ", ")+"}")
	}
	if f.Extensible {
		kv("Extensible", "true")
	}
	if f.Minimum != "" {
		kv("Minimum", goString(f.Minimum))
	}
	if f.Maximum != "" {
		kv("Maximum", goString(f.Maximum))
	}
	if f.Precision > 0 {
		kv("Precision", strconv.Itoa(f.Precision))
	}
	if f.Scale > 0 {
		kv("Scale", strconv.Itoa(f.Scale))
	}
	kv("Classification", goString(f.Class))
	kv("Group", goString(f.Group))
	for _, c := range []struct {
		name string
		c    *metadata.Condition
	}{{"RequiredWhen", f.RequiredWhen}, {"ReadonlyWhen", f.ReadonlyWhen}} {
		if c.c == nil {
			continue
		}
		s, err := e.cond(c.c)
		if err != nil {
			return "", fmt.Errorf("field %s: %w", f.Name, err)
		}
		kv(c.name, s)
	}
	if f.MigrateFrom != "" {
		kv("MigrateFrom", goString(f.MigrateFrom))
	}
	if f.Deprecated {
		kv("Deprecated", "true")
	}
	b.WriteString("}")
	return b.String(), nil
}

// statusSpec menyusun literal StatusSpec (tabel nilai dan transisi) untuk lifecycle atau satu sumbu.
func statusSpec(sm *statusModel) string {
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString("Field: " + goString(sm.Field) + ",\n")
	b.WriteString("Initial: " + goString(sm.Initial) + ",\n")
	vals := make([]string, len(sm.Values))
	for i, v := range sm.Values {
		vals[i] = goString(v)
	}
	b.WriteString("Values: []string{" + strings.Join(vals, ", ") + "},\n")
	b.WriteString("Transitions: []Transition{\n")
	for _, t := range sm.Transitions {
		b.WriteString("{" + goString(t[0]) + ", " + goString(t[1]) + "},\n")
	}
	b.WriteString("},\n}")
	return b.String()
}

func (e *emitter) ruleSpec(r ruleModel) (string, error) {
	c, err := e.cond(r.Assert)
	if err != nil {
		return "", fmt.Errorf("aturan %s: %w", r.ID, err)
	}
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString("ID: " + goString(r.ID) + ",\n")
	b.WriteString("Code: " + goString(r.Code) + ",\n")
	if r.Field != "" {
		b.WriteString("Field: " + goString(r.Field) + ",\n")
	}
	b.WriteString("MessageKey: " + goString(r.Message) + ",\n")
	b.WriteString("Assert: " + c + ",\n")
	if r.ServerOnly {
		b.WriteString("ServerOnly: true,\n")
	}
	b.WriteString("}")
	return b.String(), nil
}
