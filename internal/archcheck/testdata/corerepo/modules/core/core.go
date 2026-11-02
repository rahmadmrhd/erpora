package core

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture modul core: boleh membaca Service dan Admin (0 pelanggaran untuk dua pembacaan itu), tetapi pemasangan
// tetap hak platform (tepat 1 pelanggaran IDENT: WithPrincipal).
func read(ctx context.Context) {
	_, _ = identity.AdminFrom(ctx)
	_, _ = identity.ServiceFrom(ctx)
	_ = identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 1
}
