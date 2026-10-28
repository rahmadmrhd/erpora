package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"erpora/internal/platform/modules/compatibility"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
)

// View adalah pandangan baca atas katalog dan status tersimpan, dasar Check.
type View struct {
	Reg  *registry.Registry
	Rows map[string]store.Installation
}

// Stored mengembalikan status tersimpan modul ("available" bila tidak ada baris). platform.core selalu enabled.
func (v View) Stored(key string) runtime.Status {
	if key == registry.PlatformKey {
		return runtime.StatusEnabled
	}
	if r, ok := v.Rows[key]; ok {
		return runtime.Status(r.Status)
	}
	return runtime.StatusAvailable
}

// requiredDeps mengembalikan dependensi wajib langsung (tanpa platform.core), terurut.
func requiredDeps(reg *registry.Registry, key string) []string {
	e, ok := reg.Entry(key)
	if !ok {
		return nil
	}
	var out []string
	for _, d := range e.Manifest.Dependencies {
		if !d.Optional && d.Module != registry.PlatformKey {
			out = append(out, d.Module)
		}
	}
	slices.Sort(out)
	return out
}

// requiredDependents mengembalikan dependen wajib langsung, terurut.
func requiredDependents(reg *registry.Registry, key string) []string {
	var out []string
	for _, e := range reg.Entries() {
		for _, d := range e.Manifest.Dependencies {
			if d.Module == key && !d.Optional {
				out = append(out, e.Manifest.Key)
			}
		}
	}
	slices.Sort(out)
	return out
}

func notReady(key, msg string, related []string) *Error {
	return &Error{Code: CodeDependencyNotReady, Module: key, Message: msg, Related: related}
}

// Check memeriksa prasyarat FR-19.2 di luar tabel transisi (keputusan 5 dan 15). Error pertama dikembalikan;
// khusus module_incompatible, seluruh issue dikumpulkan.
func Check(op Op, key string, v View) error {
	e, ok := v.Reg.Entry(key)
	if !ok || key == registry.PlatformKey {
		return &Error{Code: CodeModuleNotFound, Module: key, Message: "modul tidak ada di katalog"}
	}
	g := v.Reg.Graph()

	if op == OpInstall || op == OpEnable || op == OpUpgrade {
		if issues := incompatibleIssues(g, key); len(issues) > 0 {
			var names []string
			for _, is := range issues {
				names = append(names, is.String())
			}
			return &Error{Code: CodeModuleIncompatible, Module: key, Message: strings.Join(names, "; "), Issues: issues}
		}
	}
	if (op == OpDisable || op == OpUninstall) && e.Manifest.Required {
		return &Error{Code: CodeModuleRequired, Module: key, Message: "modul wajib tidak boleh dinonaktifkan atau di-uninstall"}
	}

	switch op {
	case OpInstall:
		var bad []string
		for _, d := range requiredDeps(v.Reg, key) {
			switch v.Stored(d) {
			case runtime.StatusInstalled, runtime.StatusEnabled, runtime.StatusDisabled:
			default:
				bad = append(bad, d)
			}
		}
		if len(bad) > 0 {
			return notReady(key, "dependensi wajib belum ter-install: "+strings.Join(bad, ", "), bad)
		}
	case OpEnable:
		var bad []string
		for _, d := range requiredDeps(v.Reg, key) {
			if v.Stored(d) != runtime.StatusEnabled {
				bad = append(bad, d)
			}
		}
		if len(bad) > 0 {
			return notReady(key, "dependensi wajib belum enabled: "+strings.Join(bad, ", "), bad)
		}
	case OpDisable:
		var active []string
		for _, d := range requiredDependents(v.Reg, key) {
			if v.Stored(d) == runtime.StatusEnabled {
				active = append(active, d)
			}
		}
		if len(active) > 0 {
			return &Error{Code: CodeDependentsActive, Module: key,
				Message: "modul yang bergantung masih enabled: " + strings.Join(active, ", "), Related: active}
		}
	case OpUninstall:
		var active []string
		for _, d := range requiredDependents(v.Reg, key) {
			if r, ok := v.Rows[d]; ok && runtime.Status(r.Status) != runtime.StatusUninstalled {
				active = append(active, d)
			}
		}
		if len(active) > 0 {
			return &Error{Code: CodeDependentsActive, Module: key,
				Message: "modul yang bergantung masih ter-install: " + strings.Join(active, ", "), Related: active}
		}
	case OpUpgrade:
		for _, d := range e.Manifest.Dependencies {
			if d.Optional || d.Module == registry.PlatformKey {
				continue
			}
			ver := v.Rows[d.Module].InstalledVersion
			okVer, err := compatibility.Satisfies(ver, d.Version)
			if err != nil || !okVer {
				return notReady(key, fmt.Sprintf("dependensi %s versi ter-install %q tidak memenuhi constraint %s", d.Module, ver, d.Version), []string{d.Module})
			}
		}
	}
	return nil
}
