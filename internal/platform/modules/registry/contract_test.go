package registry

import (
	"errors"
	"strings"
	"testing"

	"erpora/sdk/module"
)

const validContract = `openapi: 3.1.0
info: { title: t, version: '1' }
paths:
  /api/withc/items:
    get:
      operationId: listItems
      x-erp-permission: withc.items.read
      responses: { '200': { description: ok } }
`

// contractModule menambah module.ContractSource pada fakeModule.
type contractModule struct {
	fakeModule
	doc []byte
}

func (c contractModule) OpenAPI() []byte { return c.doc }

func TestBuild_StoresContracts(t *testing.T) {
	plain := newFake(manifestSpec{key: "acme.plain", slug: "plain"}, nil)
	withC := contractModule{fakeModule: newFake(manifestSpec{key: "acme.withc", slug: "withc", caps: []string{"withc.items.read"}}, nil), doc: []byte(validContract)}
	opts := testOpts()
	opts.PlatformContract = []byte(strings.ReplaceAll(validContract, "/api/withc/items", "/api/platform/x"))

	reg, err := Build(opts, plain, withC)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := reg.Entry("acme.withc"); string(e.Contract) != validContract {
		t.Errorf("Contract modul tidak tersimpan: %q", e.Contract)
	}
	if e, _ := reg.Entry("acme.plain"); e.Contract != nil {
		t.Errorf("modul tanpa ContractSource harus Contract nil, got %q", e.Contract)
	}
	if e, _ := reg.Entry(PlatformKey); string(e.Contract) != string(opts.PlatformContract) {
		t.Errorf("Contract platform.core tidak tersimpan: %q", e.Contract)
	}
	for _, e := range reg.Entries() {
		if e.Manifest.Key == "acme.withc" && string(e.Contract) != validContract {
			t.Error("Entries() harus membawa Contract")
		}
	}
}

func TestBuild_InvalidContractIsProblem(t *testing.T) {
	bad := contractModule{fakeModule: newFake(manifestSpec{key: "acme.bad", slug: "bad"}, nil), doc: []byte("openapi: [3.1.0\n")}
	_, err := Build(testOpts(), bad)
	var re *Error
	if !errors.As(err, &re) {
		t.Fatalf("err = %v, want *registry.Error", err)
	}
	found := false
	for _, p := range re.Problems {
		if p.Code == CodeInvalidContract && p.Module == "acme.bad" {
			found = true
		}
	}
	if !found {
		t.Errorf("problem invalid_contract tidak ada: %+v", re.Problems)
	}

	opts := testOpts()
	opts.PlatformContract = []byte("openapi: 3.0.3\ninfo: {title: x, version: '1'}\npaths: {}\n")
	_, err = Build(opts)
	if !errors.As(err, &re) || !hasCode(re.Problems, CodeInvalidContract, PlatformKey) {
		t.Errorf("kontrak platform rusak harus invalid_contract: %v", err)
	}
}

func hasCode(ps []Problem, code, mod string) bool {
	for _, p := range ps {
		if p.Code == code && p.Module == mod {
			return true
		}
	}
	return false
}

var _ module.ContractSource = contractModule{}
