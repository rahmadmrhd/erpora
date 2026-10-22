package registry

import (
	"fmt"
	"net/http"

	"erpora/sdk/module"
)

// recorder adalah module.Registrar per modul yang merekam dan memvalidasi pendaftaran.
type recorder struct {
	key      string
	slug     string
	platform bool            // platform.core tidak dibatasi manifest capabilities
	caps     map[string]bool // capability yang dideklarasikan manifest
	routes   []RouteInfo
	cmds     []CommandInfo
	cmdNames map[string]bool
	problems []Problem
}

func (r *recorder) problem(code, format string, args ...any) {
	r.problems = append(r.problems, Problem{Module: r.key, Code: code, Message: fmt.Sprintf(format, args...)})
}

// validRoute memeriksa method dan path; mengembalikan false bila ditolak.
func (r *recorder) validRoute(method, path string) bool {
	ok := true
	if !allowedMethods[method] {
		r.problem(CodeInvalidRoute, "method %q tidak diizinkan (GET, POST, PUT, PATCH, DELETE)", method)
		ok = false
	}
	if !validRoutePath(path) {
		r.problem(CodeInvalidRoute, "path %q tidak valid: harus diawali '/', bersih, dan tanpa host", path)
		ok = false
	}
	return ok
}

func (r *recorder) Handle(method, path, permission string, h http.Handler) {
	valid := r.validRoute(method, path)
	switch {
	case permission == "":
		r.problem(CodePermissionMissing, "route %s %s tidak punya permission; gunakan Public bila memang publik", method, path)
		valid = false
	case !r.platform && !r.caps[permission]:
		r.problem(CodePermissionUndeclared, "permission %q pada route %s %s tidak ada di capabilities manifest", permission, method, path)
		valid = false
	}
	if !valid {
		return
	}
	r.routes = append(r.routes, RouteInfo{
		Module: r.key, Method: method, Pattern: "/api/" + r.slug + path,
		Permission: permission, Handler: h,
	})
}

func (r *recorder) Public(method, path string, h http.Handler) {
	if !r.validRoute(method, path) {
		return
	}
	r.routes = append(r.routes, RouteInfo{
		Module: r.key, Method: method, Pattern: "/api/" + r.slug + path,
		Public: true, Handler: h,
	})
}

func (r *recorder) Command(c module.Command) {
	valid := true
	if !commandNameRE.MatchString(c.Name) {
		r.problem(CodeInvalidCommand, "nama command %q tidak valid (harus cocok ^[a-z][a-z0-9-]*$)", c.Name)
		valid = false
	}
	if c.Run == nil {
		r.problem(CodeInvalidCommand, "command %q tidak punya Run", c.Name)
		valid = false
	}
	if r.cmdNames[c.Name] {
		r.problem(CodeCommandConflict, "command %q didaftarkan lebih dari sekali", c.Name)
		valid = false
	}
	if !valid {
		return
	}
	if r.cmdNames == nil {
		r.cmdNames = map[string]bool{}
	}
	r.cmdNames[c.Name] = true
	r.cmds = append(r.cmds, CommandInfo{Module: r.key, Name: r.slug + ":" + c.Name, Command: c})
}
