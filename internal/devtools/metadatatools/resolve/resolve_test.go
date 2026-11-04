package resolve

import (
	"reflect"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/load"
)

func TestIncludeOwnModuleAndSDK(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "sales"},
			f("sales/timestamps.abstract.yaml", abstract("sales.stamps", "fields:\n  note: {type: text, nullable: true}\n")),
			f("sales/invoice.resource.yaml", resource("sales.invoice", `include:
  - source: sdk:document
  - source: sales.stamps
fields:
  number: {type: string, maxLength: 32}
`)))
	})
	r := mustResolve(t, c, "sales.invoice")
	want := "id,version,createdAt,createdBy,status,amendedFromId,note,number"
	if got := joined(r); got != want {
		t.Fatalf("field = %s, ingin %s", got, want)
	}
	if r.Kind != "resource" || r.Module != "sales" || !r.Generates() {
		t.Fatalf("header = %+v", r)
	}
	id := r.Field("id")
	if id.Origin.Doc != "sdk.audit" || id.Origin.File != "sdk/audit.abstract.yaml" || id.Origin.Line != 6 {
		t.Errorf("jejak asal id = %+v", id.Origin)
	}
	// K22: sdk:audit disertakan otomatis lebih dulu; field yang sama lewat sdk:document tidak ganda dan jejak asal
	// mengikuti kemunculan pertama (implisit).
	if !reflect.DeepEqual(id.Origin.Chain, []string{"sdk.audit"}) {
		t.Errorf("rantai include id = %v", id.Origin.Chain)
	}
	if n := r.Field("number"); n.Origin.Doc != "sales.invoice" || len(n.Origin.Chain) != 0 || n.Origin.Op != "declared" {
		t.Errorf("jejak asal number = %+v", n.Origin)
	}
}

func TestIncludeAccessRules(t *testing.T) {
	mk := func(deps []string, export bool) *Catalog {
		exp := ""
		if export {
			exp = "export: true\n"
		}
		return newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "crm"},
				f("crm/address.abstract.yaml", abstract("crm.address", exp+"fields:\n  street: {type: string, maxLength: 200}\n")))
			addModule(t, c, Module{Slug: "sales", Dependencies: deps},
				f("sales/invoice.resource.yaml", resource("sales.invoice", "include:\n  - source: crm.address\n")))
		})
	}
	t.Run("dependensi dan export: lolos", func(t *testing.T) {
		r := mustResolve(t, mk([]string{"crm"}, true), "sales.invoice")
		if joined(r) != auditNames+",street" {
			t.Fatalf("field = %s", joined(r))
		}
	})
	t.Run("tanpa export ditolak", func(t *testing.T) {
		_, ps := mk([]string{"crm"}, false).Resolve("sales.invoice")
		p := findProblem(ps, CodeIncludeAccess)
		if p == nil || p.Line != 5 || p.File != "sales/invoice.resource.yaml" {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("tanpa dependensi manifest ditolak", func(t *testing.T) {
		_, ps := mk(nil, true).Resolve("sales.invoice")
		if !hasCode(ps, CodeIncludeAccess) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("kunci tak ada", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "sales"},
				f("sales/invoice.resource.yaml", resource("sales.invoice", "include:\n  - source: sales.secret\n  - source: sdk:nothing\n")))
		})
		_, ps := c.Resolve("sales.invoice")
		n := 0
		for _, p := range ps {
			if p.Code == CodeIncludeNotFound {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("source bukan jalur berkas: percobaan ../ ditolak sejak pemuatan", func(t *testing.T) {
		_, ps := load.Parse("x.resource.yaml", []byte(resource("sales.invoice", "include:\n  - source: ../../etc/passwd\n")))
		if len(ps) == 0 {
			t.Fatal("source berupa jalur harus ditolak")
		}
	})
	t.Run("menyertakan fieldType ditolak", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "sales"},
				f("sales/invoice.resource.yaml", resource("sales.invoice", "include:\n  - source: sdk:email\n")))
		})
		_, ps := c.Resolve("sales.invoice")
		if !hasCode(ps, CodeIncludeKind) {
			t.Fatalf("temuan = %v", ps)
		}
	})
}

