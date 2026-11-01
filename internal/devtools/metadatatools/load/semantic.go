package load

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// conditionKeys adalah kunci yang nilainya berupa kondisi (skema memakai satu definisi kondisi untuk semuanya).
var conditionKeys = map[string]bool{
	"visibleWhen": true, "requiredWhen": true, "readonlyWhen": true, "lockWhen": true, "assert": true, "filter": true,
}

// nameMapKeys adalah kunci yang nilainya peta nama buatan developer (field, relasi, timpaan include): kunci
// anaknya adalah identifier yang dibatasi 63 byte (Requirement 1.4; JSON Schema tidak dapat membatasi panjang
// kunci pada patternProperties).
var nameMapKeys = map[string]bool{"fields": true, "relations": true, "set": true, "add": true, "pivotFields": true}

// maxNameBytes adalah batas panjang identifier.
const maxNameBytes = 63

// urlRe mengenali URL berskema (scheme://) dan skema skrip; berkas metadata hanya memuat data, tidak URL.
var urlRe = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://|javascript:`)

// conditionStats menghitung jumlah node dan kedalaman pohon kondisi (daun = kedalaman 1) secara defensif:
// bentuk yang tidak dikenal dihitung sebagai node tunggal (kesalahan bentuknya dilaporkan skema).
func conditionStats(v any, level int) (nodes, depth int) {
	m, ok := v.(map[string]any)
	if !ok || level > 64 {
		return 1, 1
	}
	nodes, depth = 1, 1
	for _, key := range []string{"all", "any"} {
		if arr, ok := m[key].([]any); ok {
			for _, it := range arr {
				n, d := conditionStats(it, level+1)
				nodes += n
				depth = max(depth, 1+d)
			}
		}
	}
	if sub, ok := m["not"]; ok {
		n, d := conditionStats(sub, level+1)
		nodes += n
		depth = max(depth, 1+d)
	}
	return nodes, depth
}

// semanticChecks menjalankan batas struktural dan penolakan URL pada nilai generik.
func semanticChecks(root map[string]any) []Problem {
	var out []Problem
	var walk func(v any, ptr, key string)
	walk = func(v any, ptr, key string) {
		switch t := v.(type) {
		case map[string]any:
			if conditionKeys[key] {
				n, d := conditionStats(t, 1)
				if d > MaxConditionDepth {
					out = append(out, Problem{Pointer: ptr, Code: CodeConditionDepth,
						Message: fmt.Sprintf("kedalaman kondisi %d melebihi batas %d", d, MaxConditionDepth)})
				}
				if n > MaxConditionNodes {
					out = append(out, Problem{Pointer: ptr, Code: CodeConditionNodes,
						Message: fmt.Sprintf("kondisi memuat %d node, melebihi batas %d", n, MaxConditionNodes)})
				}
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if nameMapKeys[key] {
				for _, k := range keys {
					if len(k) > maxNameBytes {
						out = append(out, Problem{Pointer: ptr + "/" + escapePointer(k), Code: CodeSchema,
							Message: fmt.Sprintf("nama %q (%d byte) melebihi batas %d byte", k, len(k), maxNameBytes)})
					}
				}
			}
			for _, k := range keys {
				walk(t[k], ptr+"/"+escapePointer(k), k)
			}
		case []any:
			for i, it := range t {
				walk(it, ptr+"/"+strconv.Itoa(i), key)
			}
		case string:
			// Nilai pattern adalah regex (boleh memuat "://" sebagai teks pola); semua nilai lain bukan URL.
			if key != "pattern" && urlRe.MatchString(t) {
				out = append(out, Problem{Pointer: ptr, Code: CodeForbiddenValue,
					Message: "nilai tidak boleh berupa URL; berkas metadata hanya memuat data"})
			}
		}
	}
	walk(root, "", "")
	return out
}

// ConditionStats menghitung jumlah node dan kedalaman pohon kondisi generik (daun = kedalaman 1); dipakai
// resolver untuk memeriksa ulang batas Requirement 1.3 pada kondisi hasil penggabungan.
func ConditionStats(v any) (nodes, depth int) { return conditionStats(v, 1) }
