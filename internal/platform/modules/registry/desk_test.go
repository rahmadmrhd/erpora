package registry

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/sdk/module"
)

// deskFS adalah katalog id.json yang memuat kunci label yang dipakai test.
func deskFS() fstest.MapFS {
	return fstest.MapFS{"id.json": {Data: []byte(`{"desk":{"title":"Catatan","dashboard":"Beranda"},"nav":{"notes":"Catatan","archive":"Arsip","group":"Grup"}}`)}}
}

func deskItem(id, to, perm string) map[string]any {
	return map[string]any{"id": id, "label": "nav.notes", "to": to, "permission": perm}
}

func deskSpec(menus ...any) map[string]any {
	return map[string]any{"label": "desk.title", "icon": "file-text", "menus": menus}
}

func deskModule(desk any, fsys fstest.MapFS) module.Module {
	base := newFake(manifestSpec{key: "acme.notes", slug: "notes", caps: []string{"notes.items.read", "notes.items.write"},
		i18n: i18nSpec("id"), desk: desk}, nil)
	return catalogFake{fakeModule: base, fsys: fsys}
}

func problemCodes(t *testing.T, err error, mod string) []string {
	t.Helper()
	var codes []string
	for _, p := range problemsOf(t, err) {
		if p.Module == mod {
			codes = append(codes, p.Code)
		}
	}
	return codes
}

func TestBuild_DeskValidAndExposedOnEntry(t *testing.T) {
	desk := deskSpec(
		deskItem("notes", "/notes", "notes.items.read"),
		map[string]any{"id": "group", "label": "nav.group", "children": []any{deskItem("archive", "/archive", "notes.items.write")}},
	)
	reg, err := Build(testOpts(), deskModule(desk, deskFS()))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	e, _ := reg.Entry("acme.notes")
	if e.Manifest.Desk == nil || len(e.Manifest.Desk.Menus) != 2 || e.Manifest.Desk.Menus[1].Children[0].ID != "archive" {
		t.Errorf("Desk = %+v", e.Manifest.Desk)
	}
}

func TestBuild_DeskProblems(t *testing.T) {
	cases := map[string]struct {
		desk any
		fsys fstest.MapFS
		code string
	}{
		"id ganda antar item": {
			deskSpec(deskItem("notes", "/a", "notes.items.read"), deskItem("notes", "/b", "notes.items.read")), deskFS(), "desk_duplicate_id"},
		"id grup sama dengan item": {
			deskSpec(deskItem("x", "/a", "notes.items.read"),
				map[string]any{"id": "x", "label": "nav.group", "children": []any{deskItem("y", "/b", "notes.items.read")}}), deskFS(), "desk_duplicate_id"},
		"id anak sama dengan item": {
			deskSpec(deskItem("x", "/a", "notes.items.read"),
				map[string]any{"id": "g", "label": "nav.group", "children": []any{deskItem("x", "/b", "notes.items.read")}}), deskFS(), "desk_duplicate_id"},
		"permission di luar capabilities": {
			deskSpec(deskItem("notes", "/a", "notes.items.delete")), deskFS(), "desk_unknown_permission"},
		"permission milik modul lain": {
			deskSpec(deskItem("notes", "/a", "platform.modules.read")), deskFS(), "desk_unknown_permission"},
		"permission anak grup di luar capabilities": {
			deskSpec(map[string]any{"id": "g", "label": "nav.group", "children": []any{deskItem("y", "/b", "notes.items.delete")}}), deskFS(), "desk_unknown_permission"},
		"to dengan segmen titik": {
			deskSpec(deskItem("notes", "/a/../b", "notes.items.read")), deskFS(), "desk_invalid_to"},
		"to dengan segmen satu titik": {
			deskSpec(deskItem("notes", "/./b", "notes.items.read")), deskFS(), "desk_invalid_to"},
		"to dengan persen": {
			deskSpec(deskItem("notes", "/a%2Fb", "notes.items.read")), deskFS(), "desk_invalid_to"},
		"label menu tidak ada di katalog": {
			deskSpec(map[string]any{"id": "notes", "label": "nav.tidakAda", "to": "/a", "permission": "notes.items.read"}), deskFS(), "desk_label_unknown"},
		"label desk tidak ada di katalog": {
			map[string]any{"label": "desk.tidakAda", "icon": "file-text", "menus": []any{}}, deskFS(), "desk_label_unknown"},
		"label dashboard tidak ada di katalog": {
			map[string]any{"label": "desk.title", "icon": "file-text", "dashboard": map[string]any{"label": "desk.tidakAda"}, "menus": []any{}}, deskFS(), "desk_label_unknown"},
		"label grup tidak ada di katalog": {
			deskSpec(map[string]any{"id": "g", "label": "nav.tidakAda", "children": []any{deskItem("y", "/b", "notes.items.read")}}), deskFS(), "desk_label_unknown"},
		"kunci label hanya awalan katalog": {
			deskSpec(map[string]any{"id": "notes", "label": "nav", "to": "/a", "permission": "notes.items.read"}), deskFS(), "desk_label_unknown"},
		"id dashboard dilarang": {
			deskSpec(deskItem("dashboard", "/a", "notes.items.read")), deskFS(), "desk_dashboard_locked"},
		"to indeks dilarang": {
			deskSpec(deskItem("notes", "/", "notes.items.read")), deskFS(), "desk_dashboard_locked"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Build(testOpts(), deskModule(c.desk, c.fsys))
			if codes := problemCodes(t, err, "acme.notes"); !slices.Contains(codes, c.code) {
				t.Errorf("kode problem = %v, want %s", codes, c.code)
			}
		})
	}
}

