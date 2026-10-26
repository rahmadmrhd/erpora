// Package moduletest berisi helper untuk menguji modul tanpa platform: recorder module.Registrar
// yang merekam route dan command yang didaftarkan modul. Modul memakainya dari file test agar tidak
// perlu mengimplementasikan module.Registrar sendiri (Q26 butir 13).
package moduletest

import (
	"net/http"

	"erpora/sdk/module"
)

// Route adalah satu route yang direkam. Permission kosong untuk route Public.
type Route struct {
	Method, Path, Permission string
	Public                   bool
	Handler                  http.Handler
}

// Recorder adalah module.Registrar yang hanya merekam pendaftaran, berurutan sesuai pemanggilan.
type Recorder struct {
	Routes   []Route
	Commands []module.Command
}

var _ module.Registrar = (*Recorder)(nil)

// NewRecorder membuat Recorder kosong.
func NewRecorder() *Recorder { return &Recorder{} }

// Handle merekam route yang dilindungi permission.
func (r *Recorder) Handle(method, path, permission string, h http.Handler) {
	r.Routes = append(r.Routes, Route{Method: method, Path: path, Permission: permission, Handler: h})
}

// Public merekam route tanpa permission.
func (r *Recorder) Public(method, path string, h http.Handler) {
	r.Routes = append(r.Routes, Route{Method: method, Path: path, Public: true, Handler: h})
}

// Command merekam perintah CLI modul.
func (r *Recorder) Command(c module.Command) { r.Commands = append(r.Commands, c) }

// Route mencari route terekam berdasarkan method dan path relatif modul.
func (r *Recorder) Route(method, path string) (Route, bool) {
	for _, rt := range r.Routes {
		if rt.Method == method && rt.Path == path {
			return rt, true
		}
	}
	return Route{}, false
}

// Permission mengembalikan permission route terekam ("" bila route tidak ada atau Public).
func (r *Recorder) Permission(method, path string) string {
	rt, _ := r.Route(method, path)
	return rt.Permission
}
