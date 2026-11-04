package gen

import (
	"slices"
	"strings"
	"testing"

	"erpora/sdk/module"
)

const editManifest = `{
  "schema_version": "1",
  "key": "acme.inventory",
  "slug": "inventory",
  "capabilities": [],
  "resources": [
    {
      "key": "inventory.items",
      "label": "resource.items",
      "actions": ["read"],
      "field_groups": [{ "key": "general", "label": "fieldGroup.general", "classification": "internal", "default": true }]
    }
  ],
  "offline": { "mode": "online-only" }
}
`

func TestEnsureManifestPermission(t *testing.T) {
	t.Run("sudah turunan: tidak berubah", func(t *testing.T) {
		out, nr, err := EnsureManifestPermission([]byte(editManifest), "inventory.items.read")
		if err != nil || nr != "" || string(out) != editManifest {
			t.Fatalf("out=%q nr=%q err=%v", out, nr, err)
		}
	})
	t.Run("aksi standar baru pada resource yang ada", func(t *testing.T) {
		out, nr, err := EnsureManifestPermission([]byte(editManifest), "inventory.items.approve")
		if err != nil || nr != "" {
			t.Fatalf("nr=%q err=%v", nr, err)
		}
		if !strings.Contains(string(out), `"actions": ["read", "approve"],`) {
			t.Errorf("actions tidak bertambah:\n%s", out)
		}
		m, err := module.ParseManifest(out)
		if err != nil || !slices.Contains(m.Permissions(), "inventory.items.approve") {
			t.Fatalf("permission tidak turunan: %v %v", m.Permissions(), err)
		}
	})
	t.Run("resource baru", func(t *testing.T) {
		out, nr, err := EnsureManifestPermission([]byte(editManifest), "inventory.counts.read")
		if err != nil || nr != "inventory.counts" {
			t.Fatalf("nr=%q err=%v", nr, err)
		}
		m, err := module.ParseManifest(out)
		if err != nil {
			t.Fatalf("hasil bukan JSON valid: %v\n%s", err, out)
		}
		if len(m.Resources) != 2 || m.Resources[1].Key != "inventory.counts" || m.Resources[1].Label != "resource.counts" ||
			!m.Resources[1].FieldGroups[0].Default || !slices.Contains(m.Permissions(), "inventory.counts.read") {
			t.Fatalf("resources = %+v", m.Resources)
		}
		if !strings.Contains(string(out), "  ],\n  \"offline\"") {
			t.Errorf("format penutup resources berubah:\n%s", out)
		}
	})
	t.Run("resources kosong", func(t *testing.T) {
		src := strings.Replace(editManifest, editManifest[strings.Index(editManifest, "\"resources\""):strings.Index(editManifest, "\"offline\"")], "\"resources\": [],\n  ", 1)
		out, nr, err := EnsureManifestPermission([]byte(src), "inventory.items.read")
		if err != nil || nr != "inventory.items" {
			t.Fatalf("nr=%q err=%v\n%s", nr, err, out)
		}
		if _, err := module.ParseManifest(out); err != nil {
			t.Fatalf("hasil bukan JSON valid: %v\n%s", err, out)
		}
	})
	t.Run("aksi non-standar ditolak dengan petunjuk", func(t *testing.T) {
		_, _, err := EnsureManifestPermission([]byte(editManifest), "inventory.items.adjust_price")
		if err == nil || !strings.Contains(err.Error(), "custom_actions") {
			t.Fatalf("err = %v", err)
		}
	})
}
