package desks

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	"erpora/sdk/module"
)

// Permission API Desk. platform.desks.update dipakai API tulis Desk (M3b): Desk bawaan dan Desk bersama hanya oleh
// pemegang hak itu atas Desk yang bersangkutan, Desk pribadi oleh pemilik (D-4; nama final menurut aturan
// kapabilitas "<slug>.<resource>.<aksi>" registry).
const (
	PermRead   = "platform.desks.read"
	PermUpdate = "platform.desks.update"
)

// Capabilities adalah capability milik platform.core untuk Desk (ditambahkan ke daftar capability platform di
// composition root).
var Capabilities = []string{PermRead, PermUpdate}

// Backend dipenuhi *Service; fake pada test. List tidak menerima tenant atau pengguna: keduanya dari context.
type Backend interface {
	List(ctx context.Context) (View, error)
}

// API adalah handler /api/platform/desks. Backend di-bind setelah perakitan (route didaftarkan lebih dulu lewat
// registry); sebelum itu endpoint membalas 503 not_ready, seperti API manajemen modul dan katalog i18n.
type API struct {
	backend atomic.Pointer[Backend]
}

// New membuat API tanpa backend.
func New() *API { return &API{} }

// Bind memasang backend; dipanggil composition root.
func (a *API) Bind(b Backend) { a.backend.Store(&b) }

// Register mendaftarkan endpoint (path relatif platform.core). Bukan route publik: daftar Desk bergantung pada
// pengguna. Endpoint tidak menerima id apa pun, sehingga tidak ada titik masuk IDOR; bila endpoint per-Desk
// ditambah kelak, wajib authz per record dan tes IDOR pengguna A vs B.
func (a *API) Register(r module.Registrar) error {
	r.Handle(http.MethodGet, "/desks", PermRead, http.HandlerFunc(a.list), module.Classify(module.ClassInternal))
	return nil
}

// listData adalah isi `data` respons daftar Desk.
type listData struct {
	Desks    []Desk `json:"desks"`
	Revision string `json:"revision"`
}

// list melayani GET /api/platform/desks. ETag = hash isi respons akhir; Cache-Control private + Vary Cookie dan
// Authorization mencegah cache perantara menyajikan daftar satu pengguna (atau 304) ke pengguna lain (D-6).
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	bp := a.backend.Load()
	if bp == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
		return
	}
	if _, ok := tenancy.FromContext(r.Context()); !ok {
		httpx.WriteError(w, r, http.StatusBadRequest, "tenant_unresolved", "tenant tidak dapat ditentukan", nil)
		return
	}
	view, err := (*bp).List(r.Context())
	if err != nil {
		if errors.Is(err, ErrNoTenant) {
			httpx.WriteError(w, r, http.StatusBadRequest, "tenant_unresolved", "tenant tidak dapat ditentukan", nil)
			return
		}
		httpx.RespondError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "private, no-cache")
	h.Add("Vary", "Cookie")
	h.Add("Vary", "Authorization")
	if httpserver.CheckETag(w, r, view.ETag) {
		return
	}
	desks := view.Desks
	if desks == nil {
		desks = []Desk{}
	}
	httpx.WriteData(w, http.StatusOK, listData{Desks: desks, Revision: view.Revision})
}
