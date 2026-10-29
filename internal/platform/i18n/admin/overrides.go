package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	"erpora/sdk/module"
)

// Capabilities adalah capability milik platform.core untuk override istilah (ditambahkan ke daftar capability
// platform di composition root).
var Capabilities = []string{PermRead, PermManage}

// Permission API override.
const (
	PermRead   = "platform.i18n.read"
	PermManage = "platform.i18n.manage"
)

// Actor adalah actor audit untuk operasi lewat API sebelum PRD-001 (autentikasi) tersedia.
const Actor = "api:dev"

const (
	overridesPrefix = "/api/platform/i18n/overrides/"
	maxBodyBytes    = 8 << 10
	defaultPageSize = 25
	maxPageSize     = 200
)

// IsMutation menandai route mutasi override (PUT dan DELETE); dipakai runtime.SkipRoute agar gerbang
// ERP_ENV/ERP_MODULES_ADMIN_API menjawab 404 sebelum permission (PRD-000 FR-20.2, Requirement 12.8).
func IsMutation(rt registry.RouteInfo) bool {
	return rt.Module == registry.PlatformKey && (rt.Method == http.MethodPut || rt.Method == http.MethodDelete) &&
		strings.HasPrefix(rt.Pattern, overridesPrefix)
}

// registerOverrides mendaftarkan route override.
func (a *API) registerOverrides(r module.Registrar) {
	r.Handle(http.MethodGet, "/i18n/overrides", PermRead, http.HandlerFunc(a.listOverrides))
	r.Handle(http.MethodPut, "/i18n/overrides/{namespace}/{locale}/{key}", PermManage, http.HandlerFunc(a.putOverride),
		module.MaxBodyBytes(maxBodyBytes))
	r.Handle(http.MethodDelete, "/i18n/overrides/{namespace}/{locale}/{key}", PermManage, http.HandlerFunc(a.deleteOverride),
		module.MaxBodyBytes(maxBodyBytes))
}

