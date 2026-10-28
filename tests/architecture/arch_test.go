package architecture

import (
	"strings"
	"testing"

	"erpora/internal/archcheck"
)

func load(t *testing.T) archcheck.Repo {
	t.Helper()
	r, err := archcheck.Load("../..")
	if err != nil {
		t.Fatalf("archcheck.Load: %v", err)
	}
	return r
}

// TestRepositoryArchitecture menegakkan aturan D1-D11 pada repo nyata.
func TestRepositoryArchitecture(t *testing.T) {
	var lines []string
	for _, v := range archcheck.Check(load(t)) {
		lines = append(lines, v.Rule+": "+v.Package+" — "+v.Detail)
	}
	if len(lines) > 0 {
		t.Fatalf("pelanggaran arsitektur:\n%s", strings.Join(lines, "\n"))
	}
}

// TestGoVersion memeriksa keseragaman go/toolchain di semua go.mod (task check:gomod).
func TestGoVersion(t *testing.T) {
	for _, v := range archcheck.Check(load(t)) {
		if v.Rule == "GOVERSION" {
			t.Errorf("%s: %s — %s", v.Rule, v.Package, v.Detail)
		}
	}
}

// TestRepositorySecurityRules menegakkan EGRESS dan DECODE (FR-24.2) pada modul di repo nyata.
func TestRepositorySecurityRules(t *testing.T) {
	pkgs, err := archcheck.LoadTypedRepo("../..")
	if err != nil {
		t.Fatalf("archcheck.LoadTypedRepo: %v", err)
	}
	var lines []string
	for _, v := range archcheck.CheckSecurity(pkgs) {
		lines = append(lines, v.Rule+": "+v.Package+" — "+v.Detail)
	}
	if len(lines) > 0 {
		t.Fatalf("pelanggaran keamanan modul:\n%s", strings.Join(lines, "\n"))
	}
}
