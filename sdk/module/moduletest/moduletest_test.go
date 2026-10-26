package moduletest

import (
	"net/http"
	"testing"

	"erpora/sdk/module"
)

func TestRecorder_RecordsRoutesAndCommands(t *testing.T) {
	r := NewRecorder()
	var reg module.Registrar = r
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	reg.Handle("GET", "/notes", "x.notes.read", h)
	reg.Public("GET", "/status", h)
	reg.Command(module.Command{Name: "recount"})

	if len(r.Routes) != 2 || len(r.Commands) != 1 || r.Commands[0].Name != "recount" {
		t.Fatalf("routes=%+v commands=%+v", r.Routes, r.Commands)
	}
	if got := r.Permission("GET", "/notes"); got != "x.notes.read" {
		t.Errorf("Permission = %q", got)
	}
	rt, ok := r.Route("GET", "/status")
	if !ok || !rt.Public || rt.Permission != "" || rt.Handler == nil {
		t.Errorf("Route = %+v ok=%v", rt, ok)
	}
	if _, ok := r.Route("POST", "/notes"); ok {
		t.Error("route yang tidak terdaftar tidak boleh ditemukan")
	}
	if got := r.Permission("POST", "/notes"); got != "" {
		t.Errorf("Permission route tak terdaftar = %q", got)
	}
}
