package authztest

import (
	"context"
	"reflect"
	"testing"

	"erpora/sdk/authz"
)

func TestAllowAllFields(t *testing.T) {
	ctx := authz.WithFieldMasker(context.Background(), AllowAllFields)
	got := authz.MaskFields(ctx, "x.note", map[string]any{"title": "a", "cost": "9"})
	if !reflect.DeepEqual(got, map[string]any{"title": "a", "cost": "9"}) {
		t.Errorf("got %#v", got)
	}
}