// OverrideView adalah bentuk override di respons API.
type OverrideView struct {
	Namespace string    `json:"namespace"`
	Locale    string    `json:"locale"`
	Key       string    `json:"key"`
	Message   string    `json:"message"`
	ETag      string    `json:"etag"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// rowETag adalah ETag kuat sebuah override: hash isi pesan (bukan counter), sehingga dua penulis yang menyimpan
// nilai sama tidak saling bentrok dan If-Match mengikuti isi.
func rowETag(o store.Override) string {
	sum := sha256.Sum256([]byte(o.Message))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

func toView(o store.Override) OverrideView {
	return OverrideView{
		Namespace: o.Namespace, Locale: o.Locale, Key: o.Key, Message: o.Message,
		ETag: rowETag(o), UpdatedBy: o.UpdatedBy, UpdatedAt: o.UpdatedAt,
	}
}

// preconditionError menandai If-Match yang tidak terpenuhi (412 version_mismatch); current adalah record terbaru
// (nil bila barisnya belum ada), dikirim di error.details.current (api-guidelines §5).
type preconditionError struct{ current *store.Override }

func (*preconditionError) Error() string { return "i18n: precondition If-Match gagal" }

// ifMatchPrecondition membangun store.Precondition dari header If-Match (RFC 9110 section 13.1.1): tanpa header
// tidak ada pemeriksaan (last-write-wins); "*" cocok bila barisnya ada; daftar entity-tag dibandingkan KUAT
// (tag lemah tidak pernah cocok); baris yang belum ada tidak pernah memenuhi If-Match.
func ifMatchPrecondition(header string) store.Precondition {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	return func(cur *store.Override) error {
		if cur == nil || !matchStrong(header, rowETag(*cur)) {
			return &preconditionError{current: cur}
		}
		return nil
	}
}

func matchStrong(header, etag string) bool {
	if header == "*" {
		return true
	}
	matched := false
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimSpace(part)
		if p == "" || strings.HasPrefix(p, "W/") {
			continue
		}
		if p == etag {
			matched = true
		}
	}
	return matched
}

// requireJSON menolak mutasi tanpa Content-Type application/json (charset utf-8) dengan 415; false bila sudah membalas.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		httpserver.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type",
			"Content-Type harus application/json (charset utf-8).",
			[]httpx.Detail{{Code: "unsupported_media_type", Message: "Content-Type harus application/json (charset utf-8)."}})
		return false
	}
	return true
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) (Backend, tenancy.TenantID, bool) {
	bp := a.backend.Load()
	if bp == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
		return nil, tenancy.TenantID{}, false
	}
	t, ok := tenancy.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusBadRequest, "tenant_unresolved", "tenant tidak dapat ditentukan", nil)
		return nil, tenancy.TenantID{}, false
	}
	return *bp, t, true
}

// writeOverrideError memetakan galat override ke respons: validasi 400 invalid_i18n_override, If-Match 412,
// tidak ada 404, penyimpanan tidak terpasang 503; selain itu 500 generik tanpa detail internal.
func writeOverrideError(w http.ResponseWriter, r *http.Request, err error) {
	var oe *i18n.OverrideError
	var pe *preconditionError
	switch {
	case errors.As(err, &oe):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_i18n_override", "Override istilah tidak valid.",
			[]httpx.Detail{{Field: oe.Field, Code: oe.Code, Message: oe.Detail}})
	case errors.As(err, &pe):
		var details any
		if pe.current != nil {
			details = map[string]any{"current": toView(*pe.current)}
		}
		httpx.WriteError(w, r, http.StatusPreconditionFailed, "version_mismatch", "Override telah berubah; muat ulang lalu coba lagi.", details)
	case errors.Is(err, i18n.ErrNotFound), errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "override tidak ditemukan", nil)
	case errors.Is(err, i18n.ErrOverridesUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
	default:
		httpx.RespondError(w, r, err)
	}
}

type putBody struct {
	Message *string `json:"message"`
}

func (a *API) putOverride(w http.ResponseWriter, r *http.Request) {
	be, t, ok := a.ready(w, r)
	if !ok || !requireJSON(w, r) {
		return
	}
	body, err := httpx.DecodeJSON[putBody](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	if body.Message == nil {
		httpx.RespondError(w, r, httpx.NewError(http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.",
			httpx.Detail{Field: "message", Code: "required", Message: "message wajib diisi"}))
		return
	}
	ns, loc, key := r.PathValue("namespace"), r.PathValue("locale"), r.PathValue("key")
	_, err = be.SetOverride(r.Context(), t, i18n.Override{Namespace: ns, Locale: loc, Key: key, Message: *body.Message},
		Actor, ifMatchPrecondition(r.Header.Get("If-Match")))
	if err != nil {
		writeOverrideError(w, r, err)
		return
	}
	row, err := be.GetOverride(r.Context(), t, ns, loc, key)
	if err != nil {
		writeOverrideError(w, r, err)
		return
	}
	v := toView(row)
	w.Header().Set("ETag", v.ETag)
	httpx.WriteData(w, http.StatusOK, v)
}

func (a *API) deleteOverride(w http.ResponseWriter, r *http.Request) {
	be, t, ok := a.ready(w, r)
	if !ok || !requireJSON(w, r) {
		return
	}
	existed, err := be.DeleteOverride(r.Context(), t, r.PathValue("namespace"), r.PathValue("locale"), r.PathValue("key"),
		Actor, ifMatchPrecondition(r.Header.Get("If-Match")))
	if err != nil {
		writeOverrideError(w, r, err)
		return
	}
	if !existed {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "override tidak ditemukan", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listOverrides(w http.ResponseWriter, r *http.Request) {
	be, t, ok := a.ready(w, r)
	if !ok {
		return
	}
	page, size := 1, defaultPageSize
	q := r.URL.Query()
	bad := func(field, msg string) {
		httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", msg, []httpx.Detail{{Field: field, Code: "invalid", Message: msg}})
	}
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			bad("page", "page harus bilangan bulat >= 1")
			return
		}
		page = n
	}
	if v := q.Get("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPageSize {
			bad("pageSize", "pageSize harus antara 1 dan 200")
			return
		}
		size = n
	}
	if page-1 > math.MaxInt/size {
		bad("page", "page terlalu besar untuk pageSize ini")
		return
	}
	f := store.Filter{Namespace: q.Get("namespace"), Locale: q.Get("locale")}
	if len(f.Namespace) > 50 || len(f.Locale) > 16 {
		bad("namespace", "filter terlalu panjang")
		return
	}
	rows, total, err := be.ListOverrides(r.Context(), t, f, page, size)
	if err != nil {
		writeOverrideError(w, r, err)
		return
	}
	views := make([]OverrideView, 0, len(rows))
	for _, o := range rows {
		views = append(views, toView(o))
	}
	httpserver.WritePage(w, http.StatusOK, views, httpserver.PageInfo{Page: page, PageSize: size, Total: total, HasMore: int64(page*size) < total})
}
