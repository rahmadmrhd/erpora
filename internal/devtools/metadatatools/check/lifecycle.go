package check

import (
	"fmt"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/sdk/metadata"
)

// checkMigrateFrom memeriksa pasangan migrateFrom (Requirement 12.6): field asal ada, dan fieldGroup serta
// classification identik (CheckWrite dan MaskFields dihitung atas field kanonik).
func (x *rctx) checkMigrateFrom() {
	for _, f := range x.r.Fields {
		src := f.Str("migrateFrom")
		if src == "" {
			continue
		}
		old := x.fields[src]
		if old == nil || src == f.Name {
			if old == nil && x.isAbs {
				continue
			}
			x.err(f, "migrateFrom", CodeMigrateFromUnknown, "migrateFrom %q pada field %s bukan field lain yang ada", src, f.Name)
			continue
		}
		if x.eff.group[f.Name] != x.eff.group[src] {
			x.err(f, "migrateFrom", CodeMigrateFromGroup,
				"pasangan migrateFrom %s dan %s harus berada di fieldGroup yang sama (%s berbeda dari %s)", f.Name, src, x.eff.group[f.Name], x.eff.group[src])
		}
		if x.eff.class[f.Name] != x.eff.class[src] {
			x.err(f, "migrateFrom", CodeMigrateFromClassification,
				"pasangan migrateFrom %s dan %s harus berklasifikasi sama (%s berbeda dari %s)", f.Name, src, x.eff.class[f.Name], x.eff.class[src])
		}
	}
}

// checkSnapshots memeriksa deklarasi salinan field pada relasi (Requirement 14.1, 14.5).
func (x *rctx) checkSnapshots() {
	for _, rel := range x.r.Relations {
		snaps, ok := rel.Attrs["snapshot"].([]any)
		if !ok || len(snaps) == 0 {
			continue
		}
		base := "/relations/" + rel.Name + "/snapshot"
		if relType(rel) != "belongsTo" {
			x.atDoc(SeverityError, base, CodeSnapshotRelationType, "snapshot hanya sah pada relasi belongsTo, bukan %s (Requirement 14.1)", relType(rel))
			continue
		}
		tkey := stringAttr(rel.Attrs, "to")
		target := x.c.res.Resource(tkey)
		slug, _, _ := naming.SplitResourceKey(tkey)
		cross := slug != x.r.Module
		for i, sv := range snaps {
			sm, _ := sv.(map[string]any)
			from, to := stringAttr(sm, "from"), stringAttr(sm, "to")
			ptr := fmt.Sprintf("%s/%d", base, i)
			df := x.fields[to]
			if df == nil {
				x.atDoc(SeverityError, ptr, CodeSnapshotToUnknown, "snapshot: field tujuan %q tidak ada di resource %s", to, x.r.Key)
			} else {
				if !df.Bool("readOnly") {
					x.err(df, "", CodeSnapshotToNotReadOnly, "field tujuan salinan %s harus readOnly bagi klien (Requirement 14.1)", to)
				}
				_, hasGroup := df.Attrs["fieldGroup"]
				_, hasClass := df.Attrs["classification"]
				if !hasGroup || !hasClass {
					x.err(df, "", CodeSnapshotToExplicit, "field tujuan salinan %s wajib menyatakan fieldGroup dan classification secara eksplisit, tanpa nilai bawaan (Requirement 14.5)", to)
				}
			}
			if target == nil {
				continue
			}
			sf := target.Field(from)
			if sf == nil {
				x.atDoc(SeverityError, ptr, CodeSnapshotFromUnknown, "snapshot: field sumber %q tidak ada di resource %s", from, tkey)
				continue
			}
			teff := x.c.effective(target)
			if cross && !sf.Bool("snapshotSource") {
				x.atDoc(SeverityError, ptr, CodeSnapshotSourceFlag,
					"field sumber %s.%s harus dideklarasikan snapshotSource: true oleh modul pemilik; grup dan klasifikasi efektifnya yang dipakai (Requirement 14.5)", tkey, from)
			}
			if df == nil {
				continue
			}
			if classRank(x.eff.class[to]) < classRank(teff.class[from]) {
				x.err(df, "", CodeSnapshotLooser,
					"field salinan %s (%s) lebih longgar daripada sumbernya %s.%s (%s) (Requirement 14.5)", to, x.eff.class[to], tkey, from, teff.class[from])
			}
			if !teff.groups.isDefault(teff.group[from]) && x.eff.groups.isDefault(x.eff.group[to]) {
				x.err(df, "", CodeSnapshotGroupDowngrade,
					"salinan dari grup non-default (%s) ke grup default ditolak: alasan tertulis untuk pelonggaran belum didefinisikan (Requirement 14.5)", teff.group[from])
			}
		}
	}
}

// checkLockWhenSQL memastikan lockWhen resource dokumen dapat diterjemahkan ke SQL pemicu: hanya kolom fisik
// milik sendiri dengan operator eq, ne, in, dan isnull (design D-11, Requirement 13.12).
func (x *rctx) checkLockWhenSQL() {
	if !x.isDoc || x.r.LockWhen == nil {
		return
	}
	cond, err := metadata.ParseCondition(x.r.LockWhen)
	if err != nil {
		return // sudah dilaporkan checkLockWhen
	}
	origin := x.r.Source
	if n := len(x.r.LockWhenOrigins); n > 0 {
		origin = x.r.LockWhenOrigins[n-1]
	}
	cols := x.physicalColumns()
	var walk func(c *metadata.Condition, path string)
	walk = func(c *metadata.Condition, path string) {
		switch c.Kind {
		case metadata.CondAll, metadata.CondAny, metadata.CondNot:
			for i, ch := range c.Children {
				walk(ch, fmt.Sprintf("%s[%d]", path, i))
			}
			return
		}
		bad := func(format string, args ...any) {
			x.atOrigin(SeverityError, origin, "/lockWhen", CodeLockWhenNotSQL,
				"lockWhen resource dokumen harus dapat diterjemahkan ke SQL pemicu (Requirement 13.12): "+format, args...)
		}
		switch c.Op {
		case metadata.OpEq, metadata.OpNe, metadata.OpIn, metadata.OpIsNull:
		default:
			bad("operator %s pada field %s tidak didukung (hanya eq, ne, in, isnull)", c.Op, c.Field)
		}
		for _, n := range []string{c.Field, c.Ref} {
			if n != "" && !cols[n] && x.fields[n] != nil {
				bad("%s bukan kolom fisik (accessor expr atau func tidak dapat dipakai)", n)
			}
		}
	}
	walk(cond, "$")
}
