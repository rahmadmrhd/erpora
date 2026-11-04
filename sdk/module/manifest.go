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
	// Desk menyatakan Desk modul (Q40 butir 10-13): label, ikon, dan menu yang dibaca server untuk daftar Desk
	// per pengguna. nil = modul tanpa Desk (modul tanpa antarmuka web); module:check mewajibkannya bagi modul
	// berdirektori web/.
	Desk *Desk `json:"desk,omitempty"`
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

// Desk adalah Desk satu modul. Label, Icon, dan Label menu berupa kunci i18n / nama ikon; teks diterjemahkan di
// klien dari katalog namespace modul. Dashboard (indeks /<slug>) dibangkitkan platform dan tidak dideklarasikan
// sebagai menu.
type Desk struct {
	Label     string         `json:"label"`
	Icon      string         `json:"icon"`
	Dashboard *DeskDashboard `json:"dashboard,omitempty"`
	Menus     []DeskMenu     `json:"menus"`
	Settings  map[string]any `json:"settings,omitempty"`
}

// DeskDashboard menimpa label dan ikon item Dashboard bawaan; kosong = bawaan platform.
type DeskDashboard struct {
	Label string `json:"label,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// DeskAccessAuthenticated adalah nilai DeskMenu.Access: item terlihat bagi setiap pengguna yang login, tanpa
// permission (PRD-000 Q43).
const DeskAccessAuthenticated = "authenticated"

// DeskMenu adalah satu item menu (To terisi, plus TEPAT SATU dari Permission atau Access) atau satu grup satu
// tingkat (Children terisi). To relatif base route modul (mis. "/notes"); Permission wajib salah satu capabilities
// modul; Access hanya boleh DeskAccessAuthenticated (pengganti eksplisit Permission, PRD-000 Q43).
type DeskMenu struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Icon       string     `json:"icon,omitempty"`
	To         string     `json:"to,omitempty"`
	Permission string     `json:"permission,omitempty"`
	Access     string     `json:"access,omitempty"`
	Children   []DeskMenu `json:"children,omitempty"`
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
