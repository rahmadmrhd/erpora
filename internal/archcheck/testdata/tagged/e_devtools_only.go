//go:build devtools && !integration

package tagged

import "os/exec"

var _ = exec.ErrNotFound
