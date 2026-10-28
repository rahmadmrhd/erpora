package apicontract

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Doc adalah satu dokumen kontrak: "common", "platform", atau slug modul.
type Doc struct {
	Name string         // "common" | "platform" | slug modul
	Spec map[string]any // YAML -> tipe JSON (map[string]any, []any, string, float64, int64, bool, nil)
}

// Parse menguraikan satu dokumen OpenAPI 3.1. Ditolak: YAML tidak valid, anchor/alias, kunci non-string,
// `openapi` selain 3.1.x, path di luar "/api/", dan $ref selain bentuk yang diizinkan ParseRef.
func Parse(name string, data []byte) (*Doc, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("kontrak %s: YAML tidak valid: %w", name, err)
	}
	if err := checkNode(&root); err != nil {
		return nil, fmt.Errorf("kontrak %s: %w", name, err)
	}
	var raw any
	if err := root.Decode(&raw); err != nil {
		return nil, fmt.Errorf("kontrak %s: %w", name, err)
	}
	spec, ok := normalize(raw).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("kontrak %s: dokumen harus berupa objek", name)
	}
	if v, _ := spec["openapi"].(string); !strings.HasPrefix(v, "3.1.") {
		return nil, fmt.Errorf("kontrak %s: openapi harus 3.1.x, ditemukan %v", name, spec["openapi"])
	}
	if paths, ok := spec["paths"].(map[string]any); ok {
		for p := range paths {
			if !strings.HasPrefix(p, "/api/") {
				return nil, fmt.Errorf("kontrak %s: path %q harus diawali /api/", name, p)
			}
		}
	}
	d := &Doc{Name: name, Spec: spec}
	var refErr error
	walkRefs(d.Spec, func(ref string) string {
		if _, err := ParseRef(name, ref); err != nil && refErr == nil {
			refErr = fmt.Errorf("kontrak %s: %w", name, err)
		}
		return ref
	})
	if refErr != nil {
		return nil, refErr
	}
	return d, nil
}

// checkNode menolak anchor, alias, merge key, dan kunci non-string.
func checkNode(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode {
		return errors.New("alias YAML (*x) tidak diizinkan")
	}
	if n.Anchor != "" {
		return fmt.Errorf("anchor YAML (&%s) tidak diizinkan", n.Anchor)
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return fmt.Errorf("baris %d: kunci harus string (kutip kunci seperti '200')", k.Line)
			}
		}
	}
	for _, c := range n.Content {
		if err := checkNode(c); err != nil {
			return err
		}
	}
	return nil
}

// normalize mengubah int dan map yang dihasilkan decoder menjadi tipe JSON (int64, map[string]any).
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
		return x
	case int:
		return int64(x)
	case uint64: // hanya muncul untuk nilai di atas MaxInt64
		return float64(x)
	case time.Time:
		// Contoh berisi timestamp tanpa kutip didekode sebagai time.Time; kembalikan ke teks aslinya.
		return x.UTC().Format(time.RFC3339Nano)
	}
	return v
}

// walkRefs memanggil fn untuk setiap nilai "$ref" (string) dan mengganti nilainya dengan hasil fn.
func walkRefs(node any, fn func(ref string) string) {
	switch x := node.(type) {
	case map[string]any:
		for k, v := range x {
			if s, ok := v.(string); ok && k == "$ref" {
				x[k] = fn(s)
				continue
			}
			walkRefs(v, fn)
		}
	case []any:
		for _, v := range x {
			walkRefs(v, fn)
		}
	}
}

// ExternalRefs mengembalikan nama dokumen unik yang dirujuk lewat erp:, terurut.
func (d *Doc) ExternalRefs() []string {
	seen := map[string]bool{}
	walkRefs(d.Spec, func(ref string) string {
		if r, err := ParseRef(d.Name, ref); err == nil && strings.HasPrefix(ref, "erp:") {
			seen[r.Doc] = true
		}
		return ref
	})
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
