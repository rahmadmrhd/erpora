package module

import (
	"bytes"
	"testing"
)

func TestParseManifest_FillsFieldsAndRaw(t *testing.T) {
	input := []byte(`{
  "schema_version": "1",
  "key": "acme.example",
  "slug": "example",
  "name": "Example",
  "version": "0.1.0",
  "trust": "builtin",
  "dependencies": [{ "module": "platform.core", "version": "^0.1", "optional": false }]
}`)
	m, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Key != "acme.example" {
		t.Errorf("Key = %q", m.Key)
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0].Module != "platform.core" {
		t.Errorf("Dependencies = %+v", m.Dependencies)
	}
	if !bytes.Equal(m.Raw, input) {
		t.Errorf("Raw tidak sama dengan input")
	}
}

func TestParseManifest_InvalidJSON(t *testing.T) {
	if _, err := ParseManifest([]byte(`{`)); err == nil {
		t.Fatal("error diharapkan untuk JSON tidak valid")
	}
}

func TestParseManifest_Desk(t *testing.T) {
	input := []byte(`{
  "schema_version": "1", "key": "acme.example", "slug": "example", "name": "Example", "version": "0.1.0",
  "desk": {
    "label": "desk.title", "icon": "file-text",
    "dashboard": {"label": "desk.dashboard", "icon": "gauge"},
    "menus": [
      {"id": "notes", "label": "nav.notes", "to": "/notes", "permission": "example.notes.read"},
      {"id": "more", "label": "nav.more", "icon": "folder", "children": [
        {"id": "archive", "label": "nav.archive", "icon": "archive", "to": "/archive", "permission": "example.notes.read"}
      ]}
    ],
    "settings": {"x": 1}
  }
}`)
	m, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	d := m.Desk
	if d == nil || d.Label != "desk.title" || d.Icon != "file-text" {
		t.Fatalf("Desk = %+v", d)
	}
	if d.Dashboard == nil || d.Dashboard.Label != "desk.dashboard" || d.Dashboard.Icon != "gauge" {
		t.Errorf("Dashboard = %+v", d.Dashboard)
	}
	if len(d.Menus) != 2 || d.Menus[0].To != "/notes" || d.Menus[0].Permission != "example.notes.read" {
		t.Fatalf("Menus = %+v", d.Menus)
	}
	if g := d.Menus[1]; g.ID != "more" || len(g.Children) != 1 || g.Children[0].To != "/archive" || g.Children[0].Icon != "archive" {
		t.Errorf("grup = %+v", g)
	}
	if d.Settings["x"] != float64(1) {
		t.Errorf("Settings = %+v", d.Settings)
	}
}

func TestParseManifest_WithoutDeskIsNil(t *testing.T) {
	m, err := ParseManifest([]byte(`{"schema_version":"1","key":"acme.example","slug":"example"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Desk != nil {
		t.Errorf("Desk = %+v, want nil", m.Desk)
	}
}
