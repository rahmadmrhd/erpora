package registry

import (
	"errors"
	"strings"
	"testing"

	"erpora/sdk/module"
)

func TestRegister_RouteOptionsDefaultsAndValues(t *testing.T) {
	reg, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Handle("GET", "/items", "notes.items.read", okHandler)
		r.Handle("POST", "/items", "notes.items.read", okHandler,
			module.MaxBodyBytes(4<<10), module.Classify(module.ClassConfidential))
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	byMethod := map[string]RouteInfo{}
	for _, rt := range reg.Routes() {
		byMethod[rt.Method] = rt
	}
	if g := byMethod["GET"]; g.MaxBodyBytes != 1<<20 || g.Classification != "" {
		t.Errorf("default = %+v", g)
	}
	if p := byMethod["POST"]; p.MaxBodyBytes != 4<<10 || p.Classification != module.ClassConfidential {
		t.Errorf("opsi = %+v", p)
	}
}

func TestRegister_InvalidRouteOptionsCollected(t *testing.T) {
	_, err := Build(testOpts(), notes(func(r module.Registrar) error {
		r.Handle("GET", "/a", "notes.items.read", okHandler, module.MaxBodyBytes(-1))
		r.Handle("GET", "/b", "notes.items.read", okHandler, module.MaxBodyBytes(10<<20+1))
		r.Public("GET", "/c", okHandler, module.Classify("secret"))
		return nil
	}))
	var be *Error
	if !errors.As(err, &be) {
		t.Fatalf("err = %v", err)
	}
	n := 0
	for _, p := range be.Problems {
		if p.Code != CodeInvalidRouteOption {
			t.Errorf("problem tak terduga: %+v", p)
			continue
		}
		n++
		if p.Module != "acme.notes" || !strings.Contains(p.Message, "GET /") {
			t.Errorf("problem = %+v", p)
		}
	}
	if n != 3 {
		t.Errorf("invalid_route_option = %d, want 3", n)
	}
}
