//go:build windows && !devtools

package tagged

import "hash/crc32"

var _ = crc32.IEEE
