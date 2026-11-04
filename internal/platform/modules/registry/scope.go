package registry

import "erpora/sdk/authz"

// CodeInvalidScopeProvider: Registrar.Scope menerima provider yang tidak sah (key tanpa awalan slug,
// duplikat, Filter/Match/Label kosong; PRD-001 FR-111).
const CodeInvalidScopeProvider = "invalid_scope_provider"

// Scope mengimplementasi module.Registrar: merekam scope kustom modul. Provider yang tidak sah dicatat
// sebagai Problem dan tidak direkam.
func (r *recorder) Scope(p authz.ScopeProvider) {
	if err := p.Validate(r.slug); err != nil {
		r.problem(CodeInvalidScopeProvider, "%v", err)
		return
	}
	for _, have := range r.scopes {
		if have.Key == p.Key {
			r.problem(CodeInvalidScopeProvider, "scope %q didaftarkan lebih dari sekali", p.Key)
			return
		}
	}
	r.scopes = append(r.scopes, p)
}
