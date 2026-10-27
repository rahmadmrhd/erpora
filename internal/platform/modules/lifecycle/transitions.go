package lifecycle

import (
	"fmt"

	"erpora/internal/platform/modules/runtime"
)

// Op adalah operasi lifecycle.
type Op string

// Operasi lifecycle.
const (
	OpInstall   Op = "install"
	OpEnable    Op = "enable"
	OpDisable   Op = "disable"
	OpUpgrade   Op = "upgrade"
	OpUninstall Op = "uninstall"
	OpRetry     Op = "retry"
)

func invalidTransition(op Op, from runtime.Status) error {
	return &Error{Code: CodeInvalidTransition, Message: fmt.Sprintf("operasi %s tidak diizinkan dari status %s", op, from)}
}

// Transition menerapkan tabel transisi FR-19.2. from adalah status tersimpan ("available" bila tidak ada baris).
// noop true berarti status tujuan sudah tercapai (keputusan 2): tidak ada operasi, event, atau revision.
// Untuk upgrade dari upgrade_pending, to dikosongkan: pemanggil memasang previous_status.
// OpRetry tidak ditangani di sini (lihat Service.Retry) dan selalu invalid_transition.
func Transition(op Op, from runtime.Status, sameVersion bool) (to runtime.Status, noop bool, err error) {
	switch op {
	case OpInstall:
		switch from {
		case runtime.StatusAvailable, runtime.StatusUninstalled:
			return runtime.StatusInstalled, false, nil
		case runtime.StatusInstalled, runtime.StatusEnabled, runtime.StatusDisabled:
			return from, true, nil
		}
	case OpEnable:
		switch from {
		case runtime.StatusInstalled, runtime.StatusDisabled:
			return runtime.StatusEnabled, false, nil
		case runtime.StatusEnabled:
			return from, true, nil
		}
	case OpDisable:
		switch from {
		case runtime.StatusEnabled:
			return runtime.StatusDisabled, false, nil
		case runtime.StatusDisabled:
			return from, true, nil
		}
	case OpUpgrade:
		switch from {
		case runtime.StatusUpgradePending:
			return "", false, nil
		case runtime.StatusInstalled, runtime.StatusEnabled, runtime.StatusDisabled:
			if sameVersion {
				return from, true, nil
			}
		}
	case OpUninstall:
		switch from {
		case runtime.StatusInstalled, runtime.StatusDisabled:
			return runtime.StatusUninstalled, false, nil
		case runtime.StatusUninstalled:
			return from, true, nil
		}
	}
	return "", false, invalidTransition(op, from)
}
