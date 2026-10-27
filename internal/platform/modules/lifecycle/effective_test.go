package lifecycle

import (
	"testing"

	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
)

func rowList(m map[string]runtime.Status) []store.Installation {
	var out []store.Installation
	for k, st := range mkRows(m) {
		_ = k
		out = append(out, st)
	}
	return out
}

func TestEffective(t *testing.T) {
	reg := prereqRegistry(t)
	t.Run("tanpa baris = available, tanpa platform.core", func(t *testing.T) {
		got := Effective(reg, nil)
		if got["acme.a"] != runtime.StatusAvailable {
			t.Errorf("acme.a = %q", got["acme.a"])
		}
		if _, ok := got[registry.PlatformKey]; ok {
			t.Error("platform.core tidak boleh dimuat")
		}
	})
	t.Run("baris tanpa modul = missing", func(t *testing.T) {
		got := Effective(reg, rowList(map[string]runtime.Status{"acme.gone": runtime.StatusEnabled}))
		if got["acme.gone"] != runtime.StatusMissing {
			t.Errorf("acme.gone = %q", got["acme.gone"])
		}
	})
	t.Run("incompatible langsung dan transitif", func(t *testing.T) {
		got := Effective(reg, rowList(map[string]runtime.Status{
			"acme.bad": runtime.StatusEnabled, "acme.uses-bad": runtime.StatusEnabled,
		}))
		if got["acme.bad"] != runtime.StatusIncompatible || got["acme.uses-bad"] != runtime.StatusIncompatible {
			t.Errorf("got = %v", got)
		}
	})
	t.Run("uninstalled dan failed tidak ditimpa", func(t *testing.T) {
		got := Effective(reg, rowList(map[string]runtime.Status{
			"acme.uses-bad": runtime.StatusUninstalled, "acme.bad": runtime.StatusFailed,
		}))
		if got["acme.uses-bad"] != runtime.StatusUninstalled || got["acme.bad"] != runtime.StatusFailed {
			t.Errorf("got = %v", got)
		}
	})
	t.Run("status biasa dipertahankan", func(t *testing.T) {
		got := Effective(reg, rowList(map[string]runtime.Status{"acme.a": runtime.StatusEnabled, "acme.b": runtime.StatusUpgradePending}))
		if got["acme.a"] != runtime.StatusEnabled || got["acme.b"] != runtime.StatusUpgradePending {
			t.Errorf("got = %v", got)
		}
	})
}
