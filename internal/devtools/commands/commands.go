// Package commands berisi command cobra developer tools (make:*, db:*, contracts:*, module:check, doctor, release:*).
package commands

import "github.com/spf13/cobra"

// ID grup command dev. GroupModule dipakai bersama grup modul yang sudah ada di root.
const (
	GroupMake      = "make"
	GroupDB        = "db"
	GroupContracts = "contracts"
	GroupModule    = "module"
	GroupDevtools  = "devtools"
)

// Options adalah dependensi command dev.
type Options struct {
	EnvFile func() string // nilai flag --env-file saat command berjalan
}

// Register mendaftarkan semua command developer tools ke root. Task berikutnya menambah command di sini.
func Register(root *cobra.Command, opts Options) {
	registerContracts(root)
	registerMake(root)
	registerModule(root)
	registerDB(root, opts)
	root.AddCommand(newDoctor(opts))
}
