package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "Daftar semua command yang tersedia",
		GroupID: groupInspect,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			root := cmd.Root()
			printed := map[*cobra.Command]bool{}
			emit := func(c *cobra.Command) {
				fmt.Fprintf(w, "  %-12s %s\n", c.Name(), c.Short)
				printed[c] = true
			}
			for _, g := range root.Groups() {
				first := true
				for _, c := range root.Commands() {
					if c.GroupID != g.ID || c.Hidden || c.Name() == "help" {
						continue
					}
					if first {
						fmt.Fprintf(w, "%s:\n", g.Title)
						first = false
					}
					emit(c)
				}
			}
			first := true
			for _, c := range root.Commands() {
				if printed[c] || c.Hidden || c.Name() == "help" {
					continue
				}
				if first {
					fmt.Fprintln(w, "Lainnya:")
					first = false
				}
				emit(c)
			}
			return nil
		},
	}
}
