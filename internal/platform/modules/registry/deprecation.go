package registry

import (
	"slices"

	"erpora/contracts"
	"erpora/internal/platform/apicontract"
)

// applyDeprecation mengisi RouteInfo.Deprecated dan DeprecatedParams dari kontrak setiap Entry (PRD-000 FR-23.3).
// Operasi dicocokkan dengan route lewat method dan path persis sama; platform.core lewat dispatch ServeMux, dengan
// route gabungan deprecated hanya bila semua operasi yang dilayaninya deprecated. Kontrak yang tidak dapat diuraikan
// sudah dilaporkan sebagai invalid_contract oleh Build, jadi di sini dilewati.
func applyDeprecation(entries []Entry, routes []RouteInfo) {
	var docs []*apicontract.Doc
	if common, err := apicontract.Parse("common", contracts.CommonOpenAPI); err == nil {
		docs = append(docs, common)
	}
	docOf := map[string]string{}
	for _, e := range entries {
		if e.Contract == nil {
			continue
		}
		name := e.Manifest.Slug
		if e.Manifest.Key == PlatformKey {
			name = PlatformSlug
		}
		if d, err := apicontract.Parse(name, e.Contract); err == nil {
			docs = append(docs, d)
			docOf[e.Manifest.Key] = name
		}
	}
	set, err := apicontract.NewSet(docs...)
	if err != nil {
		return
	}
	for key, name := range docOf {
		var specs []apicontract.RouteSpec
		for _, r := range routes {
			if r.Module == key {
				specs = append(specs, apicontract.RouteSpec{Module: key, Method: r.Method, Pattern: r.Pattern})
			}
		}
		byRoute := set.RouteOperations(name, specs, key == PlatformKey)
		for i := range routes {
			if routes[i].Module != key {
				continue
			}
			ops := byRoute[routes[i].Method+" "+routes[i].Pattern]
			if len(ops) == 0 {
				continue
			}
			all := true
			var params []string
			for _, op := range ops {
				all = all && op.Deprecated
				params = append(params, op.DeprecatedQueryParams...)
			}
			slices.Sort(params)
			routes[i].Deprecated = all
			routes[i].DeprecatedParams = slices.Compact(params)
		}
	}
}
