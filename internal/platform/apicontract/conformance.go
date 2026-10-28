package apicontract

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	"erpora/sdk/module"
)

// RouteSpec adalah satu route terdaftar (dari registry.RouteInfo, tanpa handler).
type RouteSpec struct {
	Module, Method, Pattern, Permission string
	Public                              bool
	Classification                      module.Classification
}

// Mismatch adalah satu ketidaksesuaian route terdaftar dengan kontrak.
// Kind: missing_operation | missing_route | permission_mismatch | public_mismatch | undeclared_permission |
// classification_mismatch | missing_contract.
type Mismatch struct{ Kind, Module, Method, Path, Detail string }

// ConformanceInput adalah masukan CheckConformance.
type ConformanceInput struct {
	Set          *Set
	DocOf        map[string]string   // key modul -> nama doc ("platform.core" -> "platform")
	Routes       []RouteSpec         // semua route terdaftar
	Capabilities map[string][]string // key modul -> capabilities manifest (platform.core: admin.Capabilities)
	// DispatchEquivalent: hanya {"platform.core": true} (keputusan 9). Operasi kontrak dicocokkan lewat dispatch
	// http.ServeMux atas route terdaftar, bukan kesamaan pola.
	DispatchEquivalent map[string]bool
}

var pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)

// CheckConformance mencocokkan route terdaftar dengan operasi kontrak (PRD-000 FR-23.2): method, path penuh,
// permission, x-erp-public, deklarasi permission di capabilities, dan klasifikasi respons. Hasil terurut
// (Module, Path, Method, Kind).
func CheckConformance(in ConformanceInput) []Mismatch {
	var out []Mismatch
	add := func(kind, mod, method, path, format string, args ...any) {
		out = append(out, Mismatch{Kind: kind, Module: mod, Method: method, Path: path, Detail: fmt.Sprintf(format, args...)})
	}

	modules := map[string]bool{}
	routesOf := map[string][]RouteSpec{}
	for k := range in.DocOf {
		modules[k] = true
	}
	for k := range in.Capabilities {
		modules[k] = true
	}
	for _, r := range in.Routes {
		modules[r.Module] = true
		routesOf[r.Module] = append(routesOf[r.Module], r)
	}
	keys := make([]string, 0, len(modules))
	for k := range modules {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		docName := in.DocOf[key]
		if _, ok := in.Set.Doc(docName); docName == "" || !ok {
			add("missing_contract", key, "", "", "modul %s tidak punya kontrak OpenAPI (modules/<dir>/openapi.yaml)", key)
			continue
		}
		ops := in.Set.Operations(docName)
		routes := routesOf[key]
		declared := map[string]bool{}
		for _, c := range in.Capabilities[key] {
			declared[c] = true
		}

		compare := func(op Operation, r RouteSpec) {
			if r.Public != op.Public {
				add("public_mismatch", key, op.Method, op.Path, "route public=%v, kontrak x-erp-public=%v", r.Public, op.Public)
			}
			if r.Permission != op.Permission {
				add("permission_mismatch", key, op.Method, op.Path, "route permission %q, kontrak x-erp-permission %q", r.Permission, op.Permission)
			}
			opClass, err := in.Set.ResponseClassification(op)
			if err != nil {
				add("classification_mismatch", key, op.Method, op.Path, "klasifikasi respons kontrak tidak dapat dihitung: %v", err)
				return
			}
			routeClass := r.Classification
			if routeClass == "" {
				routeClass = module.ClassInternal
			}
			if routeClass != opClass {
				add("classification_mismatch", key, op.Method, op.Path, "route klasifikasi %q, kontrak %q (klasifikasi tertinggi respons)", routeClass, opClass)
			}
		}

		for _, op := range ops {
			if op.Permission != "" && !op.Public && !declared[op.Permission] {
				add("undeclared_permission", key, op.Method, op.Path, "x-erp-permission %q tidak ada di capabilities manifest %s", op.Permission, key)
			}
		}

		if in.DispatchEquivalent[key] {
			checkDispatch(in, key, ops, routes, add, compare)
			continue
		}

		routeByKey := map[string]RouteSpec{}
		for _, r := range routes {
			routeByKey[r.Method+" "+r.Pattern] = r
		}
		opByKey := map[string]bool{}
		for _, op := range ops {
			k := op.Method + " " + op.Path
			opByKey[k] = true
			r, ok := routeByKey[k]
			if !ok {
				add("missing_route", key, op.Method, op.Path, "operasi kontrak tidak punya route terdaftar")
				continue
			}
			compare(op, r)
		}
		for _, r := range routes {
			if !opByKey[r.Method+" "+r.Pattern] {
				add("missing_operation", key, r.Method, r.Pattern, "route terdaftar tidak punya operasi di kontrak")
			}
		}
	}

	slices.SortStableFunc(out, func(a, b Mismatch) int {
		for _, c := range []int{
			strings.Compare(a.Module, b.Module), strings.Compare(a.Path, b.Path),
			strings.Compare(a.Method, b.Method), strings.Compare(a.Kind, b.Kind),
		} {
			if c != 0 {
				return c
			}
		}
		return 0
	})
	return out
}

