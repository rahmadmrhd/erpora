package apicontract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Ref adalah rujukan terurai: nama dokumen dan JSON pointer (diawali "/", masih ter-escape ~0/~1).
type Ref struct{ Doc, Pointer string }

var docNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ParseRef menguraikan nilai $ref. "#/a/b" dirujuk ke dokumen from; "erp:<name>#/a/b" ke dokumen name.
// Path file relatif, URL, dan pointer kosong ditolak (kontrak hanya boleh merujuk lewat erp:).
func ParseRef(from, ref string) (Ref, error) {
	switch {
	case strings.HasPrefix(ref, "#/"):
		return Ref{Doc: from, Pointer: ref[1:]}, nil
	case strings.HasPrefix(ref, "erp:"):
		name, ptr, ok := strings.Cut(ref[len("erp:"):], "#")
		if !ok || !docNameRe.MatchString(name) || !strings.HasPrefix(ptr, "/") {
			return Ref{}, fmt.Errorf("$ref %q tidak valid: bentuknya erp:<dokumen>#/<pointer>", ref)
		}
		return Ref{Doc: name, Pointer: ptr}, nil
	}
	return Ref{}, fmt.Errorf("$ref %q ditolak: hanya \"#/…\" dan \"erp:<dokumen>#/…\" yang diizinkan (path file relatif dan URL dilarang)", ref)
}

// splitPointer memecah JSON pointer ("/a/b~1c") menjadi segmen yang sudah di-unescape.
func splitPointer(ptr string) []string {
	parts := strings.Split(ptr[1:], "/")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~1", "/")
		parts[i] = strings.ReplaceAll(p, "~0", "~")
	}
	return parts
}

// escapePointerSeg meng-escape satu segmen JSON pointer.
func escapePointerSeg(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

// walkPointer menelusuri node mengikuti pointer; ok=false bila tidak ditemukan.
func walkPointer(node any, ptr string) (any, bool) {
	cur := node
	for _, seg := range splitPointer(ptr) {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(v) {
				return nil, false
			}
			cur = v[i]
		default:
			return nil, false
		}
	}
	return cur, true
}
