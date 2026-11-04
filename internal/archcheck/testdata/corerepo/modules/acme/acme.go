package core

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture penyamar: go.mod berpath `erpora/modules/core` tetapi berada di direktori modules/acme. Tidak boleh
// mendapat hak core (AdminFrom/ServiceFrom): 3 pelanggaran IDENT (AdminFrom, ServiceFrom, WithPrincipal).
func read(ctx context.Context) {
	_, _ = identity.AdminFrom(ctx)                                   // 1
	_, _ = identity.ServiceFrom(ctx)                                 // 2
	_ = identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 3
}
