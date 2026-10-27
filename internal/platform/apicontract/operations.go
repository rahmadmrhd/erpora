package apicontract

import (
	"maps"
	"slices"
	"sort"
	"strings"
)

// Operation adalah satu operasi HTTP di kontrak.
type Operation struct {
	Doc, Method, Path, OperationID, Permission string // Method huruf besar
	Public, Deprecated                         bool
	// DeprecatedQueryParams: nama parameter query yang ditandai deprecated (FR-23.3).
	DeprecatedQueryParams []string
	// Node adalah node operasi (salinan dangkal) dengan "parameters" = parameter level path digabung dengan
	// parameter operasi (parameter operasi menimpa yang sama nama dan lokasi).
	Node map[string]any
}

var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// Operations mengembalikan operasi dokumen, terurut (Path, Method). Rujukan lintas dokumen (erp:) tidak
// diselesaikan di sini; pakai Set.Operations bila dokumen memuat parameter yang dirujuk lintas dokumen.
func (d *Doc) Operations() []Operation {
	return d.operations(func(from, ref string) (string, any, bool) {
		r, err := ParseRef(from, ref)
		if err != nil || r.Doc != d.Name {
			return "", nil, false
		}
		n, ok := walkPointer(d.Spec, r.Pointer)
		return r.Doc, n, ok
	})
}

// Operations mengembalikan operasi dokumen name dengan seluruh rujukan diselesaikan lewat Set.
func (s *Set) Operations(name string) []Operation {
	d, ok := s.docs[name]
	if !ok {
		return nil
	}
	return d.operations(func(from, ref string) (string, any, bool) {
		doc, n, err := s.Resolve(from, ref)
		return doc, n, err == nil
	})
}

func (d *Doc) operations(resolve func(from, ref string) (string, any, bool)) []Operation {
	paths, _ := d.Spec["paths"].(map[string]any)
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	var out []Operation
	for _, p := range pathKeys {
		item, ok := paths[p].(map[string]any)
		if !ok {
			continue
		}
		pathParams, _ := item["parameters"].([]any)
		for _, m := range httpMethods {
			node, ok := item[m].(map[string]any)
			if !ok {
				continue
			}
			op := Operation{Doc: d.Name, Method: strings.ToUpper(m), Path: p}
			op.OperationID, _ = node["operationId"].(string)
			op.Permission, _ = node["x-erp-permission"].(string)
			op.Public, _ = node["x-erp-public"].(bool)
			op.Deprecated, _ = node["deprecated"].(bool)

			merged := mergeParams(d.Name, pathParams, asSlice(node["parameters"]), resolve)
			cp := maps.Clone(node)
			if len(merged) > 0 {
				cp["parameters"] = merged
			}
			op.Node = cp
			for _, raw := range merged {
				pm := paramNode(d.Name, raw, resolve)
				if pm == nil {
					continue
				}
				if in, _ := pm["in"].(string); in == "query" {
					if dep, _ := pm["deprecated"].(bool); dep {
						name, _ := pm["name"].(string)
						op.DeprecatedQueryParams = append(op.DeprecatedQueryParams, name)
					}
				}
			}
			slices.Sort(op.DeprecatedQueryParams)
			out = append(out, op)
		}
	}
	return out
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// paramNode menyelesaikan satu entri parameter (inline atau $ref) menjadi objek parameter; nil bila gagal.
func paramNode(from string, raw any, resolve func(from, ref string) (string, any, bool)) map[string]any {
	doc := from
	cur := raw
	for i := 0; i < 8; i++ {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		ref, isRef := m["$ref"].(string)
		if !isRef {
			return m
		}
		d2, n, ok := resolve(doc, ref)
		if !ok {
			return nil
		}
		doc, cur = d2, n
	}
	return nil
}

// mergeParams menggabungkan parameter level path dengan parameter operasi (operasi menimpa).
func mergeParams(from string, pathLevel, opLevel []any, resolve func(from, ref string) (string, any, bool)) []any {
	key := func(raw any) string {
		pm := paramNode(from, raw, resolve)
		if pm == nil {
			return ""
		}
		name, _ := pm["name"].(string)
		in, _ := pm["in"].(string)
		return in + ":" + name
	}
	overridden := map[string]bool{}
	for _, raw := range opLevel {
		if k := key(raw); k != "" {
			overridden[k] = true
		}
	}
	var out []any
	for _, raw := range pathLevel {
		if k := key(raw); k != "" && overridden[k] {
			continue
		}
		out = append(out, raw)
	}
	return append(out, opLevel...)
}
