// Package i18n adalah stub minimal SDK untuk fixture arch test I1.
package i18n

import "context"

// Args meniru i18n.Args asli.
type Args map[string]any

// T meniru signature i18n.T asli.
func T(ctx context.Context, key string, args ...Args) string { return key }

// Lookup meniru signature i18n.Lookup asli.
func Lookup(ctx context.Context, key string, args ...Args) (string, bool) { return key, true }
