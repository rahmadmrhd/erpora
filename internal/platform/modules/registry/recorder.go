package registry

import (
	"net/http"

	"erpora/sdk/module"
)

// recorder adalah module.Registrar per modul yang hanya merekam pendaftaran.
type recorder struct {
	key    string
	slug   string
	routes []RouteInfo
	cmds   []CommandInfo
}

func (r *recorder) Handle(method, path, permission string, h http.Handler) {
	r.routes = append(r.routes, RouteInfo{
		Module: r.key, Method: method, Pattern: "/api/" + r.slug + path,
		Permission: permission, Handler: h,
	})
}

func (r *recorder) Public(method, path string, h http.Handler) {
	r.routes = append(r.routes, RouteInfo{
		Module: r.key, Method: method, Pattern: "/api/" + r.slug + path,
		Public: true, Handler: h,
	})
}

func (r *recorder) Command(c module.Command) {
	r.cmds = append(r.cmds, CommandInfo{Module: r.key, Name: r.slug + ":" + c.Name, Command: c})
}
