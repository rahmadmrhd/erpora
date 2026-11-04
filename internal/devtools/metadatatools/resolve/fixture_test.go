package resolve

import (
	"os"
	"testing"

	"erpora/internal/devtools/metadatatools/load"
)

// Modul fixture (testdata/repo) dibaca langsung (hanya-baca) dan harus terselesaikan tanpa temuan bersama
// metadata bawaan SDK.
func TestFixtureModulesResolve(t *testing.T) {
	c, ps := NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	root := os.DirFS("../testdata/repo/modules")
	for _, m := range []struct {
		mod  Module
		path string
	}{
		{Module{Slug: "fixturecore"}, "fixturecore/metadata"},
		{Module{Slug: "fixturesales", Dependencies: []string{"fixturecore"}}, "fixturesales/metadata"},
	} {
		docs, lps := load.LoadFS(root, m.path)
		if len(lps) != 0 || len(docs) == 0 {
			t.Fatalf("%s: dokumen=%d temuan=%v", m.path, len(docs), lps)
		}
		if aps := c.AddModule(m.mod, docs...); len(aps) != 0 {
			t.Fatalf("%s: %v", m.path, aps)
		}
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatalf("temuan resolusi fixture: %v", res.Problems)
	}
	inv := res.Resource("fixturesales.invoice")
	if inv == nil {
		t.Fatal("fixturesales.invoice tidak terselesaikan")
	}
	if inv.Field("shippingStreet") == nil || inv.Field("status") == nil || inv.LockWhen == nil {
		t.Errorf("invoice = %s", joined(inv))
	}
	if len(res.Users("fixturecore.address")) == 0 {
		t.Error("fixturecore.address dipakai lintas modul")
	}
}
