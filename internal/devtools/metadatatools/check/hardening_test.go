package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- I-1: identitas modul (direktori, slug, slug tercadang, duplikat) ----

func moduleAt(t *testing.T, dir, slug, key string, files ...file) Module {
	t.Helper()
	m := mod(t, slug, nil, files...)
	m.Dir = dir
	m.Manifest.Key = key
	return m
}

func TestModuleDirectoryMustMatchSlug(t *testing.T) {
	res := f("m/r.resource.yaml", resource("core.thing", "fields:\n  a: {type: text}\n"))
	t.Run("direktori berbeda dari slug (PoC I-1)", func(t *testing.T) {
		rep := run(t, moduleAt(t, "modules/evil", "core", "x.evil", res))
		p := wantErr(t, rep, CodeModuleDirMismatch, "")
		if p.File != "modules/evil/module.json" || p.Module != "core" || !strings.Contains(p.Message, "core") {
			t.Errorf("temuan = %+v", p)
		}
		// dokumen modul yang identitasnya salah tidak masuk katalog sehingga tidak dapat dibangkitkan.
		if rep.Resolved.Resource("core.thing") != nil {
			t.Error("resource modul bermasalah tidak boleh diselesaikan")
		}
	})
	t.Run("slug berhubung tanda hubung: direktori tanpa tanda hubung", func(t *testing.T) {
		ok := moduleAt(t, "modules/mymod", "my-mod", "acme.my-mod", f("m/r.resource.yaml", resource("my-mod.thing", "fields:\n  a: {type: text}\n")))
		noErrors(t, run(t, ok))
		bad := moduleAt(t, "modules/my-mod", "my-mod", "acme.my-mod", f("m/r.resource.yaml", resource("my-mod.thing", "fields:\n  a: {type: text}\n")))
		wantErr(t, run(t, bad), CodeModuleDirMismatch, "")
	})
	t.Run("direktori bersarang atau berjalur aneh ditolak", func(t *testing.T) {
		for _, dir := range []string{"modules/../core", "other/core", "core", "modules/core/sub"} {
			rep := run(t, moduleAt(t, dir, "core", "acme.core", res))
			wantErr(t, rep, CodeModuleDirMismatch, "")
		}
	})
}

func TestReservedAndInvalidAndDuplicateModuleIdentities(t *testing.T) {
	t.Run("slug tercadang (registry)", func(t *testing.T) {
		for _, slug := range []string{"sdk", "platform", "ui", "api", "desks"} {
			m := moduleAt(t, "modules/"+slug, slug, "x."+slug, f("m/r.resource.yaml", resource(slug+".thing", "fields:\n  a: {type: text}\n")))
			rep := run(t, m)
			wantErr(t, rep, CodeModuleSlugReserved, "")
			if rep.Resolved.Resource(slug+".thing") != nil {
				t.Errorf("%s: modul slug tercadang tidak boleh diselesaikan", slug)
			}
		}
	})
	t.Run("sdk tidak dianggap metadata bawaan (PoC I-1)", func(t *testing.T) {
		m := moduleAt(t, "modules/sdk", "sdk", "x.evil", f("m/a.abstract.yaml", abstract("sdk.audit", "fields:\n  hijack: {type: text}\n")))
		rep := run(t, m)
		wantErr(t, rep, CodeModuleSlugReserved, "")
		if d := rep.Resolved; d != nil {
			for _, a := range d.Abstracts {
				if a.Key == "sdk.audit" && a.Field("hijack") != nil {
					t.Error("sdk.audit bawaan tidak boleh ditimpa modul lain")
				}
			}
		}
	})
	t.Run("slug tidak sah", func(t *testing.T) {
		m := moduleAt(t, "modules/BadSlug", "BadSlug", "x.bad", f("m/r.resource.yaml", resource("m.thing", "fields:\n  a: {type: text}\n")))
		wantErr(t, run(t, m), CodeModuleSlugInvalid, "")
	})
	t.Run("slug ganda antar modul", func(t *testing.T) {
		a := moduleAt(t, "modules/core", "core", "acme.core", f("m/a.resource.yaml", resource("core.a", "fields:\n  a: {type: text}\n")))
		b := moduleAt(t, "modules/evil", "core", "x.evil", f("m/b.resource.yaml", resource("core.b", "fields:\n  a: {type: text}\n")))
		rep := run(t, a, b)
		wantErr(t, rep, CodeModuleSlugDuplicate, "")
		if rep.Resolved.Resource("core.b") != nil {
			t.Error("modul kedua ber-slug sama tidak boleh disatukan ke modul pertama")
		}
		if rep.Resolved.Resource("core.a") == nil {
			t.Error("modul pertama yang sah tetap diselesaikan")
		}
	})
	t.Run("kunci modul ganda", func(t *testing.T) {
		a := moduleAt(t, "modules/one", "one", "acme.same", f("m/a.resource.yaml", resource("one.a", "fields:\n  a: {type: text}\n")))
		b := moduleAt(t, "modules/two", "two", "acme.same", f("m/b.resource.yaml", resource("two.b", "fields:\n  a: {type: text}\n")))
		wantErr(t, run(t, a, b), CodeModuleKeyDuplicate, "")
	})
	t.Run("modul sah tetap lolos dan Dir diekspos untuk pembangkit", func(t *testing.T) {
		m := moduleAt(t, "modules/mymod", "my-mod", "acme.my-mod", f("m/r.resource.yaml", resource("my-mod.thing", "fields:\n  a: {type: text}\n")))
		rep := run(t, m)
		noErrors(t, rep)
		if got := rep.ModuleDirs["my-mod"]; got != "modules/mymod" {
			t.Errorf("ModuleDirs = %v", rep.ModuleDirs)
		}
		if info := rep.Resource("my-mod.thing"); info == nil || info.Dir != "modules/mymod" {
			t.Errorf("ResourceInfo.Dir = %+v", info)
		}
	})
}

