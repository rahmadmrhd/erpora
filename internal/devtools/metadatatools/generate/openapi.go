package generate

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Skema OpenAPI dibangun sebagai node YAML (bukan teks): pengodean skalar oleh pustaka YAML menjamin semua
// literal (kutip, baris baru, tanda YAML, "*/", backtick) di-escape dengan benar (design D-32).

const generatedMarkerKey = "x-erp-generated"

// decimalRef adalah skema desimal bersama (pola kontrak; server lebih ketat: bentuk kanonik, presisi, skala).
const decimalRef = "erp:common#/components/schemas/Decimal"

func refNode() *yaml.Node {
	n := yMap()
	n.Style = yaml.FlowStyle
	yPut(n, "$ref", yStr(decimalRef))
	return n
}

func nullNode() *yaml.Node {
	n := yMap()
	n.Style = yaml.FlowStyle
	yPut(n, "type", yStr("null"))
	return n
}

type schemaMode int

const (
	modeRead schemaMode = iota
	modeCreate
	modeUpdate
)

type namedSchema struct {
	Name string
	Node *yaml.Node // mapping
}

func yMap() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }

func yPut(m *yaml.Node, key string, v *yaml.Node) {
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}

func yStr(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

func yBool(b bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(b)}
}

func yInt(text string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: text}
}

func yNull() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"} }

func ySeq(flow bool, items ...*yaml.Node) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
	if flow {
		n.Style = yaml.FlowStyle
	}
	return n
}

func yStrSeq(items []string, withNull bool) *yaml.Node {
	nodes := make([]*yaml.Node, 0, len(items)+1)
	for _, s := range items {
		nodes = append(nodes, yStr(s))
	}
	if withNull {
		nodes = append(nodes, yNull())
	}
	return ySeq(true, nodes...)
}

// yNum membuat skalar angka dari teks bilangan bulat kanonik.
func yNum(text string) *yaml.Node { return yInt(text) }

func yValue(v any) *yaml.Node {
	switch t := v.(type) {
	case nil:
		return yNull()
	case bool:
		return yBool(t)
	case string:
		return yStr(t)
	case json.Number:
		return yInt(t.String())
	}
	return yStr("")
}

// typeNode menyusun `type`: skalar, atau [tipe, 'null'] bila nullable (OpenAPI 3.1, selaras common.yaml).
func typeNode(t string, nullable bool) *yaml.Node {
	if !nullable {
		return yStr(t)
	}
	return ySeq(true, yStr(t), yStr("null"))
}

