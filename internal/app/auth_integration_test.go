//go:build integration

package app

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
)

// authedUsers menyimpan satu pengguna nyata per assembly: ETag bootstrap bersifat per pengguna, jadi tes yang
// membandingkan ETag atau memakai If-None-Match antar panggilan authed(t) harus selalu memakai pengguna yang sama.
var authedUsers sync.Map // *assembly -> store.User

// authedUser mengembalikan pengguna nyata (baris di database, anggota tenant, aktif) milik assembly ini; dibuat sekali.
func (a *assembly) authedUser(t *testing.T) store.User {
	t.Helper()
	if v, ok := authedUsers.Load(a); ok {
		return v.(store.User)
	}
	u, err := a.Identity.Store.CreateUser(tenancy.WithTenant(context.Background(), a.Tenant), a.Tenant,
		store.NewUser{Email: uuid.NewString() + "@example.test", Name: "Penguji", Status: store.UserActive})
	if err != nil {
		t.Fatalf("buat pengguna uji: %v", err)
	}
	authedUsers.Store(a, u)
	return u
}

// authed mengembalikan handler assembly yang menambahkan access token uji sah (untuk pengguna nyata assembly ini) pada
// setiap request tanpa header Authorization. Autentikasi tetap nyata (Authenticate memverifikasi tanda tangan, tid,
// dan sid); hanya token yang disuplai helper. Request yang membawa Authorization sendiri dibiarkan (untuk tes 401).
func (a *assembly) authed(t *testing.T) http.Handler {
	t.Helper()
	return a.authedAs(t, a.authedUser(t).ID)
}

// authedAs seperti authed tetapi untuk pengguna userID (mis. membandingkan dua pengguna: IDOR, ETag per pengguna).
func (a *assembly) authedAs(t *testing.T, userID string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			raw, err := a.Identity.Issuer.Issue(token.Claims{
				Subject: userID, SessionID: uuid.NewString(), TenantID: a.Tenant.String(), AMR: []string{"pwd"},
			})
			if err != nil {
				t.Errorf("terbitkan token uji: %v", err)
				http.Error(w, "token", http.StatusInternalServerError)
				return
			}
			r.Header.Set("Authorization", "Bearer "+raw)
		}
		a.Router.ServeHTTP(w, r)
	})
}
