package check

import (
	"strings"
	"testing"
)

const customerSrc = `fields:
  name: {type: string, maxLength: 100, filterable: true, snapshotSource: true}
  status: {type: enum, values: [active, inactive], filterable: true}
  hidden: {type: text, nullable: true}
`

func crmModule(t *testing.T) Module {
	return mod(t, "crm", nil, f("crm/customer.resource.yaml", resource("crm.customer", "export: true\n"+customerSrc)))
}

func TestRelationsSameModulePass(t *testing.T) {
	rep := single(t,
		f("m/c.resource.yaml", resource("m.customer", customerSrc)),
		f("m/line.resource.yaml", resource("m.line", "fields:\n  qty: {type: integer}\n")),
		f("m/tag.resource.yaml", resource("m.tag", "fields:\n  label: {type: string, maxLength: 20}\n")),
		f("m/note.resource.yaml", resource("m.note", "fields:\n  body: {type: text}\n")),
		f("m/r.resource.yaml", resource("m.r", `fields:
  customerName: {type: string, maxLength: 100, readOnly: true, fieldGroup: general, classification: internal}
relations:
  customer: {type: belongsTo, to: m.customer, required: true, onDelete: restrict, filter: {field: status, op: eq, value: active}}
  profile: {type: hasOne, to: m.customer}
  lines: {type: hasMany, of: m.line, embedded: true}
  tags: {type: belongsToMany, to: m.tag, pivotFields: {note: {type: text, nullable: true}}}
  subject: {type: morphTo, types: [m.customer, m.tag]}
  notes: {type: morphMany, of: m.note, as: subject}
  labels: {type: morphToMany, to: m.tag}
indexes:
  - fields: [customerId]
`)))
	noErrors(t, rep)
}

func TestRelationTargetsMustExist(t *testing.T) {
	t.Run("satu modul tidak ada", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "relations:\n  c: {type: belongsTo, to: m.ghost}\n")))
		wantErr(t, rep, CodeRelationTargetMissing, "/relations/c")
	})
	t.Run("target abstrak bukan resource", func(t *testing.T) {
		rep := single(t,
			f("m/a.abstract.yaml", abstract("m.blk", "fields:\n  a: {type: text}\n")),
			f("m/r.resource.yaml", resource("m.r", "relations:\n  c: {type: belongsTo, to: m.blk}\n")))
		wantErr(t, rep, CodeRelationTargetMissing, "/relations/c")
	})
	t.Run("hasMany of tidak ada", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "relations:\n  c: {type: hasMany, of: m.ghost}\n")))
		wantErr(t, rep, CodeRelationTargetMissing, "/relations/c")
	})
	t.Run("morph types tidak ada", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "relations:\n  s: {type: morphTo, types: [m.ghost]}\n")))
		wantErr(t, rep, CodeRelationTargetMissing, "/relations/s/types/0")
	})
	t.Run("modul tidak dikenal", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "relations:\n  c: {type: belongsTo, to: ghost.thing}\n")))
		wantErr(t, rep, CodeRelationTargetMissing, "/relations/c")
	})
}

func TestCrossModuleRelationsRequireManifestDependency(t *testing.T) {
	rSrc := f("sales/r.resource.yaml", resource("sales.r", `relations:
  customer: {type: belongsTo, to: crm.customer}
  types: {type: morphTo, types: [crm.customer]}
`))
	t.Run("dengan dependensi", func(t *testing.T) {
		rep := run(t, crmModule(t), mod(t, "sales", []string{"acme.crm"}, rSrc))
		noErrors(t, rep)
	})
	t.Run("tanpa dependensi", func(t *testing.T) {
		rep := run(t, crmModule(t), mod(t, "sales", nil, rSrc))
		wantErr(t, rep, CodeRelationModuleDependency, "/relations/customer")
		wantErr(t, rep, CodeRelationModuleDependency, "/relations/types/types/0")
	})
	t.Run("dependensi ada tetapi resource tidak ada", func(t *testing.T) {
		rep := run(t, crmModule(t), mod(t, "sales", []string{"acme.crm"},
			f("sales/r.resource.yaml", resource("sales.r", "relations:\n  c: {type: belongsTo, to: crm.ghost}\n"))))
		wantErr(t, rep, CodeRelationTargetMissing, "/relations/c")
	})
}