// ---- I-3: ref pada kondisi if dalam accessor ikut mewarisi klasifikasi ----

func TestAccessorIfConditionRefInheritsClassification(t *testing.T) {
	src := `fields:
  name: {type: string, maxLength: 50}
  price: {type: decimal, precision: 18, scale: 2}
  costPrice: {type: decimal, precision: 18, scale: 2, classification: confidential}
  priceBand:
    type: string
    maxLength: 10
    accessor: {kind: expr, expr: {if: {field: price, op: lt, ref: costPrice}, then: {lit: low}, else: {lit: high}}}
`
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "labelTemplate: \"{name} {priceBand}\"\n"+src)))
	if got := rep.Resource("m.r").Class["priceBand"]; got != "confidential" {
		t.Fatalf("Class[priceBand] = %q, harus mewarisi confidential dari ref costPrice", got)
	}
	wantErr(t, rep, CodeLabelFieldClassification, "/labelTemplate")
	// accessor yang menyatakan klasifikasi lebih longgar dari field pada ref ditolak.
	loose := strings.Replace(src, "    maxLength: 10\n", "    maxLength: 10\n    classification: internal\n", 1)
	wantErr(t, single(t, f("m/r.resource.yaml", resource("m.r", loose))), CodeAccessorLooserClassification, "/fields/priceBand")
}

func TestAccessorIfConditionRefInheritsGroup(t *testing.T) {
	src := `fields:
  price: {type: decimal, precision: 18, scale: 2}
  costPrice: {type: decimal, precision: 18, scale: 2, fieldGroup: pricing}
  band:
    type: string
    maxLength: 10
    accessor: {kind: expr, expr: {if: {field: price, op: lt, ref: costPrice}, then: {lit: low}, else: {lit: high}}}
`
	rep := single(t, f("m/r.resource.yaml", resource("m.r", src)))
	if got := rep.Resource("m.r").Group["band"]; got != "pricing" {
		t.Errorf("Group[band] = %q", got)
	}
}

// ---- I-5: pelonggaran grup non-default lewat set ----

