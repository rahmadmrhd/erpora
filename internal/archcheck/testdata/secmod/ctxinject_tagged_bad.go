//go:build devtools

package secfixture

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture: tepat 1 pelanggaran IDENT di berkas ber-build-tag `devtools`; harus tetap terdeteksi karena LoadTyped
// memeriksa paket dengan dan tanpa tag yang dipakai repo (M-5), bukan hanya konfigurasi build bawaan.
func ctxInjectTaggedBad(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 1
}
