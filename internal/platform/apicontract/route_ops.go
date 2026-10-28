package apicontract

import (
	"net/http"
	"net/url"
)

// examplePath mengganti setiap {param} pada path operasi dengan nilai contohnya (example parameter, example skema,
// atau "x"), agar path dapat didispatch oleh http.ServeMux.
func examplePath(s *Set, op Operation) string {
	return pathParamRe.ReplaceAllStringFunc(op.Path, func(m string) string {
		return url.PathEscape(paramExample(s, op, m[1:len(m)-1]))
	})
}

// RouteOperations memetakan setiap route (kunci "METHOD pattern") ke operasi kontrak dokumen doc yang dilayaninya.
// Tanpa dispatch, operasi dicocokkan persis pada method dan path; dengan dispatch (platform.core, keputusan 9),
// operasi dicocokkan lewat http.ServeMux atas route yang diberikan sehingga satu route gabungan dapat melayani
// beberapa operasi. Route yang tidak melayani operasi apa pun tidak muncul di hasil.
func (s *Set) RouteOperations(doc string, routes []RouteSpec, dispatch bool) map[string][]Operation {
	out := map[string][]Operation{}
	ops := s.Operations(doc)
	if !dispatch {
		have := map[string]bool{}
		for _, r := range routes {
			have[r.Method+" "+r.Pattern] = true
		}
		for _, op := range ops {
			if k := op.Method + " " + op.Path; have[k] {
				out[k] = append(out[k], op)
			}
		}
		return out
	}
	mux := http.NewServeMux()
	patterns := map[string]bool{}
	func() {
		defer func() { _ = recover() }() // route bentrok sudah dilaporkan registry; di sini cukup tidak memetakan
		for _, r := range routes {
			pat := r.Method + " " + r.Pattern
			mux.Handle(pat, http.NotFoundHandler())
			patterns[pat] = true
		}
	}()
	for _, op := range ops {
		req, err := http.NewRequest(op.Method, examplePath(s, op), http.NoBody)
		if err != nil {
			continue
		}
		if _, pattern := mux.Handler(req); pattern != "" && patterns[pattern] {
			out[pattern] = append(out[pattern], op)
		}
	}
	return out
}
