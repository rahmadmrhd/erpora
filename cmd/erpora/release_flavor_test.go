//go:build !devtools

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// stripBanner: binary release tidak punya banner.
func stripBanner(s string) string { return s }

// Bukti binary release tidak memuat developer tools (PRD-000 FR-22.1).
func TestReleaseFlavor_NoDevtoolsPackages(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "erpora/cmd/erpora").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		t.Fatal("daftar dependensi kosong")
	}
	hasApp := false
	for _, l := range lines {
		if strings.HasPrefix(l, "erpora/internal/devtools") {
			t.Errorf("binary release memuat paket devtools: %s", l)
		}
		if l == "erpora/internal/app" {
			hasApp = true
		}
	}
	if !hasApp {
		t.Error("daftar dependensi tidak memuat erpora/internal/app (pemeriksaan tidak bermakna)")
	}
}

func TestReleaseFlavor_UnknownDevCommands(t *testing.T) {
	cases := [][]string{
		{"make:module", "x"}, {"make:handler"}, {"make:migration"}, {"make:entity"}, {"make:usecase"}, {"make:contract"}, {"make:command"}, {"make:repository"},
		{"db:fresh"}, {"db:seed"}, {"contracts:breaking"},
		{"contracts:lint"}, {"contracts:generate"}, {"module:check"}, {"doctor"},
		{"release:verify-tag", "v1"},
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		root := newRootCmd(&out, &errOut)
		root.SetArgs(args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%v: err = %v, want unknown command", args, err)
			continue
		}
		if got := exitCode(err); got != 1 {
			t.Errorf("%v: exitCode = %d, want 1", args, got)
		}
	}
}

func TestReleaseFlavor_VersionNoBanner(t *testing.T) {
	var out, errOut bytes.Buffer
	root := newRootCmd(&out, &errOut)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "flavor: release") {
		t.Errorf("stdout = %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr harus kosong, got %q", errOut.String())
	}
}
