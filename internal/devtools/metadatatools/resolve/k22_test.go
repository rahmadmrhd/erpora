package resolve

import "testing"

// auditNames adalah field sdk:audit mini pada katalog uji (helpers_test.go) yang kini selalu mendahului field lain
// pada setiap resource (K22).
const auditNames = "id,version,createdAt,createdBy"

func TestK22ImplicitAuditOnEveryResource(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/plain.resource.yaml", resource("m.plain", "fields:\n  name: {type: text}\n")),
			f("m/empty.resource.yaml", resource("m.empty", "")),
			f("m/blk.abstract.yaml", abstract("m.blk", "fields:\n  x: {type: text}\n")),
			f("m/doc.resource.yaml", resource("m.doc", "include:\n  - source: sdk:document\nfields:\n  total: {type: integer}\n")))
	})
	plain := mustResolve(t, c, "m.plain")
	if joined(plain) != auditNames+",name" {
		t.Fatalf("resource tanpa include: %s", joined(plain))
	}
	if id := plain.Field("id"); id.ProtectedBy != KeyAudit || id.Origin.Doc != KeyAudit || id.Origin.Op != OpDeclared {
		t.Errorf("id = %+v", id)
	}
	hasUse := false
	for _, u := range plain.Uses {
		if u.Key == KeyAudit && u.Kind == "include" && u.Direct {
			hasUse = true
		}
	}
	if !hasUse {
		t.Errorf("sdk.audit harus tercatat di Uses: %+v", plain.Uses)
	}
	if got := joined(mustResolve(t, c, "m.empty")); got != auditNames {
		t.Errorf("resource tanpa field: %s", got)
	}
	// abstrak TIDAK mendapat audit otomatis.
	if got := joined(mustResolve(t, c, "m.blk")); got != "x" {
		t.Errorf("abstrak = %s", got)
	}
	// dokumen: audit lebih dulu, lalu field dokumen; tanpa duplikat; lockWhen bawaan tetap.
	doc := mustResolve(t, c, "m.doc")
	if joined(doc) != auditNames+",status,amendedFromId,total" || doc.LockWhen["field"] != "status" {
		t.Errorf("dokumen = %s lockWhen=%v", joined(doc), doc.LockWhen)
	}
	if doc.Field("status").ProtectedBy != KeyDocument {
		t.Errorf("status = %+v", doc.Field("status"))
	}
}

func TestK22ExplicitAndIndirectAuditIncludesAreDeduplicated(t *testing.T) {
	c := newCatalog(t, func(c *Catalog) {
		addModule(t, c, Module{Slug: "m"},
			f("m/withaudit.abstract.yaml", abstract("m.withAudit", "include:\n  - source: sdk:audit\nfields:\n  a: {type: text}\n")),
			f("m/other.abstract.yaml", abstract("m.other", "include:\n  - source: sdk:audit\nfields:\n  b: {type: text}\n")),
			f("m/explicit.resource.yaml", resource("m.explicit", "include:\n  - source: sdk:audit\nfields:\n  n: {type: text}\n")),
			f("m/indirect.resource.yaml", resource("m.indirect", "include:\n  - source: m.withAudit\n")),
			f("m/two.resource.yaml", resource("m.two", "include:\n  - source: m.withAudit\n  - source: m.other\n  - source: sdk:document\n")))
	})
	if got := joined(mustResolve(t, c, "m.explicit")); got != auditNames+",n" {
		t.Errorf("include eksplisit: %s", got)
	}
	if got := joined(mustResolve(t, c, "m.indirect")); got != auditNames+",a" {
		t.Errorf("include tak langsung: %s", got)
	}
	// dua abstrak yang sama-sama menyertakan audit dan sdk:document: audit hanya sekali (bukan berlian).
	if got := joined(mustResolve(t, c, "m.two")); got != auditNames+",a,b,status,amendedFromId" {
		t.Errorf("beberapa jalur: %s", got)
	}
}

func TestK22AuditFieldsRemainProtectedAndNamesReserved(t *testing.T) {
	t.Run("set atas field audit lewat include eksplisit tetap ditolak", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: sdk:audit, set: {version: {label: x.y.z}}}\n")))
		})
		if _, ps := c.Resolve("m.r"); !hasCode(ps, CodeOverrideProtected) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("omit atas field audit lewat include eksplisit tetap ditolak", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: sdk:audit, omit: [createdBy]}\n")))
		})
		if _, ps := c.Resolve("m.r"); !hasCode(ps, CodeOverrideProtected) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("field sendiri bernama field audit adalah duplicate_field", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "fields:\n  id: {type: uuid}\n")))
		})
		if _, ps := c.Resolve("m.r"); !hasCode(ps, CodeDuplicateField) {
			t.Fatalf("temuan = %v", ps)
		}
	})
	t.Run("include audit berprefix ditolak (field bawaan terlindungi tidak boleh diganti nama)", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: sdk:audit, prefix: legacy}\n")))
		})
		if _, ps := c.Resolve("m.r"); !hasCode(ps, CodeIncludePrefixProtected) {
			t.Fatalf("temuan = %v", ps)
		}
	})
}

func TestK22SkippedWhenCatalogHasNoAudit(t *testing.T) {
	c := NewCatalog()
	addModule(t, c, Module{Slug: "m"}, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\n")))
	if got := joined(mustResolve(t, c, "m.r")); got != "a" {
		t.Errorf("tanpa metadata bawaan: %s", got)
	}
}
