package bootstrap

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/tenancy"
)

func TestBuild_ModulesBasePathOfflineAndEmptyPermissions(t *testing.T) {
	tid := tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	m := Build("1.2.3", tid, []ModuleInput{
		{Key: "acme.example", Slug: "example", Version: "1.0.0", Status: "enabled", OfflineMode: "online-only"},
		{Key: "acme.notes", Slug: "notes", Version: "2.0.0", Status: "available", OfflineMode: "offline-read"},
	}, 7)
	if m.AppVersion != "1.2.3" || m.TenantID != tid.String() || m.Revision != 7 || len(m.Modules) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
	a, b := m.Modules[0], m.Modules[1]
	if a.BasePath != "/example" || a.Offline || a.Status != "enabled" || a.Version != "1.0.0" {
		t.Errorf("modul 0 = %+v", a)
	}
	if b.BasePath != "/notes" || !b.Offline {
		t.Errorf("modul 1 = %+v", b)
	}
	raw, _ := json.Marshal(m)
	if !strings.Contains(string(raw), `"permissions":[]`) {
		t.Errorf("permissions harus [] : %s", raw)
	}
}

func TestBuild_NilModulesAreEmptyArrays(t *testing.T) {
	raw, _ := json.Marshal(Build("v", tenancy.TenantID{}, nil, 0))
	if !strings.Contains(string(raw), `"modules":[]`) || !strings.Contains(string(raw), `"permissions":[]`) {
		t.Errorf("json = %s", raw)
	}
}

func TestManifest_I18nOmittedWhenNil(t *testing.T) {
	raw, _ := json.Marshal(Build("v", tenancy.TenantID{}, nil, 3))
	if strings.Contains(string(raw), "i18n") {
		t.Errorf("i18n harus hilang saat nil (additive): %s", raw)
	}
}

func TestManifest_I18nSerialization(t *testing.T) {
	m := Build("v", tenancy.TenantID{}, nil, 3)
	m.I18n = NewI18n("id", []I18nLocale{{Code: "id", Name: "Bahasa Indonesia"}, {Code: "en", Name: "English"}}, map[string]string{"ui": "r1", "platform": "r2"})
	raw, _ := json.Marshal(m)
	for _, want := range []string{`"i18n":{`, `"defaultLocale":"id"`, `"locales":[{"code":"id","name":"Bahasa Indonesia"},{"code":"en","name":"English"}]`, `"revisions":{"platform":"r2","ui":"r1"}`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("json tidak memuat %s: %s", want, raw)
		}
	}
	empty := NewI18n("id", nil, nil)
	raw, _ = json.Marshal(empty)
	if !strings.Contains(string(raw), `"locales":[]`) || !strings.Contains(string(raw), `"revisions":{}`) {
		t.Errorf("locales dan revisions tidak boleh null: %s", raw)
	}
}

func TestManifest_ETagFollowsRevisionAndCatalogRevisions(t *testing.T) {
	base := Build("v", tenancy.TenantID{}, nil, 7)
	if got := base.ETag(); got != `"7"` {
		t.Errorf("tanpa i18n ETag = %s, want \"7\" (format lama tidak berubah)", got)
	}
	a := base
	a.I18n = NewI18n("id", nil, map[string]string{"ui": "r1"})
	b := base
	b.I18n = NewI18n("id", nil, map[string]string{"ui": "r2"})
	c := base
	c.I18n = NewI18n("id", nil, map[string]string{"ui": "r1"})
	if a.ETag() == b.ETag() {
		t.Error("revisi katalog berubah, ETag harus berubah")
	}
	if a.ETag() != c.ETag() {
		t.Error("isi sama, ETag harus sama (deterministik walau urutan map acak)")
	}
	if a.ETag() == base.ETag() {
		t.Error("ETag dengan i18n harus berbeda dari tanpa i18n")
	}
	other := a
	other.Revision = 8
	if other.ETag() == a.ETag() {
		t.Error("revision snapshot berubah, ETag harus berubah")
	}
}

func TestManifest_FeaturesOmittedWhenNilAndSerialized(t *testing.T) {
	raw, _ := json.Marshal(Build("v", tenancy.TenantID{}, nil, 1))
	if strings.Contains(string(raw), "features") {
		t.Errorf("features harus hilang saat nil (additive): %s", raw)
	}
	for _, on := range []bool{true, false} {
		m := Build("v", tenancy.TenantID{}, nil, 1)
		m.Features = &Features{ModulesAdminAPI: on}
		raw, _ := json.Marshal(m)
		want := `"features":{"modulesAdminApi":` + strconv.FormatBool(on) + `}`
		if !strings.Contains(string(raw), want) {
			t.Errorf("json tidak memuat %s: %s", want, raw)
		}
	}
}

func TestManifest_FeaturesDoNotChangeETag(t *testing.T) {
	a := Build("v", tenancy.TenantID{}, nil, 4)
	b := a
	b.Features = &Features{ModulesAdminAPI: true}
	if a.ETag() != b.ETag() {
		t.Error("features statis per proses; tidak boleh mengubah ETag")
	}
}

func TestManifest_UserAndPermRevSerialization(t *testing.T) {
	m := Build("1.0.0", tenancy.TenantID(uuid.New()), nil, 3)
	if b, _ := json.Marshal(m); strings.Contains(string(b), `"user"`) || !strings.Contains(string(b), `"permRev":0`) {
		t.Errorf("tanpa user: user dihilangkan, permRev tetap ada: %s", b)
	}
	m.User = &User{ID: "u-1", Email: "a@example.test", DisplayName: "Ani"}
	m.PermRev = 4
	b, _ := json.Marshal(m)
	for _, w := range []string{`"user":{"id":"u-1","email":"a@example.test","displayName":"Ani"}`, `"permRev":4`} {
		if !strings.Contains(string(b), w) {
			t.Errorf("json %s tidak memuat %s", b, w)
		}
	}
}

// ETag bervariasi per pengguna dan per revisi permission; format lama tetap untuk manifest tanpa user.
func TestManifest_ETagVariesPerUserAndPermRev(t *testing.T) {
	base := Build("1.0.0", tenancy.TenantID(uuid.New()), nil, 7)
	a, b, c := base, base, base
	a.User = &User{ID: "u-1", Email: "a@example.test", DisplayName: "Ani"}
	b.User = &User{ID: "u-2", Email: "b@example.test", DisplayName: "Budi"}
	c.User = &User{ID: "u-1", Email: "a@example.test", DisplayName: "Ani"}
	if a.ETag() == b.ETag() {
		t.Error("dua pengguna berbeda tidak boleh berbagi ETag")
	}
	if a.ETag() != c.ETag() || a.ETag() == base.ETag() {
		t.Error("pengguna sama harus ETag sama (deterministik) dan berbeda dari tanpa user")
	}
	c.User.DisplayName = "Ani Baru"
	if a.ETag() == c.ETag() {
		t.Error("nama tampilan berubah, ETag harus berubah")
	}
	d := a
	d.PermRev = 1
	if a.ETag() == d.ETag() {
		t.Error("permRev berubah, ETag harus berubah")
	}
}
