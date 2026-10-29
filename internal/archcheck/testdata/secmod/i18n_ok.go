package secfixture

import (
	"context"

	"erpora/sdk/i18n"
)

// Fixture: pemakaian yang sah, 0 pelanggaran: kunci relatif, namespace sendiri (secfixture), ui, platform,
// dan kunci dinamis (ditegakkan runtime: T menolak namespace modul lain).
func i18nOK(ctx context.Context, dyn string) {
	_ = i18n.T(ctx, "note.title")
	_ = i18n.T(ctx, "secfixture::note.title")
	_ = i18n.T(ctx, "ui::actions.save")
	_ = i18n.T(ctx, "platform::errors.not_found")
	_ = i18n.T(ctx, dyn)
	_ = i18n.T(ctx, "a:b") // satu titik dua bukan pemisah namespace
}
