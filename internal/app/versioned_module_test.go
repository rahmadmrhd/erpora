//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing/fstest"

	"erpora/sdk/module"
)

// upgradeRecorder merekam panggilan Upgrade(from, to).
type upgradeRecorder struct{ calls [][2]string }

// versionedModule membungkus modul referensi sebagai "versi lain" untuk simulasi upgrade/downgrade
// (keputusan 23): versi manifest diganti (Raw ditulis ulang), migrasi dibatasi pada subset nama file,
// dan Upgrade merekam. Install menjadi no-op karena kode modul asli (versi 1.1.0) menyemai kolom 0002
// yang belum ada pada simulasi versi 1.0.0. Register dan Health didelegasikan.
type versionedModule struct {
	inner module.Module
	mf    module.Manifest
	migs  fs.FS
	rec   *upgradeRecorder
}

// versioned membuat pembungkus. migrations nil = semua migrasi modul asli.
func versioned(m module.Module, version string, migrations []string, rec *upgradeRecorder) module.Module {
	var doc map[string]any
	if err := json.Unmarshal(m.Manifest().Raw, &doc); err != nil {
		panic(err)
	}
	doc["version"] = version
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	mf, err := module.ParseManifest(raw)
	if err != nil {
		panic(err)
	}
	src := m.(module.MigrationSource).Migrations()
	migs := src
	if migrations != nil {
		mem := fstest.MapFS{}
		for _, name := range migrations {
			b, err := fs.ReadFile(src, name)
			if err != nil {
				panic(err)
			}
			mem[name] = &fstest.MapFile{Data: b}
		}
		migs = mem
	}
	vm := &versionedModule{inner: m, mf: mf, migs: migs, rec: rec}
	// Modul referensi ber-katalog (manifest memuat bagian i18n) harus tetap memenuhi aturan registry: manifest
	// i18n berpasangan dengan module.CatalogSource. Pembungkus hanya ber-CatalogSource bila modul aslinya begitu.
	if cs, ok := m.(module.CatalogSource); ok {
		return &versionedCatalogModule{versionedModule: vm, catalogs: cs}
	}
	return vm
}

// versionedCatalogModule menambah module.CatalogSource pada versionedModule (didelegasikan ke modul asli).
type versionedCatalogModule struct {
	*versionedModule
	catalogs module.CatalogSource
}

func (v *versionedCatalogModule) Catalogs() fs.FS { return v.catalogs.Catalogs() }

func (v *versionedModule) Manifest() module.Manifest         { return v.mf }
func (v *versionedModule) Register(r module.Registrar) error { return v.inner.Register(r) }
func (v *versionedModule) Migrations() fs.FS                 { return v.migs }

func (v *versionedModule) Install(context.Context, module.Env) error { return nil }

func (v *versionedModule) Health(ctx context.Context, env module.Env) error {
	if h, ok := v.inner.(module.HealthChecker); ok {
		return h.Health(ctx, env)
	}
	return nil
}

func (v *versionedModule) Upgrade(_ context.Context, _ module.Env, from, to string) error {
	if v.rec != nil {
		v.rec.calls = append(v.rec.calls, [2]string{from, to})
	}
	return nil
}
