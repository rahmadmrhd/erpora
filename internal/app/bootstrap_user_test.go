package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"erpora/internal/platform/bootstrap"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
)

type fakeDirectory struct {
	users map[string]identitystore.User
	err   error
	calls []string
}

func (f *fakeDirectory) GetUser(_ context.Context, _ tenancy.TenantID, id string) (identitystore.User, error) {
	f.calls = append(f.calls, id)
	if f.err != nil {
		return identitystore.User{}, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return identitystore.User{}, identitystore.ErrNotFound
	}
	return u, nil
}

func baseManifest(context.Context, tenancy.TenantID) bootstrap.Manifest {
	return bootstrap.Build("1.0.0", tenancy.TenantID{}, nil, 9)
}

func withPrincipal(uid string) context.Context {
	return sdkidentity.WithPrincipal(context.Background(), sdkidentity.Principal{UserID: uid, Kind: sdkidentity.KindHuman})
}

func TestWithCurrentUser(t *testing.T) {
	const a, b = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4aa1", "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4bb2"
	dir := &fakeDirectory{users: map[string]identitystore.User{
		a: {ID: a, Email: "a@example.test", Name: "Ani", Status: identitystore.UserActive, PasswordHash: "RAHASIA"},
		b: {ID: b, Email: "b@example.test", Name: "Budi", Status: identitystore.UserDisabled},
	}}
	fn := withCurrentUser(baseManifest, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))

	m := fn(withPrincipal(a), tenancy.TenantID{})
	if m.User == nil || *m.User != (bootstrap.User{ID: a, Email: "a@example.test", DisplayName: "Ani"}) || m.PermRev != 0 || m.Unavailable || m.Revision != 9 {
		t.Fatalf("pengguna aktif = %+v user=%+v", m, m.User)
	}
	if len(dir.calls) != 1 || dir.calls[0] != a {
		t.Errorf("sumber hanya boleh dikueri untuk Principal: %v", dir.calls)
	}
	// Pengguna nonaktif, tak dikenal, dan tanpa Principal: tanpa user (handler menjawab 401); tanpa Principal tidak
	// mengueri sumber sama sekali.
	for name, ctx := range map[string]context.Context{
		"nonaktif":        withPrincipal(b),
		"tak dikenal":     withPrincipal("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4cc3"),
		"tanpa principal": context.Background(),
	} {
		dir.calls = nil
		if m := fn(ctx, tenancy.TenantID{}); m.User != nil || m.Unavailable {
			t.Errorf("%s: user = %+v unavailable=%v", name, m.User, m.Unavailable)
		}
		if name == "tanpa principal" && len(dir.calls) != 0 {
			t.Error("tanpa principal tidak boleh mengueri sumber")
		}
	}
	// Galat sementara bukan "tidak ditemukan": Unavailable, tanpa user.
	dir.err = errors.New("koneksi putus")
	if m := fn(withPrincipal(a), tenancy.TenantID{}); m.User != nil || !m.Unavailable {
		t.Errorf("galat sementara: user=%+v unavailable=%v", m.User, m.Unavailable)
	}
}
