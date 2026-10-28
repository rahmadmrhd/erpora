// Package admin memuat endpoint HTTP i18n milik platform.core (PRD-000 FR-25): penyaji katalog terjemahan
// per namespace dan bahasa. Override istilah (tahap I5) ditambahkan di paket ini.
package admin

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
)

// Backend dipenuhi *i18n.Service; fake pada test.
type Backend interface {
	Catalog(ctx context.Context, t tenancy.TenantID, ns string, loc sdki18n.Locale) (i18n.View, error)
}

// API adalah handler /api/platform/i18n. Backend di-bind setelah perakitan (route didaftarkan lebih dulu
// lewat registry); sebelum itu endpoint membalas 503 not_ready, seperti API manajemen modul.
type API struct {
	backend atomic.Pointer[Backend]
}

// New membuat API tanpa backend.
func New() *API { return &API{} }

// Bind memasang backend; dipanggil composition root.
func (a *API) Bind(b Backend) { a.backend.Store(&b) }

// Register mendaftarkan endpoint (path relatif platform.core). Katalog publik pada fase ini (hanya teks
// terjemahan, keputusan D-4); PRD-001 mengetatkan namespace modul ke pengguna terautentikasi.
func (a *API) Register(r module.Registrar) error {
	r.Public(http.MethodGet, "/i18n/{locale}/{namespace}", http.HandlerFunc(a.catalog))
	return nil
}

// catalogData adalah isi `data` respons katalog.
type catalogData struct {
	Locale    string            `json:"locale"`
	Namespace string            `json:"namespace"`
	Revision  string            `json:"revision"`
	Messages  map[string]string `json:"messages"`
}

// catalog melayani GET /api/platform/i18n/{locale}/{namespace}. locale divalidasi terhadap registri dan
// namespace terhadap daftar tertutup (ui, platform, modul aktif) di dalam Service; keduanya tidak pernah
// dipakai sebagai path berkas (Requirement 10.2). Penyebab 404 sengaja tidak dibedakan (Requirement 5.2).
func (a *API) catalog(w http.ResponseWriter, r *http.Request) {
	bp := a.backend.Load()
	if bp == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
		return
	}
	t, ok := tenancy.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusBadRequest, "tenant_unresolved", "tenant tidak dapat ditentukan", nil)
		return
	}
	view, err := (*bp).Catalog(r.Context(), t, r.PathValue("namespace"), sdki18n.Locale(r.PathValue("locale")))
	if err != nil {
		if errors.Is(err, i18n.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "katalog tidak ditemukan", nil)
			return
		}
		httpx.RespondError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	if httpserver.CheckETag(w, r, view.ETag) {
		return
	}
	httpx.WriteData(w, http.StatusOK, catalogData{
		Locale: view.Locale, Namespace: view.Namespace, Revision: view.Revision, Messages: view.Messages,
	})
}