func TestEmbeddedChildMustBeSameModule(t *testing.T) {
	rep := run(t,
		mod(t, "crm", nil, f("crm/line.resource.yaml", resource("crm.line", "export: true\nfields:\n  q: {type: integer}\n"))),
		mod(t, "sales", []string{"acme.crm"}, f("sales/r.resource.yaml", resource("sales.r", "relations:\n  lines: {type: hasMany, of: crm.line, embedded: true}\n"))))
	wantErr(t, rep, CodeRelationEmbeddedModule, "/relations/lines")
}

func TestRelationFilters(t *testing.T) {
	tests := []struct {
		name, rel, code, ptr string
	}{
		{"field seberang tidak ada", "customer: {type: belongsTo, to: m.customer, filter: {field: ghost, op: eq, value: x}}", CodeRelationFilterUnknownField, "/relations/customer/filter"},
		{"field seberang tidak filterable", "customer: {type: belongsTo, to: m.customer, filter: {field: hidden, op: isnull, value: true}}", CodeRelationFilterNotFilterable, "/relations/customer/filter"},
		{"ref ke field sendiri tidak ada", "customer: {type: belongsTo, to: m.customer, filter: {field: name, op: eq, ref: ghost}}", CodeRelationFilterUnknownRef, "/relations/customer/filter"},
		{"nilai tidak cocok tipe field seberang", "customer: {type: belongsTo, to: m.customer, filter: {field: status, op: eq, value: ghost}}", CodeConditionValueType, "/relations/customer/filter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t,
				f("m/c.resource.yaml", resource("m.customer", customerSrc)),
				f("m/r.resource.yaml", resource("m.r", "fields:\n  zone: {type: string, maxLength: 10}\nrelations:\n  "+tc.rel+"\n")))
			wantErr(t, rep, tc.code, tc.ptr)
		})
	}
	t.Run("ref ke field sendiri dan ke relasi sah", func(t *testing.T) {
		rep := single(t,
			f("m/c.resource.yaml", resource("m.customer", customerSrc)),
			f("m/r.resource.yaml", resource("m.r", `fields:
  zone: {type: string, maxLength: 100}
relations:
  customer: {type: belongsTo, to: m.customer, filter: {field: name, op: eq, ref: zone}}
`)))
		noErrors(t, rep)
	})
	t.Run("siklus rujukan antar relasi", func(t *testing.T) {
		rep := single(t,
			f("m/c.resource.yaml", resource("m.customer", "fields:\n  zone: {type: string, maxLength: 10, filterable: true}\n")),
			f("m/r.resource.yaml", resource("m.r", `relations:
  a: {type: belongsTo, to: m.customer, filter: {field: zone, op: eq, ref: b}}
  b: {type: belongsTo, to: m.customer, filter: {field: zone, op: eq, ref: a}}
`)))
		wantErr(t, rep, CodeRelationFilterCycle, "/relations/")
	})
	t.Run("rujukan diri sendiri", func(t *testing.T) {
		rep := single(t,
			f("m/c.resource.yaml", resource("m.customer", "fields:\n  zone: {type: string, maxLength: 10, filterable: true}\n")),
			f("m/r.resource.yaml", resource("m.r", "relations:\n  a: {type: belongsTo, to: m.customer, filter: {field: zone, op: eq, ref: a}}\n")))
		wantErr(t, rep, CodeRelationFilterCycle, "/relations/a")
	})
}

