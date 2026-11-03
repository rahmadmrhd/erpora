//go:build integration

package app

import (
	"net/http"
	"testing"

	"erpora/internal/platform/testsupport/authtest"
)

// authed mengembalikan handler assembly yang menambahkan access token uji sah pada setiap request tanpa header
// Authorization. Autentikasi tetap nyata (Authenticate memverifikasi tanda tangan, tid, dan sid); hanya token yang
// disuplai helper. Request yang membawa Authorization sendiri dibiarkan (untuk tes 401).
func (a *assembly) authed(t *testing.T) http.Handler {
	t.Helper()
	return authtest.Authed(t, a.Router, a.Identity.Issuer, a.Tenant)
}
