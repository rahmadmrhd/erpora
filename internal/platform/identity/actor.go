package identity

import (
	"context"

	sdkidentity "erpora/sdk/identity"
)

// ActorPrefixUser mengawali aktor audit milik pengguna terautentikasi (`user:<id>`). Aktor non-pengguna memakai
// awalan lain (`cli:<name>`, `system:<alasan>`) dan tidak dibentuk di sini.
const ActorPrefixUser = "user:"

// ActorFromContext menurunkan aktor audit dari Principal di ctx (`user:<id>`), satu-satunya sumber aktor untuk
// aksi lewat API (menggantikan aktor tetap stub PRD-000; PRD-001 FR-102, FR-106). ok false bila tidak ada Principal
// atau UserID kosong: pemanggil WAJIB menolak request (gagal tertutup), tidak boleh memakai aktor bawaan.
func ActorFromContext(ctx context.Context) (string, bool) {
	p, err := sdkidentity.RequirePrincipal(ctx)
	if err != nil {
		return "", false
	}
	return ActorPrefixUser + p.UserID, true
}
