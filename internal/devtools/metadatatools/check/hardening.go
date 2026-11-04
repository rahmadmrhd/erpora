package check

import (
	"fmt"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
)

// checkGroupLoosening menolak set fieldGroup yang memindahkan field ke grup yang lebih longgar menurut urutan
// grup manifest (Requirement 3.6, 3.11): grup default paling longgar, selainnya menurut klasifikasi grup.
// Resolver sendiri hanya menolak perpindahan ke grup default; pemeriksaan ini melengkapinya untuk grup
// non-default berperingkat lebih rendah. Memperketat atau memindahkan ke grup berperingkat sama diizinkan.
func (x *rctx) checkGroupLoosening() {
	for _, f := range x.r.Fields {
		for _, ch := range f.History {
			if ch.Attr != "fieldGroup" {
				continue
			}
			from, to := x.groupOf(ch.Old), x.groupOf(ch.New)
			if x.eff.groups.rank(to) < x.eff.groups.rank(from) {
				x.atOrigin(SeverityError, ch.Origin, "/fields/"+f.Name+"/fieldGroup", CodeFieldGroupLoosened,
					"set %s.fieldGroup memindahkan field dari grup %q ke grup %q yang lebih longgar menurut klasifikasi grup manifest (Requirement 3.6, 3.11)", f.Name, from, to)
			}
		}
	}
}

// groupOf mengembalikan kunci grup (snake_case manifest) dari nilai atribut fieldGroup; kosong = grup default.
func (x *rctx) groupOf(v any) string {
	if s, _ := v.(string); s != "" {
		return snake(s)
	}
	return x.eff.groups.def
}

// checkCrossModuleRequires memastikan go.mod modul pemanggil memuat require ke modul sasaran untuk setiap
// penyertaan, tipe domain, dan relasi lintas modul, selain dependencies manifest (Requirement 6.2, 6.3, 3.5;
// D1, D8). Dilewati bila go.mod tidak diketahui (Module.GoRequires nil).
func (x *rctx) checkCrossModuleRequires() {
	if x.mod.in.GoRequires == nil {
		return
	}
	for _, u := range x.r.Uses {
		if !u.Direct || u.Module == x.r.Module || u.Module == resolve.SDKSlug {
			continue
		}
		ptr, origin := x.useLocation(u)
		x.requireGoMod(u.Module, ptr, origin, fmt.Sprintf("%s %s", useKindText(u.Kind), u.Key))
	}
}

func useKindText(kind string) string {
	if kind == "fieldType" {
		return "tipe domain"
	}
	return "penyertaan"
}

// useLocation mencari pointer dan asal sebuah pemakaian langsung pada dokumen yang diperiksa.
func (x *rctx) useLocation(u resolve.Use) (string, *resolve.Origin) {
	if u.Kind == "fieldType" {
		for _, f := range x.r.Fields {
			if f.DomainType == u.Key {
				o := f.Origin
				if ao, ok := f.AttrOrigin["type"]; ok {
					o = ao
				}
				return "/fields/" + f.Name + "/type", &o
			}
		}
		return "/fields", nil
	}
	if x.doc != nil {
		if list, ok := x.doc.Value["include"].([]any); ok {
			for i, it := range list {
				if m, ok := it.(map[string]any); ok {
					if s, _ := m["source"].(string); s == u.Key {
						return fmt.Sprintf("/include/%d/source", i), nil
					}
				}
			}
		}
	}
	return "/include", nil
}

// requireGoMod menerbitkan galat bila go.mod modul pemanggil tidak me-require modul sasaran.
func (x *rctx) requireGoMod(targetSlug, ptr string, origin *resolve.Origin, what string) {
	want := "erpora/modules/" + strings.ReplaceAll(targetSlug, "-", "")
	if x.mod.in.GoRequires[want] {
		return
	}
	msg := fmt.Sprintf("%s lintas modul ke modul %q ditolak: %s/go.mod tidak memuat require %s (D1, D8; Requirement 6.2, 6.3)", what, targetSlug, x.mod.in.Dir, want)
	if origin != nil {
		x.atOrigin(SeverityError, *origin, ptr, CodeModuleGoModRequire, "%s", msg)
		return
	}
	x.atDoc(SeverityError, ptr, CodeModuleGoModRequire, "%s", msg)
}

// checkPivotFields memeriksa kolom pivot relasi seperti field biasa (Requirement 2.1, 2.3, 2.4, 2.7, 2.11,
// 1.4): nama, tipe, batasan, default, enum, nama sensitif, dan highly_confidential. Tipe domain tidak
// didukung pada kolom pivot karena tidak diselesaikan oleh resolver.
func (x *rctx) checkPivotFields(rel *resolve.Relation) {
	pf, ok := rel.Attrs["pivotFields"].(map[string]any)
	if !ok || len(pf) == 0 {
		return
	}
	names := make([]string, 0, len(pf))
	for n := range pf {
		names = append(names, n)
	}
	sort.Strings(names)
	prefix := "/relations/" + rel.Name + "/pivotFields/"
	for _, n := range names {
		attrs, _ := pf[n].(map[string]any)
		if attrs == nil {
			continue
		}
		ptr := prefix + n
		for _, v := range naming.Validate(n) {
			x.atRelation(SeverityError, rel, ptr, nameCode(v.Code), "kolom pivot %s: %s", n, v.Error())
		}
		fld := &resolve.Field{Name: n, SourceName: n, Attrs: attrs, Origin: rel.Origin, AttrOrigin: map[string]resolve.Origin{}}
		px := *x
		px.pivotRel, px.pivotPtr = rel, ptr
		typ, _ := attrs["type"].(string)
		if !builtinTypes[typ] {
			px.err(fld, "type", CodePivotFieldType, "kolom pivot %s bertipe %q: hanya tipe bawaan (string, text, integer, decimal, boolean, date, datetime, uuid, enum) yang didukung pada pivotFields", n, typ)
			continue
		}
		px.checkField(fld)
	}
}

// atRelation menunjuk pointer di dalam relasi: pada dokumen resource bila relasi dideklarasikan di sana, atau
// pada asal relasi bila berasal dari dokumen yang disertakan.
func (x *rctx) atRelation(sev Severity, rel *resolve.Relation, ptr, code, format string, args ...any) {
	if x.doc != nil && rel.Origin.File == x.doc.File {
		x.atDoc(sev, ptr, code, format, args...)
		return
	}
	x.atOrigin(sev, rel.Origin, ptr, code, format, args...)
}

// requireGoModForTarget memeriksa require go.mod untuk sasaran relasi lintas modul (dilewati bila go.mod tidak
// diketahui).
func (x *rctx) requireGoModForTarget(slug, ptr, key string) {
	if x.mod.in.GoRequires == nil {
		return
	}
	x.requireGoMod(slug, ptr, nil, "relasi ke "+key)
}
