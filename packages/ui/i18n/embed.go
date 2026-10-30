// Package uii18n menanam katalog terjemahan namespace "ui" (komponen generik @erpora/ui) ke dalam binary,
// mengikuti pola web/embed.go. Berkas JSON tinggal di samping paket JS agar penerjemah dan developer UI
// bekerja di satu tempat.
package uii18n

import (
	"embed"
	"io/fs"
)

//go:embed id.json en.json
var files embed.FS

// FS mengembalikan filesystem berisi berkas "<locale>.json" di akar.
func FS() fs.FS { return files }
