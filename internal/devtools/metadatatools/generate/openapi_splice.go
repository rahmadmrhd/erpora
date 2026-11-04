package generate

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Penyuntikan terstruktur skema bangkitan ke components.schemas pada openapi.yaml (Requirement 8.3).
//
// Seperti internal/devtools/gen (openapi_edit.go), suntingan dilakukan sebagai splice teks yang posisinya
// dihitung dari yaml.Node, sehingga komentar, urutan, dan gaya bagian lain yang tidak disentuh tetap persis
// sama. Skema bangkitan dikenali dari penanda x-erp-generated: true; skema bernama sama tanpa penanda adalah
// tulis tangan dan tidak pernah ditimpa (ErrConflict).

type lineEdit struct {
	start, end int // baris 0-based, inklusif; end < start berarti sisip sebelum start
	text       []string
}

func splitLines(s string) []string { return strings.Split(s, "\n") }

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// blockEnd mengembalikan indeks baris terakhir yang bukan kosong atau komentar kepala kunci berikutnya sebelum
// baris `next` (eksklusif).
func blockEnd(lines []string, from, next int) int {
	end := min(next, len(lines)) - 1
	nextIndent := -1
	if next < len(lines) {
		nextIndent = indentOf(lines[next])
	}
	for end > from {
		t := strings.TrimSpace(lines[end])
		if t == "" || (strings.HasPrefix(t, "#") && nextIndent >= 0 && indentOf(lines[end]) <= nextIndent) {
			end--
			continue
		}
		break
	}
	return end
}

func childEnd(lines []string, m *yaml.Node, i, limit int) int {
	next := limit
	if i+2 < len(m.Content) {
		next = m.Content[i+2].Line - 1
	}
	return blockEnd(lines, m.Content[i].Line-1, next)
}

func mapEntry(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func isGenerated(schema *yaml.Node) bool {
	if schema == nil || schema.Kind != yaml.MappingNode {
		return false
	}
	i := mapEntry(schema, generatedMarkerKey)
	return i >= 0 && schema.Content[i+1].Kind == yaml.ScalarNode && schema.Content[i+1].Value == "true"
}

func encodeNode(n *yaml.Node) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// schemaLines mengodekan satu skema bernama pada indentasi tertentu.
func schemaLines(s namedSchema, indent int) ([]string, error) {
	wrapper := yMap()
	yPut(wrapper, s.Name, s.Node)
	text, err := encodeNode(wrapper)
	if err != nil {
		return nil, err
	}
	pad := strings.Repeat(" ", indent)
	var out []string
	for _, l := range splitLines(text) {
		if strings.TrimSpace(l) == "" {
			out = append(out, "")
		} else {
			out = append(out, pad+l)
		}
	}
	return out, nil
}

func applyEdits(lines []string, edits []lineEdit) []string {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		if e.end < e.start { // sisip sebelum start
			lines = append(lines[:e.start], append(append([]string{}, e.text...), lines[e.start:]...)...)
			continue
		}
		tail := append([]string{}, lines[e.end+1:]...)
		lines = append(append(lines[:e.start], e.text...), tail...)
	}
	return lines
}

