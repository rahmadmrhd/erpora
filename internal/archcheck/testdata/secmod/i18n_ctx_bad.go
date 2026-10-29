package secfixture

import (
	"context"

	"erpora/sdk/i18n"
)

// Fixture: tepat 5 pelanggaran I1 (pemasang konteks i18n milik platform dipakai modul; R-5). Modul hanya boleh
// memakai T/Lookup; mengubah scope atau Resolver membuka namespace modul lain (Requirement 1.2, P12).
func i18nCtxBad(ctx context.Context) {
	ctx = i18n.WithScope(ctx, "orders") // 1
	ctx = i18n.WithResolver(ctx, nil)   // 2
	_, _ = i18n.ResolverFrom(ctx)       // 3
	ctx = i18n.WithLocale(ctx, "en")    // 4
	f := i18n.WithScope                 // 5 (referensi tanpa pemanggilan)
	_ = f
	_ = ctx
}
