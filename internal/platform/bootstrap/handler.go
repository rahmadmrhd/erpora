// Package bootstrap membentuk manifest bootstrap yang dikonsumsi frontend (GET /api/bootstrap).
package bootstrap

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

// Snapshot membangun manifest M1: tanpa modul/permission, revision 0.
func Snapshot(appVersion, tenantID string) Manifest {
	return Manifest{
		AppVersion: appVersion, TenantID: tenantID,
		Modules: []Module{}, Permissions: []string{}, Revision: 0,
	}
}
