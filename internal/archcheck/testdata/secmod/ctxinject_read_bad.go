package secfixture

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture: tepat 3 pelanggaran IDENT di modul non-core (Service dan Admin hanya boleh dibaca modul core;
// keputusan pemilik produk 3 Nov 2026, Q16).
func ctxInjectReadBad(ctx context.Context) {
	_, _ = identity.AdminFrom(ctx)   // 1
	_, _ = identity.ServiceFrom(ctx) // 2
	f := identity.ServiceFrom        // 3 (referensi tanpa pemanggilan)
	_ = f
}
