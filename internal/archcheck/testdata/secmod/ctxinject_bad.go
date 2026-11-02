package secfixture

import (
	"context"

	"erpora/sdk/identity"
	"erpora/sdk/mail"
)

// Fixture: tepat 5 pelanggaran IDENT (modul memasang identitas atau pengirim email di context; itu hak platform,
// modul memakai identitytest di tes).
func ctxInjectBad(ctx context.Context) {
	ctx = identity.WithPrincipal(ctx, identity.Principal{UserID: "x"}) // 1
	ctx = identity.WithService(ctx, nil)                               // 2
	ctx = identity.WithAdmin(ctx, identity.Admin{})                    // 3
	ctx = mail.WithSender(ctx, nil)                                    // 4
	f := identity.WithPrincipal                                        // 5 (referensi tanpa pemanggilan)
	_ = f
	_ = ctx
}
