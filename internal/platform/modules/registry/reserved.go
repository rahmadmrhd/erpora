package registry

import (
	"path"
	"regexp"
	"strings"
)

// reservedSlugs adalah slug yang dicadangkan untuk platform (PRD-000 FR-05);
// modul tidak boleh memakainya sebagai prefix /api/{slug}.
var reservedSlugs = map[string]bool{
	"settings": true,
	"db":       true,
	"assets":   true,
}

// allowedMethods adalah method HTTP yang boleh didaftarkan modul.
var allowedMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

var commandNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// validRoutePath memeriksa path relatif modul: diawali "/", sudah bersih
// (path.Clean), tanpa "//", host, query, fragmen, atau spasi.
func validRoutePath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.Contains(p, "//") || strings.ContainsAny(p, "?# \t\r\n") {
		return false
	}
	return p == "/" || path.Clean(p) == p
}
