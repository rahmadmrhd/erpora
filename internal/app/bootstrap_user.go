package app

import (
	"context"
	"errors"
	"log/slog"

	"erpora/internal/platform/bootstrap"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
)

// userDirectory adalah bagian kecil store identitas yang dibutuhkan bootstrap (dipenuhi *identitystore.Store).
type userDirectory interface {
	GetUser(ctx context.Context, t tenancy.TenantID, id string) (identitystore.User, error)
}

// withCurrentUser melengkapi manifest bootstrap dengan pengguna saat ini (PRD-001 FR-106): `user` (id, email, nama
// tampilan; tanpa hash sandi, status, atau data keamanan) dan `permRev` (stub 0 sampai C2).
//
// Satu-satunya masukan identitas adalah Principal di context (dipasang Authenticate dari token), jadi respons selalu
// milik pemanggil. Gagal tertutup: tanpa Principal, pengguna tak ditemukan di tenant ini (keanggotaan), atau
// nonaktif, manifest dikembalikan tanpa user dan handler menjawab 401. Galat store (sementara) menandai Unavailable
// (503) agar klien tidak mengira sesinya berakhir.
func withCurrentUser(base func(context.Context, tenancy.TenantID) bootstrap.Manifest, users userDirectory, log *slog.Logger) func(context.Context, tenancy.TenantID) bootstrap.Manifest {
	return func(ctx context.Context, t tenancy.TenantID) bootstrap.Manifest {
		m := base(ctx, t)
		p, err := sdkidentity.RequirePrincipal(ctx)
		if err != nil {
			return m
		}
		u, err := users.GetUser(ctx, t, p.UserID)
		switch {
		case errors.Is(err, identitystore.ErrNotFound):
			return m
		case err != nil:
			log.ErrorContext(ctx, "bootstrap: muat pengguna saat ini gagal", "error", err)
			m.Unavailable = true
			return m
		// locked hanya menolak login baru; sesi yang sudah ada tetap valid (PRD-001 Q30 butir 4), jadi tetap dilayani.
		case u.Status != identitystore.UserActive && u.Status != identitystore.UserLocked:
			return m
		}
		// ID dari Principal (kunci kueri), bukan dari baris: satu bentuk kanonik untuk aktor audit dan ETag.
		m.User = &bootstrap.User{ID: p.UserID, Email: u.Email, DisplayName: u.Name}
		m.PermRev = 0
		return m
	}
}