func TestForeignKeyWithoutIndexWarns(t *testing.T) {
	c := f("m/c.resource.yaml", resource("m.customer", customerSrc))
	warn := func(rel, idx string) *Report {
		return single(t, c, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\nrelations:\n  "+rel+"\n"+idx)))
	}
	rep := warn("customer: {type: belongsTo, to: m.customer}", "")
	noErrors(t, rep)
	wantWarning(t, rep, CodeRelationFKNoIndex, "/relations/customer")
	// indeks yang diawali kolom FK menutup peringatan.
	rep = warn("customer: {type: belongsTo, to: m.customer}", "indexes:\n  - fields: [customerId, a]\n")
	for _, p := range rep.Problems {
		if p.Code == CodeRelationFKNoIndex {
			t.Errorf("tidak boleh ada peringatan: %+v", p)
		}
	}
	// kolom FK bukan kolom pertama: tetap peringatan.
	rep = warn("customer: {type: belongsTo, to: m.customer}", "indexes:\n  - fields: [a, customerId]\n")
	wantWarning(t, rep, CodeRelationFKNoIndex, "/relations/customer")
	// lintas modul tidak punya FK: tanpa peringatan.
	rep2 := run(t, crmModule(t), mod(t, "sales", []string{"acme.crm"},
		f("sales/r.resource.yaml", resource("sales.r", "relations:\n  customer: {type: belongsTo, to: crm.customer}\n"))))
	noCode(t, rep2, CodeRelationFKNoIndex)
}

func TestIndexesReferenceExistingColumns(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\nindexes:\n  - fields: [a, ghost]\n")))
	wantErr(t, rep, CodeIndexUnknownField, "/indexes/0")
}

// ---- integrasi manifest ----

func manifestMod(t *testing.T, slug string, resources []ManifestResource, files ...file) Module {
	m := mod(t, slug, nil, files...)
	m.Manifest.HasResources = true
	m.Manifest.Resources = resources
	return m
}

func TestResourceKeyMustMatchManifest(t *testing.T) {
	src := f("m/r.resource.yaml", resource("m.invoiceLine", "fields:\n  a: {type: text}\n"))
	t.Run("sebelum PRD-001: prefiks capability", func(t *testing.T) {
		m := mod(t, "m", nil, src)
		m.Manifest.Capabilities = []string{"m.other.read"}
		wantErr(t, run(t, m), CodeManifestCapabilityPrefix, "")
		noErrors(t, single(t, src))
	})
	t.Run("manifest memuat resources: kunci harus terdaftar", func(t *testing.T) {
		rep := run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.other", Actions: []string{"read"}}}, src))
		wantErr(t, rep, CodeManifestResourceMissing, "")
	})
	t.Run("kunci multi-kata dikonversi ke snake_case", func(t *testing.T) {
		rep := run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.invoice_line", Actions: []string{"read"}, FieldGroups: []ManifestGroup{{Key: "general", Default: true}}}}, src))
		noErrors(t, rep)
	})
}

func TestFieldGroupMembershipAgainstManifest(t *testing.T) {
	src := f("m/r.resource.yaml", resource("m.r", `fields:
  plain: {type: text}
  priced: {type: decimal, precision: 10, scale: 2, fieldGroup: pricing}
  ghosted: {type: text, fieldGroup: ghost}
`))
	groups := []ManifestGroup{{Key: "general", Default: true}, {Key: "pricing", Classification: "confidential"}}
	rep := run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: []string{"read"}, FieldGroups: groups}}, src))
	wantErr(t, rep, CodeManifestGroupUnknown, "/fields/ghosted")
	for _, p := range rep.Problems {
		if p.Code == CodeManifestGroupUnknown && !strings.Contains(p.Pointer, "ghosted") {
			t.Errorf("hanya ghosted yang salah grup: %+v", p)
		}
	}
	info := rep.Resource("m.r")
	if info.Group["plain"] != "general" || info.Group["priced"] != "pricing" {
		t.Errorf("grup default mengikuti grup bertanda default manifest: %v", info.Group)
	}
	t.Run("grup default literal bukan kunci khusus", func(t *testing.T) {
		rep := run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: []string{"read"}, FieldGroups: groups}},
			f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text, fieldGroup: default}\n"))))
		wantErr(t, rep, CodeManifestGroupUnknown, "/fields/a")
	})
	t.Run("definisi resource tanpa field_groups: hanya default implisit", func(t *testing.T) {
		rep := run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: []string{"read"}}},
			f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text, fieldGroup: pricing}\n"))))
		wantErr(t, rep, CodeManifestGroupUnknown, "/fields/a")
	})
}

