package app

import (
	"errors"
	"slices"
	"testing"
)

func TestBuildRegistry_ReferenceModulesValid(t *testing.T) {
	reg, err := BuildRegistry()
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	var keys []string
	for _, e := range reg.Entries() {
		keys = append(keys, e.Manifest.Key)
	}
	// Modul hasil `make:module` boleh menambah katalog (internal/app/modules.go); modul referensi harus ada,
	// platform.core pertama, dan example mendahului example-addon (dependensi lebih dulu).
	if len(keys) < 3 || keys[0] != "platform.core" {
		t.Fatalf("Entries = %v", keys)
	}
	ex, addon := slices.Index(keys, "acme.example"), slices.Index(keys, "acme.example-addon")
	if ex < 0 || addon < 0 || ex > addon {
		t.Errorf("Entries = %v, want acme.example sebelum acme.example-addon", keys)
	}

	have := map[string]bool{}
	for _, r := range reg.Routes() {
		have[r.Method+" "+r.Pattern] = true
	}
	for _, p := range []string{"GET /api/example/ping", "GET /api/example/notes", "GET /api/example-addon/summary"} {
		if !have[p] {
			t.Errorf("route %q tidak ada; ada %v", p, have)
		}
	}

	var cmds []string
	for _, c := range reg.Commands() {
		cmds = append(cmds, c.Name)
	}
	if !slices.Contains(cmds, "example:recount") {
		t.Errorf("Commands = %v", cmds)
	}
}

func TestExitError_Unwrap(t *testing.T) {
	inner := errors.New("x")
	var err error = &ExitError{Code: 2, Err: inner}
	if !errors.Is(err, inner) {
		t.Error("ExitError harus unwrap ke Err")
	}
}
