package lifecycle

import (
	"errors"
	"testing"

	"erpora/internal/platform/modules/runtime"
)

var allStatuses = []runtime.Status{
	runtime.StatusAvailable, runtime.StatusInstalled, runtime.StatusEnabled, runtime.StatusDisabled,
	runtime.StatusUpgradePending, runtime.StatusFailed, runtime.StatusUninstalled,
	runtime.StatusMissing, runtime.StatusIncompatible,
}

type tr struct {
	to   runtime.Status
	noop bool
}

func TestTransition_Table(t *testing.T) {
	cases := map[Op]map[runtime.Status]tr{
		OpInstall: {
			runtime.StatusAvailable:   {runtime.StatusInstalled, false},
			runtime.StatusUninstalled: {runtime.StatusInstalled, false},
			runtime.StatusInstalled:   {runtime.StatusInstalled, true},
			runtime.StatusEnabled:     {runtime.StatusEnabled, true},
			runtime.StatusDisabled:    {runtime.StatusDisabled, true},
		},
		OpEnable: {
			runtime.StatusInstalled: {runtime.StatusEnabled, false},
			runtime.StatusDisabled:  {runtime.StatusEnabled, false},
			runtime.StatusEnabled:   {runtime.StatusEnabled, true},
		},
		OpDisable: {
			runtime.StatusEnabled:  {runtime.StatusDisabled, false},
			runtime.StatusDisabled: {runtime.StatusDisabled, true},
		},
		OpUpgrade: {
			// status tujuan = previous_status, dipasang pemanggil
			runtime.StatusUpgradePending: {"", false},
		},
		OpUninstall: {
			runtime.StatusInstalled:   {runtime.StatusUninstalled, false},
			runtime.StatusDisabled:    {runtime.StatusUninstalled, false},
			runtime.StatusUninstalled: {runtime.StatusUninstalled, true},
		},
	}
	for op, byFrom := range cases {
		for _, from := range allStatuses {
			to, noop, err := Transition(op, from, false)
			want, ok := byFrom[from]
			if !ok {
				var le *Error
				if !errors.As(err, &le) || le.Code != CodeInvalidTransition {
					t.Errorf("%s dari %s: err = %v, want invalid_transition", op, from, err)
				}
				continue
			}
			if err != nil || to != want.to || noop != want.noop {
				t.Errorf("%s dari %s = (%q, %v, %v), want (%q, %v)", op, from, to, noop, err, want.to, want.noop)
			}
		}
	}
}

func TestTransition_UpgradeSameVersionIsNoop(t *testing.T) {
	for _, from := range []runtime.Status{runtime.StatusInstalled, runtime.StatusEnabled, runtime.StatusDisabled} {
		to, noop, err := Transition(OpUpgrade, from, true)
		if err != nil || !noop || to != from {
			t.Errorf("upgrade %s sameVersion = (%q, %v, %v)", from, to, noop, err)
		}
		if _, _, err := Transition(OpUpgrade, from, false); err == nil {
			t.Errorf("upgrade %s tanpa sameVersion harus invalid_transition", from)
		}
	}
}

func TestTransition_RetryNotHandled(t *testing.T) {
	for _, from := range allStatuses {
		_, _, err := Transition(OpRetry, from, false)
		var le *Error
		if !errors.As(err, &le) || le.Code != CodeInvalidTransition {
			t.Errorf("retry dari %s: err = %v", from, err)
		}
	}
}