// propSchema membangun skema satu properti.
func propSchema(f *fld, mode schemaMode) (*yaml.Node, error) {
	n := yMap()
	nullable := f.Nullable
	switch f.Type {
	case "string", "text":
		yPut(n, "type", typeNode("string", nullable))
		if f.Format != "" {
			format := f.Format
			if format == "url" {
				format = "uri"
			}
			yPut(n, "format", yStr(format))
		}
		if f.MinLength > 0 {
			yPut(n, "minLength", yInt(strconv.Itoa(f.MinLength)))
		}
		if f.MaxLength > 0 {
			yPut(n, "maxLength", yInt(strconv.Itoa(f.MaxLength)))
		}
		if f.Pattern != "" {
			yPut(n, "pattern", yStr(f.Pattern))
		}
	case "integer":
		yPut(n, "type", typeNode("integer", nullable))
		yPut(n, "format", yStr("int64"))
		for _, b := range []struct{ key, val string }{{"minimum", f.Minimum}, {"maximum", f.Maximum}} {
			if b.val == "" {
				continue
			}
			t, ok := intText(b.val)
			if !ok {
				return nil, inputErr("field %s: %s integer bukan bilangan bulat", f.Name, b.key)
			}
			yPut(n, b.key, yNum(t))
		}
	case "decimal":
		// Linter kontrak (erp-decimal-not-number) mewajibkan $ref ke skema Decimal bersama; pada nullable dipakai
		// oneOf [Decimal, null] agar null tetap sah.
		if nullable {
			yPut(n, "oneOf", ySeq(false, refNode(), nullNode()))
		} else {
			yPut(n, "$ref", yStr(decimalRef))
		}
		if f.Precision > 0 || f.Scale > 0 || f.Minimum != "" || f.Maximum != "" {
			d := yMap()
			d.Style = yaml.FlowStyle
			if f.Precision > 0 {
				yPut(d, "precision", yInt(strconv.Itoa(f.Precision)))
			}
			if f.Scale > 0 {
				yPut(d, "scale", yInt(strconv.Itoa(f.Scale)))
			}
			if f.Minimum != "" {
				yPut(d, "minimum", yStr(f.Minimum))
			}
			if f.Maximum != "" {
				yPut(d, "maximum", yStr(f.Maximum))
			}
			yPut(n, "x-erp-decimal", d)
		}
	case "boolean":
		yPut(n, "type", typeNode("boolean", nullable))
	case "date":
		yPut(n, "type", typeNode("string", nullable))
		yPut(n, "format", yStr("date"))
	case "datetime":
		yPut(n, "type", typeNode("string", nullable))
		yPut(n, "format", yStr("date-time"))
	case "uuid":
		yPut(n, "type", typeNode("string", nullable))
		yPut(n, "format", yStr("uuid"))
	case "enum":
		yPut(n, "type", typeNode("string", nullable))
		if f.Morph {
			// Kunci resource bertitik bukan enum snake_case (erp-enum-snake-case); daftar putih lewat pola.
			quoted := make([]string, len(f.Enum))
			for i, k := range f.Enum {
				quoted[i] = regexp.QuoteMeta(k)
			}
			yPut(n, "pattern", yStr("^(?:"+strings.Join(quoted, "|")+")$"))
			break
		}
		yPut(n, "enum", yStrSeq(f.Enum, nullable))
		if f.Extensible {
			yPut(n, "x-erp-extensible-enum", yBool(true))
		}
	default:
		return nil, inputErr("field %s: tipe %q tidak dikenal", f.Name, f.Type)
	}
	if mode == modeCreate && f.HasDefault {
		switch {
		case f.Default == nil:
			yPut(n, "default", yNull())
		case f.Type == "boolean" || f.Type == "integer":
			yPut(n, "default", yValue(f.Default))
		default:
			yPut(n, "default", yStr(defaultText(f.Default)))
		}
	}
	if mode == modeRead && f.ReadOnly {
		yPut(n, "readOnly", yBool(true))
	}
	if f.Deprecated {
		yPut(n, "deprecated", yBool(true))
	}
	if f.Description != "" {
		yPut(n, "description", yStr(f.Description))
	}
	yPut(n, "x-erp-classification", yStr(f.Class))
	yPut(n, "x-erp-field-group", yStr(f.Group))
	if mode == modeRead {
		if f.Filterable {
			yPut(n, "x-erp-filterable", yBool(true))
		}
		if f.Sortable {
			yPut(n, "x-erp-sortable", yBool(true))
		}
	}
	return n, nil
}

func defaultText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	}
	return ""
}

func schemaName(m *resModel, mode schemaMode) string {
	switch mode {
	case modeCreate:
		return m.Pascal + "Create"
	case modeUpdate:
		return m.Pascal + "Update"
	}
	return m.Pascal
}

// buildSchemas membangun tiga skema resource: baca, buat, ubah (design Komponen 6).
func buildSchemas(m *resModel) ([]namedSchema, error) {
	var out []namedSchema
	for _, mode := range []schemaMode{modeRead, modeCreate, modeUpdate} {
		s := yMap()
		yPut(s, "type", yStr("object"))
		switch mode {
		case modeRead:
			if d := m.Res.Description; d != "" {
				yPut(s, "description", yStr(d))
			}
		case modeCreate:
			yPut(s, "description", yStr("Body pembuatan "+m.Res.Key+"; hanya field yang boleh ditulis."))
		default:
			yPut(s, "description", yStr("Body perubahan sebagian (PATCH) "+m.Res.Key+"; semua field opsional."))
		}
		var required []string
		props := yMap()
		for _, f := range m.Fields {
			if mode != modeRead && !f.Writable() {
				continue
			}
			p, err := propSchema(f, mode)
			if err != nil {
				return nil, err
			}
			yPut(props, f.Name, p)
			switch mode {
			case modeRead:
				// Field di luar grup default dapat dihilangkan dari respons (api-guidelines section 4.4).
				if f.Group == m.DefaultGroup {
					required = append(required, f.Name)
				}
			case modeCreate:
				if f.Required {
					required = append(required, f.Name)
				}
			}
		}
		if len(required) > 0 {
			yPut(s, "required", yStrSeq(required, false))
		}
		if mode != modeRead {
			yPut(s, "additionalProperties", yBool(false))
		}
		if mode == modeUpdate {
			yPut(s, "minProperties", yInt("1"))
		}
		yPut(s, "properties", props)
		yPut(s, generatedMarkerKey, yBool(true))
		out = append(out, namedSchema{Name: schemaName(m, mode), Node: s})
	}
	return out, nil
}
