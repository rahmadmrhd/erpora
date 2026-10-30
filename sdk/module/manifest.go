package module

import (
	"encoding/json"
	"fmt"
)

// Manifest adalah representasi terketik dari module.json.
type Manifest struct {
	SchemaVersion string        `json:"schema_version"`
	Key           string        `json:"key"`
	Slug          string        `json:"slug"`
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Publisher     Publisher     `json:"publisher"`
	Trust         string        `json:"trust"`
	Required      bool          `json:"required,omitempty"`
	Compatibility Compatibility `json:"compatibility"`
	Dependencies  []Dependency  `json:"dependencies"`
	Capabilities  []string      `json:"capabilities"`
	Provides      []string      `json:"provides,omitempty"`
	Migrations    Migrations    `json:"migrations"`
	Offline       Offline       `json:"offline"`
	// I18n menyatakan bahasa katalog terjemahan modul; nil = modul tanpa namespace i18n.
	I18n *I18n `json:"i18n,omitempty"`
	// Raw adalah byte module.json asli; divalidasi platform terhadap schema.
	Raw []byte `json:"-"`
}

// Publisher mengidentifikasi penerbit modul.
type Publisher struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Compatibility memuat rentang versi platform dan SDK yang didukung (constraint SemVer).
type Compatibility struct {
	Platform string `json:"platform"`
	SDK      string `json:"sdk"`
}

// Dependency adalah dependensi modul lain beserta constraint versinya.
type Dependency struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Optional bool   `json:"optional"`
}

// Migrations menyatakan strategi migrasi modul: "versioned" atau "none".
type Migrations struct {
	Strategy string `json:"strategy"`
}

// Offline menyatakan mode offline modul: online-only, offline-read, offline-draft, atau offline-write.
type Offline struct {
	Mode string `json:"mode"`
}

// I18n menyatakan katalog terjemahan modul (PRD-000 FR-25): bahasa yang disediakan dan bahasa dasar
// kunci (sumber tipe kunci terketik). Setiap bahasa wajib terdaftar di registri bahasa platform.
type I18n struct {
	Locales       []string `json:"locales"`
	DefaultLocale string   `json:"defaultLocale"`
}

// ParseManifest mengurai module.json dan menyalin byte aslinya ke Raw.
func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("module: manifest tidak valid: %w", err)
	}
	m.Raw = append([]byte(nil), b...)
	return m, nil
}

// MustParseManifest seperti ParseManifest tetapi panic bila gagal; dipakai modul
// dengan //go:embed module.json.
func MustParseManifest(b []byte) Manifest {
	m, err := ParseManifest(b)
	if err != nil {
		panic(err)
	}
	return m
}
