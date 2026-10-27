package contracttools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var exactVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// NFR-02/NFR-06: tool npm terpin exact (tanpa ^, ~, latest, atau rentang) dan terkunci di bun.lock.
func TestTooling_PinnedExactVersions(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(realRoot, "packages", "*", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join(realRoot, "web", "package.json"))
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // path dari glob repo
		if err != nil {
			t.Fatal(err)
		}
		var pkg struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
			for name, ver := range deps {
				if ver != "workspace:*" && !exactVersionRe.MatchString(ver) {
					t.Errorf("%s: %s = %q; wajib versi exact X.Y.Z atau workspace:*", f, name, ver)
				}
			}
		}
	}
}

// Tool kontrak wajib terdaftar di web/package.json (bukan diunduh ad hoc lewat bun x/npx).
func TestTooling_ContractToolsDeclared(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(realRoot, "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"@stoplight/spectral-core", "@stoplight/spectral-functions", "@stoplight/spectral-rulesets",
		"@stoplight/spectral-parsers", "@stoplight/spectral-ref-resolver", "yaml",
		"openapi-typescript", "json-schema-to-typescript",
	} {
		if pkg.DevDependencies[want] == "" {
			t.Errorf("web/package.json devDependencies tidak memuat %s", want)
		}
	}
}
