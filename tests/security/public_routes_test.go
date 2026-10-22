// Package security berisi test keamanan lintas modul, mis. allowlist route publik.
package security

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"

	"erpora/internal/app"
)

// TestEveryRouteHasPermissionOrIsPublic: setiap route modul harus punya permission atau Public.
func TestEveryRouteHasPermissionOrIsPublic(t *testing.T) {
	reg, err := app.BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reg.Routes() {
		if !r.Public && r.Permission == "" {
			t.Errorf("route %s %s (%s) tanpa permission dan tidak Public", r.Method, r.Pattern, r.Module)
		}
	}
}

// TestPublicRoutesMatchAllowlist: route Public harus persis sama dengan public_routes.txt.
// Menambah route publik berarti sengaja mengubah allowlist (dan melewati review keamanan).
func TestPublicRoutesMatchAllowlist(t *testing.T) {
	reg, err := app.BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	for _, r := range reg.Routes() {
		if r.Public {
			actual = append(actual, r.Method+" "+r.Pattern)
		}
	}
	allowed := readAllowlist(t, "public_routes.txt")

	var unexpected, missing []string
	for _, p := range actual {
		if !slices.Contains(allowed, p) {
			unexpected = append(unexpected, p)
		}
	}
	for _, p := range allowed {
		if !slices.Contains(actual, p) {
			missing = append(missing, p)
		}
	}
	if len(unexpected) > 0 {
		t.Errorf("route publik di luar allowlist: %v", unexpected)
	}
	if len(missing) > 0 {
		t.Errorf("entri allowlist tanpa route publik: %v", missing)
	}
}

func readAllowlist(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
