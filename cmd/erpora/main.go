// Command erpora adalah entry point binary platform ERPora.
package main

import (
	"fmt"
	"os"
)

func main() {
	root := newRootCmd(os.Stdout, os.Stderr)
	err := root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(exitCode(err))
}
