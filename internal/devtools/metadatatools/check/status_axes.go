package check

import "regexp"

var reAxisValue = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// checkStatusAxes memvalidasi statusAxes (K24, Requirement 13.14, 13.15, 13.18): hanya pada resource dokumen,
// nama bukan status dan bebas tabrakan (tabrakan antar field termasuk field sumbu ditangani resolver dan
// pemeriksaan penamaan), nilai snake_case unik, initial dan setiap from/to termasuk values, tanpa pasangan ganda,
// dan field sumbu tetap readOnly. Transisi mundur atau siklik sah; tidak ada urutan tersirat.
func (x *rctx) checkStatusAxes() {
	for _, ax := range x.r.StatusAxes {
		ptr := "/statusAxes/" + ax.Name
		report := func(code, format string, args ...any) {
			x.atOrigin(SeverityError, ax.Origin, ptr, code, format, args...)
		}
		if !x.isDoc {
			report(CodeStatusAxisNotDocument,
				"sumbu status %s hanya sah pada resource dokumen: sertakan sdk:document (Requirement 13.14)", ax.Name)
		}
		if ax.Name == "status" {
			report(CodeStatusAxisName, "sumbu tidak boleh bernama status: status adalah siklus hidup baku dokumen dan tidak dapat didefinisikan ulang atau diperluas (Requirement 13.18)")
		}
		if len(ax.Values) == 0 {
			report(CodeStatusAxisValues, "sumbu %s tidak punya values", ax.Name)
		}
		known := map[string]bool{}
		for _, v := range ax.Values {
			if !reAxisValue.MatchString(v) {
				report(CodeStatusAxisValues, "nilai %q pada sumbu %s harus snake_case ^[a-z][a-z0-9_]{0,62}$ (Requirement 2.11)", v, ax.Name)
			}
			if known[v] {
				report(CodeStatusAxisValues, "nilai %q pada sumbu %s ganda", v, ax.Name)
			}
			known[v] = true
		}
		if !known[ax.Initial] {
			report(CodeStatusAxisInitial, "initial %q bukan anggota values sumbu %s (%s)", ax.Initial, ax.Name, joinNames(ax.Values))
		}
		seen := map[[2]string]bool{}
		for i, t := range ax.Transitions {
			for _, end := range []struct{ role, val string }{{"from", t.From}, {"to", t.To}} {
				if !known[end.val] {
					report(CodeStatusAxisTransition, "transisi %d sumbu %s: %s %q bukan anggota values (%s)", i, ax.Name, end.role, end.val, joinNames(ax.Values))
				}
			}
			key := [2]string{t.From, t.To}
			if seen[key] {
				report(CodeStatusAxisTransitionDuplicate, "pasangan transisi %s -> %s pada sumbu %s dideklarasikan lebih dari sekali", t.From, t.To, ax.Name)
			}
			seen[key] = true
		}
		if f := x.fields[ax.Name]; f != nil && (!f.Bool("readOnly") || f.Bool("nullable") || f.Type() != "enum") {
			report(CodeStatusAxisReadOnly, "field sumbu %s harus enum readOnly bukan-nullable (Requirement 13.14, 13.17)", ax.Name)
		}
	}
}
