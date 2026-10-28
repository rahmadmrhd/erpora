// Package bootstrap membentuk manifest bootstrap yang dikonsumsi frontend (GET /api/bootstrap).
package bootstrap

import "erpora/internal/platform/tenancy"

// Manifest adalah isi `data` dari GET /api/bootstrap. Daftar modul diisi dari runtime snapshot (M2).
type Manifest struct {
	AppVersion  string   `json:"appVersion"`
	TenantID    string   `json:"tenantId"`
	Modules     []Module `json:"modules"`
	Permissions []string `json:"permissions"`
	Revision    int64    `json:"revision"`
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
