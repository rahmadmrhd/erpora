package metadata

import "strings"

// LabelTemplate adalah templat label yang sudah diurai (Requirement 5.5): teks tetap dan placeholder {field}.
// Hasil render selalu teks biasa (tidak pernah HTML).
type LabelTemplate struct {
	segs []labelSeg
}

type labelSeg struct {
	lit   string
	field string // tidak kosong: placeholder
}

// LabelResult adalah hasil render. Placeholder bernilai null, tidak tersedia, atau tidak sah dirender
// sebagai teks kosong; sebab yang bukan null dicatat di Reasons (path berbentuk "{namaField}").
type LabelResult struct {
	Text    string
	Reasons []Reason
}

// isPlaceholderName: ^[a-z][A-Za-z0-9]*$ dengan panjang <= MaxIdentifierLen. Keabsahan nama sebagai
// identifier metadata (camelCase ketat) diperiksa statis oleh metadata:check, bukan di sini.
func isPlaceholderName(s string) bool {
	if len(s) == 0 || len(s) > MaxIdentifierLen || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// ParseLabelTemplate menguraikan templat. Galat berupa *Error (kode invalid_template).
func ParseLabelTemplate(tpl string) (*LabelTemplate, error) {
	bad := &Error{Reason{ReasonInvalidTemplate, "$"}}
	if tooLongTemplate(tpl) {
		return nil, bad
	}
	var segs []labelSeg
	var lit strings.Builder
	placeholders := 0
	for i := 0; i < len(tpl); i++ {
		switch c := tpl[i]; c {
		case '{':
			end := strings.IndexByte(tpl[i+1:], '}')
			if end < 0 {
				return nil, bad
			}
			name := tpl[i+1 : i+1+end]
			if !isPlaceholderName(name) {
				return nil, bad
			}
			if placeholders++; placeholders > MaxTemplatePlaceholders {
				return nil, bad
			}
			if lit.Len() > 0 {
				segs = append(segs, labelSeg{lit: lit.String()})
				lit.Reset()
			}
			segs = append(segs, labelSeg{field: name})
			i += end + 1
		case '}':
			return nil, bad
		default:
			lit.WriteByte(c)
		}
	}
	if lit.Len() > 0 {
		segs = append(segs, labelSeg{lit: lit.String()})
	}
	return &LabelTemplate{segs: segs}, nil
}

// tooLongTemplate: panjang dalam satuan UTF-16 melebihi MaxTemplateLen.
func tooLongTemplate(s string) bool {
	if len(s) <= MaxTemplateLen {
		return false
	}
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n > MaxTemplateLen
}

// Render menyusun label dari record. Fungsi total.
func (t *LabelTemplate) Render(types Types, rec Record) LabelResult {
	if t == nil {
		return LabelResult{}
	}
	ev := &evaluator{types: types, rec: rec}
	var sb strings.Builder
	var rs []Reason
	for _, s := range t.segs {
		if s.field == "" {
			sb.WriteString(normText(s.lit))
			continue
		}
		v, code := ev.fieldValue(s.field)
		if code != "" {
			rs = append(rs, Reason{code, "{" + s.field + "}"})
			continue
		}
		sb.WriteString(valueText(v))
	}
	return LabelResult{Text: sb.String(), Reasons: rs}
}
