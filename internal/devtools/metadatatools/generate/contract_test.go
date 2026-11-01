package generate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erpora/internal/devtools/contracttools"
	"erpora/internal/devtools/metadatatools/internal/testrepo"
)

// TestInjectedSchemasPassContractTooling membuktikan bahwa skema yang disuntikkan lolos perkakas kontrak yang
// ada: pemuatan dokumen, linter Spectral (aturan erp-*), dan pembangkit tipe TS (contracts:generate). Memakai
// root salinan fixture dengan dokumen common dan platform dari repo nyata; skrip dan ruleset dari repo nyata.
func TestInjectedSchemasPassContractTooling(t *testing.T) {
	if testing.Short() {
		t.Skip("bun (lambat)")
	}
	bun, err := contracttools.FindBun()
	if err != nil {
		if os.Getenv("ERP_REQUIRE_BUN") == "1" {
			t.Fatalf("ERP_REQUIRE_BUN=1 tetapi bun tidak ada: %v", err)
		}
		t.Skip("bun tidak ada")
	}
	real := testrepo.RepoRoot(t)
	root, cat := fixtureRepo(t)
	for _, f := range []string{"common.yaml", "platform.yaml"} {
		data, err := os.ReadFile(filepath.Join(real, "contracts", "openapi", f)) //nolint:gosec // repo sendiri
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "contracts/openapi/"+f, string(data))
	}
	writeFile(t, root, "modules/fixturesales/web/package.json", `{"name":"@erpora/module-fixturesales"}`)
	if _, err := Generate(root, cat, Options{Checker: okChecker{}}); err != nil {
		t.Fatal(err)
	}
	repo, err := contracttools.LoadRepo(root)
	if err != nil {
		t.Fatalf("dokumen kontrak hasil suntikan tidak dapat dimuat: %v", err)
	}
	repo.ToolsRoot = real

	res, err := contracttools.Lint(context.Background(), repo, "fixturesales", bun)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Severity == contracttools.SeverityInfo {
			continue
		}
		t.Errorf("lint %s:%d %s: %s", r.File, r.Line, r.Rule, r.Message)
	}
	if res, err = contracttools.Lint(context.Background(), repo, "fixturecore", bun); err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Severity != contracttools.SeverityInfo {
			t.Errorf("lint %s:%d %s: %s", r.File, r.Line, r.Rule, r.Message)
		}
	}

	files, err := contracttools.Generate(context.Background(), repo, "fixturesales", bun)
	if err != nil {
		t.Fatalf("contracts:generate gagal atas skema bangkitan: %v", err)
	}
	ts := ""
	for _, f := range files {
		if strings.HasSuffix(f.Path, "contract.gen.ts") {
			ts = string(f.Content)
		}
	}
	for _, want := range []string{"InvoiceCreate", "InvoiceUpdate", "Invoice:"} {
		if !strings.Contains(ts, want) {
			t.Errorf("tipe TS kontrak tidak memuat %q", want)
		}
	}
}
