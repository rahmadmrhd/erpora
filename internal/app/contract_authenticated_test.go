package app

import (
	"net/http"
	"slices"
	"testing"

	"erpora/internal/platform/apicontract"
	"erpora/internal/platform/httpserver"
)

// C1.13: bootstrap wajib login tanpa permission, dan kontrak menyatakannya (x-erp-authenticated). Menandainya Public
// atau memberinya permission pada route, tanpa mengubah kontrak, harus menggagalkan test kesesuaian.
func TestContracts_BootstrapAuthenticatedMismatchIsCaught(t *testing.T) {
	cases := map[string]struct {
		route apicontract.RouteSpec
		want  string
	}{
		"route Public":         {apicontract.RouteSpec{Public: true}, "authenticated_mismatch"},
		"route ber-permission": {apicontract.RouteSpec{Permission: "platform.modules.read"}, "authenticated_mismatch"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, in := buildConformance(t)
			for i, r := range in.Routes {
				if r.Pattern == httpserver.BootstrapPath {
					c.route.Module, c.route.Method, c.route.Pattern = r.Module, http.MethodGet, httpserver.BootstrapPath
					in.Routes[i] = c.route
				}
			}
			var kinds []string
			for _, m := range apicontract.CheckConformance(in) {
				if m.Path == httpserver.BootstrapPath {
					kinds = append(kinds, m.Kind)
				}
			}
			if !slices.Contains(kinds, c.want) {
				t.Errorf("kinds = %v, want memuat %s", kinds, c.want)
			}
		})
	}
}
