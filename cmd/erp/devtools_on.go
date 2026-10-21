//go:build devtools

package main

import (
	"github.com/spf13/cobra"

	"github.com/rahmadmrhd/erp-go/internal/platform/buildinfo"
)

// registerDevtools menandai flavor "dev"; command `dev` ditambahkan di milestone berikutnya.
func registerDevtools(_ *cobra.Command) {
	buildinfo.SetFlavor("dev")
}