func TestIncludeResourceBringsOnlyFields(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "sales"},
			f("sales/base.resource.yaml", resource("sales.base", `fields:
  code: {type: string, maxLength: 10}
relations:
  owner: {type: belongsTo, to: crm.customer}
indexes:
  - fields: [code]
generate: {crud: true}
`)),
			f("sales/child.resource.yaml", resource("sales.child", "include:\n  - source: sales.base\nfields:\n  extra: {type: text}\n")))
	})
	r := mustResolve(t, c, "sales.child")
	if joined(r) != auditNames+",code,extra" || len(r.Relations) != 0 || len(r.Indexes) != 0 || r.Generate != nil {
		t.Fatalf("hasil = %s relasi=%d indeks=%d generate=%v", joined(r), len(r.Relations), len(r.Indexes), r.Generate)
	}
}

func TestOverridesSetOmitAdd(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "sales"},
			f("sales/address.abstract.yaml", abstract("sales.address", `fields:
  street: {type: string, maxLength: 200, label: sales.address.street}
  line2: {type: string, maxLength: 200, nullable: true}
  city: {type: string, maxLength: 100}
`)),
			f("sales/order.resource.yaml", resource("sales.order", `include:
  - source: sales.address
    omit: [line2]
    set:
      street: {maxLength: 120, filterable: true}
    add:
      zone: {type: string, maxLength: 5}
`)))
	})
	r := mustResolve(t, c, "sales.order")
	if joined(r) != auditNames+",street,city,zone" {
		t.Fatalf("field = %s", joined(r))
	}
	s := r.Field("street")
	if s.Num("maxLength") != "120" || !s.Bool("filterable") {
		t.Fatalf("attrs street = %v", s.Attrs)
	}
	if s.Str("label") != "sales.address.street" {
		t.Errorf("label tidak terbawa: %v", s.Attrs)
	}
	o := s.AttrOrigin["maxLength"]
	if o.Op != "set" || o.Doc != "sales.order" || o.Line != 8 {
		t.Errorf("asal maxLength = %+v", o)
	}
	if o := s.AttrOrigin["label"]; o.Doc != "sales.address" || o.Op != "declared" {
		t.Errorf("asal label = %+v", o)
	}
	if len(s.History) == 0 || s.History[0].Attr != "maxLength" {
		t.Errorf("riwayat = %+v", s.History)
	}
	if z := r.Field("zone"); z.Origin.Op != "add" || z.Origin.Doc != "sales.order" {
		t.Errorf("asal zone = %+v", z.Origin)
	}
}

func TestOverrideErrors(t *testing.T) {
	base := abstract("sales.address", "fields:\n  street: {type: string, maxLength: 200}\n  city: {type: string, maxLength: 100}\n")
	tests := []struct {
		name string
		inc  string
		code string
	}{
		{"omit tak ada", "    omit: [zip]\n", CodeOverrideMissing},
		{"set tak ada", "    set:\n      zip: {maxLength: 5}\n", CodeOverrideMissing},
		{"add menabrak nama ada", "    add:\n      city: {type: text}\n", CodeDuplicateField},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newCatalog(t, func(c *Catalog) {
				addModule(t, c, Module{Slug: "sales"}, f("sales/a.abstract.yaml", base),
					f("sales/o.resource.yaml", resource("sales.order", "include:\n  - source: sales.address\n"+tc.inc)))
			})
			_, ps := c.Resolve("sales.order")
			if !hasCode(ps, tc.code) {
				t.Fatalf("ingin %s, temuan = %v", tc.code, ps)
			}
		})
	}
}

