package admin

import (
	"errors"
	"net/http"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/lifecycle"
)

// detail adalah satu entri error.details (api-guidelines section 8); related menyebut key modul terkait.
type detail struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Module  string   `json:"module,omitempty"`
	Related []string `json:"related,omitempty"`
}

// writeError memetakan error lifecycle ke katalog error api-guidelines (keputusan 1): kode FR-19 tetap
// terbaca di error.details[].code. Error lain menjadi 500 internal_error tanpa detail internal.
func (a *API) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var le *lifecycle.Error
	if !errors.As(err, &le) {
		a.log(r).ErrorContext(r.Context(), "operasi API modul gagal", "error", err)
		httpserver.WriteError(w, r, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server", nil)
		return
	}
	one := []detail{{Code: le.Code, Message: le.Message, Module: le.Module, Related: le.Related}}
	switch le.Code {
	case lifecycle.CodeModuleNotFound, lifecycle.CodeOperationNotFound:
		httpserver.WriteError(w, r, http.StatusNotFound, "not_found", "sumber daya tidak ditemukan", nil)
	case lifecycle.CodeVersionMismatch:
		httpserver.WriteError(w, r, http.StatusPreconditionFailed, "version_mismatch", le.Message, map[string]any{"revision": le.Revision})
	case lifecycle.CodeInvalidTransition:
		httpserver.WriteError(w, r, http.StatusConflict, "invalid_state", le.Message, one)
	case lifecycle.CodeOperationInProgress:
		httpserver.WriteError(w, r, http.StatusConflict, "request_in_progress", le.Message, one)
	case lifecycle.CodeModuleIncompatible:
		ds := make([]detail, 0, len(le.Issues))
		for _, is := range le.Issues {
			ds = append(ds, detail{Code: lifecycle.CodeModuleIncompatible, Message: is.String(), Module: is.Module})
		}
		if len(ds) == 0 {
			ds = one
		}
		httpserver.WriteError(w, r, http.StatusUnprocessableEntity, "business_rule_violated", "modul tidak kompatibel", ds)
	default: // module_required, dependency_not_ready, dependents_active, downgrade_not_supported
		httpserver.WriteError(w, r, http.StatusUnprocessableEntity, "business_rule_violated", le.Message, one)
	}
}
