package data_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/rahmadmrhd/erp-go/sdk/data"
)

type ctxKey struct{}

type fakeProvider struct {
	gotCtx context.Context
	db     *gorm.DB
}

func (f *fakeProvider) DB(ctx context.Context) (*gorm.DB, error) {
	f.gotCtx = ctx
	return f.db, nil
}

func TestDataDB_WithoutProviderReturnsError(t *testing.T) {
	_, err := data.DB(context.Background())
	if !errors.Is(err, data.ErrNoTenantContext) {
		t.Fatalf("err = %v", err)
	}
}

func TestDataDB_DelegatesToProvider(t *testing.T) {
	want := &gorm.DB{}
	p := &fakeProvider{db: want}
	ctx := context.WithValue(data.WithProvider(context.Background(), p), ctxKey{}, "marker")
	got, err := data.DB(ctx)
	if err != nil || got != want {
		t.Fatalf("got %v, %v", got, err)
	}
	if p.gotCtx != ctx {
		t.Fatal("provider must receive the same ctx")
	}
}
