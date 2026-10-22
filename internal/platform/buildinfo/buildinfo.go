// Package buildinfo memuat metadata build yang diisi lewat -ldflags -X.
package buildinfo

import "sync/atomic"

// Diisi lewat -ldflags "-X .../buildinfo.Version=..." dst.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var flavor atomic.Value

// Flavor mengembalikan "dev" bila build memakai tag devtools (lewat SetFlavor), selain itu "release".
func Flavor() string {
	if f, ok := flavor.Load().(string); ok && f != "" {
		return f
	}
	return "release"
}

// SetFlavor dipanggil dari cmd/erp/devtools_on.go.
func SetFlavor(f string) { flavor.Store(f) }
