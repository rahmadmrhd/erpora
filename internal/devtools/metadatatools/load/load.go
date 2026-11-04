package load

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Document adalah berkas metadata yang lolos pemuatan.
type Document struct {
	// File adalah jalur berkas seperti diberikan ke pemuat (untuk pesan galat).
	File string
	// Kind adalah resource, abstract, atau fieldType.
	Kind string
	// Key adalah kunci metadata (<slug>.<nama>).
	Key string
	// SchemaVersion adalah versi skema berkas.
	SchemaVersion string
	// Value adalah isi berkas sebagai nilai generik JSON-kompatibel (alias sudah diekspansi; angka berupa
	// json.Number). Tidak boleh diubah pemanggil.
	Value map[string]any

	pos map[string]Pos
}

// Pos mengembalikan posisi di berkas untuk JSON pointer (kunci untuk anggota objek). Pointer yang tidak
// ada dicocokkan ke leluhur terdekat; akar dokumen bila tidak ada.
func (d *Document) Pos(pointer string) Pos { return lookupPos(d.pos, pointer) }

// Decode memetakan isi berkas ke struct v lewat JSON (gunakan tag `json`). Bidang yang tidak ada di v
// diabaikan; skema sudah menjamin bentuknya.
func (d *Document) Decode(v any) error {
	b, err := json.Marshal(d.Value)
	if err != nil {
		return fmt.Errorf("%s: %w", d.File, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", d.File, err)
	}
	return nil
}

func lookupPos(pos map[string]Pos, pointer string) Pos {
	for {
		if p, ok := pos[pointer]; ok {
			return p
		}
		i := strings.LastIndexByte(pointer, '/')
		if i < 0 {
			return Pos{Line: 1, Column: 1}
		}
		pointer = pointer[:i]
	}
}

// Parse memuat satu berkas metadata dengan batas bawaan.
func Parse(file string, data []byte) (*Document, []Problem) {
	return ParseWithLimits(file, data, DefaultLimits())
}

// ParseWithLimits memuat satu berkas metadata: parse YAML aman, validasi skema, dan batas struktural. Semua
// temuan dikembalikan terurut menurut baris; dokumen hanya dikembalikan bila tidak ada temuan.
func ParseWithLimits(file string, data []byte, lim Limits) (*Document, []Problem) {
	c := &collector{file: file}
	if len(data) > lim.MaxFileBytes {
		c.add(nil, "", CodeFileTooLarge, fmt.Sprintf("berkas %d byte melebihi batas %d byte", len(data), lim.MaxFileBytes))
		return nil, c.probs
	}
	root := parseNode(c, data)
	if root == nil {
		return nil, c.probs
	}
	if !analyze(c, root, lim) {
		sortProblems(c.probs)
		return nil, c.probs
	}
	b := &builder{c: c, pos: map[string]Pos{"": {Line: root.Line, Column: root.Column}}}
	value, _ := b.value(root, "", nil).(map[string]any)

	var rest []Problem
	rest = append(rest, validateSchema(value)...)
	rest = append(rest, semanticChecks(value)...)
	for i := range rest {
		p := lookupPos(b.pos, rest[i].Pointer)
		rest[i].File, rest[i].Line, rest[i].Column = file, p.Line, p.Column
	}
	c.probs = append(c.probs, rest...)
	if len(c.probs) > 0 {
		sortProblems(c.probs)
		return nil, c.probs
	}
	doc := &Document{File: file, Value: value, pos: b.pos}
	doc.Kind, _ = value["kind"].(string)
	doc.Key, _ = value["key"].(string)
	doc.SchemaVersion, _ = value["schemaVersion"].(string)
	return doc, nil
}
