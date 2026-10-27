// Package release memvalidasi tag rilis (PRD-000 FR-14): versi pada tag harus sama dengan versi di sumber.
package release

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/mod/modfile"
)

// tagRe: "modules/<dir>/v<semver>", "sdk/v<semver>", atau tag platform "v<semver>" (Q32 K-6). <dir> = slug tanpa tanda
// hubung (keputusan 13).
var tagRe = regexp.MustCompile(`^(?:modules/([a-z0-9]+)/|(sdk)/)?v(.+)$`)

// VerifyTag memeriksa tag rilis terhadap pohon kerja di root:
//   - "modules/<dir>/vX.Y.Z[-pre]": modules/<dir>/go.mod ada dengan path erpora/modules/<dir>, dan module.json
//     memuat version yang sama persis dengan versi tag;
//   - "sdk/vX.Y.Z[-pre]": const SDKVersion di sdk/module/version.go (dibaca lewat go/parser) sama dengan versi tag;
//   - "vX.Y.Z[-pre]" (platform): const PlatformVersion di internal/platform/buildinfo (go/parser) sama dengan versi tag.
//
// Tag lain atau versi yang bukan semver penuh ditolak. Pesan error menyebut nilai tag dan nilai sumber.
func VerifyTag(root, tag string) error {
	m := tagRe.FindStringSubmatch(tag)
	if m == nil {
		return fmt.Errorf("format tag %q tidak dikenal: pakai modules/<dir>/vX.Y.Z, sdk/vX.Y.Z, atau vX.Y.Z", tag)
	}
	ver := m[3]
	if _, err := semver.StrictNewVersion(ver); err != nil {
		return fmt.Errorf("format tag %q: versi %q bukan semver penuh (X.Y.Z[-prerelease])", tag, ver)
	}
	switch {
	case m[2] == "sdk":
		return verifyConst(filepath.Join(root, "sdk", "module", "version.go"), "SDKVersion", tag, ver, "SDKVersion (sdk/module/version.go)")
	case m[1] == "":
		return verifyConst(filepath.Join(root, "internal", "platform", "buildinfo", "buildinfo.go"), "PlatformVersion", tag, ver, "PlatformVersion (internal/platform/buildinfo)")
	}
	return verifyModule(root, tag, m[1], ver)
}

func verifyModule(root, tag, dir, ver string) error {
	base := filepath.Join(root, "modules", dir)
	data, err := os.ReadFile(filepath.Join(base, "go.mod")) //nolint:gosec // dir dibatasi regex [a-z0-9]+
	if err != nil {
		return fmt.Errorf("tag %s: modul %q tidak ditemukan (modules/%s/go.mod): %w", tag, dir, dir, err)
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil || f.Module == nil {
		return fmt.Errorf("tag %s: modules/%s/go.mod tidak terbaca", tag, dir)
	}
	if want := "erpora/modules/" + dir; f.Module.Mod.Path != want {
		return fmt.Errorf("tag %s: path module di go.mod = %q, seharusnya %q", tag, f.Module.Mod.Path, want)
	}
	raw, err := os.ReadFile(filepath.Join(base, "module.json")) //nolint:gosec // idem
	if err != nil {
		return fmt.Errorf("tag %s: %w", tag, err)
	}
	var mf struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &mf); err != nil {
		return fmt.Errorf("tag %s: modules/%s/module.json: %w", tag, dir, err)
	}
	if mf.Version != ver {
		return fmt.Errorf("tag %s: versi tag %s berbeda dari version di modules/%s/module.json (%s)", tag, ver, dir, mf.Version)
	}
	return nil
}

// verifyConst membaca const string name dari file Go (go/parser) dan membandingkannya dengan versi tag.
func verifyConst(path, name, tag, ver, label string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("tag %s: %w", tag, err)
	}
	got, ok := "", false
	for _, d := range file.Decls {
		gd, isGen := d.(*ast.GenDecl)
		if !isGen || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec) //nolint:errcheck,forcetypeassert // GenDecl CONST selalu berisi ValueSpec
			for i, n := range vs.Names {
				if n.Name != name || i >= len(vs.Values) {
					continue
				}
				if lit, isLit := vs.Values[i].(*ast.BasicLit); isLit && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						got, ok = v, true
					}
				}
			}
		}
	}
	if !ok {
		return fmt.Errorf("tag %s: const string %s tidak ditemukan di %s", tag, name, path)
	}
	if got != ver {
		return fmt.Errorf("tag %s: versi tag %s berbeda dari %s: %s", tag, ver, label, got)
	}
	return nil
}