func TestIncludeCycles(t *testing.T) {
	t.Run("siklus dua berkas", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"},
				f("m/a.abstract.yaml", abstract("m.a", "include:\n  - source: m.b\nfields:\n  fa: {type: text}\n")),
				f("m/b.abstract.yaml", abstract("m.b", "include:\n  - source: m.a\nfields:\n  fb: {type: text}\n")),
				f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.a\n")))
		})
		_, ps := c.Resolve("m.r")
		p := findProblem(ps, CodeIncludeCycle)
		if p == nil || !strings.Contains(p.Message, "m.a") || !strings.Contains(p.Message, "m.b") {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("menyertakan diri sendiri", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.r\n")))
		})
		_, ps := c.Resolve("m.r")
		if !hasCode(ps, CodeIncludeCycle) {
			t.Fatalf("temuan = %v", ps)
		}
	})
}

func TestDuplicateFieldNames(t *testing.T) {
	mk := func(rBody string) *Catalog {
		return newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"},
				f("m/a.abstract.yaml", abstract("m.a", "fields:\n  name: {type: text}\n")),
				f("m/b.abstract.yaml", abstract("m.b", "fields:\n  name: {type: text}\n")),
				f("m/r.resource.yaml", resource("m.r", rBody)))
		})
	}
	t.Run("dua include", func(t *testing.T) {
		_, ps := mk("include:\n  - source: m.a\n  - source: m.b\n").Resolve("m.r")
		if !hasCode(ps, CodeDuplicateField) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("field sendiri menabrak include tanpa timpaan eksplisit", func(t *testing.T) {
		_, ps := mk("include:\n  - source: m.a\nfields:\n  name: {type: string, maxLength: 5}\n").Resolve("m.r")
		p := findProblem(ps, CodeDuplicateField)
		if p == nil || p.Line != 7 {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("prefix menghindari tabrakan", func(t *testing.T) {
		r := mustResolve(t, mk("include:\n  - {source: m.a, prefix: billing}\n  - {source: m.b, prefix: shipping}\n"), "m.r")
		if joined(r) != auditNames+",billingName,shippingName" {
			t.Fatalf("field = %s", joined(r))
		}
	})
	t.Run("timpaan eksplisit set tidak dianggap ganda", func(t *testing.T) {
		r := mustResolve(t, mk("include:\n  - {source: m.a, set: {name: {nullable: true}}}\n"), "m.r")
		if !r.Field("name").Bool("nullable") {
			t.Fatal("set tidak diterapkan")
		}
	})
	t.Run("set mencatat reason di model (IncludeSets)", func(t *testing.T) {
		r := mustResolve(t, mk("include:\n  - {source: m.a, reason: \"  Nama boleh kosong  \", set: {name: {nullable: true}}}\n  - {source: m.b, prefix: shipping}\n"), "m.r")
		if len(r.IncludeSets) != 1 {
			t.Fatalf("IncludeSets = %+v", r.IncludeSets)
		}
		is := r.IncludeSets[0]
		if is.Source != "m.a" || is.Reason != "Nama boleh kosong" || len(is.Fields) != 1 || is.Fields[0] != "name" || is.Origin.Line == 0 {
			t.Errorf("IncludeSet = %+v", is)
		}
	})
}

func TestOverridesCannotLoosen(t *testing.T) {
	abs := abstract("m.a", `fields:
  salary: {type: decimal, precision: 18, scale: 2, classification: confidential, fieldGroup: pay, readOnly: true}
  plain: {type: text}
`)
	tests := []struct {
		name string
		set  string
		code string
		ok   bool
	}{
		{"turunkan klasifikasi", "salary: {classification: internal}", CodeOverrideLowersClassification, false},
		{"turunkan ke public", "salary: {classification: public}", CodeOverrideLowersClassification, false},
		{"naikkan klasifikasi", "salary: {classification: highly_confidential}", "", true},
		{"klasifikasi sama", "salary: {classification: confidential}", "", true},
		{"naikkan klasifikasi plain", "plain: {classification: confidential}", "", true},
		{"longgarkan grup ke default", "salary: {fieldGroup: default}", CodeOverrideLoosensGroup, false},
		{"ketatkan grup", "plain: {fieldGroup: pay}", "", true},
		{"cabut readOnly", "salary: {readOnly: false}", CodeOverrideRevokesReadOnly, false},
		{"readOnly tetap", "salary: {readOnly: true}", "", true},
		{"jadikan readOnly", "plain: {readOnly: true}", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newCatalog(t, func(c *Catalog) {
				addModule(t, c, Module{Slug: "m"}, f("m/a.abstract.yaml", abs),
					f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.a\n    set:\n      "+tc.set+"\n")))
			})
			r, ps := c.Resolve("m.r")
			if tc.ok {
				if len(ps) != 0 || r == nil {
					t.Fatalf("seharusnya lolos: %v", ps)
				}
				return
			}
			if !hasCode(ps, tc.code) {
				t.Fatalf("ingin %s, temuan = %v", tc.code, ps)
			}
		})
	}
}

func TestProtectedFieldsCannotBeOmittedOrSet(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"omit field bawaan audit", "include:\n  - {source: sdk:document, omit: [createdBy]}\n"},
		{"set field bawaan audit", "include:\n  - {source: sdk:document, set: {version: {label: x.y.z}}}\n"},
		{"omit status dokumen", "include:\n  - {source: sdk:document, omit: [status]}\n"},
		{"set status dokumen", "include:\n  - {source: sdk:document, set: {status: {nullable: true}}}\n"},
		{"omit amendedFromId", "include:\n  - {source: sdk:document, omit: [amendedFromId]}\n"},
		{"melalui abstrak pengguna", "include:\n  - {source: m.mid, omit: [status]}\n"},
		{"set owner_field", "ownerField: owner\ninclude:\n  - {source: m.own, set: {owner: {nullable: true}}}\n"},
		{"omit field scope", "scopeFields: {inventory.warehouse: wh}\ninclude:\n  - {source: m.own, omit: [wh]}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newCatalog(t, func(c *Catalog) {
				addModule(t, c, Module{Slug: "m"},
					f("m/mid.abstract.yaml", abstract("m.mid", "include:\n  - source: sdk:document\n")),
					f("m/own.abstract.yaml", abstract("m.own", "fields:\n  owner: {type: uuid}\n  wh: {type: uuid}\n")),
					f("m/r.resource.yaml", resource("m.r", tc.body)))
			})
			_, ps := c.Resolve("m.r")
			if !hasCode(ps, CodeOverrideProtected) {
				t.Fatalf("temuan = %v", ps)
			}
		})
	}
	t.Run("field bawaan tidak ditandai pada resource tanpa dokumen", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"},
				f("m/a.abstract.yaml", abstract("m.a", "fields:\n  status: {type: text}\n")),
				f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: m.a, set: {status: {nullable: true}}}\n")))
		})
		mustResolve(t, c, "m.r")
	})
}

