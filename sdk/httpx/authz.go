package httpx

import (
	"errors"
	"net/http"
	"slices"

	"erpora/sdk/authz"
)

// WriteAuthzError memetakan galat sdk/authz ke respons error standar (api-guidelines §8) dan melaporkan
// apakah err ditangani; false (tanpa menulis apa pun) untuk nil dan galat selain galat authz, sehingga
// pemanggil menanganinya sendiri (mis. RespondError, 500).
//
//   - authz.ErrNotFound: 404 not_found. Badan identik untuk "tidak ada" dan "di luar scope" agar
//     keberadaan record tidak terungkap.
//   - authz.ErrForbidden: 403 permission_denied.
//   - *authz.FieldForbiddenError: 403 field_forbidden dengan details[] satu entri per field (nama field
//     sebagaimana diberikan pemanggil).
//   - authz.ErrUnauthenticated: 401 unauthenticated.
//
// Galat dicocokkan lewat errors.Is/errors.As sehingga galat yang dibungkus tetap dikenali. ID referensi
// di body yang di luar scope adalah urusan handler (422 validation_failed dengan details[].code =
// not_found), bukan fungsi ini.
func WriteAuthzError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	var ff *authz.FieldForbiddenError
	switch {
	case errors.As(err, &ff):
		fields := slices.Compact(slices.Sorted(slices.Values(ff.Fields)))
		details := make([]Detail, len(fields))
		for i, f := range fields {
			details[i] = Detail{Field: f, Code: "field_forbidden", Message: "Anda tidak berhak mengakses field ini."}
		}
		WriteError(w, r, http.StatusForbidden, "field_forbidden", "Anda tidak berhak mengakses sebagian field.", details)
	case errors.Is(err, authz.ErrNotFound):
		WriteError(w, r, http.StatusNotFound, "not_found", "Data tidak ditemukan.", nil)
	case errors.Is(err, authz.ErrForbidden):
		WriteError(w, r, http.StatusForbidden, "permission_denied", "Anda tidak punya izin untuk aksi ini.", nil)
	case errors.Is(err, authz.ErrUnauthenticated):
		WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "Autentikasi diperlukan.", nil)
	default:
		return false
	}
	return true
}
