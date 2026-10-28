package lifecycle

import (
	"strings"

	"erpora/internal/platform/modules/resolver"
)

// Kode error lifecycle (PRD-000 FR-19 + kode rencana M2b).
const (
	CodeInvalidTransition     = "invalid_transition"
	CodeOperationInProgress   = "operation_in_progress"
	CodeModuleRequired        = "module_required"
	CodeDependencyNotReady    = "dependency_not_ready"
	CodeDependentsActive      = "dependents_active"
	CodeModuleIncompatible    = "module_incompatible"
	CodeDowngradeNotSupported = "downgrade_not_supported"
	CodeModuleNotFound        = "module_not_found"
	CodeOperationNotFound     = "operation_not_found"
	CodeInterrupted           = "interrupted"
	CodeVersionMismatch       = "version_mismatch" // If-Match tidak cocok dengan revision tenant (RFC 9110 section 13.1.1)
)

// Error adalah error prasyarat/lock lifecycle. Related memuat key modul terkait (dependensi atau dependen),
// Issues memuat hasil resolver untuk module_incompatible.
type Error struct {
	Code, Module, Message string
	Revision              int64 // revision tenant saat ini (diisi untuk version_mismatch)
	Related               []string
	Issues                []resolver.Issue
}

func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Code)
	if e.Module != "" {
		sb.WriteString(" (")
		sb.WriteString(e.Module)
		sb.WriteString(")")
	}
	if e.Message != "" {
		sb.WriteString(": ")
		sb.WriteString(e.Message)
	}
	return sb.String()
}

// Unwrap mengembalikan *resolver.ResolutionError bila Code == module_incompatible, selain itu nil.
func (e *Error) Unwrap() error {
	if e.Code == CodeModuleIncompatible && len(e.Issues) > 0 {
		return &resolver.ResolutionError{Issues: e.Issues}
	}
	return nil
}
