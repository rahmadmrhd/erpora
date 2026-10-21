// Package web menanam hasil build frontend (web/dist) ke dalam binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist mengembalikan isi web/dist sebagai root FS.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // tidak mungkin: "dist" selalu ada saat compile
	}
	return sub
}
