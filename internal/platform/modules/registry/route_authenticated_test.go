package registry

import (
	"errors"
	"testing"

	"erpora/sdk/module"
)

// PRD-001 Q29: Handle dengan permission kosong sah hanya bersama module.Authenticated(); route yang dihasilkan
// bukan Public dan tidak punya permission.
func TestRegister_AuthenticatedRoute(t *testing.T) {
	reg, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Handle("POST", "/logout", "", okHandler, module.Authenticated())
		r.Handle("GET", "/items", "notes.items.read", okHandler)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]RouteInfo{}
	for _, rt := range reg.Routes() {
		got[rt.Method] = rt
	}
	if a := got["POST"]; !a.Authenticated || a.Public || a.Permission != "" || a.Pattern != "/api/notes/logout" {
		t.Errorf("route authenticated = %+v", a)
	}
	if g := got["GET"]; g.Authenticated {
		t.Errorf("route biasa tidak boleh Authenticated: %+v", g)
	}
}

func TestRegister_AuthenticatedMisuseRejected(t *testing.T) {
	cases := map[string]struct {
		reg  func(r module.Registrar)
		code string
	}{
		"permission kosong tanpa opsi tetap ditolak": {func(r module.Registrar) { r.Handle("GET", "/a", "", okHandler) }, CodePermissionMissing},
		"Authenticated bersama permission":           {func(r module.Registrar) { r.Handle("GET", "/a", "notes.items.read", okHandler, module.Authenticated()) }, CodeInvalidRouteOption},
		"Authenticated pada Public":                  {func(r module.Registrar) { r.Public("GET", "/a", okHandler, module.Authenticated()) }, CodeInvalidRouteOption},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Build(testOpts(), notes(func(r module.Registrar) error { c.reg(r); return nil }))
			var be *Error
			if !errors.As(err, &be) || len(be.Problems) != 1 || be.Problems[0].Code != c.code {
				t.Fatalf("err = %v, want satu problem %s", err, c.code)
			}
		})
	}
}
