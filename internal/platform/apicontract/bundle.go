package apicontract

import (
	"fmt"
	"strings"
)

// copyRewrite menyalin node secara dalam dan mengganti setiap nilai "$ref" lewat fn.
func copyRewrite(node any, fn func(ref string) (string, error)) (any, error) {
	switch x := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			if s, ok := v.(string); ok && k == "$ref" {
				ns, err := fn(s)
				if err != nil {
					return nil, err
				}
				out[k] = ns
				continue
			}
			c, err := copyRewrite(v, fn)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			c, err := copyRewrite(v, fn)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	}
	return node, nil
}

// compRef adalah rujukan ke sebuah komponen: components/<kind>/<name>[/rest…].
type compRef struct {
	kind, name string
	rest       []string // sisa segmen pointer (sudah di-unescape)
}

func parseComponentPointer(ptr string) (compRef, error) {
	segs := splitPointer(ptr)
	if len(segs) < 3 || segs[0] != "components" || segs[1] == "" || segs[2] == "" {
		return compRef{}, fmt.Errorf("pointer %q harus menunjuk /components/<jenis>/<nama>", ptr)
	}
	return compRef{kind: segs[1], name: segs[2], rest: segs[3:]}, nil
}

// local menulis ulang rujukan sebagai "#/components/<kind>/<newName>[/rest]".
func (c compRef) local(newName string) string {
	var b strings.Builder
	b.WriteString("#/components/")
	b.WriteString(escapePointerSeg(c.kind))
	b.WriteByte('/')
	b.WriteString(escapePointerSeg(newName))
	for _, r := range c.rest {
		b.WriteByte('/')
		b.WriteString(escapePointerSeg(r))
	}
	return b.String()
}

// Bundle membuat dokumen mandiri untuk satu dokumen: setiap $ref erp:<x>#/components/<kind>/<N> disalin ke
// components/<kind>/"<x>.<N>" (common menjadi "common.<N>") dan dirujuk secara lokal; $ref lokal tetap.
// Komponen salinan ikut ditulis ulang secara transitif. Hasilnya deterministik.
func (s *Set) Bundle(name string) (map[string]any, error) {
	doc, ok := s.docs[name]
	if !ok {
		return nil, fmt.Errorf("dokumen kontrak %q tidak ada", name)
	}
	type item struct{ doc, kind, name string }
	var queue []item
	queued := map[item]bool{}
	enqueue := func(it item) {
		if !queued[it] {
			queued[it] = true
			queue = append(queue, it)
		}
	}

	// rewriteFor mengembalikan fungsi penulis-ulang untuk node yang berasal dari dokumen from.
	// Node dari dokumen utama mempertahankan rujukan lokalnya; node salinan (copied) memetakan rujukan lokalnya
	// ke komponen bernamespace dokumen asalnya.
	rewriteFor := func(from string, copied bool) func(string) (string, error) {
		return func(ref string) (string, error) {
			r, err := ParseRef(from, ref)
			if err != nil {
				return "", err
			}
			if !strings.HasPrefix(ref, "erp:") && !copied {
				return ref, nil
			}
			cr, err := parseComponentPointer(r.Pointer)
			if err != nil {
				return "", fmt.Errorf("dokumen %s: $ref %q: %w", from, ref, err)
			}
			if _, ok := s.docs[r.Doc]; !ok {
				return "", fmt.Errorf("dokumen %s: $ref %q merujuk dokumen %q yang tidak ada", from, ref, r.Doc)
			}
			enqueue(item{r.Doc, cr.kind, cr.name})
			return cr.local(r.Doc + "." + cr.name), nil
		}
	}

	root, err := copyRewrite(doc.Spec, rewriteFor(name, false))
	if err != nil {
		return nil, err
	}
	out := root.(map[string]any)
	comps, _ := out["components"].(map[string]any)
	if comps == nil {
		comps = map[string]any{}
		out["components"] = comps
	}
	for i := 0; i < len(queue); i++ {
		it := queue[i]
		src, ok := walkPointer(s.docs[it.doc].Spec, "/components/"+escapePointerSeg(it.kind)+"/"+escapePointerSeg(it.name))
		if !ok {
			return nil, fmt.Errorf("komponen %s/%s tidak ada di dokumen %s", it.kind, it.name, it.doc)
		}
		cp, err := copyRewrite(src, rewriteFor(it.doc, true))
		if err != nil {
			return nil, err
		}
		kind, _ := comps[it.kind].(map[string]any)
		if kind == nil {
			kind = map[string]any{}
			comps[it.kind] = kind
		}
		kind[it.doc+"."+it.name] = cp
	}
	return out, nil
}
