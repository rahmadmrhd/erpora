// Package fixturetest memverifikasi mini-repo fixture spec resource-metadata (D-18, Requirement 20.5): berkas
// metadata lolos skema dan pemuat, memuat semua fitur yang dibutuhkan exit gate T1 sampai T4, memakai nama
// yang sah menurut paket penamaan, dan konsisten dengan manifest modul fixture.
package fixturetest

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/internal/testrepo"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/naming"
)

var fixtureModules = []string{"fixturecore", "fixturesales"}

// loadAll memuat metadata seluruh modul fixture dari salinan sementara dan gagal bila ada temuan.
func loadAll(t *testing.T) (root string, docs map[string][]*load.Document) {
	t.Helper()
	root = testrepo.Copy(t, false)
	docs = map[string][]*load.Document{}
	for _, m := range fixtureModules {
		ds, probs := load.LoadFS(os.DirFS(root), path.Join("modules", m, "metadata"))
		for _, p := range probs {
			t.Errorf("%s: %s:%d: [%s] %s", m, p.File, p.Line, p.Code, p.Message)
		}
		docs[m] = ds
	}
	if t.Failed() {
		t.FailNow()
	}
	return root, docs
}

func keysOf(docs []*load.Document) []string {
	var out []string
	for _, d := range docs {
		out = append(out, d.Kind+" "+d.Key)
	}
	sort.Strings(out)
	return out
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// walk memanggil fn untuk setiap peta (dan daftar) di pohon nilai dokumen.
func walk(v any, fn func(m map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, c := range t {
			walk(c, fn)
		}
	case []any:
		for _, c := range t {
			walk(c, fn)
		}
	}
}

func TestFixtureMetadataLoadsWithoutProblems(t *testing.T) {
	_, docs := loadAll(t)
	want := map[string][]string{
		"fixturecore": {
			"abstract fixturecore.address",
			"fieldType fixturecore.email",
			"resource fixturecore.branch",
			"resource fixturecore.customer",
		},
		"fixturesales": {
			"abstract fixturesales.pricing",
			"resource fixturesales.invoice",
			"resource fixturesales.invoiceLine",
			"resource fixturesales.note",
			"resource fixturesales.product",
			"resource fixturesales.tag",
		},
	}
	for m, w := range want {
		got := keysOf(docs[m])
		if strings.Join(got, "|") != strings.Join(w, "|") {
			t.Errorf("%s: dokumen = %v, diharapkan %v", m, got, w)
		}
	}
}

// TestFixtureCoversRequiredFeatures membuktikan fixture memuat semua fitur Requirement 20.5.
func TestFixtureCoversRequiredFeatures(t *testing.T) {
	_, docs := loadAll(t)
	var all []*load.Document
	for _, m := range fixtureModules {
		all = append(all, docs[m]...)
	}
	found := map[string]bool{}
	mark := func(f string) { found[f] = true }

	for _, d := range all {
		slug, _, _ := strings.Cut(d.Key, ".")
		if d.Kind == "fieldType" {
			mark("tipe domain")
		}
		if d.Kind == "abstract" && d.Value["export"] == true {
			mark("blok diekspor")
		}
		walk(d.Value, func(m map[string]any) {
			if src, _ := m["source"].(string); src != "" {
				if src == "sdk:document" {
					mark("dokumen sdk:document")
				}
				if _, ok := m["prefix"]; ok {
					mark("blok dengan prefix")
				}
				if s, _, _ := strings.Cut(src, "."); s != slug && !strings.HasPrefix(src, "sdk:") {
					mark("include lintas modul")
				}
				if _, ok := m["omit"]; ok {
					mark("include omit")
				}
				if _, ok := m["set"]; ok {
					mark("include set")
				}
			}
			if typ, _ := m["type"].(string); typ != "" {
				switch typ {
				case "decimal", "enum", "uuid", "integer", "boolean", "date":
					mark("tipe " + typ)
				}
				if strings.HasPrefix(typ, "sdk:") {
					mark("tipe domain sdk")
				}
				if strings.HasPrefix(typ, "fixturecore.") {
					mark("tipe domain modul")
				}
			}
			if k, _ := m["kind"].(string); k == "expr" || k == "stored" || k == "func" {
				if _, ok := m["expr"]; ok || k == "func" {
					mark("accessor " + k)
				}
			}
			if rt, _ := m["type"].(string); rt != "" && (m["to"] != nil || m["of"] != nil || m["types"] != nil) {
				mark("relasi " + rt)
				for _, key := range []string{"to", "of"} {
					if target, _ := m[key].(string); target != "" {
						ts, _, _ := strings.Cut(target, ".")
						if ts == slug {
							mark("relasi satu modul")
						} else {
							mark("relasi lintas modul")
						}
					}
				}
				if m["embedded"] == true {
					mark("hasMany embedded")
				}
				if m["snapshot"] != nil {
					mark("snapshot")
				}
				if m["pivotFields"] != nil {
					mark("pivotFields")
				}
				if m["filter"] != nil {
					mark("filter relasi")
				}
			}
			if m["snapshotSource"] == true {
				mark("snapshotSource")
			}
			if m["unique"] == true {
				mark("unique")
			}
			if m["extensible"] == true {
				mark("enum extensible")
			}
		})
		if d.Kind == "resource" {
			for _, k := range []string{"ownerField", "scopeFields", "indexes", "rules", "labelTemplate"} {
				if d.Value[k] != nil {
					mark(k)
				}
			}
		}
	}

	for _, f := range []string{
		"dokumen sdk:document", "relasi satu modul", "relasi lintas modul",
		"tipe decimal", "tipe enum", "hasMany embedded",
		"relasi morphTo", "relasi morphMany", "relasi belongsToMany", "relasi belongsTo", "relasi hasMany",
		"tipe domain", "tipe domain sdk", "tipe domain modul",
		"blok diekspor", "blok dengan prefix", "include lintas modul", "include omit", "include set",
		"accessor expr", "accessor stored", "accessor func",
		"snapshot", "snapshotSource", "pivotFields", "filter relasi", "unique", "enum extensible",
		"ownerField", "scopeFields", "indexes", "rules", "labelTemplate",
	} {
		if !found[f] {
			t.Errorf("fixture tidak memuat: %s", f)
		}
	}
}

// TestFixtureAvoidsRejectedConstructs: tanpa highly_confidential (K17) dan tanpa nama yang ditolak penamaan.
func TestFixtureAvoidsRejectedConstructs(t *testing.T) {
	root, docs := loadAll(t)

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p) //nolint:gosec // fixture tes
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(b), "highly_confidential") {
			t.Errorf("%s memuat highly_confidential (ditiadakan, K17)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	check := func(origin, name string) {
		for _, v := range naming.Validate(name) {
			t.Errorf("%s: nama %q ditolak penamaan: %v", origin, name, v)
		}
	}
	for _, m := range fixtureModules {
		for _, d := range docs[m] {
			if _, name, err := naming.SplitResourceKey(d.Key); err != nil {
				t.Errorf("%s: kunci %q: %v", d.File, d.Key, err)
			} else {
				check(d.File, name)
			}
			for _, section := range []string{"fields", "relations"} {
				for n := range asMap(d.Value[section]) {
					check(d.File+" "+section, n)
				}
			}
			for _, r := range asList(d.Value["rules"]) {
				check(d.File+" rules", asMap(r)["id"].(string))
			}
			walk(d.Value, func(mm map[string]any) {
				for n := range asMap(mm["pivotFields"]) {
					check(d.File+" pivotFields", n)
				}
			})
		}
	}
}

