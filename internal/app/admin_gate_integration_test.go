//go:build integration

package app

import (
	"net/http"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/lifecycle"
)

// B-13: wiring gate mutasi (assemble.go) end-to-end. Di luar development tanpa ERP_MODULES_ADMIN_API=true route aksi
// mutasi menjawab 404 (sebelum permission); route baca tetap tunduk pada permission. Dengan flag aktif, mutasi dilayani
// dan keputusan ada di stub otorisasi (deny -> 403, allow -> 200).
func TestAssemble_ModulesAdminMutationGate(t *testing.T) {
	const enable = "/api/platform/modules/acme.example/enable"
	cases := []struct {
		name       string
		mutate     func(*config.Config)
		wantEnable int
		wantList   int
	}{
		{"production tanpa flag, stub allow", func(c *config.Config) { c.Env = config.EnvProduction }, http.StatusNotFound, http.StatusOK},
		{"production tanpa flag, stub deny", func(c *config.Config) { c.Env = config.EnvProduction; c.AuthzStub = "deny" }, http.StatusNotFound, http.StatusForbidden},
		{"production dengan flag, stub deny", func(c *config.Config) {
			c.Env, c.AuthzStub, c.ModulesAdminAPI = config.EnvProduction, "deny", true
		}, http.StatusForbidden, http.StatusForbidden},
		{"production dengan flag, stub allow", func(c *config.Config) {
			c.Env, c.AuthzStub, c.ModulesAdminAPI = config.EnvProduction, "allow", true
		}, http.StatusOK, http.StatusOK},
		{"development tanpa flag", func(*config.Config) {}, http.StatusOK, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, a, _, _ := setupAssemblyWith(t, tc.mutate)
			// Mutasi enable pada modul yang belum ter-install: 404 gate, 403 permission, atau 409 invalid_state bila lolos
			// keduanya. Install dulu lewat lifecycle agar jalur sukses dapat dibedakan (200).
			if tc.wantEnable == http.StatusOK {
				if _, err := a.Lifecycle.Execute(t.Context(), lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: "acme.example", Actor: "cli:test"}); err != nil {
					t.Fatal(err)
				}
			}
			if rec := call(a.Router, http.MethodPost, enable, ""); rec.Code != tc.wantEnable {
				t.Errorf("POST %s = %d, want %d: %s", enable, rec.Code, tc.wantEnable, rec.Body)
			}
			if rec := call(a.Router, http.MethodGet, "/api/platform/modules", ""); rec.Code != tc.wantList {
				t.Errorf("GET /api/platform/modules = %d, want %d: %s", rec.Code, tc.wantList, rec.Body)
			}
		})
	}
}
