package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"erpora/internal/app"
)

// newTestRoot merakit root cobra dengan semua command developer tools (seperti devtools.Register) tanpa menjalankan DB.
func newTestRoot(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "erpora", SilenceUsage: true, SilenceErrors: true}
	for _, id := range []string{GroupMake, GroupDB, GroupContracts, GroupModule, GroupDevtools} {
		root.AddGroup(&cobra.Group{ID: id, Title: GroupTitle(id)})
	}
	Register(root, Options{})
	return root
}

// inRepo mengganti direktori kerja ke repo kosong yang hanya memuat go.work (cukup untuk repoRoot()).
func inRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte("go 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// exitCodeOf: kode dari *app.ExitError; 0 bila err nil; -1 bila error biasa (bukan ExitError, mis. galat argumen cobra
// atau galat generator, yang dipetakan main ke exit 1).
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var xe *app.ExitError
	if errors.As(err, &xe) {
		return xe.Code
	}
	return -1
}

// G-10: wiring cobra: aturan argumen, flag wajib, dan pemetaan error ke exit code (2 = penggunaan, selain itu error biasa).
func TestCommands_ArgumentRulesAndExitCodes(t *testing.T) {
	inRepo(t)
	cases := []struct {
		name     string
		args     []string
		wantCode int    // 0 sukses, 2 ExitError{2}, 1 ExitError{1}, -1 error biasa
		wantMsg  string // potongan pesan error (opsional)
	}{
		// make:migration
		{"migration tanpa argumen", []string{"make:migration"}, -1, "arg"},
		{"migration modul tanpa nama", []string{"make:migration", "inventory"}, 2, "make:migration <module> <name>"},
		{"migration terlalu banyak argumen", []string{"make:migration", "a", "b", "c"}, -1, "arg"},
		{"migration --platform dengan dua argumen", []string{"make:migration", "--platform", "control", "mod", "nama"}, 2, "hanya <name>"},
		{"migration --platform target tidak valid", []string{"make:migration", "--platform", "bogus", "nama"}, -1, "bogus"},
		{"migration modul tidak ada", []string{"make:migration", "tidakada", "nama"}, -1, "tidakada"},
		// make:handler
		{"handler tanpa --route dan --permission", []string{"make:handler", "inventory", "Approve"}, 2, "--route dan --permission"},
		{"handler hanya --route", []string{"make:handler", "inventory", "Approve", "--route", "GET /x"}, 2, "--route dan --permission"},
		{"handler hanya --permission", []string{"make:handler", "inventory", "Approve", "--permission", "inventory.x.read"}, 2, "--route dan --permission"},
		{"handler argumen kurang", []string{"make:handler", "inventory"}, -1, "arg"},
		{"handler modul tidak ada", []string{"make:handler", "tidakada", "Approve", "--route", "GET /items/{id}", "--permission", "tidakada.items.read"}, -1, "tidakada"},
		// make:* lain
		{"module tanpa nama", []string{"make:module"}, -1, "arg"},
		{"entity tanpa nama", []string{"make:entity", "inventory"}, -1, "arg"},
		{"usecase argumen kurang", []string{"make:usecase"}, -1, "arg"},
		{"command argumen berlebih", []string{"make:command", "a", "b", "c"}, -1, "arg"},
		// module:check, release:verify-tag, contracts
		{"module:check modul tidak dikenal", []string{"module:check", "tidakada"}, 2, "tidak dikenal"},
		{"release:verify-tag tanpa argumen", []string{"release:verify-tag"}, -1, "arg"},
		{"release:verify-tag format salah", []string{"release:verify-tag", "v1"}, 1, "format"},
		{"release:verify-tag platform tanpa buildinfo", []string{"release:verify-tag", "v1.0.0"}, 1, "buildinfo"},
		{"release:verify-tag modul tidak ada", []string{"release:verify-tag", "modules/tidakada/v1.0.0"}, 1, "tidakada"},
		{"contracts:breaking target tidak dikenal", []string{"contracts:breaking", "tidakada"}, -2, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantCode == -2 { // butuh kontrak platform di repo; dicakup test contracttools
				t.Skip("dicakup oleh test contracttools")
			}
			root := newTestRoot(t)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)
			err := root.Execute()
			if got := exitCodeOf(err); got != tc.wantCode {
				t.Fatalf("exit code = %d (err=%v), want %d", got, err, tc.wantCode)
			}
			if tc.wantMsg != "" && (err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantMsg))) {
				t.Errorf("err = %v, want memuat %q", err, tc.wantMsg)
			}
		})
	}
}

// G-5 (Q32): --force ditolak (exit 2, pesan jelas) untuk make:module, make:handler, dan make:command; tidak ada yang ditulis.
// Dijalankan di repo asli: rencana dibangun tetapi ditolak sebelum Apply.
func TestCommands_ForceRejectedForModuleHandlerCommand(t *testing.T) {
	cases := [][]string{
		{"make:module", "zzforcedemo", "--force"},
		{"make:handler", "example", "ZzForce", "--route", "GET /zz-forces/{id}", "--permission", "example.zz.read", "--force"},
		{"make:command", "example", "zz-force", "--force"},
	}
	for _, args := range cases {
		root := newTestRoot(t)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		if exitCodeOf(err) != 2 || !strings.Contains(err.Error(), "--force tidak berlaku") {
			t.Errorf("%v: err = %v (exit %d), want exit 2 dengan pesan --force tidak berlaku", args, err, exitCodeOf(err))
		}
	}
}

// --dry-run dan --force terdaftar di setiap make:* (flag bersama) dan flag spesifik ada di tempatnya.
func TestCommands_FlagsRegistered(t *testing.T) {
	root := newTestRoot(t)
	want := map[string][]string{
		"make:module":    {"frontend", "depends", "publisher", "force", "dry-run"},
		"make:migration": {"platform", "force", "dry-run"},
		"make:handler":   {"route", "permission", "force", "dry-run"},
		"make:usecase":   {"query", "force", "dry-run"},
		"make:entity":    {"force", "dry-run"},
		"db:fresh":       {"seed", "yes"},
	}
	for name, flags := range want {
		var found *cobra.Command
		for _, c := range root.Commands() {
			if c.Name() == name {
				found = c
			}
		}
		if found == nil {
			t.Errorf("command %s tidak terdaftar", name)
			continue
		}
		for _, f := range flags {
			if found.Flags().Lookup(f) == nil {
				t.Errorf("%s: flag --%s tidak ada", name, f)
			}
		}
	}
}
