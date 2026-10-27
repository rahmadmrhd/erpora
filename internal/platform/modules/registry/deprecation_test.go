package registry

import (
	"strings"
	"testing"

	"erpora/sdk/module"
)

const deprecatedContract = `openapi: 3.1.0
info: { title: t, version: '1' }
paths:
  /api/depr/old-things:
    get:
      operationId: listOldThings
      deprecated: true
      x-erp-permission: depr.things.read
      responses: { '200': { description: ok } }
  /api/depr/things:
    get:
      operationId: listThings
      x-erp-permission: depr.things.read
      parameters:
        - { name: legacy, in: query, deprecated: true, schema: { type: string } }
        - { name: other, in: query, schema: { type: string } }
        - { name: legacyHeader, in: header, deprecated: true, schema: { type: string } }
      responses: { '200': { description: ok } }
  /api/depr/fresh-things:
    get:
      operationId: listFreshThings
      x-erp-permission: depr.things.read
      responses: { '200': { description: ok } }
`

func deprModule(register func(r module.Registrar) error, contract string) contractModule {
	return contractModule{
		fakeModule: newFake(manifestSpec{key: "acme.depr", slug: "depr", caps: []string{"depr.things.read"}}, register),
		doc:        []byte(contract),
	}
}

func routeOf(t *testing.T, reg *Registry, pattern string) RouteInfo {
	t.Helper()
	for _, r := range reg.Routes() {
		if r.Pattern == pattern {
			return r
		}
	}
	t.Fatalf("route %s tidak ada", pattern)
	return RouteInfo{}
}

func TestBuild_ContractDeprecationMapped(t *testing.T) {
	m := deprModule(func(r module.Registrar) error {
		r.Handle("GET", "/old-things", "depr.things.read", okHandler)
		r.Handle("GET", "/things", "depr.things.read", okHandler)
		r.Handle("GET", "/fresh-things", "depr.things.read", okHandler)
		return nil
	}, deprecatedContract)
	reg, err := Build(testOpts(), m)
	if err != nil {
		t.Fatal(err)
	}
	old := routeOf(t, reg, "/api/depr/old-things")
	if !old.Deprecated || len(old.DeprecatedParams) != 0 {
		t.Errorf("old-things = %+v, want Deprecated tanpa parameter", old)
	}
	things := routeOf(t, reg, "/api/depr/things")
	if things.Deprecated || strings.Join(things.DeprecatedParams, ",") != "legacy" {
		t.Errorf("things = Deprecated %v params %v, want false dan [legacy] (hanya parameter query)", things.Deprecated, things.DeprecatedParams)
	}
	fresh := routeOf(t, reg, "/api/depr/fresh-things")
	if fresh.Deprecated || len(fresh.DeprecatedParams) != 0 {
		t.Errorf("fresh-things = %+v, want tidak deprecated", fresh)
	}
}

// platform.core: route gabungan GET /modules/{key}/{sub} melayani dua operasi; deprecated hanya bila semuanya deprecated.
func TestBuild_PlatformContractDeprecationByDispatch(t *testing.T) {
	contract := func(histDeprecated, opDeprecated string) []byte {
		return []byte(`openapi: 3.1.0
info: { title: p, version: '1' }
paths:
  /api/platform/modules/{key}/history:
    get:
      operationId: listModuleHistory
      deprecated: ` + histDeprecated + `
      x-erp-permission: platform.modules.read
      parameters: [{ name: key, in: path, required: true, schema: { type: string } }]
      responses: { '200': { description: ok } }
  /api/platform/modules/operations/{id}:
    get:
      operationId: getModuleOperation
      deprecated: ` + opDeprecated + `
      x-erp-permission: platform.modules.read
      parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
      responses: { '200': { description: ok } }
`)
	}
	build := func(hist, op string) *Registry {
		opts := testOpts()
		opts.PlatformCapabilities = []string{"platform.modules.read"}
		opts.PlatformContract = contract(hist, op)
		opts.Platform = func(r module.Registrar) error {
			r.Handle("GET", "/modules/{key}/{sub}", "platform.modules.read", okHandler)
			return nil
		}
		reg, err := Build(opts)
		if err != nil {
			t.Fatal(err)
		}
		return reg
	}
	const pat = "/api/platform/modules/{key}/{sub}"
	if r := routeOf(t, build("true", "true"), pat); !r.Deprecated {
		t.Error("semua operasi yang dilayani deprecated -> route deprecated")
	}
	if r := routeOf(t, build("true", "false"), pat); r.Deprecated {
		t.Error("hanya sebagian operasi deprecated -> route tidak deprecated")
	}
}
