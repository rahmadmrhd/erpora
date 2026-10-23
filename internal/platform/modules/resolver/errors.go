package resolver

import "strings"

// Kode Issue.
const (
	CodeMissingDependency       = "missing_dependency"
	CodeVersionMismatch         = "version_mismatch"
	CodeOptionalVersionMismatch = "optional_version_mismatch"
	CodeCycle                   = "cycle"
	CodeInvalidConstraint       = "invalid_constraint"
	CodePlatformIncompatible    = "platform_incompatible"
	CodeSDKIncompatible         = "sdk_incompatible"
)

// Issue adalah satu masalah hasil resolusi. Path hanya terisi untuk siklus
// (jalur lengkap, awal dan akhir sama).
type Issue struct {
	Code       string
	Module     string
	Dependency string
	Constraint string
	Found      string
	Path       []string
}

// String membentuk deskripsi satu baris.
func (i Issue) String() string {
	var sb strings.Builder
	sb.WriteString(i.Code)
	sb.WriteString(": modul ")
	sb.WriteString(i.Module)
	if i.Code == CodeCycle {
		sb.WriteString(" siklus ")
		sb.WriteString(strings.Join(i.Path, " -> "))
		return sb.String()
	}
	if i.Dependency != "" {
		sb.WriteString(" dependensi ")
		sb.WriteString(i.Dependency)
	}
	if i.Constraint != "" {
		sb.WriteString(" constraint ")
		sb.WriteString(i.Constraint)
	}
	if i.Found != "" {
		sb.WriteString(" ditemukan ")
		sb.WriteString(i.Found)
	}
	return sb.String()
}

// ResolutionError menampung seluruh masalah struktural katalog sekaligus.
type ResolutionError struct{ Issues []Issue }

// Error menggabungkan semua issue, satu per baris.
func (e *ResolutionError) Error() string {
	lines := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		lines[i] = is.String()
	}
	return strings.Join(lines, "\n")
}
