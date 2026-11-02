package gen

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/internal/platform/modules/manifest"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
)

func applyPlan(t *testing.T, root string, p Plan) {
	t.Helper()
	p.Post = nil // langkah Post (go mod tidy, bun) butuh repo penuh; dibuktikan di tests/generator
	if err := Apply(context.Background(), p, Options{Root: root}); err != nil {
		t.Fatal(err)
	}
}

func opContent(t *testing.T, ops []FileOp, path string) string {
	t.Helper()
	for _, op := range ops {
		if op.Path == path {
			return string(op.Content)
		}
	}
	t.Fatalf("op %s tidak ada; ada: %v", path, opPaths(ops))
	return ""
}

func opPaths(ops []FileOp) []string {
	out := make([]string, len(ops))
	for i, o := range ops {
		out[i] = o.Path
	}
	return out
}

func TestMakeModule_Golden(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory", Publisher: "acme", Frontend: true, Depends: []string{"acme.example"}})
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/module_inventory", plan.Ops)
	if len(plan.Post) != 4 {
		t.Errorf("Post = %d langkah, want 4 (go mod tidy, contracts:generate, bun install, biome)", len(plan.Post))
	}
}

// Review Focus 1: slug berstrip.
func TestMakeModule_Golden_Hyphenated(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Publisher: "acme", Frontend: true})
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "testdata/golden/module_stock_opname", plan.Ops)
	checks := map[string][]string{
		"modules/stockopname/go.mod":                                        {"module erpora/modules/stockopname"},
		"modules/stockopname/module.json":                                   {`"slug": "stock-opname"`, `"key": "acme.stock-opname"`, `"stock-opname.items.read"`},
		"modules/stockopname/openapi.yaml":                                  {"/api/stock-opname/items", "x-erp-permission: stock-opname.items.read", "operationId: listStockOpnameItems"},
		"modules/stockopname/migrations/0001_create_stock_opname_items.sql": {"CREATE TABLE stock_opname_items"},
		"modules/stockopname/web/package.json":                              {`"@erpora/module-stockopname"`},
		"internal/app/modules.go":                                           {"stockopname.New(),", `"erpora/modules/stockopname"`},
		"go.work":                                                           {"./modules/stockopname"},
	}
	for path, wants := range checks {
		got := opContent(t, plan.Ops, path)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s tidak memuat %q", path, w)
			}
		}
	}
	if !strings.Contains(opContent(t, plan.Ops, "modules/stockopname/module.go"), "package stockopname") {
		t.Error("package harus stockopname")
	}
}

func TestMakeModule_RejectsCollisions(t *testing.T) {
	root := fixtureRepo(t)
	first, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Publisher: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, first)
	before := checksum(t, root)
	cases := []struct {
		name string
		opts ModuleOptions
		want string
	}{
		{"Dir sama (stockopname vs stock-opname)", ModuleOptions{Name: "stockopname"}, "stockopname"},
		{"slug example", ModuleOptions{Name: "example"}, "example"},
		{"dependensi tidak ada", ModuleOptions{Name: "billing", Depends: []string{"acme.missing"}}, "acme.missing"},
		{"slug cadangan", ModuleOptions{Name: "platform"}, "dicadangkan"},
		{"nama tidak valid", ModuleOptions{Name: "Billing"}, "tidak valid"},
		// G-4: nama package yang tidak dapat dipakai atau bentrok dengan identifier di internal/app/modules.go.
		{"keyword Go", ModuleOptions{Name: "func"}, "package Go"},
		{"bentrok variabel ex di modules.go", ModuleOptions{Name: "ex"}, "modules.go"},
	}
	for _, c := range cases {
		_, err := MakeModule(root, c.opts)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want memuat %q", c.name, err, c.want)
		}
	}
	if checksum(t, root) != before {
		t.Error("penolakan tidak boleh mengubah direktori")
	}
}

