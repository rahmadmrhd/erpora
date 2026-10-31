package desks

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	"erpora/internal/platform/desks/store"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

// --- perangkat uji ---

// catalogJSON memuat semua kunci label yang dipakai test (id.json).
const catalogJSON = `{"desk":{"title":"Judul","dashboard":"Beranda Kustom"},
 "nav":{"a":"A","b":"B","c":"C","d":"D","e":"E","g":"G","h":"H"}}`

type testModule struct {
	m   module.Manifest
	cat fs.FS
}

func (t testModule) Manifest() module.Manifest     { return t.m }
func (testModule) Register(module.Registrar) error { return nil }
func (t testModule) Catalogs() fs.FS               { return t.cat }

// newModule membuat modul fiktif ber-slug slug dengan capabilities <slug>.items.{read,write,admin}.
func newModule(t *testing.T, slug string, desk *module.Desk) module.Module {
	t.Helper()
	doc := map[string]any{
		"schema_version": "1", "key": "acme." + slug, "slug": slug, "name": slug, "version": "1.0.0",
		"publisher":     map[string]string{"id": "acme", "name": "Acme"},
		"trust":         "builtin",
		"compatibility": map[string]string{"platform": "^0.1", "sdk": "^0.1"},
		"dependencies":  []module.Dependency{{Module: registry.PlatformKey, Version: "^0.1"}},
		"capabilities":  []string{slug + ".items.read", slug + ".items.write", slug + ".items.admin"},
		"migrations":    map[string]string{"strategy": "none"},
		"offline":       map[string]string{"mode": "online-only"},
		"i18n":          map[string]any{"locales": []string{"id"}, "defaultLocale": "id"},
	}
	if desk != nil {
		doc["desk"] = desk
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return testModule{m: module.MustParseManifest(b), cat: fstest.MapFS{"id.json": {Data: []byte(catalogJSON)}}}
}

func item(slug, id, perm string) module.DeskMenu {
	return module.DeskMenu{ID: id, Label: "nav." + id, To: "/" + id, Permission: slug + ".items." + perm}
}

func group(id string, children ...module.DeskMenu) module.DeskMenu {
	return module.DeskMenu{ID: id, Label: "nav." + id, Children: children}
}

func deskWith(menus ...module.DeskMenu) *module.Desk {
	if menus == nil {
		menus = []module.DeskMenu{}
	}
	return &module.Desk{Label: "desk.title", Icon: "file-text", Menus: menus}
}

func buildRegistry(t *testing.T, mods ...module.Module) *registry.Registry {
	t.Helper()
	reg, err := registry.Build(registry.Options{
		PlatformVersion: "0.1.0", SDKVersion: "0.1.0",
		PlatformCapabilities: []string{"platform.modules.read", "platform.desks.read", "platform.desks.update"},
		PlatformDesk:         PlatformDesk(),
	}, mods...)
	if err != nil {
		t.Fatalf("registry.Build: %v", err)
	}
	return reg
}

type permsKey struct{}

// ctxAuthz mengizinkan permission yang dibawa context (prinsipal uji); tanpa itu semuanya ditolak.
type ctxAuthz struct{}

func (ctxAuthz) Allow(ctx context.Context, permission string) bool {
	set, _ := ctx.Value(permsKey{}).(map[string]bool)
	return set[permission]
}

// ctxFor membuat context bertenant dengan prinsipal yang memegang perms.
func ctxFor(tn tenancy.TenantID, perms ...string) context.Context {
	set := map[string]bool{}
	for _, p := range perms {
		set[p] = true
	}
	return context.WithValue(tenancy.WithTenant(context.Background(), tn), permsKey{}, set)
}

type statuses map[string]runtime.Status

func (s statuses) Status(_ tenancy.TenantID, key string) runtime.Status {
	if key == registry.PlatformKey {
		return runtime.StatusEnabled
	}
	if st, ok := s[key]; ok {
		return st
	}
	return runtime.StatusAvailable
}

type fakeOverrides struct {
	byTenant map[tenancy.TenantID][]store.Override
	err      error
	asked    []tenancy.TenantID
}

func (f *fakeOverrides) ListSite(_ context.Context, t tenancy.TenantID) ([]store.Override, error) {
	f.asked = append(f.asked, t)
	return f.byTenant[t], f.err
}

func newService(reg *registry.Registry, st statuses, ov OverrideSource) *Service {
	return New(Deps{Registry: reg, Status: st, Authz: ctxAuthz{}, Overrides: ov})
}

func mustList(t *testing.T, s *Service, ctx context.Context) View {
	t.Helper()
	v, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return v
}

func ids(desks []Desk) []string {
	var out []string
	for _, d := range desks {
		out = append(out, d.ID)
	}
	return out
}

func find(v View, deskID string) (Desk, bool) {
	for _, d := range v.Desks {
		if d.ID == deskID {
			return d, true
		}
	}
	return Desk{}, false
}

// menuIDs mengembalikan id menu tingkat atas; grup memuat anaknya sebagai "grup/anak".
func menuIDs(d Desk) []string {
	var out []string
	for _, m := range d.Menus {
		if m.Type == TypeGroup {
			var kids []string
			for _, c := range m.Children {
				kids = append(kids, c.ID)
			}
			out = append(out, m.ID+"["+strings.Join(kids, ",")+"]")
			continue
		}
		out = append(out, m.ID)
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// allPerms memberi prinsipal semua permission modul slug dan platform.
func allPerms(slugs ...string) []string {
	perms := []string{"platform.modules.read", "platform.desks.read"}
	for _, s := range slugs {
		perms = append(perms, s+".items.read", s+".items.write", s+".items.admin")
	}
	return perms
}

// --- tes ---

func TestList_DisabledModuleHasNoDesk(t *testing.T) {
	reg := buildRegistry(t,
		newModule(t, "alpha", deskWith(item("alpha", "a", "read"))),
		newModule(t, "beta", deskWith(item("beta", "a", "read"))),
		newModule(t, "gamma", deskWith(item("gamma", "a", "read"))),
		newModule(t, "delta", deskWith(item("delta", "a", "read"))),
	)
	st := statuses{
		"acme.alpha": runtime.StatusEnabled, "acme.beta": runtime.StatusDisabled,
		"acme.gamma": runtime.StatusInstalled, // belum diaktifkan
		// acme.delta tidak punya status: available
	}
	v := mustList(t, newService(reg, st, nil), ctxFor(tenantA, allPerms("alpha", "beta", "gamma", "delta")...))
	eq(t, ids(v.Desks), []string{"platform", "alpha"})
}

func TestList_ModuleWithoutDeskIsSkipped(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", nil), newModule(t, "beta", deskWith(item("beta", "a", "read"))))
	v := mustList(t, newService(reg, statuses{"acme.alpha": runtime.StatusEnabled, "acme.beta": runtime.StatusEnabled}, nil),
		ctxFor(tenantA, allPerms("alpha", "beta")...))
	eq(t, ids(v.Desks), []string{"platform", "beta"})
}

func TestList_MenuWithoutPermissionIsNotSent(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(
		item("alpha", "a", "read"), item("alpha", "b", "write"), item("alpha", "c", "admin"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	v := mustList(t, newService(reg, st, nil), ctxFor(tenantA, "alpha.items.read", "alpha.items.admin"))
	d, ok := find(v, "alpha")
	if !ok {
		t.Fatal("Desk alpha harus ada")
	}
	eq(t, menuIDs(d), []string{"dashboard", "a", "c"})
	raw, _ := json.Marshal(v)
	for _, leak := range []string{`"id":"b"`, `/alpha/b`, "alpha.items.write"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("respons memuat menu yang tidak diizinkan (%s): %s", leak, raw)
		}
	}
}

func TestList_GroupsFilteredAndEmptyGroupDropped(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(
		item("alpha", "a", "read"),
		group("g", item("alpha", "b", "write"), item("alpha", "c", "read")),
		group("h", item("alpha", "d", "admin"), item("alpha", "e", "admin")),
	)))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	v := mustList(t, newService(reg, st, nil), ctxFor(tenantA, "alpha.items.read"))
	d, _ := find(v, "alpha")
	// grup g tinggal anak c (b ditolak); grup h kosong dibuang.
	eq(t, menuIDs(d), []string{"dashboard", "a", "g[c]"})
}

func TestList_DeskDroppedWithoutAllowedNonDashboardMenu(t *testing.T) {
	reg := buildRegistry(t,
		newModule(t, "alpha", deskWith(item("alpha", "a", "read"))),
		newModule(t, "beta", deskWith()), // tanpa menu: Dashboard saja tidak membuat Desk tampil (D-5)
		newModule(t, "gamma", deskWith(group("g", item("gamma", "a", "admin")))),
	)
	st := statuses{"acme.alpha": runtime.StatusEnabled, "acme.beta": runtime.StatusEnabled, "acme.gamma": runtime.StatusEnabled}
	// prinsipal tidak punya izin apa pun selain yang di bawah ini
	v := mustList(t, newService(reg, st, nil), ctxFor(tenantA, "beta.items.read", "gamma.items.read", "platform.modules.read"))
	eq(t, ids(v.Desks), []string{"platform"})
}

func TestList_PlatformDeskDroppedWithoutPermission(t *testing.T) {
	reg := buildRegistry(t)
	v := mustList(t, newService(reg, statuses{}, nil), ctxFor(tenantA))
	if len(v.Desks) != 0 {
		t.Fatalf("tanpa izin tidak boleh ada Desk: %v", ids(v.Desks))
	}
}

func TestList_DashboardAlwaysFirstAndLocked(t *testing.T) {
	custom := deskWith(item("alpha", "a", "read"))
	custom.Dashboard = &module.DeskDashboard{Label: "desk.dashboard", Icon: "gauge"}
	reg := buildRegistry(t, newModule(t, "alpha", custom), newModule(t, "beta", deskWith(item("beta", "a", "read"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled, "acme.beta": runtime.StatusEnabled}
	v := mustList(t, newService(reg, st, nil), ctxFor(tenantA, allPerms("alpha", "beta")...))

	alpha, _ := find(v, "alpha")
	first := alpha.Menus[0]
	if first.Type != TypeItem || first.ID != "dashboard" || !first.Locked || first.To != "/alpha" || first.OwnerDesk != "alpha" {
		t.Errorf("dashboard alpha = %+v", first)
	}
	if first.Label != (MessageRef{NS: "alpha", Key: "desk.dashboard"}) || first.Icon != "gauge" {
		t.Errorf("dashboard kustom: label=%+v icon=%q", first.Label, first.Icon)
	}
	beta, _ := find(v, "beta")
	bd := beta.Menus[0]
	if bd.ID != "dashboard" || !bd.Locked || bd.To != "/beta" || bd.Label != (MessageRef{NS: "platform", Key: "desk.dashboard"}) || bd.Icon != "layout-dashboard" {
		t.Errorf("dashboard bawaan = %+v", bd)
	}
	plat, _ := find(v, "platform")
	if pd := plat.Menus[0]; pd.ID != "dashboard" || pd.To != "/platform" || !pd.Locked {
		t.Errorf("dashboard platform = %+v", pd)
	}
	for _, d := range v.Desks {
		for _, m := range d.Menus[1:] {
			if m.Locked || m.ID == "dashboard" {
				t.Errorf("hanya item pertama boleh locked/dashboard: Desk %s menu %+v", d.ID, m)
			}
		}
	}
}

func TestList_DeskAndMenuShape(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(
		module.DeskMenu{ID: "a", Label: "nav.a", Icon: "file", To: "/a/laporan", Permission: "alpha.items.read"},
		group("g", item("alpha", "b", "read")),
	)))
	v := mustList(t, newService(reg, statuses{"acme.alpha": runtime.StatusEnabled}, nil), ctxFor(tenantA, allPerms("alpha")...))
	d, _ := find(v, "alpha")
	if d.Source != SourceModule || d.ModuleKey != "acme.alpha" || d.Label != (MessageRef{NS: "alpha", Key: "desk.title"}) || d.Icon != "file-text" {
		t.Errorf("desk = %+v", d)
	}
	a := d.Menus[1]
	if a.To != "/alpha/a/laporan" || a.Icon != "file" || a.Label != (MessageRef{NS: "alpha", Key: "nav.a"}) || a.OwnerDesk != "alpha" || a.Locked {
		t.Errorf("item = %+v", a)
	}
	g := d.Menus[2]
	if g.Type != TypeGroup || g.To != "" || g.OwnerDesk != "" || len(g.Children) != 1 || g.Children[0].To != "/alpha/b" || g.Children[0].OwnerDesk != "alpha" {
		t.Errorf("grup = %+v", g)
	}
	plat, _ := find(v, "platform")
	if plat.Source != SourcePlatform || plat.ModuleKey != "" || plat.Label.NS != "platform" {
		t.Errorf("desk platform = %+v", plat)
	}
	if m := plat.Menus[1]; m.ID != "modules" || m.To != "/platform/modules" || m.OwnerDesk != "platform" {
		t.Errorf("menu platform = %+v", m)
	}
	// bentuk JSON: grup itu sendiri tidak membawa to/ownerDesk (hanya anaknya)
	g.Children = nil
	raw, _ := json.Marshal(g)
	if strings.Contains(string(raw), `"to"`) || strings.Contains(string(raw), "ownerDesk") || !strings.Contains(string(raw), `"type":"group"`) {
		t.Errorf("JSON grup = %s", raw)
	}
}

func TestList_PlatformFirstThenRegistryOrder(t *testing.T) {
	// beta bergantung pada alpha: registry mengurutkan alpha sebelum beta walau dimasukkan terbalik.
	alpha := newModule(t, "alpha", deskWith(item("alpha", "a", "read")))
	beta := newModule(t, "beta", deskWith(item("beta", "a", "read"))).(testModule)
	beta.m.Dependencies = append(beta.m.Dependencies, module.Dependency{Module: "acme.alpha", Version: "^1.0"})
	raw, _ := json.Marshal(beta.m)
	beta.m = module.MustParseManifest(raw)
	reg := buildRegistry(t, beta, alpha)
	st := statuses{"acme.alpha": runtime.StatusEnabled, "acme.beta": runtime.StatusEnabled}
	v := mustList(t, newService(reg, st, nil), ctxFor(tenantA, allPerms("alpha", "beta")...))
	eq(t, ids(v.Desks), []string{"platform", "alpha", "beta"})
}

func cfg(tn tenancy.TenantID, desk, config string) map[tenancy.TenantID][]store.Override {
	return map[tenancy.TenantID][]store.Override{tn: {{DeskID: desk, Config: json.RawMessage(config), Revision: 1}}}
}

func TestList_OverridesHideAndOrder(t *testing.T) {
	desk := deskWith(item("alpha", "a", "read"), item("alpha", "b", "read"), item("alpha", "c", "read"),
		group("g", item("alpha", "d", "read"), item("alpha", "e", "read")))
	reg := buildRegistry(t, newModule(t, "alpha", desk))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	ctx := ctxFor(tenantA, allPerms("alpha")...)
	cases := []struct {
		name, config string
		want         []string
	}{
		{"tanpa timpaan", `{}`, []string{"dashboard", "a", "b", "c", "g[d,e]"}},
		{"sembunyikan item", `{"hidden":["b"]}`, []string{"dashboard", "a", "c", "g[d,e]"}},
		{"sembunyikan grup", `{"hidden":["g"]}`, []string{"dashboard", "a", "b", "c"}},
		{"sembunyikan anak grup", `{"hidden":["d"]}`, []string{"dashboard", "a", "b", "c", "g[e]"}},
		{"semua anak grup disembunyikan membuang grup", `{"hidden":["d","e"]}`, []string{"dashboard", "a", "b", "c"}},
		{"urutan", `{"order":["c","g","a"]}`, []string{"dashboard", "c", "g[d,e]", "a", "b"}},
		{"urutan dengan id tidak dikenal dan ganda diabaikan", `{"order":["zzz","c","c","a"]}`, []string{"dashboard", "c", "a", "b", "g[d,e]"}},
		{"sembunyi dan urut", `{"hidden":["a"],"order":["c","a"]}`, []string{"dashboard", "c", "b", "g[d,e]"}},
		{"dashboard tidak dapat disembunyikan", `{"hidden":["dashboard","b"]}`, []string{"dashboard", "a", "c", "g[d,e]"}},
		{"dashboard tidak dapat dipindah", `{"order":["c","dashboard"]}`, []string{"dashboard", "c", "a", "b", "g[d,e]"}},
		{"hidden id tidak dikenal diabaikan", `{"hidden":["zzz"]}`, []string{"dashboard", "a", "b", "c", "g[d,e]"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := mustList(t, newService(reg, st, &fakeOverrides{byTenant: cfg(tenantA, "alpha", c.config)}), ctx)
			d, ok := find(v, "alpha")
			if !ok {
				t.Fatal("Desk alpha harus ada")
			}
			eq(t, menuIDs(d), c.want)
		})
	}
}

func TestList_OverrideHidingEverythingDropsDesk(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	v := mustList(t, newService(reg, st, &fakeOverrides{byTenant: cfg(tenantA, "alpha", `{"hidden":["a"]}`)}), ctxFor(tenantA, allPerms("alpha")...))
	eq(t, ids(v.Desks), []string{"platform"})
}

func TestList_InvalidOverrideConfigIsIgnored(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"), item("alpha", "b", "read"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	for name, config := range map[string]string{
		"bukan objek":          `[1]`,
		"hidden bukan array":   `{"hidden":"b"}`,
		"properti tak dikenal": `{"hidden":["b"],"x":1}`,
		"id tidak sah":         `{"hidden":["B b"]}`,
		"null":                 `null`,
		"json rusak":           `{`,
	} {
		t.Run(name, func(t *testing.T) {
			v := mustList(t, newService(reg, st, &fakeOverrides{byTenant: cfg(tenantA, "alpha", config)}), ctxFor(tenantA, allPerms("alpha")...))
			d, _ := find(v, "alpha")
			eq(t, menuIDs(d), []string{"dashboard", "a", "b"}) // timpaan rusak tidak menyembunyikan apa pun
		})
	}
}

func TestList_OverrideForUnknownOrHiddenDeskIsIgnored(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	ov := &fakeOverrides{byTenant: cfg(tenantA, "tidak-ada", `{"hidden":["a"]}`)}
	v := mustList(t, newService(reg, st, ov), ctxFor(tenantA, allPerms("alpha")...))
	eq(t, ids(v.Desks), []string{"platform", "alpha"})
}

func TestList_OverrideStoreErrorIsReturned(t *testing.T) {
	reg := buildRegistry(t)
	boom := errors.New("db mati")
	_, err := newService(reg, statuses{}, &fakeOverrides{err: boom}).List(ctxFor(tenantA, allPerms()...))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want membungkus galat store", err)
	}
}

func TestList_OverridesAreScopedByTenantFromContext(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"), item("alpha", "b", "read"))))
	st := statuses{"acme.alpha": runtime.StatusEnabled}
	ov := &fakeOverrides{byTenant: cfg(tenantA, "alpha", `{"hidden":["b"]}`)}
	s := newService(reg, st, ov)
	va := mustList(t, s, ctxFor(tenantA, allPerms("alpha")...))
	vb := mustList(t, s, ctxFor(tenantB, allPerms("alpha")...))
	da, _ := find(va, "alpha")
	db, _ := find(vb, "alpha")
	eq(t, menuIDs(da), []string{"dashboard", "a"})
	eq(t, menuIDs(db), []string{"dashboard", "a", "b"})
	if len(ov.asked) != 2 || ov.asked[0] != tenantA || ov.asked[1] != tenantB {
		t.Errorf("store dipanggil untuk tenant %v, want [A B]", ov.asked)
	}
}

func TestList_NoTenantInContextFails(t *testing.T) {
	reg := buildRegistry(t)
	ov := &fakeOverrides{}
	_, err := newService(reg, statuses{}, ov).List(context.Background())
	if !errors.Is(err, ErrNoTenant) {
		t.Fatalf("err = %v, want ErrNoTenant", err)
	}
	if len(ov.asked) != 0 {
		t.Error("store tidak boleh dipanggil tanpa tenant")
	}
}

// Kontrak tidak menerima tenantId/userId (CLAUDE.md: parameter kontrak tidak boleh membawa tenantId/userId):
// List hanya menerima context.
func TestList_SignatureHasOnlyContext(t *testing.T) {
	m, ok := reflect.TypeOf(&Service{}).MethodByName("List")
	if !ok {
		t.Fatal("List tidak ada")
	}
	if m.Type.NumIn() != 2 || m.Type.In(1) != reflect.TypeFor[context.Context]() {
		t.Errorf("tanda tangan List = %v, want (ctx)", m.Type)
	}
}

func TestList_ETagStableAndChangesWithPermissions(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"), item("alpha", "b", "write"))))
	s := newService(reg, statuses{"acme.alpha": runtime.StatusEnabled}, nil)
	ctxAll := ctxFor(tenantA, allPerms("alpha")...)
	v1 := mustList(t, s, ctxAll)
	v2 := mustList(t, s, ctxAll)
	if v1.ETag == "" || v1.ETag != v2.ETag || v1.Revision != v2.Revision {
		t.Errorf("ETag harus stabil untuk isi sama: %q vs %q", v1.ETag, v2.ETag)
	}
	if !strings.HasPrefix(v1.ETag, `"`) || !strings.HasSuffix(v1.ETag, `"`) || strings.Trim(v1.ETag, `"`) != v1.Revision {
		t.Errorf("ETag %q harus berupa Revision %q yang dikutip", v1.ETag, v1.Revision)
	}
	// izin berubah -> ETag berubah (ETag berbasis isi respons akhir, bukan revisi modul; D-6)
	v3 := mustList(t, s, ctxFor(tenantA, "platform.modules.read", "alpha.items.read"))
	if v3.ETag == v1.ETag {
		t.Error("ETag harus berubah ketika izin berubah")
	}
	// status modul berubah -> ETag berubah
	s2 := newService(reg, statuses{"acme.alpha": runtime.StatusDisabled}, nil)
	if v4 := mustList(t, s2, ctxAll); v4.ETag == v1.ETag {
		t.Error("ETag harus berubah ketika modul dinonaktifkan")
	}
}

// IDOR kontrak: dua prinsipal dengan izin berbeda mendapat respons berbeda; tidak ada kebocoran silang.
func TestList_TwoPrincipalsGetDifferentResponsesWithoutCrossLeak(t *testing.T) {
	reg := buildRegistry(t,
		newModule(t, "alpha", deskWith(item("alpha", "a", "read"), item("alpha", "b", "admin"))),
		newModule(t, "beta", deskWith(item("beta", "c", "read"))),
	)
	st := statuses{"acme.alpha": runtime.StatusEnabled, "acme.beta": runtime.StatusEnabled}
	s := newService(reg, st, nil)
	vA := mustList(t, s, ctxFor(tenantA, "alpha.items.read", "alpha.items.admin"))
	vB := mustList(t, s, ctxFor(tenantA, "beta.items.read", "platform.modules.read"))

	eq(t, ids(vA.Desks), []string{"alpha"})
	eq(t, ids(vB.Desks), []string{"platform", "beta"})
	if vA.ETag == vB.ETag {
		t.Error("ETag dua prinsipal berbeda tidak boleh sama (304 silang)")
	}
	rawA, _ := json.Marshal(vA)
	rawB, _ := json.Marshal(vB)
	for _, leak := range []string{"beta", `"id":"platform"`, "/platform", `"id":"c"`} {
		if strings.Contains(string(rawA), leak) {
			t.Errorf("respons A memuat milik B (%s): %s", leak, rawA)
		}
	}
	for _, leak := range []string{"alpha", `"id":"a"`, `"id":"b"`} {
		if strings.Contains(string(rawB), leak) {
			t.Errorf("respons B memuat milik A (%s): %s", leak, rawB)
		}
	}
}

func TestList_ResultIsNotSharedBetweenCalls(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"))))
	s := newService(reg, statuses{"acme.alpha": runtime.StatusEnabled}, nil)
	ctx := ctxFor(tenantA, allPerms("alpha")...)
	v1 := mustList(t, s, ctx)
	v1.Desks[1].Menus[1].To = "/dimutasi"
	v1.Desks[1].Label.Key = "dimutasi"
	v2 := mustList(t, s, ctx)
	if v2.Desks[1].Menus[1].To != "/alpha/a" || v2.Desks[1].Label.Key != "desk.title" {
		t.Errorf("mutasi hasil memengaruhi pemanggilan berikutnya: %+v", v2.Desks[1])
	}
}

func TestNew_NilDepsFailClosed(t *testing.T) {
	reg := buildRegistry(t)
	s := New(Deps{Registry: reg, Status: statuses{}}) // tanpa Authz: tolak semua
	v, err := s.List(ctxFor(tenantA, allPerms()...))
	if err != nil || len(v.Desks) != 0 {
		t.Fatalf("tanpa Authorizer: desks=%v err=%v, want kosong", ids(v.Desks), err)
	}
}

func TestDeskMessageRefAndTypesJSON(t *testing.T) {
	m := Menu{Type: TypeItem, ID: "dashboard", Label: MessageRef{NS: "platform", Key: "desk.dashboard"}, To: "/x", Icon: "layout-dashboard", Locked: true, OwnerDesk: "x"}
	raw, _ := json.Marshal(m)
	want := `{"type":"item","id":"dashboard","label":{"ns":"platform","key":"desk.dashboard"},"to":"/x","icon":"layout-dashboard","locked":true,"ownerDesk":"x"}`
	if string(raw) != want {
		t.Errorf("JSON = %s\nwant   %s", raw, want)
	}
}
