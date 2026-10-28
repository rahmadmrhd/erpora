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
	rec := spaGet(NewSPA(spaFS(), false), "/assets/app-abc123.js", "")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestSPA_IndexNoCache(t *testing.T) {
	rec := spaGet(NewSPA(spaFS(), false), "/", "text/html")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" || !strings.Contains(rec.Body.String(), "INDEX") {
		t.Fatalf("%d %q %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
}

func TestSPA_ServiceWorkerNoCache(t *testing.T) {
	rec := spaGet(NewSPA(spaFS(), false), "/sw.js", "")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestSPA_FallbackRoutes(t *testing.T) {
	h := NewSPA(spaFS(), false)
	// Tanpa ekstensi -> index.html apa pun Accept-nya (FR-04: curl mengirim */*), termasuk tanpa Accept.
	for _, accept := range []string{"text/html,application/xhtml+xml", "*/*", "application/json", ""} {
		rec := spaGet(h, "/inventory/products", accept)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "INDEX") {
			t.Errorf("tanpa ekstensi, Accept=%q: %d %s", accept, rec.Code, rec.Body)
		}
	}
	// Berekstensi -> index.html hanya bila Accept memuat text/html.
	if rec := spaGet(h, "/reports/2026.10", "text/html"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "INDEX") {
		t.Errorf("berekstensi + text/html: %d %s", rec.Code, rec.Body)
	}
	for _, accept := range []string{"*/*", "application/json", ""} {
		for _, p := range []string{"/reports/2026.10", "/missing.js"} {
			rec := spaGet(h, p, accept)
			if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"not_found"`) {
				t.Errorf("%s Accept=%q: %d %s", p, accept, rec.Code, rec.Body)
			}
		}
	}
}

// /api/* tidak pernah mendapat index.html, bahkan bila SPA dipanggil langsung.
func TestSPA_NeverServesIndexForAPI(t *testing.T) {
	for _, p := range []string{"/api", "/api/unknown", "/api/x/y"} {
		rec := spaGet(NewSPA(spaFS(), true), p, "text/html")
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "INDEX") {
			t.Errorf("%s: %d %s", p, rec.Code, rec.Body)
		}
	}
}

func TestSPA_MissingAssetIs404(t *testing.T) {
	rec := spaGet(NewSPA(spaFS(), false), "/assets/missing.js", "text/html")
	if rec.Code != 404 {
		t.Fatalf("%d", rec.Code)
	}
}

func TestSPA_TraversalRejected(t *testing.T) {
	for _, p := range []string{"/../../etc/passwd", "/assets/%2e%2e/x", "/assets/../index.html"} {
		rec := spaGet(NewSPA(spaFS(), false), p, "text/html")
		if rec.Code != 404 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

func TestSPA_NotBuiltPage_DevOnly(t *testing.T) {
	empty := fstest.MapFS{".gitkeep": {}}
	rec := spaGet(NewSPA(empty, true), "/", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "frontend belum di-build") ||
		!strings.Contains(rec.Body.String(), "<title>ERPora</title>") || !strings.Contains(rec.Body.String(), "<h1>ERPora</h1>") {
		t.Fatalf("dev: %d %s", rec.Code, rec.Body)
	}
	// Di luar dev: 404 JSON not_found, bukan halaman.
	rec = spaGet(NewSPA(empty, false), "/", "text/html")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"not_found"`) || strings.Contains(rec.Body.String(), "belum di-build") {
		t.Fatalf("non-dev: %d %s", rec.Code, rec.Body)
	}
}

func TestSPA_MethodNotAllowedHasAllowHeader(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	rec := httptest.NewRecorder()
	NewSPA(spaFS(), false).ServeHTTP(rec, req)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("%d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

// "/assets" (tanpa slash) diperlakukan sama dengan /assets/*: tidak ada -> 404, bukan fallback index.html.
func TestSPA_AssetsRootIs404(t *testing.T) {
	for _, accept := range []string{"text/html", "*/*", ""} {
		rec := spaGet(NewSPA(spaFS(), false), "/assets", accept)
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "INDEX") {
			t.Errorf("Accept=%q: %d %s", accept, rec.Code, rec.Body)
		}
	}
}
