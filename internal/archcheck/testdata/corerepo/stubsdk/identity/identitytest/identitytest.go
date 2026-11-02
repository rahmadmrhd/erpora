// Package identitytest adalah stub minimal helper uji SDK untuk fixture arch test IDENT.
package identitytest

import (
	"context"

	"erpora/sdk/identity"
)

// WithPrincipal meniru helper uji asli yang memasang Principal palsu di context.
func WithPrincipal(ctx context.Context, p identity.Principal) context.Context {
	return identity.WithPrincipal(ctx, p)
}
