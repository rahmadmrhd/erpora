package registry

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"erpora/sdk/authz"
	"erpora/sdk/module"
)

func testScope(key string) authz.ScopeProvider {
	return authz.ScopeProvider{
		Key: key, Label: "notes.scope.label",
		Filter: func(_ context.Context, db *gorm.DB) *gorm.DB { return db },
		Match:  func(context.Context, authz.Record) (bool, error) { return true, nil },
	}
}

func TestRegistrarScopeValidated(t *testing.T) {
	cases := map[string]func(r module.Registrar){
		"tanpa awalan slug": func(r module.Registrar) { r.Scope(testScope("other.recent")) },
		"duplikat": func(r module.Registrar) {
			r.Scope(testScope("notes.recent"))
			r.Scope(testScope("notes.recent"))
		},
		"filter nil": func(r module.Registrar) {
			p := testScope("notes.recent")
			p.Filter = nil
			r.Scope(p)
		},
		"match nil": func(r module.Registrar) {
			p := testScope("notes.recent")
			p.Match = nil
			r.Scope(p)
		},
		"label kosong": func(r module.Registrar) {
			p := testScope("notes.recent")
			p.Label = ""
			r.Scope(p)
		},
	}
	for name, reg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Build(testOpts(), notes(func(r module.Registrar) error { reg(r); return nil }))
			var be *Error
			if !errors.As(err, &be) || len(be.Problems) != 1 || be.Problems[0].Code != CodeInvalidScopeProvider {
				t.Fatalf("err = %v, want satu problem %s", err, CodeInvalidScopeProvider)
			}
		})
	}
}

func TestRegistrarScopeValidAccepted(t *testing.T) {
	if _, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Scope(testScope("notes.recent"))
		r.Scope(testScope("notes.assigned_to_me"))
		return nil
	})); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func TestRecorderKeepsValidScopes(t *testing.T) {
	r := &recorder{key: "acme.notes", slug: "notes"}
	r.Scope(testScope("notes.recent"))
	r.Scope(testScope("other.bad"))
	if len(r.scopes) != 1 || r.scopes[0].Key != "notes.recent" || len(r.problems) != 1 {
		t.Fatalf("scopes=%v problems=%v", r.scopes, r.problems)
	}
}
