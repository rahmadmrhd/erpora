package bootstrap

import (
	"encoding/json"
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
