package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "sdk/module/version.go", "package module\n\n// SDKVersion adalah versi SDK.\nconst SDKVersion = \"0.1.0\"\n")
	write(t, root, "modules/example/go.mod", "module erpora/modules/example\n\ngo 1.27\n")
	write(t, root, "modules/example/module.json", `{"key":"acme.example","slug":"example","version":"1.0.0"}`)
	write(t, root, "modules/exampleaddon/go.mod", "module erpora/modules/exampleaddon\n\ngo 1.27\n")
	write(t, root, "modules/exampleaddon/module.json", `{"key":"acme.example-addon","slug":"example-addon","version":"1.0.0"}`)
	write(t, root, "modules/rc/go.mod", "module erpora/modules/rc\n\ngo 1.27\n")
	write(t, root, "modules/rc/module.json", `{"key":"acme.rc","slug":"rc","version":"1.1.0-rc.1"}`)
	write(t, root, "modules/wrongpath/go.mod", "module erpora/modules/other\n\ngo 1.27\n")
	write(t, root, "modules/wrongpath/module.json", `{"key":"acme.wrongpath","slug":"wrongpath","version":"1.0.0"}`)
	return root
}

func TestVerifyTag_Table(t *testing.T) {
	root := fixture(t)
	tests := []struct {
		tag     string
		wantErr string // "" = sukses; selain itu potongan pesan error
	}{
		{"modules/example/v1.0.0", ""},
		{"modules/example/v1.0.1", "1.0.0"},
		{"modules/exampleaddon/v1.0.0", ""},
		{"modules/missing/v1.0.0", "missing"},
		{"modules/wrongpath/v1.0.0", "erpora/modules/other"},
		{"sdk/v0.1.0", ""},
		{"sdk/v0.2.0", "0.1.0"},
		{"v1.0.0", "format"},
		{"modules/example/1.0.0", "format"},
		{"modules/example/v1.0", "format"},
		{"modules/../sdk/v0.1.0", "format"},
		{"modules/rc/v1.1.0-rc.1", ""},
		{"modules/rc/v1.1.0", "1.1.0-rc.1"},
		// K-11: metakarakter shell pada tag ditolak oleh regex format, tidak pernah sampai ke pencocokan versi.
		{"modules/example/v1.0.0;id", "format"},
		{"modules/example/v1.0.0$(id)", "format"},
		{"modules/example/v1.0.0`id`", "format"},
		{"modules/ex ample/v1.0.0", "format"},
	}
	for _, tc := range tests {
		t.Run(tc.tag, func(t *testing.T) {
			err := VerifyTag(root, tc.tag)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want memuat %q", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyTag_RepositoryTagsMatchCurrentVersions(t *testing.T) {
	// Tag untuk versi yang kini ada di repo harus lolos: menjaga parser tetap cocok dengan file asli.
	if err := VerifyTag("../../..", "modules/example/v1.0.0"); err != nil {
		t.Errorf("modules/example: %v", err)
	}
	if err := VerifyTag("../../..", "sdk/v0.1.0"); err != nil {
		t.Errorf("sdk: %v", err)
	}
}
