//go:build devtools

package main

import (
	"github.com/spf13/cobra"

	"erpora/internal/platform/buildinfo"
)

// registerDevtools menandai flavor "dev"; command `dev` ditambahkan di milestone berikutnya.
func registerDevtools(_ *cobra.Command) {
	buildinfo.SetFlavor("dev")
}
