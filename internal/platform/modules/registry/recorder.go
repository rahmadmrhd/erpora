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
	caps     map[string]bool // capability yang dideklarasikan manifest (atau Options.PlatformCapabilities)
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

// validHandler menolak handler nil (akan panic saat request pertama).
func (r *recorder) validHandler(method, path string, h http.Handler) bool {
	if h == nil {
		r.problem(CodeInvalidRoute, "route %s %s memakai handler nil", method, path)
		return false
	}
	return true
}

func (r *recorder) Handle(method, path, permission string, h http.Handler, opts ...module.RouteOption) {
	valid := r.validRoute(method, path) && r.validHandler(method, path, h)
	cfg, optsOK := r.resolveOptions(method, path, opts)
	valid = valid && optsOK
	switch {
	case permission == "":
		r.problem(CodePermissionMissing, "route %s %s tidak punya permission; gunakan Public bila memang publik", method, path)
		valid = false
	case !r.caps[permission]:
		r.problem(CodePermissionUndeclared, "permission %q pada route %s %s tidak ada di capabilities manifest", permission, method, path)
		valid = false
	}
	if !valid {
		return
	}
	r.routes = append(r.routes, RouteInfo{
		Module: r.key, Method: method, Pattern: "/api/" + r.slug + path,
		Permission: permission, Handler: h, MaxBodyBytes: cfg.MaxBodyBytes, Classification: cfg.Classification,
	})
}

func (r *recorder) Public(method, path string, h http.Handler, opts ...module.RouteOption) {
	valid := r.validRoute(method, path) && r.validHandler(method, path, h)
	cfg, optsOK := r.resolveOptions(method, path, opts)
	if !valid || !optsOK {
		return
	}
	r.routes = append(r.routes, RouteInfo{
		Module: r.key, Method: method, Pattern: "/api/" + r.slug + path,
		Public: true, Handler: h, MaxBodyBytes: cfg.MaxBodyBytes, Classification: cfg.Classification,
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

// resolveOptions menerapkan opsi route, memvalidasinya, dan mengisi default batas body.
// Semua pelanggaran dicatat sebelum return (FR-05 aturan 7).
func (r *recorder) resolveOptions(method, path string, opts []module.RouteOption) (module.RouteConfig, bool) {
	cfg := module.ResolveRouteOptions(opts...)
	ok := true
	if (cfg.MaxBodyBytesSet && cfg.MaxBodyBytes < 1) || cfg.MaxBodyBytes < 0 || cfg.MaxBodyBytes > module.MaxRouteBodyBytes {
		r.problem(CodeInvalidRouteOption, "route %s %s: MaxBodyBytes %d di luar rentang 1..%d", method, path, cfg.MaxBodyBytes, module.MaxRouteBodyBytes)
		ok = false
	}
	if cfg.Classification != "" && !cfg.Classification.Valid() {
		r.problem(CodeInvalidRouteOption, "route %s %s: klasifikasi %q tidak dikenal", method, path, cfg.Classification)
		ok = false
	}
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = module.DefaultMaxBodyBytes
	}
	return cfg, ok
}
