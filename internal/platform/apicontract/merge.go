package apicontract

import (
	"fmt"
	"sort"
	"strings"
)

// Merge menggabungkan dokumen (urutan order) menjadi satu spec. "common" dan "platform" tidak diberi namespace;
// komponen dokumen lain diberi namespace "<slug>.<N>" dan rujukannya ditulis ulang. Path atau operationId
// yang bentrok adalah error yang menyebut kedua dokumen.
func (s *Set) Merge(order []string) (map[string]any, error) {
	paths := map[string]any{}
	comps := map[string]any{}
	pathOwner := map[string]string{}
	opIDOwner := map[string]string{}
	compOwner := map[string]string{}
	namespaced := func(n string) bool { return n != "common" && n != "platform" }

	inOrder := map[string]bool{}
	for _, n := range order {
		if _, ok := s.docs[n]; !ok {
			return nil, fmt.Errorf("dokumen kontrak %q tidak ada", n)
		}
		inOrder[n] = true
	}

	for _, name := range order {
		doc := s.docs[name]
		rewrite := func(ref string) (string, error) {
			r, err := ParseRef(name, ref)
			if err != nil {
				return "", err
			}
			cr, err := parseComponentPointer(r.Pointer)
			if err != nil {
				if !strings.HasPrefix(ref, "erp:") {
					return ref, nil // rujukan lokal di luar components dibiarkan
				}
				return "", fmt.Errorf("dokumen %s: $ref %q: %w", name, ref, err)
			}
			if !inOrder[r.Doc] {
				return "", fmt.Errorf("dokumen %s: $ref %q merujuk dokumen %q yang tidak ikut digabung", name, ref, r.Doc)
			}
			if namespaced(r.Doc) {
				return cr.local(r.Doc + "." + cr.name), nil
			}
			return cr.local(cr.name), nil
		}

		docPaths, _ := doc.Spec["paths"].(map[string]any)
		pkeys := make([]string, 0, len(docPaths))
		for p := range docPaths {
			pkeys = append(pkeys, p)
		}
		sort.Strings(pkeys)
		for _, p := range pkeys {
			if other, dup := pathOwner[p]; dup {
				return nil, fmt.Errorf("path %s didefinisikan oleh dokumen %s dan %s", p, other, name)
			}
			pathOwner[p] = name
			cp, err := copyRewrite(docPaths[p], rewrite)
			if err != nil {
				return nil, err
			}
			paths[p] = cp
		}
		for _, op := range doc.Operations() {
			if op.OperationID == "" {
				continue
			}
			if other, dup := opIDOwner[op.OperationID]; dup {
				return nil, fmt.Errorf("operationId %s didefinisikan oleh dokumen %s dan %s", op.OperationID, other, name)
			}
			opIDOwner[op.OperationID] = name
		}

		docComps, _ := doc.Spec["components"].(map[string]any)
		kinds := make([]string, 0, len(docComps))
		for k := range docComps {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			entries, _ := docComps[kind].(map[string]any)
			dst, _ := comps[kind].(map[string]any)
			if dst == nil {
				dst = map[string]any{}
				comps[kind] = dst
			}
			names := make([]string, 0, len(entries))
			for n := range entries {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				key := n
				if namespaced(name) {
					key = name + "." + n
				}
				ck := kind + "/" + key
				if other, dup := compOwner[ck]; dup {
					return nil, fmt.Errorf("komponen %s didefinisikan oleh dokumen %s dan %s", ck, other, name)
				}
				compOwner[ck] = name
				cp, err := copyRewrite(entries[n], rewrite)
				if err != nil {
					return nil, err
				}
				dst[key] = cp
			}
		}
	}
	return map[string]any{
		"openapi":    "3.1.0",
		"info":       map[string]any{"title": "ERPora", "version": "merged"},
		"paths":      paths,
		"components": comps,
	}, nil
}
