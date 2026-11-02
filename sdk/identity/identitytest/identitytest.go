// Package identitytest berisi helper identity untuk test modul.
package identitytest

import (
	"context"
	"testing"

	"erpora/sdk/identity"
)

// guard panik bila bukan di dalam biner test. Helper memanggilnya agar kode non-test tidak dapat
// memakai identitytest (mis. dengan new(testing.T)) untuk memalsukan Principal di produksi.
func guard(inTest bool) {
	if !inTest {
		panic("identitytest hanya untuk tes")
	}
}

// WithPrincipal memasang p di ctx untuk test; panik bila dipanggil di luar biner test.
func WithPrincipal(t testing.TB, ctx context.Context, p identity.Principal) context.Context {
	guard(testing.Testing())
	t.Helper()
	return identity.WithPrincipal(ctx, p)
}
