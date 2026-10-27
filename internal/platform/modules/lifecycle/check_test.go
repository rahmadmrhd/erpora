package lifecycle

import (
	"errors"
	"slices"
	"testing"

	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/resolver"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
)

// prereqRegistry: a, b (wajib -> a), c (opsional -> a), req (required), bad (platform ^9.0), uses-bad (wajib -> bad).
func prereqRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	rec := &fxRec{}
	return buildFxRegistry(t,
		newFx(rec, fxSpec{key: "acme.a"}),
		newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}}),
		newFx(rec, fxSpec{key: "acme.c", deps: []moduleDep{optDep("acme.a")}}),
		newFx(rec, fxSpec{key: "acme.req", required: true}),
		newFx(rec, fxSpec{key: "acme.bad", platformC: "^9.0"}),
		newFx(rec, fxSpec{key: "acme.uses-bad", deps: []moduleDep{dep("acme.bad")}}),
	)
}

func mkRows(m map[string]runtime.Status) map[string]store.Installation {
	out := map[string]store.Installation{}
	for k, st := range m {
		out[k] = store.Installation{ModuleKey: k, Status: string(st), InstalledVersion: "1.0.0"}
	}
	return out
}

func wantCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var le *Error
	if !errors.As(err, &le) {
		t.Fatalf("err = %v, want *Error %s", err, code)
	}
	if le.Code != code {
		t.Fatalf("code = %s (%v), want %s", le.Code, err, code)
	}
	return le
}

