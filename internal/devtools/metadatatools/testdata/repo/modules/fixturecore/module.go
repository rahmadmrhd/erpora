// Package fixturecore adalah modul fixture spec resource-metadata (master data bersama: pelanggan, cabang,
// tipe domain, dan blok alamat). Tidak didaftarkan ke internal/app.
package fixturecore

import "erpora/sdk/module"

// Module (fixture).
type Module struct{}

// Register (fixture): tanpa route; kode data dibangkitkan pada tahap berikutnya.
func (m *Module) Register(_ module.Registrar) error { return nil }
