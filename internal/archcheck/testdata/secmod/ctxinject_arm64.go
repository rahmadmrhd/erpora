package secfixture

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture: tepat 1 pelanggaran IDENT di berkas khusus arm64; matriks rilis memuat linux/arm64 (ci.yml, job build)
// sehingga varian arsitektur itu harus dimuat (I-3).
func ctxInjectArm64(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 1
}
