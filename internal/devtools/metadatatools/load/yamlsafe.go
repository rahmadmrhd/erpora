package load

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// allowedTags adalah tag inti YAML yang boleh ditulis eksplisit; tag lain (kustom, !!binary, !!set,
// !!python/...) ditolak.
var allowedTags = map[string]bool{
	"!!str": true, "!!int": true, "!!float": true, "!!bool": true, "!!null": true, "!!map": true, "!!seq": true,
}

const satCap = 1 << 40

func satAdd(a, b int) int {
	if a+b > satCap || a+b < 0 {
		return satCap
	}
	return a + b
}

type collector struct {
	file  string
	probs []Problem
}

func (c *collector) add(n *yaml.Node, ptr, code, msg string) {
	p := Problem{File: c.file, Pointer: ptr, Code: code, Message: msg}
	if n != nil {
		p.Line, p.Column = n.Line, n.Column
	}
	c.probs = append(c.probs, p)
}

var lineRe = regexp.MustCompile(`line (\d+)`)

// parseNode membaca tepat satu dokumen YAML ke pohon node tanpa ekspansi alias.
func parseNode(c *collector, data []byte) *yaml.Node {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			c.add(nil, "", CodeYAMLShape, "berkas kosong")
			return nil
		}
		p := Problem{File: c.file, Code: CodeYAMLSyntax, Message: "YAML tidak valid: " + err.Error()}
		if m := lineRe.FindStringSubmatch(err.Error()); m != nil {
			p.Line, _ = strconv.Atoi(m[1])
			p.Column = 1
		}
		c.probs = append(c.probs, p)
		return nil
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		c.add(&extra, "", CodeYAMLMultiDoc, "berkas hanya boleh memuat satu dokumen YAML")
		return nil
	} else if !errors.Is(err, io.EOF) {
		c.add(nil, "", CodeYAMLSyntax, "YAML tidak valid: "+err.Error())
		return nil
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		c.add(nil, "", CodeYAMLShape, "berkas kosong")
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		c.add(root, "", CodeYAMLShape, "akar dokumen harus berupa objek (pemetaan kunci-nilai)")
		return nil
	}
	return root
}

type analysis struct {
	c        *collector
	lim      Limits
	aliases  int
	literal  int
	size     map[*yaml.Node]int
	depth    map[*yaml.Node]int
	visiting map[*yaml.Node]bool
	reported map[string]bool
}

func (a *analysis) once(key string, n *yaml.Node, code, msg string) {
	if a.reported[key] {
		return
	}
	a.reported[key] = true
	a.c.add(n, "", code, msg)
}

// analyze memeriksa tag, alias, kedalaman, dan ukuran ekspansi tanpa membentuk nilai. Rekursi dibatasi
// MaxDepth sehingga aman terhadap masukan bersarang dalam. Mengembalikan true bila tidak ada pelanggaran.
func analyze(c *collector, root *yaml.Node, lim Limits) bool {
	before := len(c.probs)
	a := &analysis{c: c, lim: lim, size: map[*yaml.Node]int{}, depth: map[*yaml.Node]int{}, visiting: map[*yaml.Node]bool{}, reported: map[string]bool{}}
	sz, _ := a.walk(root, 1)
	if extra := sz - a.literal; extra > lim.MaxAliasExpansion {
		a.once("expansion", root, CodeYAMLAlias,
			fmt.Sprintf("ekspansi alias menambah %d node (batas %d); kemungkinan alias bomb", min(extra, satCap), lim.MaxAliasExpansion))
	}
	return len(c.probs) == before
}