func TestMakeModule_MarkerMissing(t *testing.T) {
	root := fixtureRepo(t)
	p := filepath.Join(root, "internal", "app", "modules.go")
	data, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(string(data), "// erp:generated:modules", "")), 0o600); err != nil {
		t.Fatal(err)
	}
	before := checksum(t, root)
	_, err := MakeModule(root, ModuleOptions{Name: "inventory"})
	if !errors.Is(err, ErrMarkerNotFound) {
		t.Fatalf("err = %v, want ErrMarkerNotFound", err)
	}
	if exists(root, "modules/inventory/module.go") || checksum(t, root) != before {
		t.Error("tidak boleh ada file baru")
	}
}

func TestMakeModule_ApplyTwiceWithoutForce(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory"})
	if err != nil {
		t.Fatal(err)
	}
	applyPlan(t, root, plan)
	if _, err := MakeModule(root, ModuleOptions{Name: "inventory"}); !errors.Is(err, ErrExists) {
		t.Errorf("rencana kedua: err = %v, want ErrExists", err)
	}
	// Penerapan ulang rencana yang sama (tanpa membangun ulang) juga ditolak tanpa --force.
	plan.Post = nil
	if err := Apply(context.Background(), plan, Options{Root: root}); !errors.Is(err, ErrExists) {
		t.Errorf("Apply kedua: err = %v, want ErrExists", err)
	}
}

