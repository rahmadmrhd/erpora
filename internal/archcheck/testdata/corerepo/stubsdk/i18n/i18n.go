// Package i18n adalah stub minimal SDK untuk fixture arch test I1.
package i18n

import "context"

// Args meniru i18n.Args asli.
type Args map[string]any

// T meniru signature i18n.T asli.
func T(ctx context.Context, key string, args ...Args) string { return key }

// Lookup meniru signature i18n.Lookup asli.
func Lookup(ctx context.Context, key string, args ...Args) (string, bool) { return key, true }

// Locale meniru i18n.Locale asli.
type Locale string

// Resolver meniru i18n.Resolver asli.
type Resolver interface {
	Resolve(ctx context.Context, ns, key string, loc Locale) (string, bool)
}

// WithScope, WithResolver, ResolverFrom, dan WithLocale meniru API pemasang konteks milik platform.
func WithScope(ctx context.Context, namespace string) context.Context { return ctx }

// WithResolver meniru signature asli.
func WithResolver(ctx context.Context, r Resolver) context.Context { return ctx }

// ResolverFrom meniru signature asli.
func ResolverFrom(ctx context.Context) (Resolver, bool) { return nil, false }

// WithLocale meniru signature asli.
func WithLocale(ctx context.Context, loc Locale) context.Context { return ctx }
