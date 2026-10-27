package apicontract

import (
	"fmt"
	"strings"
	"testing"

	"erpora/sdk/module"
)

// alphaRoutes adalah route yang persis sama dengan operasi di testdata/alpha.yaml.
func alphaRoutes() []RouteSpec {
	return []RouteSpec{
		{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/items", Permission: "alpha.items.read"},
		{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/health", Public: true},
		{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/customers", Permission: "alpha.items.read", Classification: module.ClassConfidential},
		{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/secrets", Permission: "alpha.items.read", Classification: module.ClassHighlyConfidential},
		{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/bad-class", Permission: "alpha.items.read"},
	}
}

func platformRoutes() []RouteSpec {
	return []RouteSpec{
		{Module: "platform.core", Method: "GET", Pattern: "/api/bootstrap", Public: true},
		{Module: "platform.core", Method: "GET", Pattern: "/api/platform/modules/{key}/{sub}", Permission: "platform.modules.read"},
	}
}

func kinds(ms []Mismatch) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = fmt.Sprintf("%s:%s %s", m.Kind, m.Method, m.Path)
	}
	return strings.Join(parts, "; ")
}

func hasKind(ms []Mismatch, kind, path string) bool {
	for _, m := range ms {
		if m.Kind == kind && (path == "" || m.Path == path) {
			return true
		}
	}
	return false
}

func TestCheckConformance_Table(t *testing.T) {
	s := loadSet(t, "common", "platform", "alpha", "beta")
	base := func(routes []RouteSpec) ConformanceInput {
		return ConformanceInput{
			Set:          s,
			DocOf:        map[string]string{"acme.alpha": "alpha"},
			Routes:       routes,
			Capabilities: map[string][]string{"acme.alpha": {"alpha.items.read"}},
		}
	}
	// bad-class gagal dihitung (nilai klasifikasi tidak valid); dikeluarkan dari baseline kecuali diuji.
	cleanRoutes := alphaRoutes()[:4]

	t.Run("sesuai", func(t *testing.T) {
		in := base(cleanRoutes)
		// operasi bad-class tidak punya padanan route -> hanya itu yang tersisa.
		got := CheckConformance(in)
		if len(got) != 1 || got[0].Kind != "missing_route" || got[0].Path != "/api/alpha/bad-class" {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	full := func(mutate func([]RouteSpec) []RouteSpec) []Mismatch {
		r := mutate(append([]RouteSpec{}, cleanRoutes...))
		return CheckConformance(base(r))
	}
	t.Run("route tanpa operasi", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec {
			return append(r, RouteSpec{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/extra", Permission: "alpha.items.read"})
		})
		if !hasKind(got, "missing_operation", "/api/alpha/extra") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("operasi tanpa route", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { return r[1:] }) // buang /items
		if !hasKind(got, "missing_route", "/api/alpha/items") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("permission berbeda", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { r[0].Permission = "alpha.items.write"; return r })
		if !hasKind(got, "permission_mismatch", "/api/alpha/items") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("public berbeda", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { r[1].Public = false; r[1].Permission = ""; return r })
		if !hasKind(got, "public_mismatch", "/api/alpha/health") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("permission tidak ada di capabilities", func(t *testing.T) {
		in := base(cleanRoutes)
		in.Capabilities = map[string][]string{"acme.alpha": {"alpha.other.read"}}
		got := CheckConformance(in)
		if !hasKind(got, "undeclared_permission", "/api/alpha/items") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("klasifikasi: route kosong vs kontrak confidential", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { r[2].Classification = ""; return r })
		if !hasKind(got, "classification_mismatch", "/api/alpha/customers") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("klasifikasi: route confidential vs kontrak tanpa anotasi", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { r[0].Classification = module.ClassConfidential; return r })
		if !hasKind(got, "classification_mismatch", "/api/alpha/items") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("klasifikasi lintas modul lewat $ref", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { r[3].Classification = module.ClassConfidential; return r })
		if !hasKind(got, "classification_mismatch", "/api/alpha/secrets") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("klasifikasi internal eksplisit setara kosong", func(t *testing.T) {
		got := full(func(r []RouteSpec) []RouteSpec { r[0].Classification = module.ClassInternal; return r })
		if hasKind(got, "classification_mismatch", "/api/alpha/items") {
			t.Errorf("internal harus setara kosong: %s", kinds(got))
		}
	})
	t.Run("kontrak tidak ada", func(t *testing.T) {
		in := base(cleanRoutes)
		in.DocOf = map[string]string{}
		got := CheckConformance(in)
		if !hasKind(got, "missing_contract", "") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("klasifikasi tidak valid dilaporkan", func(t *testing.T) {
		got := CheckConformance(base(alphaRoutes()))
		if !hasKind(got, "classification_mismatch", "/api/alpha/bad-class") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})

	// platform.core: kesetaraan dispatch (keputusan 9).
	t.Run("platform dispatch gabungan", func(t *testing.T) {
		in := ConformanceInput{
			Set:                s,
			DocOf:              map[string]string{"platform.core": "platform"},
			Routes:             platformRoutes(),
			Capabilities:       map[string][]string{"platform.core": {"platform.modules.read"}},
			DispatchEquivalent: map[string]bool{"platform.core": true},
		}
		if got := CheckConformance(in); len(got) != 0 {
			t.Errorf("platform: mismatch = %s", kinds(got))
		}
	})
	t.Run("pola gabungan yang sama untuk modul biasa", func(t *testing.T) {
		in := ConformanceInput{
			Set:          s,
			DocOf:        map[string]string{"acme.alpha": "alpha"},
			Routes:       []RouteSpec{{Module: "acme.alpha", Method: "GET", Pattern: "/api/alpha/{key}/{sub}", Permission: "alpha.items.read"}},
			Capabilities: map[string][]string{"acme.alpha": {"alpha.items.read"}},
		}
		got := CheckConformance(in)
		if !hasKind(got, "missing_route", "/api/alpha/items") || !hasKind(got, "missing_operation", "/api/alpha/{key}/{sub}") {
			t.Errorf("modul tanpa dispatch-equivalence: mismatch = %s", kinds(got))
		}
	})
	t.Run("dispatch: permission berbeda terdeteksi", func(t *testing.T) {
		routes := platformRoutes()
		routes[1].Permission = "platform.modules.manage"
		in := ConformanceInput{
			Set: s, DocOf: map[string]string{"platform.core": "platform"}, Routes: routes,
			Capabilities:       map[string][]string{"platform.core": {"platform.modules.read", "platform.modules.manage"}},
			DispatchEquivalent: map[string]bool{"platform.core": true},
		}
		if got := CheckConformance(in); !hasKind(got, "permission_mismatch", "") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
	t.Run("dispatch: route tak terlayani operasi manapun", func(t *testing.T) {
		routes := append(platformRoutes(), RouteSpec{Module: "platform.core", Method: "POST", Pattern: "/api/platform/modules/{key}/{operation}", Permission: "platform.modules.read"})
		in := ConformanceInput{
			Set: s, DocOf: map[string]string{"platform.core": "platform"}, Routes: routes,
			Capabilities:       map[string][]string{"platform.core": {"platform.modules.read"}},
			DispatchEquivalent: map[string]bool{"platform.core": true},
		}
		if got := CheckConformance(in); !hasKind(got, "missing_operation", "/api/platform/modules/{key}/{operation}") {
			t.Errorf("mismatch = %s", kinds(got))
		}
	})
}

func TestCheckConformance_Sorted(t *testing.T) {
	s := loadSet(t, "common", "alpha", "beta")
	got := CheckConformance(ConformanceInput{
		Set: s, DocOf: map[string]string{"acme.alpha": "alpha"}, Routes: nil,
		Capabilities: map[string][]string{"acme.alpha": {"alpha.items.read"}},
	})
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		ka := a.Module + "\x00" + a.Path + "\x00" + a.Method + "\x00" + a.Kind
		kb := b.Module + "\x00" + b.Path + "\x00" + b.Method + "\x00" + b.Kind
		if ka > kb {
			t.Fatalf("tidak terurut: %v sebelum %v", a, b)
		}
	}
	if len(got) < 2 {
		t.Fatalf("want >= 2 mismatch, got %d", len(got))
	}
}
