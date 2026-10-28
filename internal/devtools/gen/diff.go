package gen

import (
	"fmt"
	"strings"
)

// UnifiedDiff mengembalikan diff unified (konteks 3 baris) before -> after untuk path; before kosong berarti file baru.
// Diff dihitung dengan LCS per baris; cukup untuk file sumber berukuran kecil yang dihasilkan generator.
func UnifiedDiff(path string, before, after []byte) string {
	a, b := splitLines(string(before)), splitLines(string(after))
	type edit struct {
		op   byte // ' ', '-', '+'
		text string
	}
	// LCS dinamis dari ujung.
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var edits []edit
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			edits = append(edits, edit{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			edits = append(edits, edit{'-', a[i]})
			i++
		default:
			edits = append(edits, edit{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		edits = append(edits, edit{'-', a[i]})
	}
	for ; j < m; j++ {
		edits = append(edits, edit{'+', b[j]})
	}

	const ctxLines = 3
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	k := 0
	for k < len(edits) {
		if edits[k].op == ' ' {
			k++
			continue
		}
		// Awal hunk: mundur ctxLines baris konteks; akhir: maju sampai > 2*ctxLines konteks berturut-turut.
		start := max(k-ctxLines, 0)
		end := k
		for end < len(edits) {
			if edits[end].op != ' ' {
				end++
				continue
			}
			run := 0
			for end+run < len(edits) && edits[end+run].op == ' ' {
				run++
			}
			if end+run >= len(edits) || run > 2*ctxLines {
				end += min(run, ctxLines)
				break
			}
			end += run
		}
		aStart, bStart := 1, 1
		for _, e := range edits[:start] {
			if e.op != '+' {
				aStart++
			}
			if e.op != '-' {
				bStart++
			}
		}
		aLen, bLen := 0, 0
		for _, e := range edits[start:end] {
			if e.op != '+' {
				aLen++
			}
			if e.op != '-' {
				bLen++
			}
		}
		if aLen == 0 { // konvensi unified diff untuk file baru
			aStart = 0
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
		for _, e := range edits[start:end] {
			sb.WriteByte(e.op)
			sb.WriteString(e.text)
			sb.WriteByte('\n')
		}
		k = end
	}
	return sb.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