func TestSetCannotLoosenToLowerRankedNonDefaultGroup(t *testing.T) {
	groups := []ManifestGroup{
		{Key: "general", Default: true},
		{Key: "finance", Classification: "confidential"},
		{Key: "ops", Classification: "internal"},
		{Key: "audit", Classification: "confidential"},
	}
	mk := func(set string) *Report {
		return run(t, manifestMod(t, "m", []ManifestResource{{Key: "m.r", Actions: []string{"read"}, FieldGroups: groups}},
			f("m/costing.abstract.yaml", abstract("m.costing", "fields:\n  unitCost: {type: decimal, precision: 18, scale: 2, fieldGroup: finance, classification: confidential}\n  note: {type: text, nullable: true}\n")),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.costing\n    set:\n      "+set+"\n"))))
	}
	p := wantErr(t, mk("unitCost: {fieldGroup: ops}"), CodeFieldGroupLoosened, "/fields/unitCost")
	if !strings.Contains(p.Message, "finance") || !strings.Contains(p.Message, "ops") {
		t.Errorf("pesan harus menyebut kedua grup: %q", p.Message)
	}
	noErrors(t, mk("unitCost: {fieldGroup: audit}")) // peringkat sama
	noErrors(t, mk("note: {fieldGroup: ops}"))       // default menjadi non-default: memperketat
	noErrors(t, mk("note: {fieldGroup: finance}"))
}

// ---- I-6: pivotFields divalidasi seperti field biasa ----

func TestPivotFieldsAreValidated(t *testing.T) {
	mk := func(pivot string) *Report {
		return single(t,
			f("m/tag.resource.yaml", resource("m.tag", "fields:\n  label: {type: string, maxLength: 20}\n")),
			f("m/post.resource.yaml", resource("m.post", "fields:\n  title: {type: string, maxLength: 20}\nrelations:\n  tags:\n    type: belongsToMany\n    to: m.tag\n    pivotFields:\n"+pivot)))
	}
	tests := []struct {
		name, pivot, code, ptr string
	}{
		{"nama sensitif", "      accessToken: {type: string, maxLength: 20}\n", CodeFieldSensitiveName, "/relations/tags/pivotFields/accessToken"},
		{"highly_confidential", "      note: {type: text, classification: highly_confidential}\n", CodeFieldHighlyConfidential, "/relations/tags/pivotFields/note"},
		{"default enum di luar values", "      kind: {type: enum, values: [a, b], default: zzz}\n", CodeFieldDefaultInvalid, "/relations/tags/pivotFields/kind"},
		{"decimal tanpa precision dan scale", "      weight: {type: decimal}\n", CodeFieldAttrRequired, "/relations/tags/pivotFields/weight"},
		{"atribut tidak berlaku untuk tipe", "      qty: {type: integer, maxLength: 3}\n", CodeFieldAttrNotForType, "/relations/tags/pivotFields/qty"},
		{"minLength melebihi maxLength", "      code: {type: string, minLength: 9, maxLength: 3}\n", CodeFieldRange, "/relations/tags/pivotFields/code"},
		{"kata tercadang SQL", "      order: {type: text}\n", CodeNameReservedSQL, "/relations/tags/pivotFields/order"},
		{"pola di luar subset", "      sku: {type: string, maxLength: 10, pattern: \"^(a+)+$\"}\n", CodeFieldPatternInvalid, "/relations/tags/pivotFields/sku"},
		{"default null pada non-nullable", "      remark: {type: string, maxLength: 10, default: null}\n", CodeFieldDefaultInvalid, "/relations/tags/pivotFields/remark"},
		{"tipe domain tidak didukung pada kolom pivot", "      mail: {type: sdk:email}\n", CodePivotFieldType, "/relations/tags/pivotFields/mail"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := wantErr(t, mk(tc.pivot), tc.code, tc.ptr)
			if p.File != "m/post.resource.yaml" || p.Line == 0 {
				t.Errorf("lokasi = %s:%d", p.File, p.Line)
			}
		})
	}
	t.Run("pivot sah lolos", func(t *testing.T) {
		noErrors(t, mk("      remark: {type: string, maxLength: 100, nullable: true}\n      weight: {type: decimal, precision: 10, scale: 2, default: \"1.5\"}\n      kind: {type: enum, values: [a, b], default: a}\n"))
	})
}

// ---- M-1: require go.mod modul pemanggil untuk rujukan lintas modul ----

func TestCrossModuleReferencesRequireGoModRequire(t *testing.T) {
	crm := mod(t, "crm", nil,
		f("crm/customer.resource.yaml", resource("crm.customer", "export: true\nfields:\n  name: {type: string, maxLength: 20}\n")),
		f("crm/address.abstract.yaml", abstract("crm.address", "export: true\nfields:\n  street: {type: string, maxLength: 20}\n")),
		f("crm/mail.type.yaml", fieldType("crm.mail", "export: true\nbase: {type: string, maxLength: 100}\n")))
	salesFiles := []file{f("sales/r.resource.yaml", resource("sales.r", `include:
  - source: crm.address
fields:
  contact: {type: crm.mail}
relations:
  customer: {type: belongsTo, to: crm.customer}
`))}
	t.Run("go.mod memuat require", func(t *testing.T) {
		sales := mod(t, "sales", []string{"acme.crm"}, salesFiles...)
		sales.GoRequires = map[string]bool{"erpora/modules/crm": true}
		noErrors(t, run(t, crm, sales))
	})
	t.Run("go.mod tanpa require ditolak untuk relasi, include, dan tipe domain", func(t *testing.T) {
		sales := mod(t, "sales", []string{"acme.crm"}, salesFiles...)
		sales.GoRequires = map[string]bool{}
		rep := run(t, crm, sales)
		n := 0
		for _, p := range rep.Problems {
			if p.Code == CodeModuleGoModRequire {
				n++
				if !strings.Contains(p.Message, "erpora/modules/crm") || p.Module != "sales" {
					t.Errorf("temuan = %+v", p)
				}
			}
		}
		if n < 2 {
			t.Fatalf("diharapkan temuan untuk relasi dan penyertaan/tipe, dapat %d: %s", n, dump(rep.Problems))
		}
		wantErr(t, rep, CodeModuleGoModRequire, "/relations/customer")
	})
	t.Run("go.mod tidak diketahui (Input manual) dilewati", func(t *testing.T) {
		sales := mod(t, "sales", []string{"acme.crm"}, salesFiles...)
		noErrors(t, run(t, crm, sales))
	})
	t.Run("slug berhubung tanda hubung memakai direktori tanpa tanda hubung", func(t *testing.T) {
		tgt := mod(t, "my-mod", nil, f("t/r.resource.yaml", resource("my-mod.thing", "fields:\n  a: {type: text}\n")))
		tgt.Dir = "modules/mymod"
		src := mod(t, "src", []string{"acme.my-mod"}, f("s/r.resource.yaml", resource("src.r", "relations:\n  t: {type: belongsTo, to: my-mod.thing}\n")))
		src.GoRequires = map[string]bool{"erpora/modules/mymod": true}
		noErrors(t, run(t, tgt, src))
		src.GoRequires = map[string]bool{"erpora/modules/my-mod": true}
		wantErr(t, run(t, tgt, src), CodeModuleGoModRequire, "/relations/t")
	})
}

func TestLoadWorkspaceReadsGoRequires(t *testing.T) {
	root := t.TempDir()
	put := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("modules/mymod/module.json", `{"key":"acme.my-mod","slug":"my-mod"}`)
	put("modules/mymod/go.mod", "module erpora/modules/mymod\n\ngo 1.27\n\nrequire erpora/modules/crm v0.0.0\n")
	put("modules/nogomod/module.json", `{"key":"acme.nogomod","slug":"nogomod"}`)
	in, probs := LoadWorkspace(root)
	if len(probs) != 0 || len(in.Modules) != 2 {
		t.Fatalf("modul=%d temuan=%v", len(in.Modules), probs)
	}
	for _, m := range in.Modules {
		switch m.Dir {
		case "modules/mymod":
			if !m.GoRequires["erpora/modules/crm"] {
				t.Errorf("GoRequires = %v", m.GoRequires)
			}
		case "modules/nogomod":
			if m.GoRequires != nil {
				t.Errorf("tanpa go.mod harus nil: %v", m.GoRequires)
			}
		}
	}
}
