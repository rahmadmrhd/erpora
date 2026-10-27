package runtime

import (
	"net/http"

	"erpora/internal/platform/modules/registry"
	"erpora/sdk/httpx"
	"erpora/sdk/module"
)

// requestLimitsMW menegakkan batas request per route sebelum handler modul (FR-05, Q26 butir 13):
// body dibatasi http.MaxBytesReader (juga untuk handler yang membaca body sendiri), batasnya dibagikan
// ke httpx.DecodeJSON lewat context, dan data confidential ke atas ditandai Cache-Control: no-store
// (security-guidelines §6) sebelum handler berjalan sehingga ikut menempel pada respons error.
func requestLimitsMW(rt registry.RouteInfo, next http.Handler) http.Handler {
	limit := rt.MaxBodyBytes
	if limit <= 0 {
		limit = module.DefaultMaxBodyBytes
	}
	noStore := rt.Classification.AtLeast(module.ClassConfidential)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if noStore {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r.WithContext(httpx.WithBodyLimit(r.Context(), limit)))
	})
}
