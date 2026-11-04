package moduletest

import (
	"context"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"erpora/sdk/authz"
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

func TestRecorder_RecordsRouteOptions(t *testing.T) {
	r := NewRecorder()
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	r.Handle("POST", "/notes", "x.notes.create", h, module.MaxBodyBytes(2048), module.Classify(module.ClassConfidential))
	r.Public("GET", "/status", h)

	rt, _ := r.Route("POST", "/notes")
	if rt.Options != (module.RouteConfig{MaxBodyBytes: 2048, MaxBodyBytesSet: true, Classification: "confidential"}) {
		t.Errorf("Options = %+v", rt.Options)
	}
	st, _ := r.Route("GET", "/status")
	if st.Options != (module.RouteConfig{}) {
		t.Errorf("Options publik = %+v", st.Options)
	}
}

func TestRecorder_RecordsScopeProviders(t *testing.T) {
	r := NewRecorder()
	var reg module.Registrar = r
	p := authz.ScopeProvider{Key: "x.recent", Label: "x.scope.recent",
		Filter: func(_ context.Context, db *gorm.DB) *gorm.DB { return db },
		Match:  func(context.Context, authz.Record) (bool, error) { return true, nil },
	}
	reg.Scope(p)
	if len(r.Scopes) != 1 || r.Scopes[0].Key != "x.recent" {
		t.Fatalf("Scopes = %+v", r.Scopes)
	}
}