func (a *analysis) walk(n *yaml.Node, abs int) (size, depth int) {
	if sz, ok := a.size[n]; ok {
		return sz, a.depth[n]
	}
	if n.Style&yaml.TaggedStyle != 0 && !allowedTags[n.Tag] {
		a.c.add(n, "", CodeYAMLTag, fmt.Sprintf("tag YAML %q tidak diizinkan (hanya tag inti !!str, !!int, !!float, !!bool, !!null, !!map, !!seq)", n.Tag))
	}
	if abs > a.lim.MaxDepth {
		a.once("depth", n, CodeYAMLDepth, fmt.Sprintf("kedalaman YAML melebihi %d tingkat", a.lim.MaxDepth))
		return 1, 1
	}
	a.literal++
	switch n.Kind {
	case yaml.AliasNode:
		a.aliases++
		if a.aliases == a.lim.MaxAliases+1 {
			a.once("aliases", n, CodeYAMLAlias, fmt.Sprintf("jumlah alias melebihi %d", a.lim.MaxAliases))
		}
		t := n.Alias
		if t == nil || a.visiting[t] {
			a.once("cycle", n, CodeYAMLAlias, "alias melingkar tidak diizinkan")
			return 1, 1
		}
		tsz, tdepth := a.walk(t, abs+1)
		if abs+tdepth > a.lim.MaxDepth {
			a.once("depth", n, CodeYAMLDepth, fmt.Sprintf("kedalaman YAML melebihi %d tingkat setelah ekspansi alias", a.lim.MaxDepth))
		}
		size, depth = satAdd(1, tsz), 1+tdepth
	case yaml.MappingNode, yaml.SequenceNode:
		a.visiting[n] = true
		size, depth = 1, 1
		for _, ch := range n.Content {
			s, d := a.walk(ch, abs+1)
			size = satAdd(size, s)
			depth = max(depth, 1+d)
		}
		delete(a.visiting, n)
	default:
		size, depth = 1, 1
	}
	a.size[n], a.depth[n] = size, depth
	return size, depth
}

// builder mengonversi pohon node menjadi nilai generik JSON-kompatibel dan merekam posisi per JSON pointer.
type builder struct {
	c   *collector
	pos map[string]Pos
}

func (b *builder) record(ptr string, n *yaml.Node, at *Pos) {
	if at != nil {
		b.pos[ptr] = *at
		return
	}
	b.pos[ptr] = Pos{Line: n.Line, Column: n.Column}
}

func (b *builder) value(n *yaml.Node, ptr string, at *Pos) any {
	switch n.Kind {
	case yaml.AliasNode:
		alias := Pos{Line: n.Line, Column: n.Column}
		if at != nil {
			alias = *at
		}
		return b.value(n.Alias, ptr, &alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str" {
				if k.Kind == yaml.ScalarNode && k.ShortTag() == "!!merge" {
					b.c.add(k, ptr, CodeYAMLMerge, "merge key \"<<\" tidak diizinkan; pakai include/set pada metadata")
				} else {
					b.c.add(k, ptr, CodeYAMLKey, "kunci harus berupa teks")
				}
				continue
			}
			child := ptr + "/" + escapePointer(k.Value)
			if _, dup := m[k.Value]; dup {
				b.c.add(k, child, CodeYAMLDuplicateKey, fmt.Sprintf("kunci %q muncul lebih dari sekali", k.Value))
				continue
			}
			b.record(child, k, at)
			m[k.Value] = b.value(v, child, at)
		}
		return m
	case yaml.SequenceNode:
		s := make([]any, len(n.Content))
		for i, ch := range n.Content {
			child := ptr + "/" + strconv.Itoa(i)
			b.record(child, ch, at)
			s[i] = b.value(ch, child, at)
		}
		return s
	default:
		v, err := scalarValue(n)
		if err != nil {
			b.c.add(n, ptr, CodeYAMLTag, err.Error())
		}
		return v
	}
}

// scalarValue mengonversi scalar ke nilai JSON-kompatibel: angka sebagai json.Number, tanggal tanpa kutip
// tetap berupa teks sumbernya.
func scalarValue(n *yaml.Node) (any, error) {
	switch n.ShortTag() {
	case "!!str", "!!timestamp":
		return n.Value, nil
	case "!!null":
		return nil, nil
	case "!!bool":
		var v bool
		if err := n.Decode(&v); err != nil {
			return nil, fmt.Errorf("nilai boolean tidak valid: %q", n.Value)
		}
		return v, nil
	case "!!int":
		var i int64
		if err := n.Decode(&i); err == nil {
			return json.Number(strconv.FormatInt(i, 10)), nil
		}
		var u uint64
		if err := n.Decode(&u); err == nil {
			return json.Number(strconv.FormatUint(u, 10)), nil
		}
		return nil, fmt.Errorf("bilangan bulat di luar jangkauan: %q", n.Value)
	case "!!float":
		var f float64
		if err := n.Decode(&f); err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("bilangan pecahan tidak valid atau tak hingga: %q", n.Value)
		}
		return json.Number(strconv.FormatFloat(f, 'g', -1, 64)), nil
	}
	return nil, fmt.Errorf("tipe scalar YAML %q tidak didukung", n.ShortTag())
}
