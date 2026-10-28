// Package authztest berisi helper authz untuk test modul.
package authztest

import (
	"context"
	"maps"

	"erpora/sdk/authz"
)

type allowAll struct{}

func (allowAll) MaskFields(_ context.Context, _ string, fields map[string]any) map[string]any {
	return maps.Clone(fields)
}

// AllowAllFields mengembalikan salinan field apa adanya; hanya untuk test (tidak pernah di produksi).
var AllowAllFields authz.FieldMasker = allowAll{}