func TestManifestFieldListsMustMatchMetadata(t *testing.T) {
	src := f("m/r.resource.yaml", resource("m.r", `fields:
  name: {type: text}
  unitPrice: {type: decimal, precision: 10, scale: 2, fieldGroup: pricing}
  total: {type: decimal, precision: 10, scale: 2, fieldGroup: pricing}
`))
	mk := func(pricingFields []string) *Report {
		return run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: []string{"read"}, FieldGroups: []ManifestGroup{
			{Key: "general", Default: true}, {Key: "pricing", Fields: pricingFields},
		}}}, src))
	}
	noErrors(t, mk([]string{"unit_price", "total"}))
	noErrors(t, mk(nil)) // fields opsional
	wantErr(t, mk([]string{"unit_price"}), CodeManifestFieldsMismatch, "/fields/total")
	wantErr(t, mk([]string{"unit_price", "total", "extra"}), CodeManifestFieldsMismatch, "")
	wantErr(t, mk([]string{"unit_price", "total", "name"}), CodeManifestFieldsMismatch, "/fields/name")
}

func TestOperationPermissionsMustBeRegistered(t *testing.T) {
	src := f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\ngenerate: {crud: true}\n"))
	res := func(actions ...string) *Report {
		return run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: actions, FieldGroups: []ManifestGroup{{Key: "general", Default: true}}}}, src))
	}
	noErrors(t, res("read", "create", "update", "delete"))
	wantErr(t, res("read", "create", "update"), CodeManifestActionMissing, "/generate")
	// capability juga harus terdaftar.
	m := manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: []string{"read", "create", "update", "delete"}}}, src)
	m.Manifest.Capabilities = []string{"m.r.read", "m.r.create", "m.r.update"}
	wantErr(t, run(t, m), CodeManifestActionMissing, "/generate")
	// tanpa crud tidak ada aksi wajib.
	noErrors(t, run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r"}}, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\n")))))
	// dokumen butuh submit, cancel, amend.
	doc := f("m/d.resource.yaml", resource("m.d", "include:\n  - source: sdk:document\ngenerate: {crud: true}\n"))
	rep := run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.d", Actions: []string{"read", "create", "update", "delete", "submit"}}}, doc))
	wantErr(t, rep, CodeManifestActionMissing, "/generate")
}

func TestOwnerAndScopeFields(t *testing.T) {
	mk := func(ownerAndScope string, manifest ManifestResource) *Report {
		src := f("m/r.resource.yaml", resource("m.r", "include:\n  - source: sdk:audit\nfields:\n  branchId: {type: uuid, nullable: true}\n  title: {type: string, maxLength: 5}\n"+ownerAndScope))
		manifest.Key = "m.r"
		return run(t, manifestMod(t, "m", []ManifestResource{manifest}, src))
	}
	ok := ManifestResource{OwnerField: "created_by", ScopeFields: map[string]string{"m.branch": "branch_id"}}
	noErrors(t, mk("ownerField: createdBy\nscopeFields: {m.branch: branchId}\n", ok))
	// hanya di manifest (metadata tidak mengulang) juga diperiksa.
	noErrors(t, mk("", ok))
	wantErr(t, mk("ownerField: ghost\n", ManifestResource{}), CodeOwnerFieldUnknown, "/ownerField")
	wantErr(t, mk("ownerField: title\n", ManifestResource{}), CodeOwnerFieldType, "/ownerField")
	wantErr(t, mk("scopeFields: {m.branch: ghost}\n", ManifestResource{}), CodeScopeFieldUnknown, "/scopeFields")
	wantErr(t, mk("scopeFields: {m.branch: title}\n", ManifestResource{}), CodeScopeFieldType, "/scopeFields")
	wantErr(t, mk("ownerField: createdBy\n", ManifestResource{OwnerField: "updated_by"}), CodeOwnerManifestMismatch, "/ownerField")
	wantErr(t, mk("scopeFields: {m.branch: branchId}\n", ManifestResource{ScopeFields: map[string]string{"m.branch": "title"}}), CodeOwnerManifestMismatch, "/scopeFields")
	wantErr(t, mk("", ManifestResource{OwnerField: "ghost_col"}), CodeOwnerFieldUnknown, "")
}
