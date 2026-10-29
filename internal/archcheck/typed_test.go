package archcheck

import (
	"strings"
	"testing"
)

func loadFixture(t *testing.T) []TypedPackage {
	t.Helper()
	pkgs, err := LoadTyped("testdata/secmod", "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
}

func byRule(vs []Violation, rule string) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Rule == rule {
			out = append(out, v)
		}
	}
	return out
}

// AC FR-24.2: arch test menolak fixture modul yang memakai http.Get (dan klien HTTP mentah lain).
func TestEgress_FixtureViolations(t *testing.T) {
	got := byRule(CheckSecurity(loadFixture(t)), "EGRESS")
	if len(got) != 18 { // 10 di egress_bad.go + 8 di egress_zero_bad.go
		t.Fatalf("EGRESS = %d, want 18:\n%+v", len(got), got)
	}
	for _, v := range got {
		if !strings.HasPrefix(v.Detail, "egress_bad.go:") && !strings.HasPrefix(v.Detail, "egress_zero_bad.go:") {
			t.Errorf("detail bukan dari fixture egress: %+v", v)
		}
	}
}

func TestDecode_FixtureViolations(t *testing.T) {
	got := byRule(CheckSecurity(loadFixture(t)), "DECODE")
	if len(got) != 5 {
		t.Fatalf("DECODE = %d, want 5:\n%+v", len(got), got)
	}
	perFile := map[string]int{}
	for _, v := range got {
		file, _, _ := strings.Cut(v.Detail, ":")
		perFile[file]++
	}
	if perFile["decode_bad.go"] != 4 || perFile["handler_bad.go"] != 1 {
		t.Errorf("per file = %v", perFile)
	}
}

// I1: argumen kunci i18n.T/Lookup yang konstan tidak boleh memuat "<ns>::" milik modul lain, dan modul tidak boleh
// memakai pemasang konteks i18n milik platform (WithScope, WithResolver, ResolverFrom, WithLocale).
func TestI1_FixtureViolations(t *testing.T) {
	got := byRule(CheckSecurity(loadFixture(t)), "I1")
	perFile := map[string]int{}
	for _, v := range got {
		file, _, _ := strings.Cut(v.Detail, ":")
		perFile[file]++
	}
	if len(got) != 10 || perFile["i18n_bad.go"] != 5 || perFile["i18n_ctx_bad.go"] != 5 { // 5 + 5; i18n_ok.go bersih
		t.Fatalf("I1 = %d per file %v, want 10 (5 i18n_bad.go + 5 i18n_ctx_bad.go): %+v", len(got), perFile, got)
	}
	for _, v := range got {
		if strings.HasPrefix(v.Detail, "i18n_ctx_bad.go:") && !strings.Contains(v.Detail, "milik platform") {
			t.Errorf("pesan harus menyebut bahwa API itu milik platform: %+v", v)
		}
	}
}
