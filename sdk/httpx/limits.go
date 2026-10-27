package httpx

import (
	"context"

	"erpora/sdk/module"
)

const (
	// DefaultMaxBodyBytes adalah batas body default (sama dengan module.DefaultMaxBodyBytes).
	DefaultMaxBodyBytes = module.DefaultMaxBodyBytes
	// DefaultMaxDepth adalah kedalaman JSON maksimal; objek akar berkedalaman 1.
	DefaultMaxDepth = 32
	// DefaultMaxArrayLen adalah panjang array maksimal (security-guidelines §3.8).
	DefaultMaxArrayLen = 1000
)

type bodyLimitKey struct{}

// WithBodyLimit memasang batas body request di context; dipanggil platform (runtime.Mount).
func WithBodyLimit(ctx context.Context, n int64) context.Context {
	return context.WithValue(ctx, bodyLimitKey{}, n)
}

// BodyLimit membaca batas body dari context.
func BodyLimit(ctx context.Context) (int64, bool) {
	n, ok := ctx.Value(bodyLimitKey{}).(int64)
	return n, ok && n > 0
}
