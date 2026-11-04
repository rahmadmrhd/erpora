package manifest

import (
	"strings"
	"testing"

	"erpora/sdk/module"
)

const itemResource = `{"key": "inventory.item", "label": "resource.item", "actions": ["read", "update"],
  "scope_fields": {"inventory.warehouse": "warehouse_id"},
  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}`

const warehouseDim = `{"key": "inventory.warehouse", "label": "dimension.warehouse"}`

// authzManifest membangun manifest slug inventory dengan bagian otorisasi yang diberikan (JSON mentah).
func authzManifest(t *testing.T, slug, resources, dims, roles string) module.Manifest {
	t.Helper()
	if resources == "" {
		resources = "[]"
	}
	if dims == "" {
		dims = "[]"
	}
	if roles == "" {
		roles = "[]"
	}
	raw := `{
  "schema_version": "1", "key": "acme.` + slug + `", "slug": "` + slug + `", "name": "X", "version": "1.0.0",
  "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"},
  "dependencies": [{"module": "platform.core", "version": "^0.1", "optional": false}],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "resources": ` + resources + `, "scope_dimensions": ` + dims + `, "default_roles": ` + roles + `
}`
	m, err := module.ParseManifest([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ps := Validate(m); len(ps) != 0 {
		t.Fatalf("manifest uji harus lolos schema: %+v", ps)
	}
	return m
}

func hasProblem(ps []Problem, code, field, msgPart string) bool {
	for _, p := range ps {
		if p.Code == code && p.Field == field && strings.Contains(p.Message, msgPart) {
			return true
		}
	}
	return false
}

func TestValidAuthorizationManifest(t *testing.T) {
	m := authzManifest(t, "inventory", `[`+itemResource+`]`, `[`+warehouseDim+`]`,
		`[{"key": "inventory.staff", "label": "role.staff",
		   "permissions": [{"resource": "inventory.item", "actions": ["read"], "scope": ["owner", "assigned:inventory.warehouse"]},
		                   {"resource": "inventory.item", "actions": ["update"], "scope": ["custom:inventory.mine"]}],
		   "field_access": [{"resource": "inventory.item", "field_group": "general", "access": "read"}]}]`)
	if ps := ValidateAuthorization(m, nil); len(ps) != 0 {
		t.Fatalf("manifest valid ditolak: %+v", ps)
	}
}

func TestManifestWithoutResourcesStaysValid(t *testing.T) {
	m := authzManifest(t, "inventory", "", "", "")
	if ps := ValidateAuthorization(m, nil); len(ps) != 0 {
		t.Fatalf("manifest lama tanpa resource harus valid: %+v", ps)
	}
}

func TestManifestRejectsCustomActionClash(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "custom_actions": [{"key": "approve", "label": "action.approve"}],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]`, "", "")
	ps := ValidateAuthorization(m, nil)
	if !hasProblem(ps, CodeInvalidResource, "/resources/0/custom_actions/0/key", `"approve"`) {
		t.Fatalf("bentrok aksi standar tidak dilaporkan: %+v", ps)
	}
	if !strings.Contains(ps[0].Message, "standar") || ps[0].Module != "acme.inventory" {
		t.Errorf("pesan harus menyebut aksi standar dan modul: %+v", ps[0])
	}
}

func TestManifestRejectsDuplicateCustomAction(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "custom_actions": [{"key": "adjust", "label": "action.adjust"}, {"key": "adjust", "label": "action.adjust"}],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]`, "", "")
	if ps := ValidateAuthorization(m, nil); !hasProblem(ps, CodeInvalidResource, "/resources/0/custom_actions/1/key", `"adjust"`) {
		t.Fatalf("aksi kustom ganda tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsNonStandardAction(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read", "adjust"],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]`, "", "")
	if ps := ValidateAuthorization(m, nil); !hasProblem(ps, CodeInvalidResource, "/resources/0/actions/1", `"adjust"`) {
		t.Fatalf("aksi non-standar di actions tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsResourceWithoutAnyAction(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": [],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]`, "", "")
	if ps := ValidateAuthorization(m, nil); !hasProblem(ps, CodeInvalidResource, "/resources/0/actions", "minimal satu aksi") {
		t.Fatalf("resource tanpa aksi tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsUnknownScopeDimension(t *testing.T) {
	m := authzManifest(t, "inventory", `[`+itemResource+`]`, "", "")
	ps := ValidateAuthorization(m, nil)
	if !hasProblem(ps, CodeInvalidResource, "/resources/0/scope_fields/inventory.warehouse", `"inventory.warehouse"`) {
		t.Fatalf("dimensi tak terdaftar tidak dilaporkan: %+v", ps)
	}
}

func TestScopeFieldsMayReferenceDependencyDimension(t *testing.T) {
	// Resource modul sales memetakan dimensi inventory.warehouse milik modul lain.
	sales := func(deps string) module.Manifest {
		m, err := module.ParseManifest([]byte(`{
  "schema_version": "1", "key": "acme.sales", "slug": "sales", "name": "S", "version": "1.0.0",
  "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"},
  "dependencies": [{"module": "platform.core", "version": "^0.1", "optional": false}` + deps + `],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "resources": [{"key": "sales.order", "label": "resource.order", "actions": ["read"],
    "scope_fields": {"inventory.warehouse": "warehouse_id"},
    "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]
}`))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	inventory := authzManifest(t, "inventory", `[`+itemResource+`]`, `[`+warehouseDim+`]`, "")

	withDep := sales(`, {"module": "acme.inventory", "version": "^1.0", "optional": false}`)
	if ps := ValidateAuthorization(withDep, []module.Manifest{inventory}); len(ps) != 0 {
		t.Fatalf("dimensi dependensi harus diterima: %+v", ps)
	}
	noDep := sales("")
	if ps := ValidateAuthorization(noDep, []module.Manifest{inventory}); !hasProblem(ps, CodeInvalidResource, "/resources/0/scope_fields/inventory.warehouse", "dependencies") {
		t.Fatalf("dimensi modul non-dependensi harus ditolak: %+v", ps)
	}
}

func TestOrgUnitDimensionIsPlatformProvided(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "scope_fields": {"core.org_unit": "branch_id"},
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]`, "", "")
	if ps := ValidateAuthorization(m, nil); len(ps) != 0 {
		t.Fatalf("core.org_unit disediakan platform: %+v", ps)
	}
}

func TestManifestRejectsResourceWithoutDefaultGroup(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general"}]}]`, "", "")
	ps := ValidateAuthorization(m, nil)
	if !hasProblem(ps, CodeInvalidResource, "/resources/0/field_groups", "grup default") {
		t.Fatalf("resource tanpa grup default tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsTwoDefaultGroups(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true},
	                   {"key": "extra", "label": "fieldGroup.extra", "default": true}]}]`, "", "")
	ps := ValidateAuthorization(m, nil)
	if !hasProblem(ps, CodeInvalidResource, "/resources/0/field_groups/1/default", `"general"`) {
		t.Fatalf("dua grup default tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsFieldInTwoGroups(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true, "fields": ["name", "cost"]},
	                   {"key": "pricing", "label": "fieldGroup.pricing", "fields": ["cost"]}]}]`, "", "")
	ps := ValidateAuthorization(m, nil)
	if !hasProblem(ps, CodeInvalidResource, "/resources/0/field_groups/1/fields/0", `"cost"`) {
		t.Fatalf("field di dua grup tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsDuplicateFieldGroupKey(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "inventory.item", "label": "resource.item", "actions": ["read"],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true},
	                   {"key": "general", "label": "fieldGroup.general"}]}]`, "", "")
	if ps := ValidateAuthorization(m, nil); !hasProblem(ps, CodeInvalidResource, "/resources/0/field_groups/1/key", `"general"`) {
		t.Fatalf("kunci grup ganda tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsResourceKeyOutsideSlug(t *testing.T) {
	m := authzManifest(t, "inventory", `[{"key": "sales.item", "label": "resource.item", "actions": ["read"],
	  "field_groups": [{"key": "general", "label": "fieldGroup.general", "default": true}]}]`, "", "")
	if ps := ValidateAuthorization(m, nil); !hasProblem(ps, CodeInvalidResource, "/resources/0/key", `"inventory."`) {
		t.Fatalf("awalan slug tidak dilaporkan: %+v", ps)
	}
}

func TestManifestRejectsDuplicateResourceAcrossModules(t *testing.T) {
	a := authzManifest(t, "inventory", `[`+itemResource+`]`, `[`+warehouseDim+`]`, "")
	// Modul lain berslug sama-prefiks tidak mungkin sah; uji keunikan lewat peers yang menduplikasi kunci.
	dup := a
	dup.Key = "acme.inventory2"
	ps := ValidateAuthorization(a, []module.Manifest{dup})
	if !hasProblem(ps, CodeInvalidResource, "/resources/0/key", "acme.inventory2") {
		t.Fatalf("resource ganda lintas modul tidak dilaporkan: %+v", ps)
	}
	if !hasProblem(ps, CodeInvalidScopeDimension, "/scope_dimensions/0/key", "acme.inventory2") {
		t.Fatalf("dimensi ganda lintas modul tidak dilaporkan: %+v", ps)
	}
}

func TestDefaultRoleRowValidated(t *testing.T) {
	res := `[` + itemResource + `]`
	dims := `[` + warehouseDim + `]`
	cases := map[string]struct {
		role           string
		field, msgPart string
	}{
		"kunci di luar slug": {
			`{"key": "sales.manager", "label": "role.manager", "permissions": []}`,
			"/default_roles/0/key", `"inventory."`,
		},
		"resource tak dikenal": {
			`{"key": "inventory.m", "label": "role.m", "permissions": [{"resource": "inventory.nope", "actions": ["read"], "scope": ["all"]}]}`,
			"/default_roles/0/permissions/0/resource", `"inventory.nope"`,
		},
		"resource modul lain": {
			`{"key": "inventory.m", "label": "role.m", "permissions": [{"resource": "sales.order", "actions": ["read"], "scope": ["all"]}]}`,
			"/default_roles/0/permissions/0/resource", `"sales.order"`,
		},
		"aksi tak dideklarasikan resource": {
			`{"key": "inventory.m", "label": "role.m", "permissions": [{"resource": "inventory.item", "actions": ["delete"], "scope": ["all"]}]}`,
			"/default_roles/0/permissions/0/actions/0", `"delete"`,
		},
		"assigned dimensi tak dipetakan resource": {
			`{"key": "inventory.m", "label": "role.m", "permissions": [{"resource": "inventory.item", "actions": ["read"], "scope": ["assigned:inventory.zone"]}]}`,
			"/default_roles/0/permissions/0/scope/0", `"inventory.zone"`,
		},
		"field group tak dikenal": {
			`{"key": "inventory.m", "label": "role.m", "permissions": [], "field_access": [{"resource": "inventory.item", "field_group": "pricing", "access": "read"}]}`,
			"/default_roles/0/field_access/0/field_group", `"pricing"`,
		},
		"field access resource tak dikenal": {
			`{"key": "inventory.m", "label": "role.m", "permissions": [], "field_access": [{"resource": "inventory.nope", "field_group": "general", "access": "read"}]}`,
			"/default_roles/0/field_access/0/resource", `"inventory.nope"`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := authzManifest(t, "inventory", res, dims, `[`+c.role+`]`)
			ps := ValidateAuthorization(m, nil)
			if !hasProblem(ps, CodeInvalidDefaultRole, c.field, c.msgPart) {
				t.Fatalf("problem tidak ditemukan di %s: %+v", c.field, ps)
			}
		})
	}
}

func TestDefaultRoleDuplicateKey(t *testing.T) {
	role := `{"key": "inventory.m", "label": "role.m", "permissions": []}`
	m := authzManifest(t, "inventory", "", "", `[`+role+`,`+role+`]`)
	if ps := ValidateAuthorization(m, nil); !hasProblem(ps, CodeInvalidDefaultRole, "/default_roles/1/key", `"inventory.m"`) {
		t.Fatalf("role ganda tidak dilaporkan: %+v", ps)
	}
}

func TestSchemaRejectsEmptyScopeAndBadToken(t *testing.T) {
	for name, scope := range map[string]string{
		"scope kosong": `[]`, "token tak dikenal": `["team"]`, "assigned tanpa dimensi": `["assigned:"]`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{
  "schema_version": "1", "key": "acme.inventory", "slug": "inventory", "name": "X", "version": "1.0.0",
  "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "default_roles": [{"key": "inventory.m", "label": "role.m",
    "permissions": [{"resource": "inventory.item", "actions": ["read"], "scope": ` + scope + `}]}]
}`
			m, err := module.ParseManifest([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			ps := Validate(m)
			if len(ps) == 0 || ps[0].Code != CodeSchemaInvalid || !strings.HasPrefix(ps[0].Field, "/default_roles/0/permissions/0/scope") {
				t.Fatalf("scope tak sah harus ditolak schema: %+v", ps)
			}
		})
	}
}

