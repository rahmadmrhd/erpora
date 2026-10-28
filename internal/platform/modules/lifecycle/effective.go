package lifecycle

import (
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/resolver"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
)

// incompatibleIssues mengumpulkan issue ketidakcocokan milik key dan dependensi wajib transitifnya.
func incompatibleIssues(g *resolver.Graph, key string) []resolver.Issue {
	inc := g.Incompatible()
	out := append([]resolver.Issue(nil), inc[key]...)
	for _, d := range g.RequiredDependencies(key) {
		out = append(out, inc[d]...)
	}
	return out
}

// Effective menghitung status untuk snapshot, bootstrap, dan list. Modul katalog tanpa baris = available;
// baris tanpa modul di katalog = missing; modul (atau dependensi wajib transitifnya) yang tidak kompatibel
// dengan status tersimpan installed|enabled|disabled|upgrade_pending dilaporkan incompatible (keputusan 5).
// platform.core tidak dimuat.
func Effective(reg *registry.Registry, rows []store.Installation) map[string]runtime.Status {
	out := map[string]runtime.Status{}
	for _, e := range reg.Entries() {
		if e.Manifest.Key == registry.PlatformKey {
			continue
		}
		out[e.Manifest.Key] = runtime.StatusAvailable
	}
	for _, r := range rows {
		if r.ModuleKey == registry.PlatformKey {
			continue
		}
		if _, ok := reg.Entry(r.ModuleKey); !ok {
			out[r.ModuleKey] = runtime.StatusMissing
			continue
		}
		st := runtime.Status(r.Status)
		switch st {
		case runtime.StatusInstalled, runtime.StatusEnabled, runtime.StatusDisabled, runtime.StatusUpgradePending:
			if len(incompatibleIssues(reg.Graph(), r.ModuleKey)) > 0 {
				st = runtime.StatusIncompatible
			}
		}
		out[r.ModuleKey] = st
	}
	return out
}
