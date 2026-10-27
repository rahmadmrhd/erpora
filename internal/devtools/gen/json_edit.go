package gen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// jsonMember adalah satu anggota objek JSON teratas beserta posisinya di teks sumber.
type jsonMember struct {
	key                        string
	keyStart, valStart, valEnd int
}

// scanObject membaca anggota objek JSON teratas dari src dan mengembalikan posisi bytenya. Edit JSON di sini
// bersifat tekstual (splice) agar urutan kunci, komentar spasi, dan format bagian lain tidak berubah.
func scanObject(src []byte) ([]jsonMember, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON tidak valid: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("JSON harus berupa objek")
	}
	var out []jsonMember
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("JSON tidak valid: %w", err)
		}
		key, ok := kt.(string)
		if !ok {
			return nil, errors.New("JSON tidak valid: kunci bukan string")
		}
		keyEnd := int(dec.InputOffset())
		keyStart := bytes.LastIndexByte(src[:keyEnd-1], '"')
		valStart := keyEnd
		for valStart < len(src) && (src[valStart] == ':' || src[valStart] == ' ' || src[valStart] == '\t' || src[valStart] == '\n' || src[valStart] == '\r') {
			valStart++
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("JSON tidak valid: %w", err)
		}
		out = append(out, jsonMember{key: key, keyStart: keyStart, valStart: valStart, valEnd: int(dec.InputOffset())})
	}
	return out, nil
}

func findMember(ms []jsonMember, key string) (jsonMember, bool) {
	for _, m := range ms {
		if m.key == key {
			return m, true
		}
	}
	return jsonMember{}, false
}

// lineIndent mengembalikan indentasi (spasi/tab) baris yang memuat posisi pos.
func lineIndent(src []byte, pos int) string {
	start := bytes.LastIndexByte(src[:pos], '\n') + 1
	end := start
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return string(src[start:end])
}

func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

// AppendJSONArrayString menambah value ke array string pada kunci key di objek teratas, mempertahankan urutan kunci dan
// format. Array satu baris tetap satu baris; array multi-baris mendapat baris baru dengan indentasi elemen terakhir.
// value yang sudah ada membuat src tak berubah.
func AppendJSONArrayString(src []byte, key, value string) ([]byte, error) {
	ms, err := scanObject(src)
	if err != nil {
		return nil, err
	}
	m, ok := findMember(ms, key)
	if !ok {
		return nil, fmt.Errorf("kunci %q tidak ada di JSON", key)
	}
	raw := src[m.valStart:m.valEnd]
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("kunci %q bukan array string: %w", key, err)
	}
	if slices.Contains(items, value) {
		return src, nil
	}
	at := m.valEnd - 1 // posisi ']'
	enc := jsonString(value)
	var ins string
	switch {
	case len(items) == 0:
		ins = enc
	case !bytes.Contains(raw, []byte("\n")):
		ins = ", " + enc
	default:
		// Sisipkan tepat setelah elemen terakhir (sebelum whitespace penutup), dengan indentasi elemen itu.
		last := m.valStart + bytes.LastIndexByte(raw[:len(raw)-1], '"')
		at = last + 1
		ins = ",\n" + lineIndent(src, last) + enc
	}
	out := append([]byte{}, src[:at]...)
	out = append(out, ins...)
	return append(out, src[at:]...), nil
}

// AddJSONDependency menambah name: value ke objek section (mis. "dependencies") di objek teratas package.json.
// Kunci disisipkan terurut alfabet; seksi yang belum ada dibuat sebelum devDependencies (atau di akhir). Nama yang sudah
// ada membuat src tak berubah.
func AddJSONDependency(src []byte, section, name, value string) ([]byte, error) {
	ms, err := scanObject(src)
	if err != nil {
		return nil, err
	}
	unit := "  "
	if len(ms) > 0 {
		if ind := lineIndent(src, ms[0].keyStart); ind != "" {
			unit = ind
		}
	}
	entry := jsonString(name) + ": " + jsonString(value)
	sec, ok := findMember(ms, section)
	if !ok {
		block := jsonString(section) + ": {\n" + unit + unit + entry + "\n" + unit + "}"
		if dev, has := findMember(ms, "devDependencies"); has {
			out := append([]byte{}, src[:dev.keyStart]...)
			out = append(out, block+",\n"+unit...)
			return append(out, src[dev.keyStart:]...), nil
		}
		last := ms[len(ms)-1]
		out := append([]byte{}, src[:last.valEnd]...)
		out = append(out, ",\n"+unit+block...)
		return append(out, src[last.valEnd:]...), nil
	}
	body := src[sec.valStart:sec.valEnd]
	inner, err := scanObject(body)
	if err != nil {
		return nil, fmt.Errorf("%q bukan objek: %w", section, err)
	}
	for _, m := range inner {
		if m.key == name {
			return src, nil
		}
	}
	if len(inner) == 0 {
		out := append([]byte{}, src[:sec.valStart]...)
		out = append(out, "{\n"+unit+unit+entry+"\n"+unit+"}"...)
		return append(out, src[sec.valEnd:]...), nil
	}
	childIndent := lineIndent(body, inner[0].keyStart)
	for _, m := range inner {
		if m.key > name {
			at := sec.valStart + m.keyStart
			out := append([]byte{}, src[:at]...)
			out = append(out, entry+",\n"+childIndent...)
			return append(out, src[at:]...), nil
		}
	}
	at := sec.valStart + inner[len(inner)-1].valEnd
	out := append([]byte{}, src[:at]...)
	out = append(out, ",\n"+childIndent+entry...)
	return append(out, src[at:]...), nil
}
