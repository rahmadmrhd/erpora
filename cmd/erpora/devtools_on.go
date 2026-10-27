//go:build devtools

package main

import (
	"github.com/spf13/cobra"

	"erpora/internal/devtools"
)

// registerDevtools menambah developer tools (grup make/db/contracts/devtools), banner dev, dan menandai flavor "dev".
// Satu-satunya file yang boleh mengimpor erpora/internal/devtools (aturan arch DEVTOOLS).
func registerDevtools(root *cobra.Command, opts *rootOpts) {
	devtools.Register(root, devtools.Options{EnvFile: func() string { return opts.envFile }})
}