func TestCheck_Prerequisites(t *testing.T) {
	reg := prereqRegistry(t)
	check := func(op Op, key string, rows map[string]runtime.Status) error {
		return Check(op, key, View{Reg: reg, Rows: mkRows(rows)})
	}

	t.Run("enable b saat a disabled", func(t *testing.T) {
		err := check(OpEnable, "acme.b", map[string]runtime.Status{"acme.a": runtime.StatusDisabled, "acme.b": runtime.StatusInstalled})
		le := wantCode(t, err, CodeDependencyNotReady)
		if !slices.Equal(le.Related, []string{"acme.a"}) {
			t.Errorf("Related = %v", le.Related)
		}
	})
	t.Run("enable b saat a enabled", func(t *testing.T) {
		if err := check(OpEnable, "acme.b", map[string]runtime.Status{"acme.a": runtime.StatusEnabled, "acme.b": runtime.StatusInstalled}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("install b saat a belum terpasang", func(t *testing.T) {
		le := wantCode(t, check(OpInstall, "acme.b", nil), CodeDependencyNotReady)
		if !slices.Equal(le.Related, []string{"acme.a"}) {
			t.Errorf("Related = %v", le.Related)
		}
	})
	t.Run("install b saat a disabled diizinkan", func(t *testing.T) {
		if err := check(OpInstall, "acme.b", map[string]runtime.Status{"acme.a": runtime.StatusDisabled}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("disable a saat b enabled", func(t *testing.T) {
		err := check(OpDisable, "acme.a", map[string]runtime.Status{"acme.a": runtime.StatusEnabled, "acme.b": runtime.StatusEnabled})
		le := wantCode(t, err, CodeDependentsActive)
		if !slices.Equal(le.Related, []string{"acme.b"}) {
			t.Errorf("Related = %v", le.Related)
		}
	})
	t.Run("disable a saat hanya c opsional enabled", func(t *testing.T) {
		if err := check(OpDisable, "acme.a", map[string]runtime.Status{"acme.a": runtime.StatusEnabled, "acme.c": runtime.StatusEnabled}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("required tidak boleh disable/uninstall", func(t *testing.T) {
		rows := map[string]runtime.Status{"acme.req": runtime.StatusEnabled}
		wantCode(t, check(OpDisable, "acme.req", rows), CodeModuleRequired)
		wantCode(t, check(OpUninstall, "acme.req", rows), CodeModuleRequired)
	})
	t.Run("install bad", func(t *testing.T) {
		err := check(OpInstall, "acme.bad", nil)
		wantCode(t, err, CodeModuleIncompatible)
		var re *resolver.ResolutionError
		if !errors.As(err, &re) || len(re.Issues) == 0 || re.Issues[0].Code != resolver.CodePlatformIncompatible {
			t.Errorf("Unwrap = %v", re)
		}
	})
	t.Run("install uses-bad menyebut bad", func(t *testing.T) {
		le := wantCode(t, check(OpInstall, "acme.uses-bad", map[string]runtime.Status{"acme.bad": runtime.StatusEnabled}), CodeModuleIncompatible)
		found := false
		for _, is := range le.Issues {
			if is.Module == "acme.bad" {
				found = true
			}
		}
		if !found {
			t.Errorf("Issues = %+v", le.Issues)
		}
	})
	t.Run("disable bad yang enabled tetap boleh", func(t *testing.T) {
		if err := check(OpDisable, "acme.bad", map[string]runtime.Status{"acme.bad": runtime.StatusEnabled}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("uninstall a saat b disabled", func(t *testing.T) {
		le := wantCode(t, check(OpUninstall, "acme.a", map[string]runtime.Status{"acme.a": runtime.StatusDisabled, "acme.b": runtime.StatusDisabled}), CodeDependentsActive)
		if !slices.Equal(le.Related, []string{"acme.b"}) {
			t.Errorf("Related = %v", le.Related)
		}
	})
	t.Run("uninstall a saat b uninstalled", func(t *testing.T) {
		if err := check(OpUninstall, "acme.a", map[string]runtime.Status{"acme.a": runtime.StatusDisabled, "acme.b": runtime.StatusUninstalled}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("upgrade b saat versi a di bawah constraint", func(t *testing.T) {
		rows := mkRows(map[string]runtime.Status{"acme.a": runtime.StatusEnabled, "acme.b": runtime.StatusUpgradePending})
		a := rows["acme.a"]
		a.InstalledVersion = "0.9.0"
		rows["acme.a"] = a
		err := Check(OpUpgrade, "acme.b", View{Reg: reg, Rows: rows})
		wantCode(t, err, CodeDependencyNotReady)
	})
	t.Run("key tidak dikenal", func(t *testing.T) {
		wantCode(t, check(OpInstall, registry.PlatformKey, nil), CodeModuleNotFound)
		wantCode(t, check(OpEnable, "acme.nope", nil), CodeModuleNotFound)
	})
}

// B-2 (Q32): invarian "modul enabled tidak punya dependensi wajib yang tidak enabled" dijaga juga oleh upgrade/retry
// yang akan mengembalikan modul ke enabled, dan disable menghitung dependen upgrade_pending/failed yang sebelumnya enabled.
func TestCheck_DependencyInvariantOnUpgradeAndDisable(t *testing.T) {
	reg := prereqRegistry(t)
	rowsOf := func(a, b runtime.Status, bPrev runtime.Status) map[string]store.Installation {
		rows := mkRows(map[string]runtime.Status{"acme.a": a, "acme.b": b})
		br := rows["acme.b"]
		br.PreviousStatus = string(bPrev)
		rows["acme.b"] = br
		return rows
	}
	t.Run("upgrade b yang akan kembali enabled saat a disabled ditolak", func(t *testing.T) {
		err := Check(OpUpgrade, "acme.b", View{Reg: reg, Rows: rowsOf(runtime.StatusDisabled, runtime.StatusUpgradePending, runtime.StatusEnabled)})
		le := wantCode(t, err, CodeDependencyNotReady)
		if !slices.Equal(le.Related, []string{"acme.a"}) {
			t.Errorf("Related = %v", le.Related)
		}
	})
	t.Run("upgrade b yang akan kembali installed tidak butuh a enabled", func(t *testing.T) {
		if err := Check(OpUpgrade, "acme.b", View{Reg: reg, Rows: rowsOf(runtime.StatusDisabled, runtime.StatusUpgradePending, runtime.StatusInstalled)}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("upgrade b yang akan kembali enabled saat a enabled lolos", func(t *testing.T) {
		if err := Check(OpUpgrade, "acme.b", View{Reg: reg, Rows: rowsOf(runtime.StatusEnabled, runtime.StatusUpgradePending, runtime.StatusEnabled)}); err != nil {
			t.Fatal(err)
		}
	})
	for _, st := range []runtime.Status{runtime.StatusUpgradePending, runtime.StatusFailed} {
		t.Run("disable a saat b "+string(st)+" dengan previous enabled ditolak", func(t *testing.T) {
			err := Check(OpDisable, "acme.a", View{Reg: reg, Rows: rowsOf(runtime.StatusEnabled, st, runtime.StatusEnabled)})
			le := wantCode(t, err, CodeDependentsActive)
			if !slices.Equal(le.Related, []string{"acme.b"}) {
				t.Errorf("Related = %v", le.Related)
			}
		})
		t.Run("disable a saat b "+string(st)+" dengan previous installed lolos", func(t *testing.T) {
			if err := Check(OpDisable, "acme.a", View{Reg: reg, Rows: rowsOf(runtime.StatusEnabled, st, runtime.StatusInstalled)}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
