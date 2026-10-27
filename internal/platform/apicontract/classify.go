package apicontract

import (
	"fmt"
	"regexp"
	"sort"

	"erpora/sdk/module"
)

var status2xxRe = regexp.MustCompile(`^(2[0-9][0-9]|2XX)$`)

// ResponseClassification menghitung klasifikasi tertinggi x-erp-classification atas skema respons 2xx operasi,
// rekursif lewat properties, patternProperties, items, prefixItems, additionalProperties, allOf/oneOf/anyOf, dan
// $ref lokal maupun erp: (aman terhadap siklus). Anotasi berlaku untuk seluruh subpohonnya (yang lebih tinggi
// menang). Daun skema tanpa anotasi dianggap internal, dan respons tanpa daun juga internal (keputusan 10).
// Nilai x-erp-classification yang tidak valid adalah error.
func (s *Set) ResponseClassification(op Operation) (module.Classification, error) {
	c := &classifier{set: s, seen: map[string]bool{}, best: -1}
	responses, _ := op.Node["responses"].(map[string]any)
	codes := make([]string, 0, len(responses))
	for code := range responses {
		if status2xxRe.MatchString(code) {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		doc, resp, err := s.resolveDeep(op.Doc, responses[code])
		if err != nil {
			return "", fmt.Errorf("%s %s respons %s: %w", op.Method, op.Path, code, err)
		}
		rm, _ := resp.(map[string]any)
		content, _ := rm["content"].(map[string]any)
		medias := make([]string, 0, len(content))
		for mt := range content {
			medias = append(medias, mt)
		}
		sort.Strings(medias)
		for _, mt := range medias {
			media, _ := content[mt].(map[string]any)
			schema, ok := media["schema"]
			if !ok {
				continue
			}
			if err := c.schema(doc, schema, -1); err != nil {
				return "", fmt.Errorf("%s %s respons %s: %w", op.Method, op.Path, code, err)
			}
		}
	}
	rank := c.best
	if rank < 0 { // tanpa daun sama sekali
		rank = module.ClassInternal.Rank()
	}
	return classByRank(rank), nil
}

func classByRank(r int) module.Classification {
	for _, c := range []module.Classification{module.ClassPublic, module.ClassInternal, module.ClassConfidential, module.ClassHighlyConfidential} {
		if c.Rank() == r {
			return c
		}
	}
	return module.ClassInternal
}

type classifier struct {
	set  *Set
	seen map[string]bool // "<doc>#<pointer>@<inherited>" yang sudah dikunjungi
	best int             // rank tertinggi di antara daun; -1 = belum ada daun
}

func (c *classifier) leaf(inherited int) {
	r := inherited
	if r < 0 {
		r = module.ClassInternal.Rank() // tanpa anotasi = internal
	}
	if r > c.best {
		c.best = r
	}
}

func (c *classifier) schema(doc string, node any, inherited int) error {
	m, ok := node.(map[string]any)
	if !ok { // skema boolean
		c.leaf(inherited)
		return nil
	}
	if raw, ok := m["x-erp-classification"]; ok {
		str, _ := raw.(string)
		cl, err := module.ParseClassification(str)
		if err != nil {
			return fmt.Errorf("x-erp-classification: %w", err)
		}
		if cl.Rank() > inherited {
			inherited = cl.Rank()
		}
	}
	if ref, ok := m["$ref"].(string); ok {
		r, err := ParseRef(doc, ref)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s#%s@%d", r.Doc, r.Pointer, inherited)
		if c.seen[key] {
			return nil
		}
		c.seen[key] = true
		doc2, target, err := c.set.Resolve(doc, ref)
		if err != nil {
			return err
		}
		return c.schema(doc2, target, inherited)
	}

	descended := false
	each := func(v any, f func(child any) error) error {
		switch x := v.(type) {
		case map[string]any:
			names := make([]string, 0, len(x))
			for k := range x {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				descended = true
				if err := f(x[k]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	recur := func(child any) error { return c.schema(doc, child, inherited) }
	if err := each(m["properties"], recur); err != nil {
		return err
	}
	if err := each(m["patternProperties"], recur); err != nil {
		return err
	}
	for _, kw := range []string{"items", "additionalProperties"} {
		if child, ok := m[kw].(map[string]any); ok {
			descended = true
			if err := c.schema(doc, child, inherited); err != nil {
				return err
			}
		}
	}
	for _, kw := range []string{"prefixItems", "allOf", "oneOf", "anyOf"} {
		if list, ok := m[kw].([]any); ok {
			for _, child := range list {
				descended = true
				if err := c.schema(doc, child, inherited); err != nil {
					return err
				}
			}
		}
	}
	if !descended {
		c.leaf(inherited)
	}
	return nil
}
