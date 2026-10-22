//go:build !devtools && !integration && linux

package tagged

import "encoding/csv"

var _ = csv.ErrQuote
