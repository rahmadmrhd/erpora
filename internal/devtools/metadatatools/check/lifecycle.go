package check

import (
	"fmt"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
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
		if old == nil && src != f.Name && !x.isAbs {
			// Field asal sudah dihapus dari metadata (rilis MAJOR yang menghapus kolom lama, K18): sah bila baseline
			// rilis terakhir memuatnya deprecated; diputuskan setelah baseline dibaca (resolvePendingMigrateFrom).
			x.c.pendingMF = append(x.c.pendingMF, pendingMigrateFrom{x: x, f: f, src: src})
			continue
		}
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
		if c.Ref != "" {
			bad("ref (%s) tidak dapat diterjemahkan untuk pemicu: bandingkan dengan nilai literal", c.Ref)
		}
		// Accessor stored adalah kolom turunan (GENERATED): pada pemicu BEFORE nilainya belum dihitung dan penerjemah
		// SQL menolaknya (design D-26: dapat diterjemahkan ke SQL); lockWhen harus memakai kolom fisik biasa.
		for _, n := range []string{c.Field, c.Ref} {
			if fld := x.fields[n]; n != "" && fld != nil {
				if acc, ok := fld.Attrs["accessor"].(map[string]any); ok {
					if k, _ := acc["kind"].(string); k == "stored" {
						bad("%s adalah accessor stored (kolom turunan): lockWhen harus memakai kolom fisik biasa", n)
					}
				}
			}
		}
	}
	walk(cond, "$")
}

type pendingMigrateFrom struct {
	x   *rctx
	f   *resolve.Field
	src string
}

// resolvePendingMigrateFrom memutuskan migrateFrom yang merujuk field yang tidak ada lagi di metadata
// (Requirement 12.2, 12.4, 12.8; K18, D-35). Rename lintas rilis: rilis N menambah field baru dan menandai yang
// lama deprecated; rilis MAJOR berikutnya menghapus field lama tetapi migrateFrom pada field baru tetap ada,
// karena migrasi penghapusan memakainya sebagai penjaga (kolom pengganti harus terisi) dan pemicu dokumen
// memakainya sebagai pengecualian salinan. Karena itu migrateFrom ke field yang sudah dihapus sah bila baseline
// rilis terakhir memuat field itu dengan deprecated; baseline memuatnya tanpa deprecated atau tidak memuatnya
// sama sekali adalah galat (salah ketik atau rename tanpa masa transisi); tanpa baseline (modul belum pernah
// dirilis, tanpa git, atau baseline tidak terbaca) hanya peringatan karena tidak dapat diverifikasi.
func (c *checker) resolvePendingMigrateFrom() {
	for _, p := range c.pendingMF {
		base := c.baselines[p.x.r.Module]
		if base == nil {
			p.x.atField(SeverityWarning, p.f, "migrateFrom", CodeMigrateFromUnbaselined,
				"migrateFrom %q pada field %s merujuk field yang tidak ada di metadata dan tidak dapat diverifikasi terhadap rilis terakhir (modul tanpa tag rilis atau baseline tidak terbaca)", p.src, p.f.Name)
			continue
		}
		var old *resolve.Field
		if r := base.Resource(p.x.r.Key); r != nil {
			old = r.Field(p.src)
		}
		if old == nil {
			p.x.err(p.f, "migrateFrom", CodeMigrateFromUnknown, "migrateFrom %q pada field %s bukan field yang ada pada metadata saat ini maupun rilis terakhir", p.src, p.f.Name)
			continue
		}
		if _, dep := old.Attrs["deprecated"].(map[string]any); !dep {
			p.x.err(p.f, "migrateFrom", CodeMigrateFromUnknown,
				"migrateFrom %q pada field %s: field asal sudah dihapus tetapi tidak ditandai deprecated pada rilis terakhir (tandai deprecated lebih dulu; Requirement 12.1)", p.src, p.f.Name)
		}
	}
}

// checkLockWhenPlacement menegakkan bahwa lockWhen hanya sah pada resource berjenis dokumen konkret: resource
// yang menyertakan sdk:document (keputusan pemilik produk, 2 Nov 2026; Requirement 13.1). Resource non-dokumen
// tidak punya status lifecycle yang dapat dikunci, dan metadata abstrak tidak menghasilkan tabel sehingga tidak
// ada pemicu yang dapat menegakkannya. Metadata bawaan SDK dikecualikan (sdk:document membawa kondisi bawaan).
func (x *rctx) checkLockWhenPlacement() {
	if x.r.Module == resolve.SDKSlug {
		return
	}
	if x.isAbs {
		if x.doc != nil && x.doc.Value["lockWhen"] != nil {
			x.atDoc(SeverityError, "/lockWhen", CodeLockWhenNotDocument,
				"lockWhen tidak sah pada metadata abstrak %s: hanya resource dokumen konkret (menyertakan sdk:document) yang boleh punya lockWhen; kondisi kunci bawaan datang dari sdk:document dan resource dokumen boleh memperketatnya", x.r.Key)
		}
		return
	}
	if x.r.LockWhen == nil || x.isDoc {
		return
	}
	origin := x.r.Source
	if n := len(x.r.LockWhenOrigins); n > 0 {
		origin = x.r.LockWhenOrigins[n-1]
	}
	x.atOrigin(SeverityError, origin, "/lockWhen", CodeLockWhenNotDocument,
		"lockWhen hanya sah pada resource dokumen konkret (menyertakan sdk:document); resource %s bukan dokumen (Requirement 13.1)", x.r.Key)
}
