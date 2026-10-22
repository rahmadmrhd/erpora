// Package migrations menanam file migrasi SQL platform (goose).
package migrations

import "embed"

// FS berisi subdirektori control/ dan tenant/.
//
//go:embed control/*.sql tenant/*.sql
var FS embed.FS
