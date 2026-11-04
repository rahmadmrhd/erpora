package secfixture

import (
	"context"
	_ "unsafe" // diperlukan //go:linkname

	"erpora/sdk/identity"
)

// Fixture: tepat 2 pelanggaran IDENT. `//go:linkname` memintas deteksi pemanggilan (archcheck hanya melihat
// types.Func dari Info.Uses): fungsi tanpa badan ini sebenarnya WithPrincipal milik platform. Dilarang: impor
// unsafe (1) dan direktif linkname (2).
//
//go:linkname forge erpora/sdk/identity.WithPrincipal
func forge(ctx context.Context, p identity.Principal) context.Context
