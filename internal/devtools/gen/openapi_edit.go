package gen

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Edit openapi.yaml dilakukan lewat splice teks yang posisinya dihitung dari yaml.Node (nomor baris), bukan dengan
// meng-encode ulang seluruh dokumen: komentar, urutan, gaya flow, dan pembungkusan baris di bagian yang tidak disentuh
// tetap persis sama. yaml.Node dipakai untuk menemukan posisi dan memeriksa duplikat.

// decodeSpec mengurai openapi.yaml dan mengembalikan node mapping akar.
func decodeSpec(src []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("openapi.yaml tidak valid: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("openapi.yaml harus berupa objek")
	}
	return doc.Content[0], nil
}

// mapEntry mengembalikan indeks pasangan (kunci di i, nilai di i+1) untuk key di mapping m; -1 bila tidak ada.
func mapEntry(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if i := mapEntry(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

// HasOperation melaporkan apakah (path, method) sudah ada di openapi.yaml.
func HasOperation(src []byte, path, method string) (bool, error) {
	root, err := decodeSpec(src)
	if err != nil {
		return false, err
	}
	paths := mapValue(root, "paths")
	if paths == nil {
		return false, nil
	}
	item := mapValue(paths, path)
	return item != nil && mapValue(item, method) != nil, nil
}

// blockEnd mengembalikan indeks baris (0-based) terakhir yang tidak kosong sebelum baris `next` (0-based, eksklusif).
func blockEnd(lines []string, from, next int) int {
	end := min(next, len(lines)) - 1
	nextIndent := -1
	if next < len(lines) {
		nextIndent = indentOf(lines[next])
	}
	for end > from {
		t := strings.TrimSpace(lines[end])
		// Komentar yang sejajar (atau lebih dangkal) dengan kunci berikutnya adalah komentar kepala kunci itu.
		if t == "" || (strings.HasPrefix(t, "#") && nextIndent >= 0 && indentOf(lines[end]) <= nextIndent) {
			end--
			continue
		}
		break
	}
	return end
}

// childEnd menghitung akhir (indeks baris 0-based) entri ke-i (pasangan kunci/nilai) pada mapping m yang berada di
// dalam mapping induk dengan batas bawah `limit` (baris 0-based kunci sesudah mapping m).
func childEnd(lines []string, m *yaml.Node, i, limit int) int {
	next := limit
	if i+2 < len(m.Content) {
		next = m.Content[i+2].Line - 1
	}
	return blockEnd(lines, m.Content[i].Line-1, next)
}

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

func indentText(text string, n int) []string {
	pad := strings.Repeat(" ", n)
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			out = append(out, "")
		} else {
			out = append(out, pad+l)
		}
	}
	return out
}

func insertLines(lines []string, after int, add []string) []string {
	out := make([]string, 0, len(lines)+len(add))
	out = append(out, lines[:after+1]...)
	out = append(out, add...)
	return append(out, lines[after+1:]...)
}

// topLimit mengembalikan batas baris (0-based, eksklusif) sesudah entri top-level ke-i di root: baris kunci berikutnya.
func topLimit(root *yaml.Node, i, total int) int {
	if i+2 < len(root.Content) {
		return root.Content[i+2].Line - 1
	}
	return total
}

