package secfixture

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture: tepat 1 pelanggaran IDENT di berkas khusus linux; harus tetap terdeteksi walau gerbang berjalan di OS lain
// (LoadTyped memuat varian GOOS, M-4).
func ctxInject_linux(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 1
}
