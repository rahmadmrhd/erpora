package tenancy

import "net/http"

// Resolver menentukan tenant untuk sebuah request. PRD Tenancy mengganti implementasinya.
type Resolver interface {
	Resolve(r *http.Request) (TenantID, error)
}

// SingleTenantResolver selalu mengembalikan tenant default (mode single-tenant).
type SingleTenantResolver struct{ ID TenantID }

// Resolve mengembalikan ID default untuk request apa pun.
func (s SingleTenantResolver) Resolve(*http.Request) (TenantID, error) { return s.ID, nil }
