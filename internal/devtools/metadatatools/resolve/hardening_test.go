package resolve

import "testing"

func TestIncludePrefixOnProtectedFieldsRejected(t *testing.T) {
	cases := map[string]string{
		"sdk:audit":                             "include:\n  - {source: sdk:audit, prefix: x}\n",
		"sdk:document":                          "include:\n  - {source: sdk:document, prefix: doc}\n",
		"abstrak yang menyertakan sdk:document": "include:\n  - {source: m.wrap, prefix: w}\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c := newCatalog(t, func(c *Catalog) {
				addModule(t, c, Module{Slug: "m"},
					f("m/wrap.abstract.yaml", abstract("m.wrap", "include:\n  - source: sdk:document\n")),
					f("m/r.resource.yaml", resource("m.r", body)))
			})
			_, ps := c.Resolve("m.r")
			p := findProblem(ps, CodeIncludePrefixProtected)
			if p == nil {
				t.Fatalf("temuan = %v", ps)
			}
			if p.Pointer != "/include/0/prefix" || p.File == "" || p.Line == 0 {
				t.Errorf("lokasi = %+v", *p)
			}
		})
	}
	t.Run("prefix pada include biasa tetap sah", func(t *testing.T) {
		c := newCatalog(t, func(c *Catalog) {
			addModule(t, c, Module{Slug: "m"},
				f("m/addr.abstract.yaml", abstract("m.addr", "fields:\n  street: {type: text}\n")),
				f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: m.addr, prefix: billing}\n")))
		})
		if r := mustResolve(t, c, "m.r"); r.Field("billingStreet") == nil {
			t.Fatalf("field = %s", joined(r))
		}
	})
}

func TestAddModuleRejectsReservedSDKSlug(t *testing.T) {
	c := NewCatalog()
	doc := mustDoc(t, "modules/evil/metadata/a.abstract.yaml", abstract("sdk.audit", "fields:\n  hijack: {type: text}\n"))
	ps := c.AddModule(Module{Slug: SDKSlug}, doc)
	if !hasCode(ps, CodeModuleSlugReserved) {
		t.Fatalf("temuan = %v", ps)
	}
	if c.Doc("sdk.audit") != nil {
		t.Error("dokumen tidak boleh terdaftar")
	}
	if _, ok := c.Module(SDKSlug); ok {
		t.Error("modul sdk tidak boleh terdaftar lewat AddModule")
	}
	// jalur bawaan SDK tetap bekerja.
	c2, probs := NewCatalogWithBuiltin()
	if len(probs) != 0 || c2.Doc("sdk.audit") == nil {
		t.Fatalf("bawaan tidak termuat: %v", probs)
	}
	// modul lain tidak dapat menimpa metadata bawaan yang sudah ada.
	if ps := c2.AddModule(Module{Slug: SDKSlug}, doc); !hasCode(ps, CodeModuleSlugReserved) {
		t.Fatalf("temuan = %v", ps)
	}
	if got := c2.Doc("sdk.audit").Value["fields"].(map[string]any); got["hijack"] != nil {
		t.Error("sdk.audit bawaan tertimpa")
	}
}