// SpliceSchemas menyuntikkan, mengganti, dan membuang skema bangkitan pada src (isi openapi.yaml) dan
// mengembalikan isi baru. Skema bangkitan yang tidak ada lagi di want dibuang (yatim).
func SpliceSchemas(src []byte, want []namedSchema) ([]byte, error) {
	text := string(src)
	crlf := strings.Contains(text, "\r\n")
	if crlf {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	out, err := spliceLF(text, want)
	if err != nil {
		return nil, err
	}
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return []byte(out), nil
}

func spliceLF(text string, want []namedSchema) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return "", fmt.Errorf("openapi.yaml tidak valid: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return "", errors.New("openapi.yaml harus berupa objek")
	}
	root := doc.Content[0]
	lines := splitLines(text)
	if len(want) == 0 && mapEntry(root, "components") < 0 {
		return text, nil
	}

	// Menyusun teks entri baru (indentasi diisi setelah posisi diketahui).
	render := func(indent int, items []namedSchema) ([]string, error) {
		var all []string
		for _, s := range items {
			l, err := schemaLines(s, indent)
			if err != nil {
				return nil, err
			}
			all = append(all, l...)
		}
		return all, nil
	}
	trimTrailingBlank := func(ls []string) []string {
		for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
			ls = ls[:len(ls)-1]
		}
		return ls
	}

	ci := mapEntry(root, "components")
	if ci < 0 {
		body, err := render(4, want)
		if err != nil {
			return "", err
		}
		lines = trimTrailingBlank(lines)
		lines = append(lines, "components:", "  schemas:")
		lines = append(lines, body...)
		return strings.Join(lines, "\n") + "\n", nil
	}
	comps := root.Content[ci+1]
	if comps.Kind != yaml.MappingNode || (len(comps.Content) == 0 && comps.Style&yaml.FlowStyle != 0) {
		return "", errors.New("`components` bukan block mapping; ubah manual agar berbentuk block lalu ulangi")
	}
	compsLimit := len(lines)
	if ci+2 < len(root.Content) {
		compsLimit = root.Content[ci+2].Line - 1
	}
	si := mapEntry(comps, "schemas")
	if si < 0 {
		if len(want) == 0 {
			return text, nil
		}
		indent := 2
		if len(comps.Content) > 0 {
			indent = comps.Content[0].Column - 1
		}
		body, err := render(indent+2, want)
		if err != nil {
			return "", err
		}
		end := childEnd(lines, root, ci, len(lines))
		add := append([]string{strings.Repeat(" ", indent) + "schemas:"}, body...)
		return strings.Join(applyEdits(lines, []lineEdit{{start: end + 1, end: end, text: add}}), "\n"), nil
	}
	schemas := comps.Content[si+1]
	keyLine := comps.Content[si].Line - 1
	keyIndent := comps.Content[si].Column - 1
	if schemas.Kind == yaml.ScalarNode && schemas.Tag == "!!null" || (schemas.Kind == yaml.MappingNode && len(schemas.Content) == 0) {
		if len(want) == 0 {
			return text, nil
		}
		body, err := render(keyIndent+2, want)
		if err != nil {
			return "", err
		}
		repl := append([]string{strings.Repeat(" ", keyIndent) + "schemas:"}, body...)
		return strings.Join(applyEdits(lines, []lineEdit{{start: keyLine, end: keyLine, text: repl}}), "\n"), nil
	}
	if schemas.Kind != yaml.MappingNode || schemas.Style&yaml.FlowStyle != 0 {
		return "", errors.New("`components.schemas` bukan block mapping; ubah manual agar berbentuk block lalu ulangi")
	}
	childIndent := schemas.Content[0].Column - 1
	limit := compsLimit
	if si+2 < len(comps.Content) {
		limit = comps.Content[si+2].Line - 1
	}

	wantByName := map[string]namedSchema{}
	for _, w := range want {
		wantByName[w.Name] = w
	}
	existing := map[string]int{}
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		existing[schemas.Content[i].Value] = i
	}
	var edits []lineEdit
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		name := schemas.Content[i].Value
		val := schemas.Content[i+1]
		w, wanted := wantByName[name]
		start, end := schemas.Content[i].Line-1, childEnd(lines, schemas, i, limit)
		switch {
		case wanted && !isGenerated(val):
			return "", fmt.Errorf("%w: components.schemas.%s sudah ada dan bukan hasil generator (tanpa %s: true); ganti nama skema tulis tangan atau resource", ErrConflict, name, generatedMarkerKey)
		case wanted:
			l, err := schemaLines(w, childIndent)
			if err != nil {
				return "", err
			}
			edits = append(edits, lineEdit{start: start, end: end, text: l})
		case isGenerated(val):
			edits = append(edits, lineEdit{start: start, end: end, text: nil}) // yatim
		}
	}
	var fresh []namedSchema
	for _, w := range want {
		if _, ok := existing[w.Name]; !ok {
			fresh = append(fresh, w)
		}
	}
	if len(fresh) > 0 {
		body, err := render(childIndent, fresh)
		if err != nil {
			return "", err
		}
		last := childEnd(lines, schemas, len(schemas.Content)-2, limit)
		edits = append(edits, lineEdit{start: last + 1, end: last, text: body})
	}
	if len(edits) == 0 {
		return text, nil
	}
	return strings.Join(applyEdits(lines, edits), "\n"), nil
}

// GeneratedSchemaNames mengembalikan nama skema bertanda x-erp-generated pada src (terurut).
func GeneratedSchemaNames(src []byte) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, errors.New("openapi.yaml harus berupa objek")
	}
	root := doc.Content[0]
	comps := mapValue(root, "components")
	if comps == nil {
		return nil, nil
	}
	schemas := mapValue(comps, "schemas")
	if schemas == nil || schemas.Kind != yaml.MappingNode {
		return nil, nil
	}
	var names []string
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		if isGenerated(schemas.Content[i+1]) {
			names = append(names, schemas.Content[i].Value)
		}
	}
	sort.Strings(names)
	return names, nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if i := mapEntry(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}