// checkDispatch mencocokkan operasi dengan route lewat dispatch http.ServeMux (platform.core, keputusan 9).
func checkDispatch(in ConformanceInput, key string, ops []Operation, routes []RouteSpec,
	add func(kind, mod, method, path, format string, args ...any), compare func(Operation, RouteSpec)) {
	mux := http.NewServeMux()
	byPattern := map[string]RouteSpec{}
	var regErr string
	func() {
		defer func() {
			if p := recover(); p != nil {
				regErr = fmt.Sprint(p)
			}
		}()
		for _, r := range routes {
			pat := r.Method + " " + r.Pattern
			byPattern[pat] = r
			mux.Handle(pat, http.NotFoundHandler())
		}
	}()
	if regErr != "" {
		add("missing_route", key, "", "", "route terdaftar tidak dapat dibuat menjadi ServeMux: %s", regErr)
		return
	}
	hit := map[string]bool{}
	for _, op := range ops {
		example := examplePath(in.Set, op)
		req, err := http.NewRequest(op.Method, example, http.NoBody)
		if err != nil {
			add("missing_route", key, op.Method, op.Path, "path contoh %q tidak valid: %v", example, err)
			continue
		}
		_, pattern := mux.Handler(req)
		r, ok := byPattern[pattern]
		if pattern == "" || !ok {
			add("missing_route", key, op.Method, op.Path, "tidak ada route terdaftar yang melayani %s %s", op.Method, example)
			continue
		}
		hit[pattern] = true
		compare(op, r)
	}
	for _, r := range routes {
		if !hit[r.Method+" "+r.Pattern] {
			add("missing_operation", key, r.Method, r.Pattern, "route terdaftar tidak melayani operasi kontrak mana pun")
		}
	}
}

// paramExample mengembalikan nilai contoh parameter path (example parameter, example skema, atau "x").
func paramExample(s *Set, op Operation, name string) string {
	resolve := func(from, ref string) (string, any, bool) {
		doc, n, err := s.Resolve(from, ref)
		return doc, n, err == nil
	}
	params, _ := op.Node["parameters"].([]any)
	for _, raw := range params {
		pm := paramNode(op.Doc, raw, resolve)
		if pm == nil {
			continue
		}
		if n, _ := pm["name"].(string); n != name {
			continue
		}
		if ex, ok := pm["example"]; ok {
			return fmt.Sprint(ex)
		}
		if sc, ok := pm["schema"].(map[string]any); ok {
			if ex, ok := sc["example"]; ok {
				return fmt.Sprint(ex)
			}
		}
	}
	return "x"
}
