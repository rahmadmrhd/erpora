//go:build !cgo

package secfixture

import (
	"context"

	"erpora/sdk/identity"
)

// Fixture: tepat 1 pelanggaran IDENT di berkas `!cgo`; rilis lintas-build memakai CGO_ENABLED=0 sedangkan gerbang CI
// Linux berjalan dengan cgo aktif, sehingga varian CGO_ENABLED=0 harus dimuat (I-3).
func ctxInjectNoCgo(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 1
}
