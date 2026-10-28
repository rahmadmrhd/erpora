// Package cli berisi command CLI modul inventory (`erpora inventory:<nama>`); hanya berjalan bila modul enabled.
package cli

import (
	"context"
	"fmt"

	"erpora/sdk/module"
)

// SyncStockCommand mendefinisikan command `inventory:sync-stock`. Daftarkan di module.go lewat r.Command(cli.SyncStockCommand()).
func SyncStockCommand() module.Command {
	return module.Command{
		Name:  "sync-stock",
		Short: "sync-stock (belum diimplementasi)",
		Run:   runSyncStock,
	}
}

// runSyncStock menulis ke inv.Out. Command berjalan dengan hak sistem teraudit; akses data lewat sdk/data di context.
func runSyncStock(_ context.Context, inv module.Invocation) error {
	_, err := fmt.Fprintln(inv.Out, "sync-stock: belum diimplementasi")
	return err
}
