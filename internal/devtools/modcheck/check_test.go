package modcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheck_RepositoryClean(t *testing.T) {
	got, err := Check("../../..", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("repo harus bersih, temuan: %+v", got)
	}
}

func TestCheck_UnknownModule(t *testing.T) {
	if _, err := Check("../../..", "tidak-ada"); err == nil || !strings.Contains(err.Error(), "tidak dikenal") {
		t.Fatalf("err = %v", err)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fixture membuat workspace mini: modul alpha dan beta (beta bergantung pada alpha), keduanya valid.
// Parameter menyesuaikan satu pelanggaran per kasus.
type fixture struct {
	alphaVersion      string // default 1.0.0
	alphaCapabilities string // isi array JSON, default "alpha.items.read"
	betaDeclaresDep   bool
	alphaGo           string // direktif go, default 1.27
}

func (f fixture) build(t *testing.T) string {
	t.Helper()
	if f.alphaVersion == "" {
		f.alphaVersion = "1.0.0"
	}
	if f.alphaCapabilities == "" {
		f.alphaCapabilities = `"alpha.items.read"`
	}
	if f.alphaGo == "" {
		f.alphaGo = "1.27"
	}
	root := t.TempDir()
	write(t, root, "go.work", "go 1.27\n\ntoolchain go1.27.3\n\nuse (\n\t./modules/alpha\n\t./modules/beta\n)\n")
	manifest := func(key, slug, version, caps, deps string) string {
		return `{
  "schema_version": "1",
  "key": "` + key + `",
  "slug": "` + slug + `",
  "name": "` + slug + `",
  "version": "` + version + `",
  "publisher": { "id": "acme", "name": "ACME" },
  "trust": "builtin",
  "compatibility": { "platform": "^0.1", "sdk": "^0.1" },
  "dependencies": [
    { "module": "platform.core", "version": "^0.1", "optional": false }` + deps + `
  ],
  "capabilities": [` + caps + `],
  "migrations": { "strategy": "none" },
  "offline": { "mode": "online-only" }
}
`
	}
	write(t, root, "modules/alpha/go.mod", "module erpora/modules/alpha\n\ngo "+f.alphaGo+"\n\ntoolchain go1.27.3\n")
	write(t, root, "modules/alpha/a.go", "package alpha\n")
	write(t, root, "modules/alpha/module.json", manifest("acme.alpha", "alpha", f.alphaVersion, f.alphaCapabilities, ""))
	deps := ""
	if f.betaDeclaresDep {
		deps = `,
    { "module": "acme.alpha", "version": "^1.0", "optional": false }`
	}
	write(t, root, "modules/beta/go.mod", "module erpora/modules/beta\n\ngo 1.27\n\ntoolchain go1.27.3\n\nrequire erpora/modules/alpha v0.0.0\n")
	write(t, root, "modules/beta/b.go", "package beta\n")
	write(t, root, "modules/beta/module.json", manifest("acme.beta", "beta", "1.0.0", `"beta.items.read"`, deps))
	return root
}

func TestCheck_Fixtures(t *testing.T) {
	t.Setenv("GOWORK", "") // fixture membawa go.work sendiri
	tests := []struct {
		name     string
		fx       fixture
		wantMod  string
		wantRule string
	}{
		{"versi bukan semver", fixture{alphaVersion: "1.0", betaDeclaresDep: true}, "acme.alpha", RuleSchema},
		{"capability berawalan lain", fixture{alphaCapabilities: `"other.x.read"`, betaDeclaresDep: true}, "acme.alpha", RuleCapabilityPrefix},
		{"require tanpa dependensi manifest", fixture{betaDeclaresDep: false}, "acme.beta", RuleD8},
		{"go tidak sama", fixture{alphaGo: "1.26", betaDeclaresDep: true}, "acme.alpha", RuleGoVersion},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(tc.fx.build(t), "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range got {
				if f.Module == tc.wantMod && f.Rule == tc.wantRule {
					found = true
				}
			}
			if !found {
				t.Fatalf("temuan %s/%s tidak ada: %+v", tc.wantMod, tc.wantRule, got)
			}
		})
	}
}

func TestCheck_FixtureCleanAndOnly(t *testing.T) {
	t.Setenv("GOWORK", "")
	root := fixture{betaDeclaresDep: true}.build(t)
	got, err := Check(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("fixture dasar harus bersih: %+v", got)
	}
	// only dicocokkan terhadap direktori, slug, atau key; hanya modul itu yang dilaporkan.
	root = fixture{alphaVersion: "1.0", betaDeclaresDep: true}.build(t)
	for _, only := range []string{"alpha", "acme.alpha"} {
		got, err = Check(root, only)
		if err != nil || len(got) == 0 {
			t.Fatalf("only=%s: got=%+v err=%v", only, got, err)
		}
	}
	got, err = Check(root, "beta")
	if err != nil || len(got) != 0 {
		t.Fatalf("only=beta harus bersih: got=%+v err=%v", got, err)
	}
}
