package httpserver

import (
	"net/http"

	"erpora/sdk/httpx"
)

// WriteError menulis amplop error standar; implementasinya ada di sdk/httpx (satu-satunya sumber amplop).
// Pesan tidak boleh memuat detail internal (5xx).
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	httpx.WriteError(w, r, status, code, message, details)
}

// WriteData menulis amplop sukses {"data": v}.
func WriteData(w http.ResponseWriter, status int, v any) { httpx.WriteData(w, status, v) }

// PageInfo adalah metadata pagination halaman (api-guidelines section 6).
type PageInfo = httpx.Page

// WritePage menulis amplop daftar {"data": [...], "page": {...}}.
func WritePage(w http.ResponseWriter, status int, data any, page PageInfo) {
	httpx.WritePage(w, status, data, page)
}
