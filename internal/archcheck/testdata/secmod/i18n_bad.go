package secfixture

import (
	"context"

	"erpora/sdk/i18n"
)

const otherModuleKey = "orders::note.title"

// Fixture: tepat 5 pelanggaran I1 (namespace modul lain pada kunci konstan i18n.T/Lookup).
func i18nBad(ctx context.Context) {
	_ = i18n.T(ctx, "orders::note.title")             // 1
	_ = i18n.T(ctx, otherModuleKey)                   // 2 (konstanta bernama)
	_ = i18n.T(ctx, "ord"+"ers::note.title")          // 3 (lipatan konstanta)
	_, _ = i18n.Lookup(ctx, "inventory::fields.name") // 4
	_ = i18n.T(ctx, "orders::x", i18n.Args{"n": 1})   // 5 (dengan argumen)
}
