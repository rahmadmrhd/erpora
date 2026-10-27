//go:build devtools

package main

import (
	"bytes"
	"strings"
	"testing"

	"erpora/internal/devtools"
)

// stripBanner membuang baris banner dev dari keluaran stderr gabungan.
func stripBanner(s string) string { return strings.ReplaceAll(s, devtools.Banner+"\n", "") }

// wantDevCommands: judul grup -> command dev yang harus terdaftar. Setiap task yang menambah command
// developer tools menambah barisnya di sini.
var wantDevCommands = map[string][]string{
	"Make":            {"make:migration", "make:module", "make:handler", "make:entity", "make:usecase", "make:contract", "make:command", "make:repository"},
	"Kontrak":         {"contracts:lint", "contracts:generate", "contracts:breaking"},
	"Modul":           {"module:check"},
	"Developer tools": {"doctor"},
}

func TestDevFlavor_BannerAndVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	root := newRootCmd(&out, &errOut)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(errOut.String(), devtools.Banner) {
		t.Errorf("stderr harus diawali banner dev, got %q", errOut.String())
	}
	if strings.Contains(out.String(), devtools.Banner) {
		t.Errorf("banner tidak boleh ke stdout: %q", out.String())
	}
	if !strings.Contains(out.String(), "flavor: dev") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestDevFlavor_ListGroups(t *testing.T) {
	var out, errOut bytes.Buffer
	root := newRootCmd(&out, &errOut)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Runtime:", "Inspect:", "Modul:"} {
		if !strings.Contains(out.String(), title) {
			t.Errorf("list tidak memuat grup %q:\n%s", title, out.String())
		}
	}
	for title, cmds := range wantDevCommands {
		if !strings.Contains(out.String(), title+":") {
			t.Errorf("list tidak memuat grup dev %q:\n%s", title, out.String())
		}
		for _, c := range cmds {
			if !strings.Contains(out.String(), c) {
				t.Errorf("list tidak memuat command %q:\n%s", c, out.String())
			}
		}
	}
}
