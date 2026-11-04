package authz

import (
	"context"
	"errors"

	"erpora/sdk/identity"
)

// stubEngine adalah Engine uji internal: setiap method dapat diganti; bawaannya menolak.
type stubEngine struct {
	require    func(ctx context.Context, permission string) error
	filter     func(ctx context.Context, resource, action string) (RowFilter, error)
	check      func(ctx context.Context, permission string, rec Record) error
	fields     func(ctx context.Context, resource string) FieldAccess
	maskFields func(ctx context.Context, resource string, fields map[string]any) map[string]any
	checkWrite func(ctx context.Context, resource string, changed []string) error
	calls      int
}

func (s *stubEngine) Require(ctx context.Context, p string) error {
	s.calls++
	if s.require != nil {
		return s.require(ctx, p)
	}
	return ErrForbidden
}

func (s *stubEngine) Filter(ctx context.Context, r, a string) (RowFilter, error) {
	s.calls++
	if s.filter != nil {
		return s.filter(ctx, r, a)
	}
	return RowFilter{}, errors.New("stub")
}

func (s *stubEngine) Check(ctx context.Context, p string, rec Record) error {
	s.calls++
	if s.check != nil {
		return s.check(ctx, p, rec)
	}
	return ErrNotFound
}

func (s *stubEngine) Fields(ctx context.Context, r string) FieldAccess {
	s.calls++
	if s.fields != nil {
		return s.fields(ctx, r)
	}
	return nil
}

func (s *stubEngine) MaskFields(ctx context.Context, r string, f map[string]any) map[string]any {
	s.calls++
	if s.maskFields != nil {
		return s.maskFields(ctx, r, f)
	}
	return nil
}

func (s *stubEngine) CheckWrite(ctx context.Context, r string, c []string) error {
	s.calls++
	if s.checkWrite != nil {
		return s.checkWrite(ctx, r, c)
	}
	return ErrForbidden
}

// asUser memasang principal dan engine pada ctx dasar.
func asUser(e Engine) context.Context {
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", Kind: identity.KindHuman})
	return WithEngine(ctx, e)
}