func TestTypeChangeOnReferencedFieldRejected(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/a.abstract.yaml", abstract("m.a", "fields:\n  cname: {type: string, maxLength: 20}\n  other: {type: string, maxLength: 20}\n")),
			f("m/r.resource.yaml", resource("m.r", `include:
  - source: m.a
    set:
      cname: {type: text}
      other: {type: text}
relations:
  customer: {type: belongsTo, to: m.customer, snapshot: [{from: name, to: cname}]}
`)))
	})
	_, ps := c.Resolve("m.r")
	n := 0
	for _, p := range ps {
		if p.Code == CodeOverrideTypeReferenced {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("hanya cname yang dirujuk relasi; temuan = %v", ps)
	}
}

func TestLockWhenOnlyTightens(t *testing.T) {
	t.Run("OR dengan kondisi bawaan", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", `include:
  - source: sdk:document
fields:
  paid: {type: boolean}
lockWhen: {field: paid, op: eq, value: true}
`)))
		})
		r := mustResolve(t, c, "m.r")
		items, ok := r.LockWhen["any"].([]any)
		if !ok || len(items) != 2 {
			t.Fatalf("LockWhen = %v", r.LockWhen)
		}
		first := items[0].(map[string]any)
		if first["field"] != "status" || first["op"] != "ne" {
			t.Errorf("kondisi bawaan harus tetap ada: %v", items)
		}
		if len(r.LockWhenOrigins) != 2 {
			t.Errorf("asal kondisi = %v", r.LockWhenOrigins)
		}
	})
	t.Run("tanpa kondisi turunan, bawaan apa adanya", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "include:\n  - source: sdk:document\n")))
		})
		r := mustResolve(t, c, "m.r")
		if r.LockWhen["field"] != "status" {
			t.Fatalf("LockWhen = %v", r.LockWhen)
		}
	})
	t.Run("gabungan melampaui batas ditolak", func(t *testing.T) {
		chain := "{not: {not: {not: {field: paid, op: eq, value: true}}}}" // kedalaman 4
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r",
				"include:\n  - source: sdk:document\nfields:\n  paid: {type: boolean}\nlockWhen: "+chain+"\n")))
		})
		_, ps := c.Resolve("m.r")
		if !hasCode(ps, CodeLockWhenLimit) {
			t.Fatalf("temuan = %v", ps)
		}
	})
}

func TestPrefixBlocks(t *testing.T) {
	block := abstract("m.address", `export: true
layout: {section: address, columns: 2, order: [street, city]}
fields:
  street: {type: string, maxLength: 200, requiredWhen: {field: city, op: isnull, value: false}}
  city: {type: string, maxLength: 100, nullable: true}
  zip: {type: string, maxLength: 10, deprecated: {since: "1.0.0", replacedBy: postal}}
  postal: {type: string, maxLength: 10, migrateFrom: zip, nullable: true}
rules:
  - {id: cityOrStreet, assert: {field: street, op: ne, ref: city}, field: street, message: m.address.error.x}
`)
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"}, f("m/address.abstract.yaml", block),
			f("m/order.resource.yaml", resource("m.order", `include:
  - {source: m.address, prefix: billing}
  - source: m.address
    prefix: shipping
    set:
      street: {requiredWhen: {field: useShipping, op: eq, value: true}}
fields:
  useShipping: {type: boolean}
`)))
	})
	r := mustResolve(t, c, "m.order")
	if got := joined(r); got != auditNames+",billingStreet,billingCity,billingZip,billingPostal,shippingStreet,shippingCity,shippingZip,shippingPostal,useShipping" {
		t.Fatalf("field = %s", got)
	}
	b := r.Field("billingStreet")
	cond := b.Attrs["requiredWhen"].(map[string]any)
	if cond["field"] != "billingCity" {
		t.Errorf("rujukan kondisi harus ikut diberi prefix: %v", cond)
	}
	if b.SourceName != "street" {
		t.Errorf("SourceName = %q", b.SourceName)
	}
	s := r.Field("shippingStreet").Attrs["requiredWhen"].(map[string]any)
	if s["field"] != "useShipping" {
		t.Errorf("rujukan ke field resource penyertaan tidak boleh diubah: %v", s)
	}
	if got := r.Field("billingZip").Attrs["deprecated"].(map[string]any)["replacedBy"]; got != "billingPostal" {
		t.Errorf("replacedBy = %v", got)
	}
	if got := r.Field("billingPostal").Str("migrateFrom"); got != "billingZip" {
		t.Errorf("migrateFrom = %v", got)
	}
	if len(r.Sections) != 2 || r.Sections[0].Key != "billingAddress" || r.Sections[1].Key != "shippingAddress" ||
		!reflect.DeepEqual(r.Sections[0].Order, []string{"billingStreet", "billingCity"}) {
		t.Errorf("sections = %+v", r.Sections)
	}
	if len(r.Rules) != 2 || r.Rules[0].ID != "billingCityOrStreet" || r.Rules[0].Attrs["field"] != "billingStreet" {
		t.Fatalf("rules = %+v", r.Rules)
	}
	assert := r.Rules[0].Attrs["assert"].(map[string]any)
	if assert["field"] != "billingStreet" || assert["ref"] != "billingCity" {
		t.Errorf("assert = %v", assert)
	}
	// deterministik
	r2 := mustResolve(t, c, "m.order")
	if !reflect.DeepEqual(r, r2) {
		t.Error("hasil resolusi tidak deterministik")
	}
}

func TestPrefixErrors(t *testing.T) {
	long := strings.Repeat("a", 40)
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/a.abstract.yaml", abstract("m.a", "fields:\n  "+strings.Repeat("b", 30)+": {type: text}\n")),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: m.a, prefix: "+long+"}\n")))
	})
	_, ps := c.Resolve("m.r")
	if !hasCode(ps, CodePrefixInvalid) {
		t.Fatalf("nama hasil prefix > 63 byte harus ditolak: %v", ps)
	}
}

func TestDomainTypes(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", `fields:
  contactEmail: {type: sdk:email, label: m.r.contactEmail}
  backupEmail: {type: sdk:email, maxLength: 100, classification: highly_confidential}
  total: {type: sdk:money, currency: IDR}
  name: {type: string, maxLength: 10}
`)))
	})
	r := mustResolve(t, c, "m.r")
	e := r.Field("contactEmail")
	if e.Type() != "string" || e.DomainType != "sdk.email" || e.Widget != "email" ||
		!reflect.DeepEqual(e.Operators, []string{"eq", "contains", "isnull"}) {
		t.Fatalf("contactEmail = %+v", e)
	}
	if e.Classification() != "confidential" {
		t.Errorf("klasifikasi tipe domain harus menular: %q", e.Classification())
	}
	if e.Format() != "email" {
		t.Errorf("format = %q", e.Format())
	}
	if o := e.AttrOrigin["maxLength"]; o.Doc != "sdk.email" || o.Op != "domainType" || o.File != "sdk/email.type.yaml" {
		t.Errorf("asal maxLength = %+v", o)
	}
	if o := e.AttrOrigin["classification"]; o.Doc != "sdk.email" {
		t.Errorf("asal classification = %+v", o)
	}
	if o := e.AttrOrigin["label"]; o.Op != "declared" {
		t.Errorf("asal label = %+v", o)
	}
	b := r.Field("backupEmail")
	if b.Attrs["maxLength"] == nil || b.AttrOrigin["maxLength"].Op != "declared" {
		t.Errorf("batasan field menimpa dasar tipe domain: %v", b.Attrs)
	}
	if b.Classification() != "highly_confidential" {
		t.Errorf("klasifikasi lebih tinggi boleh: %q", b.Classification())
	}
	m := r.Field("total")
	if m.Type() != "decimal" || m.Display != "money" || m.Str("currency") != "IDR" || m.DomainType != "sdk.money" {
		t.Errorf("total = %+v", m)
	}
	if n := r.Field("name"); n.DomainType != "" || n.Widget != "" {
		t.Errorf("name bukan tipe domain: %+v", n)
	}
}

func TestDomainTypeErrors(t *testing.T) {
	mk := func(deps []string, body string, export bool) *Catalog {
		exp := ""
		if export {
			exp = "export: true\n"
		}
		return newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "crm"},
				f("crm/phone.type.yaml", fieldType("crm.phone", exp+"base: {type: string, maxLength: 32}\nclassification: confidential\n")),
				f("crm/block.abstract.yaml", abstract("crm.block", "fields:\n  x: {type: text}\n")))
			addModule(t, c, Module{Slug: "sales", Dependencies: deps}, f("sales/r.resource.yaml", resource("sales.r", body)))
		})
	}
	t.Run("lintas modul dengan dependensi dan export", func(t *testing.T) {
		r := mustResolve(t, mk([]string{"crm"}, "fields:\n  p: {type: crm.phone}\n", true), "sales.r")
		if r.Field("p").Classification() != "confidential" {
			t.Fatal("klasifikasi tidak menular")
		}
	})
	t.Run("tanpa export", func(t *testing.T) {
		_, ps := mk([]string{"crm"}, "fields:\n  p: {type: crm.phone}\n", false).Resolve("sales.r")
		if !hasCode(ps, CodeTypeAccess) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("tanpa dependensi", func(t *testing.T) {
		_, ps := mk(nil, "fields:\n  p: {type: crm.phone}\n", true).Resolve("sales.r")
		if !hasCode(ps, CodeTypeAccess) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("tipe tak ada", func(t *testing.T) {
		_, ps := mk([]string{"crm"}, "fields:\n  p: {type: sdk:nothing}\n", true).Resolve("sales.r")
		if !hasCode(ps, CodeTypeNotFound) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("kunci bukan fieldType", func(t *testing.T) {
		_, ps := mk([]string{"crm"}, "fields:\n  p: {type: sdk:audit}\n", true).Resolve("sales.r")
		if !hasCode(ps, CodeTypeKind) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("klasifikasi field tidak boleh lebih rendah dari tipe", func(t *testing.T) {
		_, ps := mk([]string{"crm"}, "fields:\n  p: {type: crm.phone, classification: internal}\n", true).Resolve("sales.r")
		p := findProblem(ps, CodeTypeLowersClassification)
		if p == nil || p.Line != 5 {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("set mengganti ke tipe domain tidak menurunkan klasifikasi", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"},
				f("m/a.abstract.yaml", abstract("m.a", "fields:\n  contact: {type: string, maxLength: 50, classification: highly_confidential}\n")),
				f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: m.a, set: {contact: {type: sdk:email}}}\n")))
		})
		r := mustResolve(t, c, "m.r")
		if r.Field("contact").Classification() != "highly_confidential" || r.Field("contact").DomainType != "sdk.email" {
			t.Fatalf("contact = %+v", r.Field("contact"))
		}
	})
}

func TestAbstractsGenerateNothing(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/a.abstract.yaml", abstract("m.a", "fields:\n  x: {type: text}\n")),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.a\n")))
	})
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatalf("temuan = %v", res.Problems)
	}
	keys := func(rs []*Resolved) []string {
		var o []string
		for _, r := range rs {
			o = append(o, r.Key)
		}
		return o
	}
	if !reflect.DeepEqual(keys(res.Resources), []string{"m.r"}) {
		t.Errorf("Resources = %v", keys(res.Resources))
	}
	if !reflect.DeepEqual(keys(res.Abstracts), []string{"m.a", "sdk.audit", "sdk.document"}) {
		t.Errorf("Abstracts = %v", keys(res.Abstracts))
	}
	for _, a := range res.Abstracts {
		if a.Generates() {
			t.Errorf("abstrak %s tidak boleh menghasilkan apa pun", a.Key)
		}
	}
	if res.Resource("m.r") == nil || res.Resource("m.a") != nil {
		t.Error("Resource() hanya untuk kind resource")
	}
	if _, ps := c.Resolve("sdk.email"); !hasCode(ps, CodeIncludeKind) {
		t.Errorf("Resolve(fieldType) harus ditolak: %v", ps)
	}
	if _, ps := c.Resolve("m.none"); !hasCode(ps, CodeIncludeNotFound) {
		t.Errorf("Resolve(tak ada) = %v", ps)
	}
}

func TestResolvedFieldLimit(t *testing.T) {
	var a, b strings.Builder
	a.WriteString("fields:\n")
	b.WriteString("fields:\n")
	for i := 0; i < 120; i++ {
		a.WriteString("  a" + letters(i) + ": {type: text}\n")
		b.WriteString("  b" + letters(i) + ": {type: text}\n")
	}
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/a.abstract.yaml", abstract("m.a", a.String())),
			f("m/b.abstract.yaml", abstract("m.b", b.String())),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.a\n  - source: m.b\n")))
	})
	_, ps := c.Resolve("m.r")
	if !hasCode(ps, CodeLimit) {
		t.Fatalf("240 field hasil gabungan harus ditolak: %v", ps[:min(3, len(ps))])
	}
}

func letters(i int) string {
	s := ""
	for {
		s = string(rune('a'+i%26)) + s
		i = i/26 - 1
		if i < 0 {
			return s
		}
	}
}

func TestDuplicateRuleIDs(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/a.abstract.yaml", abstract("m.a", "fields:\n  x: {type: text}\nrules:\n  - {id: r1, assert: {field: x, op: isnull, value: false}, message: m.a.err}\n")),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.a\nrules:\n  - {id: r1, assert: {field: x, op: isnull, value: false}, message: m.a.err}\n")))
	})
	_, ps := c.Resolve("m.r")
	if !hasCode(ps, CodeDuplicateRule) {
		t.Fatalf("temuan = %v", ps)
	}
}

func TestAddModuleValidations(t *testing.T) {
	c := NewCatalog()
	d1 := mustDoc(t, "a/x.abstract.yaml", abstract("a.x", "fields:\n  f: {type: text}\n"))
	if ps := c.AddModule(Module{Slug: "a"}, d1); len(ps) != 0 {
		t.Fatalf("temuan = %v", ps)
	}
	dup := mustDoc(t, "a/y.abstract.yaml", abstract("a.x", "fields:\n  g: {type: text}\n"))
	if ps := c.AddModule(Module{Slug: "a2"}, dup); !hasCode(ps, CodeDuplicateKey) && !hasCode(ps, CodeKeyModule) {
		t.Fatalf("kunci ganda/slug tak cocok harus dilaporkan: %v", ps)
	}
	other := mustDoc(t, "b/z.abstract.yaml", abstract("c.z", "fields:\n  g: {type: text}\n"))
	if ps := c.AddModule(Module{Slug: "b"}, other); !hasCode(ps, CodeKeyModule) {
		t.Fatalf("slug kunci harus sama dengan slug modul: %v", ps)
	}
	same := mustDoc(t, "a/dup.abstract.yaml", abstract("a.x", "fields:\n  g: {type: text}\n"))
	if ps := c.AddModule(Module{Slug: "a"}, same); !hasCode(ps, CodeDuplicateKey) {
		t.Fatalf("kunci ganda harus dilaporkan: %v", ps)
	}
}

func TestDefaultGroupFollowsModuleDefinition(t *testing.T) {
	mk := func(set string) *Catalog {
		return newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m", DefaultGroup: "general"},
				f("m/a.abstract.yaml", abstract("m.a", "fields:\n  salary: {type: decimal, precision: 18, scale: 2, fieldGroup: pay}\n")),
				f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.a\n    set:\n      salary: {fieldGroup: "+set+"}\n")))
		})
	}
	if _, ps := mk("general").Resolve("m.r"); !hasCode(ps, CodeOverrideLoosensGroup) {
		t.Errorf("pindah ke grup default bertanda manifest harus ditolak: %v", ps)
	}
	// Kata "default" bukan grup default khusus bila manifest menamai grup default "general".
	r, ps := mk("default").Resolve("m.r")
	if len(ps) != 0 || r == nil {
		t.Fatalf("temuan = %v", ps)
	}
	if r.DefaultGroup != "general" || r.GroupOf(r.Field("salary")) != "default" {
		t.Errorf("DefaultGroup = %q, grup = %q", r.DefaultGroup, r.GroupOf(r.Field("salary")))
	}
}