// Mencegah template tertinggal dari sumbernya: setiap file non-notes modules/example punya padanan di template.
func TestMakeModule_TemplateMatchesExampleShape(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "inventory", Frontend: true, Depends: []string{"acme.example"}, Metadata: true, MetadataGenerate: noopGenerate})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, op := range plan.Ops {
		if rel, ok := strings.CutPrefix(op.Path, "modules/inventory/"); ok {
			have[rel] = true
		}
	}
	// Padanan nama: notes -> items; contract/notes.go -> contract/doc.go; dua migrasi example -> satu migrasi awal.
	rename := map[string]string{
		"internal/application/list_notes.go":            "internal/application/list_items.go",
		"internal/application/list_notes_test.go":       "internal/application/list_items_test.go",
		"internal/domain/note.go":                       "internal/domain/item.go",
		"internal/domain/note_test.go":                  "internal/domain/item_test.go",
		"internal/infrastructure/notes_repo.go":         "internal/infrastructure/items_repo.go",
		"contract/notes.go":                             "contract/doc.go",
		"migrations/0001_create_example_notes.sql":      "migrations/0001_create_inventory_items.sql",
		"migrations/0002_add_example_notes_version.sql": "", // version/updated_at sudah ada di migrasi awal
		// Metadata resource (spec resource-metadata task 8.2 dan 18.3): example dimigrasikan ke metadata; kerangka metadata
		// pada make:module bersifat opsional (--metadata) dan menghasilkan metadata/item.resource.yaml.
		"metadata/note.resource.yaml":                       "metadata/item.resource.yaml",
		"migrations/0003_add_example_notes_audit_users.sql": "", // created_by/updated_by (sdk:audit) aditif pada example
		"internal/infrastructure/notes_repo_test.go":        "", // uji konversi id pengguna nullable (sdk:audit) khusus example
		"internal/domain/meta_runtime.gen.go":               "",
		"internal/domain/note_meta.gen.go":                  "",
		"internal/presentation/http/meta_runtime.gen.go":    "",
		"internal/presentation/http/note_dto.gen.go":        "",
		"web/src/metadata/note.meta.gen.ts":                 "",
		// Frontend: halaman dan pendukung khusus example (notes, koleksi DB, test halaman) tidak ada di template; halaman awal
		// generik items-page (anak /<slug>/items; indeks /<slug> adalah Dashboard bawaan shell) menggantikan notes-page. api/contract.gen.ts dibangkitkan oleh langkah Post contracts:generate.
		"catalogs_test.go":                     "", // template menyertakan TestCatalogs di module_test.go
		"web/src/pages/notes-page.tsx":         "web/src/pages/items-page.tsx",
		"web/src/pages/notes-page.test.tsx":    "",
		"web/src/db/notes-collection.ts":       "",
		"web/src/db/notes-collection.test.tsx": "",
		"web/src/notes-api.ts":                 "",
		"web/src/testing.tsx":                  "", // dukungan test halaman example
		"web/api/contract.gen.ts":              "",
	}
	exampleDir := filepath.Join("..", "..", "..", "modules", "example")
	err = filepath.WalkDir(exampleDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(exampleDir, p)
		rel = filepath.ToSlash(rel)
		if rel == "go.sum" || strings.Contains("/"+rel, "/node_modules/") { // node_modules hasil bun install lokal, bukan sumber
			return nil
		}
		want := rel
		if r, ok := rename[rel]; ok {
			want = r
		}
		if want != "" && !have[want] {
			t.Errorf("file example %s tidak punya padanan template (%s); template tertinggal dari sumbernya", rel, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"deps.go", "README.md", "web/package.json", "web/tsconfig.json", "web/bunfig.toml", "web/src/index.ts", "web/src/info.ts", "web/src/module.ts", "web/src/routes.tsx", "web/src/pages/items-page.tsx", "web/src/i18n/keys.gen.ts", "web/src/module.test.ts", "i18n/id.json", "i18n/en.json", "internal/domain/errors.go"} {
		if !have[extra] {
			t.Errorf("file %s harus dihasilkan", extra)
		}
	}
}

// Revisi M3a (R11.6): templat --frontend mendeklarasikan "desk" di module.json (menu contoh ke halaman anak
// /items, label dari katalog templat), tanpa navigation dan tanpa rute indeks di web (indeks /<slug> adalah
// Dashboard bawaan shell), serta tes modul memakai assertDeskRoutes.
func TestMakeModule_DeskDeclaredAndNoNavigationOrIndexRoute(t *testing.T) {
	root := fixtureRepo(t)
	for _, name := range []string{"inventory", "stock-opname"} {
		t.Run(name, func(t *testing.T) {
			plan, err := MakeModule(root, ModuleOptions{Name: name, Publisher: "acme", Frontend: true})
			if err != nil {
				t.Fatal(err)
			}
			dir := "modules/" + strings.ReplaceAll(name, "-", "") + "/"
			mf, err := module.ParseManifest([]byte(opContent(t, plan.Ops, dir+"module.json")))
			if err != nil {
				t.Fatal(err)
			}
			if problems := manifest.Validate(mf); len(problems) != 0 {
				t.Fatalf("manifest templat tidak valid: %+v", problems)
			}
			d := mf.Desk
			if d == nil || d.Label != "desk.title" || d.Icon == "" || len(d.Menus) != 1 {
				t.Fatalf("Desk = %+v", d)
			}
			if m := d.Menus[0]; m.ID != "items" || m.To != "/items" || m.Permission != name+".items.read" || m.Label != "nav.items" {
				t.Errorf("menu = %+v", m)
			}
			// Aturan registry/module:check (DESK) terhadap katalog templat: semua kunci label ada.
			cat := fstest.MapFS{"id.json": {Data: []byte(opContent(t, plan.Ops, dir+"i18n/id.json"))}}
			if ps := registry.CheckDesk(mf, cat); len(ps) != 0 {
				t.Errorf("CheckDesk: %+v", ps)
			}

			moduleTS := opContent(t, plan.Ops, dir+"web/src/module.ts")
			if strings.Contains(moduleTS, "navigation") {
				t.Errorf("module.ts tidak boleh memuat navigation: %s", moduleTS)
			}
			routes := opContent(t, plan.Ops, dir+"web/src/routes.tsx")
			if strings.Contains(routes, `path: "/",`) || !strings.Contains(routes, `path: "/items"`) || !strings.Contains(routes, "items-page") {
				t.Errorf("routes.tsx harus mendaftarkan hanya /items: %s", routes)
			}
			// K9: halaman tanpa judul sendiri (breadcrumb terakhir = satu-satunya h1); deskripsi muted tetap.
			page := opContent(t, plan.Ops, dir+"web/src/pages/items-page.tsx")
			if strings.Contains(page, "<h1") || strings.Contains(page, "<header") || strings.Contains(page, "page.items.title") {
				t.Errorf("items-page.tsx tidak boleh memuat judul halaman: %s", page)
			}
			if !strings.Contains(page, "page.items.description") {
				t.Errorf("items-page.tsx harus tetap memuat deskripsi: %s", page)
			}
			testTS := opContent(t, plan.Ops, dir+"web/src/module.test.ts")
			for _, w := range []string{"assertDeskRoutes", `from "@erpora/module-sdk/testing"`} {
				if !strings.Contains(testTS, w) {
					t.Errorf("module.test.ts tidak memuat %q", w)
				}
			}
			if strings.Contains(testTS, "mod.navigation") {
				t.Error("module.test.ts tidak boleh menguji navigation")
			}
			for _, path := range opPaths(plan.Ops) {
				if strings.HasSuffix(path, "web/src/pages/home-page.tsx") {
					t.Errorf("home-page.tsx tidak boleh dihasilkan lagi: %s", path)
				}
			}
			for _, loc := range []string{"id", "en"} {
				cat := opContent(t, plan.Ops, dir+"i18n/"+loc+".json")
				for _, k := range []string{`"desk"`, `"title"`, `"items"`} {
					if !strings.Contains(cat, k) {
						t.Errorf("katalog %s tidak memuat %s", loc, k)
					}
				}
			}
		})
	}
}

// Modul tanpa --frontend tidak punya Desk (R11.1: hanya modul ber-web/ yang wajib) dan katalognya tidak membawa
// kunci yatim yang hanya dipakai Desk atau halaman web (desk.*, nav.*, page.*).
func TestMakeModule_WithoutFrontendHasNoDeskAndNoOrphanI18nKeys(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Publisher: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	mf, err := module.ParseManifest([]byte(opContent(t, plan.Ops, "modules/stockopname/module.json")))
	if err != nil {
		t.Fatal(err)
	}
	if problems := manifest.Validate(mf); len(problems) != 0 {
		t.Fatalf("manifest tidak valid: %+v", problems)
	}
	if mf.Desk != nil {
		t.Errorf("Desk = %+v, want nil untuk modul tanpa --frontend", mf.Desk)
	}
	if raw := opContent(t, plan.Ops, "modules/stockopname/module.json"); strings.Contains(raw, `"desk"`) {
		t.Errorf("module.json tidak boleh memuat desk: %s", raw)
	}
	cats := fstest.MapFS{"id.json": {Data: []byte(opContent(t, plan.Ops, "modules/stockopname/i18n/id.json"))}}
	if ps := registry.CheckDesk(mf, cats); len(ps) != 0 {
		t.Errorf("CheckDesk (modul tanpa frontend tidak wajib desk): %+v", ps)
	}
	for _, loc := range []string{"id", "en"} {
		raw := opContent(t, plan.Ops, "modules/stockopname/i18n/"+loc+".json")
		var cat map[string]any
		if err := json.Unmarshal([]byte(raw), &cat); err != nil {
			t.Fatalf("katalog %s bukan JSON valid: %v\n%s", loc, err, raw)
		}
		if len(cat) != 1 || cat["errors"] == nil {
			t.Errorf("katalog %s harus hanya memuat errors (kunci desk/nav/page yatim): %v", loc, cat)
		}
	}
	for _, path := range opPaths(plan.Ops) {
		if strings.Contains(path, "/web/") {
			t.Errorf("tanpa --frontend tidak boleh ada berkas web: %s", path)
		}
	}
}

func noopGenerate(context.Context, string, string) error { return nil }

// Task 18.3: make:module --metadata menambah kerangka metadata resource; tanpa flag keluaran tidak berubah.
func TestMakeModule_MetadataSkeleton(t *testing.T) {
	root := fixtureRepo(t)
	ops := func(o ModuleOptions) map[string]string {
		plan, err := MakeModule(root, o)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, op := range plan.Ops {
			out[op.Path] = string(op.Content)
		}
		return out
	}
	plain := ops(ModuleOptions{Name: "inventory"})
	withMeta := ops(ModuleOptions{Name: "inventory", Metadata: true, MetadataGenerate: noopGenerate})

	if _, ok := plain["modules/inventory/metadata/item.resource.yaml"]; ok {
		t.Error("tanpa --metadata tidak boleh ada berkas metadata")
	}
	for _, f := range []string{"module.json", "openapi.yaml", "README.md"} {
		c := plain["modules/inventory/"+f]
		if strings.Contains(c, "x-erp-generated") || strings.Contains(c, "inventory.item.read") || strings.Contains(c, "metadata/item.resource.yaml") {
			t.Errorf("%s tanpa --metadata tidak boleh menyinggung metadata", f)
		}
	}
	meta := withMeta["modules/inventory/metadata/item.resource.yaml"]
	for _, want := range []string{"key: inventory.item", "kind: resource", "title:", "label: fields.item.title"} {
		if !strings.Contains(meta, want) {
			t.Errorf("metadata tidak memuat %q:\n%s", want, meta)
		}
	}
	if !strings.Contains(withMeta["modules/inventory/module.json"], `"inventory.items.read", "inventory.item.read"`) {
		t.Error("module.json harus menambah capability inventory.item.read (awalan kunci resource metadata)")
	}
	if !strings.Contains(withMeta["modules/inventory/openapi.yaml"], "    Item:\n      type: object\n      x-erp-generated: true\n") {
		t.Error("skema Item harus bertanda x-erp-generated agar digantikan pembangkit")
	}
	// Selain tiga berkas itu, semua keluaran identik dengan tanpa flag.
	for p, c := range plain {
		if strings.HasSuffix(p, "/module.json") || strings.HasSuffix(p, "/openapi.yaml") || strings.HasSuffix(p, "/README.md") {
			continue
		}
		if withMeta[p] != c {
			t.Errorf("%s berubah oleh --metadata", p)
		}
	}
}

func TestMakeModule_MetadataGenerateStepAndRequirement(t *testing.T) {
	root := fixtureRepo(t)
	if _, err := MakeModule(root, ModuleOptions{Name: "inventory", Metadata: true}); err == nil {
		t.Fatal("--metadata tanpa pembangkit harus galat")
	}
	var gotDir string
	plan, err := MakeModule(root, ModuleOptions{Name: "stock-opname", Metadata: true, MetadataGenerate: func(_ context.Context, _ string, dir string) error {
		gotDir = dir
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Post) != 2 || !strings.HasPrefix(plan.Post[1].Name, "metadata:generate stockopname") {
		t.Fatalf("Post = %v", plan.Post)
	}
	if err := plan.Post[1].Run(context.Background(), root); err != nil || gotDir != "stockopname" {
		t.Errorf("err=%v dir=%q", err, gotDir)
	}
	// dengan frontend: metadata:generate sebelum contracts:generate
	plan, err = MakeModule(root, ModuleOptions{Name: "inventory", Frontend: true, Metadata: true, MetadataGenerate: noopGenerate})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Post) != 5 || !strings.HasPrefix(plan.Post[1].Name, "metadata:generate") || !strings.HasPrefix(plan.Post[2].Name, "contracts:generate") {
		var names []string
		for _, p := range plan.Post {
			names = append(names, p.Name)
		}
		t.Errorf("urutan Post = %v", names)
	}
}