type manifest struct {
	Slug         string   `json:"slug"`
	Key          string   `json:"key"`
	Capabilities []string `json:"capabilities"`
	Dependencies []struct {
		Module string `json:"module"`
	} `json:"dependencies"`
	Resources []struct {
		Key         string `json:"key"`
		FieldGroups []struct {
			Key string `json:"key"`
		} `json:"field_groups"`
	} `json:"resources"`
}

func readManifest(t *testing.T, root, mod string) manifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "modules", mod, "module.json")) //nolint:gosec // fixture tes
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s/module.json: %v", mod, err)
	}
	return m
}

// TestFixtureConsistentWithManifests: kunci resource metadata ada di manifest (setelah konversi snake_case),
// permission baca terdaftar sebagai capability, grup field terpakai terdefinisi, dan rujukan lintas modul
// hanya ke dependensi manifest.
func TestFixtureConsistentWithManifests(t *testing.T) {
	root, docs := loadAll(t)
	manifests := map[string]manifest{}
	for _, m := range fixtureModules {
		manifests[m] = readManifest(t, root, m)
	}
	byKey := map[string]manifest{}
	for _, mf := range manifests {
		byKey[mf.Key] = mf
	}

	for _, m := range fixtureModules {
		mf := manifests[m]
		if mf.Slug != m {
			t.Errorf("%s: slug manifest %q", m, mf.Slug)
		}
		deps := map[string]bool{}
		for _, d := range mf.Dependencies {
			deps[d.Module] = true
		}
		caps := map[string]bool{}
		for _, c := range mf.Capabilities {
			caps[c] = true
		}
		groups := map[string]map[string]bool{}
		for _, r := range mf.Resources {
			groups[r.Key] = map[string]bool{}
			for _, g := range r.FieldGroups {
				groups[r.Key][g.Key] = true
			}
		}

		for _, d := range docs[m] {
			slug, _, _ := strings.Cut(d.Key, ".")
			if slug != m {
				t.Errorf("%s: kunci %q berslug %q, bukan %q", d.File, d.Key, slug, m)
			}
			if d.Kind != "resource" {
				continue
			}
			mk, err := naming.ManifestResourceKey(d.Key)
			if err != nil {
				t.Errorf("%s: %v", d.File, err)
				continue
			}
			if groups[mk] == nil {
				t.Errorf("%s: resource %q tidak ada di manifest %s", d.File, mk, m)
				continue
			}
			if rd, err := naming.CapabilityKey(d.Key, "read"); err != nil || !caps[rd] {
				t.Errorf("%s: capability %q tidak terdaftar (%v)", d.File, rd, err)
			}
			// Grup field yang dinyatakan langsung pada field resource harus terdefinisi di manifest.
			for fname, f := range asMap(d.Value["fields"]) {
				if g, _ := asMap(f)["fieldGroup"].(string); g != "" && !groups[mk][g] {
					t.Errorf("%s: field %s memakai grup %q yang tidak ada di manifest %s", d.File, fname, g, mk)
				}
			}
			// Rujukan relasi lintas modul hanya ke modul dependensi manifest.
			for rname, r := range asMap(d.Value["relations"]) {
				rm := asMap(r)
				var targets []string
				for _, key := range []string{"to", "of"} {
					if s, _ := rm[key].(string); s != "" {
						targets = append(targets, s)
					}
				}
				for _, ty := range asList(rm["types"]) {
					targets = append(targets, ty.(string))
				}
				for _, tg := range targets {
					ts, _, _ := strings.Cut(tg, ".")
					if ts == m {
						continue
					}
					ok := false
					for dep := range deps {
						if depMf, found := byKey[dep]; found && depMf.Slug == ts {
							ok = true
						}
					}
					if !ok {
						t.Errorf("%s: relasi %s merujuk %q di luar dependensi manifest", d.File, rname, tg)
					}
				}
			}
		}
	}

	// Setiap capability berpola registry: setelah slug hanya huruf kecil, angka, dan garis bawah.
	for _, m := range fixtureModules {
		for _, c := range manifests[m].Capabilities {
			if !strings.HasPrefix(c, m+".") || strings.ToLower(c) != c {
				t.Errorf("%s: capability %q tidak sah", m, c)
			}
		}
	}
}

// TestFixtureGoWorkUsesOnlyFixtureModules: go.work fixture hanya memuat direktori yang ada pada salinan
// beserta sdk asli, sehingga salinan dapat dikompilasi tanpa mengubah go.work root.
func TestFixtureGoWorkUsesOnlyFixtureModules(t *testing.T) {
	root := testrepo.Copy(t, true)
	b, err := os.ReadFile(filepath.Join(root, "go.work")) //nolint:gosec // fixture tes
	if err != nil {
		t.Fatal(err)
	}
	var uses []string
	in := false
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "use (":
			in = true
		case line == ")":
			in = false
		case in && line != "":
			uses = append(uses, line)
		}
	}
	want := []string{".", "./modules/fixturecore", "./modules/fixturesales", "./sdk"}
	sorted := append([]string(nil), uses...)
	sort.Strings(sorted)
	if strings.Join(sorted, "|") != strings.Join(want, "|") {
		t.Fatalf("go.work use = %v, diharapkan %v", uses, want)
	}
	for _, u := range uses {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(u), "go.mod")); err != nil {
			t.Errorf("use %s: %v", u, err)
		}
	}
}
