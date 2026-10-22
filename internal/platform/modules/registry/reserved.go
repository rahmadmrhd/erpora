package registry

import (
	"path"
	"regexp"
	"strings"
)

// reservedSlugs adalah slug yang dicadangkan untuk platform (keputusan rencana 4,
// ADR-006 keputusan 7). Slug dipakai di /api/{slug}, basePath frontend /{slug},
// dan nama command <slug>:<name>, sehingga tidak boleh bentrok dengan platform.
var reservedSlugs = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range []string{
		"platform", "api", "assets", "settings", "bootstrap", "healthz", "readyz", "sw", "make", "db",
		"module", "modules", "contracts", "route", "permission", "migrate", "serve", "inspect",
		"version", "list", "doctor", "completion", "help",
	} {
		m[s] = true
	}
	return m
}()

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