func TestSchemaRejectsLiteralLabel(t *testing.T) {
	raw := `{
  "schema_version": "1", "key": "acme.inventory", "slug": "inventory", "name": "X", "version": "1.0.0",
  "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "resources": [{"key": "inventory.item", "label": "Barang", "actions": ["read"],
    "field_groups": [{"key": "general", "label": "Umum", "default": true}]}]
}`
	m, err := module.ParseManifest([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	ps := Validate(m)
	fields := map[string]bool{}
	for _, p := range ps {
		fields[p.Field] = p.Code == CodeSchemaInvalid
	}
	if !fields["/resources/0/label"] || !fields["/resources/0/field_groups/0/label"] {
		t.Fatalf("label literal harus ditolak (K-C2.5): %+v", ps)
	}
}

func TestCapabilitiesMustBeDerivedFromResources(t *testing.T) {
	raw := func(caps string) module.Manifest {
		m, err := module.ParseManifest([]byte(`{
  "schema_version": "1", "key": "acme.inventory", "slug": "inventory", "name": "X", "version": "1.0.0",
  "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": ` + caps + `, "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "resources": [` + itemResource + `], "scope_dimensions": [` + warehouseDim + `]
}`))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if ps := ValidateAuthorization(raw(`["inventory.item.read"]`), nil); len(ps) != 0 {
		t.Fatalf("capability turunan harus diterima: %+v", ps)
	}
	if ps := ValidateAuthorization(raw(`[]`), nil); len(ps) != 0 {
		t.Fatalf("capabilities kosong harus diterima: %+v", ps)
	}
	ps := ValidateAuthorization(raw(`["inventory.notes.read"]`), nil)
	if !hasProblem(ps, CodeCapabilityNotDerived, "/capabilities/0", `"inventory.notes.read"`) {
		t.Fatalf("capability di luar resource harus ditolak: %+v", ps)
	}
}

func TestValidFullFixtureHasValidAuthorization(t *testing.T) {
	m := load(t, "valid_full.json")
	if ps := ValidateAuthorization(m, nil); len(ps) != 0 {
		t.Fatalf("fixture valid_full harus lolos: %+v", ps)
	}
}
