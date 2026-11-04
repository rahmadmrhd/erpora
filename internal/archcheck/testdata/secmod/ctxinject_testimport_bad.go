package secfixture

import (
	"context"
	"testing"

	"erpora/sdk/identity"
	"erpora/sdk/identity/identitytest"
)

// Fixture: tepat 2 pelanggaran IDENT (kode non-test modul mengimpor helper uji identitas dan paket testing;
// helper uji dapat memasang Principal palsu dan hanya sah di berkas _test.go).
var _ *testing.T // impor `testing` di berkas non-test

func ctxInjectTestImportBad(ctx context.Context) context.Context {
	return identitytest.WithPrincipal(ctx, identity.Principal{UserID: "x"})
}
