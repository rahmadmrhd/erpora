// Package catalog menanam katalog terjemahan namespace "platform" (internal/platform/i18n/catalog/<locale>.json).
package catalog

import (
	"embed"
	"io/fs"
)

//go:embed id.json en.json
var files embed.FS

// FS mengembalikan filesystem berisi berkas "<locale>.json" di akar.
func FS() fs.FS { return files }
