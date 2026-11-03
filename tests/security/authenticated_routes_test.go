package security

import (
	"slices"
	"testing"

	"erpora/internal/app"
	"erpora/internal/platform/httpserver"
)

// TestAuthenticatedRoutesMatchAllowlist: route terautentikasi-tanpa-permission (registry modul + rute platform langsung
// di router) harus persis sama dengan authenticated_routes.txt. Route tanpa permission melewati mesin permission, jadi
// menambahnya berarti sengaja mengubah allowlist (dan melewati review keamanan; PRD-001 Q29).
func TestAuthenticatedRoutesMatchAllowlist(t *testing.T) {
	reg, err := app.BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	for _, r := range reg.Routes() {
		if r.Authenticated {
			if r.Public || r.Permission != "" {
				t.Errorf("route %s %s: Authenticated tidak boleh Public atau ber-permission", r.Method, r.Pattern)
			}
			actual = append(actual, r.Method+" "+r.Pattern)
		}
	}
	actual = append(actual, httpserver.AuthenticatedRoutes()...)
	allowed := readAllowlist(t, "authenticated_routes.txt")

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
		t.Errorf("route terautentikasi-saja di luar allowlist: %v", unexpected)
	}
	if len(missing) > 0 {
		t.Errorf("entri allowlist tanpa route terautentikasi-saja: %v", missing)
	}
}