func TestBuild_DeskProblemCarriesFieldPointer(t *testing.T) {
	desk := deskSpec(deskItem("a", "/a", "notes.items.read"),
		map[string]any{"id": "g", "label": "nav.group", "children": []any{deskItem("b", "/b", "notes.items.delete")}})
	_, err := Build(testOpts(), deskModule(desk, deskFS()))
	p := expect(t, problemsOf(t, err), "desk_unknown_permission", "acme.notes")
	if p.Field != "/desk/menus/1/children/0/permission" {
		t.Errorf("Field = %q", p.Field)
	}
}

func TestBuild_DeskWithoutCatalogReportsLabelUnknown(t *testing.T) {
	// Modul dengan desk tetapi tanpa katalog: label tidak mungkin terpenuhi.
	m := newFake(manifestSpec{key: "acme.notes", slug: "notes", caps: []string{"notes.items.read"},
		desk: deskSpec(deskItem("notes", "/a", "notes.items.read"))}, nil)
	_, err := Build(testOpts(), m)
	if codes := problemCodes(t, err, "acme.notes"); !slices.Contains(codes, "desk_label_unknown") {
		t.Errorf("kode problem = %v", codes)
	}
}

func TestBuild_DeskAbsentIsFine(t *testing.T) {
	if _, err := Build(testOpts(), notes(nil)); err != nil {
		t.Fatalf("modul tanpa desk harus valid: %v", err)
	}
}

func TestCheckDesk_NilDeskNoProblems(t *testing.T) {
	if ps := CheckDesk(module.Manifest{Key: "acme.x"}, nil); len(ps) != 0 {
		t.Errorf("problems = %+v", ps)
	}
}

func TestBuild_PlatformDeskValidated(t *testing.T) {
	opts := testOpts()
	opts.PlatformCapabilities = []string{"platform.modules.read"}
	opts.PlatformDesk = &module.Desk{Label: "desk.title", Icon: "layers", Menus: []module.DeskMenu{
		{ID: "modules", Label: "desk.modules", To: "/modules", Permission: "platform.modules.read"},
	}}
	reg, err := Build(opts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if pe, _ := reg.Entry(PlatformKey); pe.Manifest.Desk == nil || pe.Manifest.Desk.Menus[0].ID != "modules" {
		t.Errorf("Desk platform tidak tersimpan: %+v", pe.Manifest.Desk)
	}

	opts.PlatformDesk = &module.Desk{Label: "desk.title", Icon: "layers", Menus: []module.DeskMenu{
		{ID: "modules", Label: "desk.modules", To: "/modules", Permission: "platform.tidak-ada.read"},
	}}
	_, err = Build(opts)
	if codes := problemCodes(t, err, PlatformKey); !slices.Contains(codes, "desk_unknown_permission") {
		t.Errorf("kode problem = %v", codes)
	}
}

func TestBuild_DeskProblemsAreSortedInErrorText(t *testing.T) {
	_, err := Build(testOpts(), deskModule(deskSpec(deskItem("notes", "/a", "notes.items.delete")), deskFS()))
	if err == nil || !strings.Contains(err.Error(), "[desk_unknown_permission]") {
		t.Fatalf("err = %v", err)
	}
}
