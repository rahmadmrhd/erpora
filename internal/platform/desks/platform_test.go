package desks

import (
	"strings"
	"testing"

	"erpora/internal/platform/i18n/catalog"
	"erpora/internal/platform/modules/admin"
	"erpora/internal/platform/modules/registry"
	sdki18n "erpora/sdk/i18n"
)

// Label Desk platform harus ada di katalog namespace "platform" untuk semua bahasa (registry tidak memeriksa
// label Desk platform; test ini menggantikannya).
func TestPlatformDesk_LabelsExistInPlatformCatalog(t *testing.T) {
	d := PlatformDesk()
	keys := []string{d.Label, DashboardLabelKey}
	for _, m := range d.Menus {
		keys = append(keys, m.Label)
		for _, c := range m.Children {
			keys = append(keys, c.Label)
		}
	}
	for _, loc := range []string{"id", "en"} {
		f, err := catalog.FS().Open(loc + ".json")
		if err != nil {
			t.Fatal(err)
		}
		flat, issues := sdki18n.ParseCatalog(f, sdki18n.DefaultLimits())
		_ = f.Close()
		if len(issues) > 0 {
			t.Fatalf("katalog platform %s: %v", loc, issues)
		}
		for _, k := range keys {
			if _, ok := flat[k]; !ok {
				t.Errorf("kunci %q tidak ada di katalog platform (%s)", k, loc)
			}
		}
	}
}

func TestPlatformDesk_ShapeAndPermissions(t *testing.T) {
	d := PlatformDesk()
	if d.Icon == "" || len(d.Menus) != 1 {
		t.Fatalf("Desk platform = %+v", d)
	}
	m := d.Menus[0]
	if m.ID != "modules" || m.To != "/modules" || m.Permission != "platform.modules.read" {
		t.Errorf("menu = %+v", m)
	}
	have := map[string]bool{}
	for _, c := range admin.Capabilities {
		have[c] = true
	}
	if !have[m.Permission] {
		t.Errorf("permission %q bukan kapabilitas platform (%v)", m.Permission, admin.Capabilities)
	}
	// Dashboard dibangkitkan platform; tidak boleh dideklarasikan.
	if d.Dashboard != nil {
		t.Error("Dashboard dibangkitkan, bukan dideklarasikan")
	}
	// Desk platform lolos pemeriksaan struktur registry (id, to, permission).
	reg := buildRegistry(t)
	e, _ := reg.Entry(registry.PlatformKey)
	if e.Manifest.Desk == nil || !strings.HasPrefix(e.Manifest.Desk.Label, "desk.") {
		t.Errorf("Desk platform tidak terpasang di registry: %+v", e.Manifest.Desk)
	}
}