// AddOperationText menambah operasi (opText = isi operasi sebagai YAML mapping, indentasi nol) pada path dengan method
// huruf kecil. Path item dibuat bila belum ada (di akhir `paths`); operasi yang sudah ada ditolak.
func AddOperationText(src []byte, path, method string, opText []byte) ([]byte, error) {
	root, err := decodeSpec(src)
	if err != nil {
		return nil, err
	}
	pi := mapEntry(root, "paths")
	if pi < 0 {
		return nil, errors.New("openapi.yaml tidak punya `paths`")
	}
	paths := root.Content[pi+1]
	if paths.Kind != yaml.MappingNode || len(paths.Content) == 0 {
		return nil, errors.New("`paths` kosong atau bukan block mapping; tambahkan satu path manual dulu")
	}
	lines := strings.Split(string(src), "\n")
	limit := topLimit(root, pi, len(lines))
	pathIndent := paths.Content[0].Column - 1

	if ii := mapEntry(paths, path); ii >= 0 {
		item := paths.Content[ii+1]
		if mapValue(item, method) != nil {
			return nil, fmt.Errorf("operasi %s %s sudah ada di openapi.yaml", method, path)
		}
		if item.Kind != yaml.MappingNode || len(item.Content) == 0 {
			return nil, fmt.Errorf("path item %s kosong atau bukan block mapping", path)
		}
		end := childEnd(lines, paths, ii, limit)
		methodIndent := item.Content[0].Column - 1
		add := append([]string{strings.Repeat(" ", methodIndent) + method + ":"}, indentText(string(opText), methodIndent+2)...)
		return []byte(strings.Join(insertLines(lines, end, add), "\n")), nil
	}

	end := childEnd(lines, paths, len(paths.Content)-2, limit)
	add := []string{strings.Repeat(" ", pathIndent) + path + ":", strings.Repeat(" ", pathIndent+2) + method + ":"}
	add = append(add, indentText(string(opText), pathIndent+4)...)
	return []byte(strings.Join(insertLines(lines, end, add), "\n")), nil
}

// AddOperation seperti AddOperationText, tetapi operasi diberikan sebagai yaml.Node (di-encode dengan indentasi 2).
func AddOperation(src []byte, path, method string, op *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(op); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return AddOperationText(src, path, method, buf.Bytes())
}

// AddComponentSchemasText menambah skema (schemasText = mapping nama -> skema, indentasi nol) ke components.schemas;
// nama yang sudah ada ditolak. `schemas: {}` kosong diganti menjadi block.
func AddComponentSchemasText(src []byte, schemasText []byte) ([]byte, error) {
	root, err := decodeSpec(src)
	if err != nil {
		return nil, err
	}
	incoming, err := parseYAMLMapping(schemasText)
	if err != nil {
		return nil, err
	}
	ci := mapEntry(root, "components")
	if ci < 0 {
		return nil, errors.New("openapi.yaml tidak punya `components`")
	}
	comps := root.Content[ci+1]
	si := mapEntry(comps, "schemas")
	if si < 0 {
		return nil, errors.New("openapi.yaml tidak punya `components.schemas`")
	}
	schemas := comps.Content[si+1]
	for i := 0; i+1 < len(incoming.Content); i += 2 {
		if mapValue(schemas, incoming.Content[i].Value) != nil {
			return nil, fmt.Errorf("skema %s sudah ada di components.schemas openapi.yaml", incoming.Content[i].Value)
		}
	}
	lines := strings.Split(string(src), "\n")
	keyLine := comps.Content[si].Line - 1
	keyIndent := comps.Content[si].Column - 1
	if len(schemas.Content) == 0 { // `schemas: {}`
		lines[keyLine] = strings.Repeat(" ", keyIndent) + "schemas:"
		return []byte(strings.Join(insertLines(lines, keyLine, indentText(string(schemasText), keyIndent+2)), "\n")), nil
	}
	// Batas bawah: baris kunci berikutnya di `components` atau di root, atau akhir file.
	limit := topLimit(root, ci, len(lines))
	if si+2 < len(comps.Content) {
		limit = comps.Content[si+2].Line - 1
	}
	end := childEnd(lines, schemas, len(schemas.Content)-2, limit)
	return []byte(strings.Join(insertLines(lines, end, indentText(string(schemasText), schemas.Content[0].Column-1)), "\n")), nil
}

// parseYAMLMapping mengurai teks YAML menjadi node mapping.
func parseYAMLMapping(text []byte) (*yaml.Node, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(text, &n); err != nil {
		return nil, err
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) != 1 || n.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("template YAML bukan mapping")
	}
	return n.Content[0], nil
}
