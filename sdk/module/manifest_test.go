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
