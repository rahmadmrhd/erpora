package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func spaFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>INDEX</html>")},
		"assets/app-abc123.js": {Data: []byte("console.log(1)")},
		"sw.js":                {Data: []byte("self.skip()")},
	}
}

func spaGet(h http.Handler, path string, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSPA_AssetImmutable(t *testing.T) {
	rec := spaGet(NewSPA(spaFS()), "/assets/app-abc123.js", "")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestSPA_IndexNoCache(t *testing.T) {
	rec := spaGet(NewSPA(spaFS()), "/", "text/html")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" || !strings.Contains(rec.Body.String(), "INDEX") {
		t.Fatalf("%d %q %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
}

func TestSPA_ServiceWorkerNoCache(t *testing.T) {
	rec := spaGet(NewSPA(spaFS()), "/sw.js", "")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestSPA_FallbackForHTMLRoutes(t *testing.T) {
	for _, p := range []string{"/inventory/products", "/reports/2026.10"} {
		rec := spaGet(NewSPA(spaFS()), p, "text/html,application/xhtml+xml")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "INDEX") {
			t.Errorf("%s: %d %s", p, rec.Code, rec.Body)
		}
	}
	if rec := spaGet(NewSPA(spaFS()), "/inventory/products", "application/json"); rec.Code != 404 {
		t.Errorf("non-html accept: %d", rec.Code)
	}
}

func TestSPA_MissingAssetIs404(t *testing.T) {
	rec := spaGet(NewSPA(spaFS()), "/assets/missing.js", "text/html")
	if rec.Code != 404 {
		t.Fatalf("%d", rec.Code)
	}
}

func TestSPA_TraversalRejected(t *testing.T) {
	for _, p := range []string{"/../../etc/passwd", "/assets/%2e%2e/x", "/assets/../index.html"} {
		rec := spaGet(NewSPA(spaFS()), p, "text/html")
		if rec.Code != 404 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

func TestSPA_NotBuiltPage(t *testing.T) {
	rec := spaGet(NewSPA(fstest.MapFS{".gitkeep": {}}), "/", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "frontend belum di-build") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestSPA_MethodNotAllowedHasAllowHeader(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	rec := httptest.NewRecorder()
	NewSPA(spaFS()).ServeHTTP(rec, req)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("%d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}
