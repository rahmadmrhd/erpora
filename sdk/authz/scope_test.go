package authz

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func okProvider(key string) ScopeProvider {
	return ScopeProvider{
		Key: key, Label: "x.scope.label",
		Filter: func(_ context.Context, db *gorm.DB) *gorm.DB { return db },
		Match:  func(context.Context, Record) (bool, error) { return true, nil },
	}
}

func TestScopeProviderValidate(t *testing.T) {
	if err := okProvider("example.recent").Validate("example"); err != nil {
		t.Fatalf("sah: %v", err)
	}
	cases := map[string]func(p *ScopeProvider){
		"tanpa awalan slug":  func(p *ScopeProvider) { p.Key = "other.recent" },
		"key tanpa titik":    func(p *ScopeProvider) { p.Key = "example" },
		"nama kosong":        func(p *ScopeProvider) { p.Key = "example." },
		"nama huruf besar":   func(p *ScopeProvider) { p.Key = "example.Recent" },
		"nama dengan titik":  func(p *ScopeProvider) { p.Key = "example.a.b" },
		"label kosong":       func(p *ScopeProvider) { p.Label = "" },
		"filter nil":         func(p *ScopeProvider) { p.Filter = nil },
		"match nil":          func(p *ScopeProvider) { p.Match = nil },
		"awalan slug serupa": func(p *ScopeProvider) { p.Key = "examplex.recent" },
	}
	for name, mut := range cases {
		p := okProvider("example.recent")
		mut(&p)
		if err := p.Validate("example"); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
	}
}
