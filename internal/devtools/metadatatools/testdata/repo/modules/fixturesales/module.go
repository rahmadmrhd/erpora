// Package fixturesales adalah modul fixture spec resource-metadata (dokumen faktur beserta baris, relasi satu
// modul dan lintas modul, morph, dan blok). Tidak didaftarkan ke internal/app.
package fixturesales

import "erpora/sdk/module"

// Module (fixture).
type Module struct{}

// Register (fixture): tanpa route; kode data dibangkitkan pada tahap berikutnya.
func (m *Module) Register(_ module.Registrar) error { return nil }
