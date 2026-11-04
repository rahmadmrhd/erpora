package naming_test

import (
	"math/rand"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/sdk/authz"
)

// TestCanonicalFieldParityWithNaming: authz.CanonicalField (sdk/authz, yang dipakai pencocokan grup field
// camelCase DTO terhadap snake_case manifest; Review Focus 2) harus identik dengan naming.ToSnake untuk
// setiap camelCase ketat. Tes ini hidup di modul root karena sdk tidak boleh bergantung pada root (D5).
// Nama yang ditolak naming harus menjadi "" (tidak pernah jatuh ke grup default).
func TestCanonicalFieldParityWithNaming(t *testing.T) {
	fixed := []string{
		"a", "id", "title", "customerName", "customerId", "address2Line", "pointX", "taxId", "a1B2",
		"createdAt", "createdBy", "costPrice", "invoiceLine", "httpServer", "urlId", "idNumber", "x", "aB", "a1",
		strings.Repeat("a", 63),
	}
	rng := rand.New(rand.NewSource(20261105))
	words := []string{"a", "b", "cost", "price", "id", "x", "line2", "t9", "url", "http", "my"}
	for i := 0; i < 500; i++ {
		var sb strings.Builder
		n := 1 + rng.Intn(4)
		for j := 0; j < n; j++ {
			w := words[rng.Intn(len(words))]
			if j > 0 {
				w = strings.ToUpper(w[:1]) + w[1:]
			}
			sb.WriteString(w)
		}
		fixed = append(fixed, sb.String())
	}
	for _, name := range fixed {
		want, err := naming.ToSnake(name)
		if err != nil {
			if got := authz.CanonicalField(name); got != "" {
				t.Errorf("%q ditolak naming (%v) tetapi CanonicalField = %q (harus \"\")", name, err, got)
			}
			continue
		}
		if got := authz.CanonicalField(name); got != want {
			t.Errorf("CanonicalField(%q) = %q, naming.ToSnake = %q", name, got, want)
		}
	}
	// nama yang ditolak naming (huruf kapital berurutan, kapital di awal, simbol) menjadi ""
	for _, bad := range []string{"taxID", "HTTPServer", "Title", "x-y", "x y", "", "aBC"} {
		if _, err := naming.ToSnake(bad); err == nil {
			t.Fatalf("asumsi tes: naming harus menolak %q", bad)
		}
		if got := authz.CanonicalField(bad); got != "" {
			t.Errorf("CanonicalField(%q) = %q, want \"\"", bad, got)
		}
	}
}
