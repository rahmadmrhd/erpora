package archcheck

import (
	"path/filepath"
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

// IDENT: kode non-test di modul tidak boleh memasang identitas, layanan identitas, hak admin, atau pengirim email
// di context (identity.WithPrincipal/WithService/WithAdmin, mail.WithSender); itu hak platform. Modul memakai
// identitytest di tes (tes tidak dimuat LoadTyped).
func TestIdent_FixtureViolations(t *testing.T) {
	got := byRule(CheckSecurity(loadFixture(t)), "IDENT")
	perFile := map[string]int{}
	for _, v := range got {
		file, _, _ := strings.Cut(v.Detail, ":")
		perFile[file]++
	}
	want := map[string]int{
		"ctxinject_bad.go":            5, // pemasang konteks
		"ctxinject_testimport_bad.go": 2, // impor identitytest dan testing
		"ctxinject_tagged_bad.go":     1, // berkas ber-tag devtools (M-5)
		"ctxinject_read_bad.go":       3, // AdminFrom/ServiceFrom di modul non-core
		"linkname_bad.go":             2, // impor unsafe + //go:linkname (I-1a)
		"unsafe_bad.go":               1, // impor unsafe
		"ctxinject_windows.go":        1, // varian GOOS=windows (M-4)
		"ctxinject_linux.go":          1, // varian GOOS=linux (M-4)
		"ctxinject_arm64.go":          1, // varian linux/arm64 dari matriks rilis (I-3)
		"ctxinject_nocgo.go":          1, // varian CGO_ENABLED=0 (I-3)
		"unsafe_reflect_bad.go":       2, // unsafe.Pointer tanpa impor unsafe (C-1)
	}
	total := 0
	for f, n := range want {
		total += n
		if perFile[f] != n {
			t.Errorf("IDENT di %s = %d, want %d", f, perFile[f], n)
		}
	}
	if len(got) != total { // ctxinject_ok.go bersih
		t.Fatalf("IDENT = %d, want %d: per file %v: %+v", len(got), total, perFile, got)
	}
	for _, v := range got {
		if !strings.Contains(v.Detail, "milik platform") && !strings.Contains(v.Detail, "hanya untuk berkas _test.go") &&
			!strings.Contains(v.Detail, "hanya modul core") && !strings.Contains(v.Detail, "unsafe") && !strings.Contains(v.Detail, "linkname") {
			t.Errorf("pesan IDENT tidak menjelaskan sebab: %+v", v)
		}
	}
}

// M-5: tiap pelanggaran dilaporkan sekali walau paket dimuat untuk beberapa kombinasi build tag.
func TestCheckSecurity_NoDuplicateViolations(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range CheckSecurity(loadFixture(t)) {
		k := v.Rule + "|" + v.Package + "|" + v.Detail
		if seen[k] {
			t.Errorf("pelanggaran ganda: %s", k)
		}
		seen[k] = true
	}
}

// loadCoreRepo memuat satu modul fixture di bawah root repo tiruan testdata/corerepo.
func loadCoreRepo(t *testing.T, dir string) []TypedPackage {
	t.Helper()
	pkgs, err := LoadTypedFrom("testdata/corerepo", "testdata/corerepo/"+dir, "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
}

// IDENT: modul core (path erpora/modules/core DAN direktori persis <root>/modules/core) boleh membaca
// identity.AdminFrom dan identity.ServiceFrom (Q16, 3 Nov 2026), tetapi pemasang konteks tetap dilarang di dalamnya.
func TestIdent_CoreModuleMayReadServiceAndAdmin(t *testing.T) {
	got := byRule(CheckSecurity(loadCoreRepo(t, "modules/core")), "IDENT")
	if len(got) != 1 || !strings.Contains(got[0].Detail, "identity.WithPrincipal") {
		t.Fatalf("IDENT di modul core = %+v, want tepat 1 (WithPrincipal) dan tanpa AdminFrom/ServiceFrom", got)
	}
}

// M-3: go.mod berpath erpora/modules/core di direktori lain (modules/acme) tidak mendapat hak core.
func TestIdent_CorePathInOtherDirectoryGetsNoCoreRights(t *testing.T) {
	got := byRule(CheckSecurity(loadCoreRepo(t, "modules/acme")), "IDENT")
	if len(got) != 3 {
		t.Fatalf("IDENT di modules/acme berpath core = %d, want 3 (AdminFrom, ServiceFrom, WithPrincipal): %+v", len(got), got)
	}
}

// M-3 (jangkar): direktori yang hanya BERAKHIRAN /modules/core (other/modules/core) bukan <root>/modules/core.
func TestIdent_CoreRightsAnchoredToRepoRoot(t *testing.T) {
	got := byRule(CheckSecurity(loadCoreRepo(t, "other/modules/core")), "IDENT")
	if len(got) != 3 {
		t.Fatalf("IDENT di other/modules/core = %d, want 3 (tanpa hak core): %+v", len(got), got)
	}
}

// C-1: berkas hasil cgo (_cgo_gotypes.go dan sejenisnya) tidak dipindai untuk unsafe.Pointer (kode bangkitan
// toolchain, bukan kode modul); berkas lain, termasuk yang berkepala "Code generated", tetap dipindai.
func TestIsCgoGenerated(t *testing.T) {
	for name, want := range map[string]bool{
		"_cgo_gotypes.go": true, "_cgo_export.go": true, "x.go": false, "cgo.go": false, "gen.gen.go": false,
	} {
		if got := isCgoGenerated(filepath.Join("a", "b", name)); got != want {
			t.Errorf("isCgoGenerated(%q) = %v, want %v", name, got, want)
		}
	}
}

// M-4/I-3: varian dari matriks rilis (ci.yml job build) selain target host, tanpa cgo; host dicakup konfigurasi bawaan.
func TestBuildTypedVariants_FromReleaseMatrix(t *testing.T) {
	has := func(vs []typedVariant, goos, goarch string) bool {
		for _, v := range vs {
			joined := strings.Join(v.env, " ")
			if strings.Contains(joined, "GOOS="+goos) && strings.Contains(joined, "GOARCH="+goarch) {
				return true
			}
		}
		return false
	}
	win := buildTypedVariants("windows", "amd64")
	lin := buildTypedVariants("linux", "amd64")
	if has(win, "windows", "amd64") || !has(win, "linux", "amd64") || !has(win, "linux", "arm64") {
		t.Errorf("varian host windows/amd64 salah: %+v", win)
	}
	if has(lin, "linux", "amd64") || !has(lin, "linux", "arm64") || !has(lin, "windows", "amd64") {
		t.Errorf("varian host linux/amd64 salah: %+v", lin)
	}
	for _, vs := range [][]typedVariant{win, lin} {
		hasDevtools := false
		for _, v := range vs {
			if len(v.flags) == 1 && v.flags[0] == "-tags=devtools" {
				hasDevtools = true
			}
			if len(v.env) > 0 && !strings.Contains(strings.Join(v.env, " "), "CGO_ENABLED=0") {
				t.Errorf("varian lintas-target harus CGO_ENABLED=0: %+v", v)
			}
		}
		if !hasDevtools {
			t.Errorf("varian tag devtools hilang: %+v", vs)
		}
	}
}
