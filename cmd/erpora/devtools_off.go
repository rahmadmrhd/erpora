//go:build !devtools

package main

import "github.com/spf13/cobra"

// registerDevtools tidak melakukan apa pun pada build release.
func registerDevtools(_ *cobra.Command) {}
