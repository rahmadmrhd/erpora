package httpserver

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

const notBuiltPage = `<!doctype html><html lang="id"><head><meta charset="utf-8"><title>ERPora</title></head>` +
	`<body><h1>ERPora</h1><p>frontend belum di-build. Jalankan <code>task build</code> atau <code>bun run build</code> di folder web.</p></body></html>`

// NewSPA melayani aset statis dan fallback index.html untuk rute SPA.
// Aturan (PRD-000 FR-04): file ada -> sajikan; /assets/* tidak ada -> 404; /api/* -> 404 (tidak pernah index.html);
// path tanpa ekstensi -> index.html apa pun Accept-nya (mis. curl); path berekstensi -> index.html hanya bila
// Accept memuat text/html, selain itu 404. dev=true menampilkan halaman "frontend belum di-build" (FR-12) saat
// index.html tidak ada; di luar dev respons 404 JSON.
func NewSPA(assets fs.FS, dev bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "metode tidak diizinkan", nil)
			return
		}
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if seg == ".." {
				notFound(w, r)
				return
			}
		}
		clean := path.Clean("/" + r.URL.Path)
		name := strings.TrimPrefix(clean, "/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			notFound(w, r)
			return
		}
		if clean == "/api" || strings.HasPrefix(clean, "/api/") {
			notFound(w, r)
			return
		}
		isAsset := strings.HasPrefix(clean, "/assets/")

		if b, ok := readFile(assets, name); ok {
			serve(w, r, name, b, isAsset)
			return
		}
		if isAsset {
			notFound(w, r)
			return
		}
		index, built := readFile(assets, "index.html")
		// Tanpa ekstensi = rute SPA; berekstensi hanya bila klien meminta HTML.
		spaRoute := path.Ext(clean) == "" || strings.Contains(r.Header.Get("Accept"), "text/html")
		switch {
		case !built && dev && spaRoute:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(notBuiltPage))
		case built && spaRoute:
			serve(w, r, "index.html", index, false)
		default:
			notFound(w, r)
		}
	})
}

func readFile(fsys fs.FS, name string) ([]byte, bool) {
	st, err := fs.Stat(fsys, name)
	if err != nil || st.IsDir() {
		return nil, false
	}
	b, err := fs.ReadFile(fsys, name)
	return b, err == nil
}

func serve(w http.ResponseWriter, r *http.Request, name string, b []byte, immutable bool) {
	if immutable {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

func notFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusNotFound, "not_found", "sumber daya tidak ditemukan", nil)
}
