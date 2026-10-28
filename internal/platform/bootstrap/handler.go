// Package bootstrap membentuk manifest bootstrap yang dikonsumsi frontend (GET /api/bootstrap).
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"erpora/internal/platform/tenancy"
)

// Manifest adalah isi `data` dari GET /api/bootstrap. Daftar modul diisi dari runtime snapshot (M2).
type Manifest struct {
	AppVersion  string   `json:"appVersion"`
	TenantID    string   `json:"tenantId"`
	Modules     []Module `json:"modules"`
	Permissions []string `json:"permissions"`
	Revision    int64    `json:"revision"`
	// I18n adalah bahasa yang didukung dan revisi katalog per namespace aktif (PRD-000 FR-25); nil = tanpa i18n
	// (bidang dihilangkan; perubahan bersifat additive, api-guidelines §14.1).
	I18n *I18n `json:"i18n,omitempty"`
}

// I18n adalah bagian i18n manifest bootstrap. Frontend memuat ulang katalog sebuah namespace bila revisinya berubah.
type I18n struct {
	DefaultLocale string            `json:"defaultLocale"`
	Locales       []I18nLocale      `json:"locales"`
	Revisions     map[string]string `json:"revisions"`
}

// I18nLocale adalah satu bahasa terdaftar.
type I18nLocale struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// NewI18n membentuk bagian i18n; locales dan revisions tidak pernah null di JSON.
func NewI18n(defaultLocale string, locales []I18nLocale, revisions map[string]string) *I18n {
	if locales == nil {
		locales = []I18nLocale{}
	}
	if revisions == nil {
		revisions = map[string]string{}
	}
	return &I18n{DefaultLocale: defaultLocale, Locales: locales, Revisions: revisions}
}

// ETag mengembalikan ETag bootstrap: `"<revision>"` bila tanpa i18n (format lama), selain itu
// `"<revision>-<hash revisi katalog>"` sehingga perubahan katalog terjemahan (mis. override istilah)
// membuat klien memuat ulang bootstrap dan lalu katalognya.
func (m Manifest) ETag() string {
	rev := strconv.FormatInt(m.Revision, 10)
	if m.I18n == nil {
		return `"` + rev + `"`
	}
	keys := make([]string, 0, len(m.I18n.Revisions))
	for k := range m.I18n.Revisions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	fmt.Fprintf(h, "%s;", m.I18n.DefaultLocale)
	for _, l := range m.I18n.Locales {
		fmt.Fprintf(h, "%s=%s;", l.Code, l.Name)
	}
	for _, k := range keys {
		fmt.Fprintf(h, "%s:%s;", k, m.I18n.Revisions[k])
	}
	return `"` + rev + "-" + hex.EncodeToString(h.Sum(nil)[:6]) + `"`
}

// Module adalah entri modul pada bootstrap (FR-19).
type Module struct {
	Key      string `json:"key"`
	Slug     string `json:"slug"`
	Version  string `json:"version"`
	Status   string `json:"status"`
	BasePath string `json:"basePath"`
	Offline  bool   `json:"offline"`
}

// ModuleInput adalah data satu modul untuk Build (diisi composition root dari registry dan runtime snapshot).
type ModuleInput struct{ Key, Slug, Version, Status, OfflineMode string }

// Build membangun manifest bootstrap. offline = OfflineMode bukan "online-only"; basePath = "/" + slug.
// Modules dan Permissions tidak pernah nil (JSON selalu array). Permissions diisi PRD-001.
func Build(appVersion string, t tenancy.TenantID, mods []ModuleInput, revision int64) Manifest {
	out := make([]Module, 0, len(mods))
	for _, m := range mods {
		out = append(out, Module{
			Key: m.Key, Slug: m.Slug, Version: m.Version, Status: m.Status,
			BasePath: "/" + m.Slug, Offline: m.OfflineMode != "online-only",
		})
	}
	return Manifest{AppVersion: appVersion, TenantID: t.String(), Modules: out, Permissions: []string{}, Revision: revision}
}
