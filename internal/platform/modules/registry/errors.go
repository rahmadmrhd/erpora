package registry

import (
	"fmt"
	"slices"
	"strings"
)

// Problem adalah satu temuan saat membangun registry. Code meneruskan kode asli
// dari validasi manifest dan resolver (schema_invalid, missing_dependency,
// cycle, dst.) ditambah kode milik registry.
type Problem struct {
	Module  string
	Field   string
	Code    string
	Message string
}

// Error menampung semua Problem hasil Build.
type Error struct{ Problems []Problem }

// Error menulis satu baris per problem, terurut (Module, Field, Code).
func (e *Error) Error() string {
	ps := slices.Clone(e.Problems)
	sortProblems(ps)
	lines := make([]string, len(ps))
	for i, p := range ps {
		loc := p.Module
		if p.Field != "" {
			loc += " " + p.Field
		}
		lines[i] = fmt.Sprintf("%s [%s]: %s", loc, p.Code, p.Message)
	}
	return strings.Join(lines, "\n")
}

func sortProblems(ps []Problem) {
	slices.SortStableFunc(ps, func(a, b Problem) int {
		if c := strings.Compare(a.Module, b.Module); c != 0 {
			return c
		}
		if c := strings.Compare(a.Field, b.Field); c != 0 {
			return c
		}
		return strings.Compare(a.Code, b.Code)
	})
}

// Kode Problem milik registry.
const (
	CodeRegisterFailed = "register_failed"
	CodeDuplicateKey   = "duplicate_key"

	CodeDuplicateSlug        = "duplicate_slug"
	CodeReservedSlug         = "reserved_slug"
	CodeDuplicateCapability  = "duplicate_capability"
	CodeCapabilityPrefix     = "capability_prefix"
	CodePermissionMissing    = "permission_missing"
	CodePermissionUndeclared = "permission_undeclared"
	CodeInvalidRoute         = "invalid_route"
	CodeRouteConflict        = "route_conflict"
	CodeInvalidCommand       = "invalid_command"
	CodeCommandConflict      = "command_conflict"
	CodeMigrationsMismatch   = "migrations_mismatch"
)
