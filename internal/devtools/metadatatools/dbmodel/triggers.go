package dbmodel

import (
	"fmt"
	"strings"

	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
)

// deriveTriggers membuat pemicu penguncian untuk resource dokumen dan pemicu anak untuk anak embedded, pivot
// belongsToMany, dan tabel seberang hasOne (Requirement 13.2, 13.8, 13.11; design D-19, D-26, D-37, D-38).
// Seluruh parameter berasal dari metadata terselesaikan: lockWhen, tabel transisi lifecycle bawaan, transisi
// sumbu status, dan pasangan migrateFrom. Badan fungsi dibuat sqlgen dari templat tetap.
func (d *deriver) deriveTriggers(rs []*resolve.Resolved) {
	funcs := naming.NewNamespace("fungsi")
	for _, r := range rs {
		d.resource = r
		t := d.tables[r.Key]
		if t == nil || !r.IsDocument() {
			continue
		}
		d.documentLock(r, t, funcs)
		d.childLocks(r, t, funcs)
	}
	d.resource = nil
}

func (d *deriver) triggerNames(t *Table, purpose string, funcs *naming.Namespace) (trg, fn string, ok bool) {
	trg, err := naming.TriggerName(t.Name, purpose)
	if err != nil {
		d.nameErr(err)
		return "", "", false
	}
	snake, err := naming.ToSnake(purpose)
	if err != nil {
		d.nameErr(err)
		return "", "", false
	}
	fn, err = naming.JoinName(t.Name, snake, "fn")
	if err != nil {
		d.nameErr(err)
		return "", "", false
	}
	d.claimCon(t.Name, trg, "pemicu "+purpose)
	if err := funcs.Claim(fn, "fungsi pemicu "+purpose+" pada "+t.Name); err != nil {
		d.problem(CodeCollision, err.Error())
		return "", "", false
	}
	return trg, fn, true
}

func (d *deriver) documentLock(r *resolve.Resolved, t *Table, funcs *naming.Namespace) {
	if t.Lock == nil {
		d.problem(CodeLock, "resource dokumen tanpa lockWhen yang dapat diturunkan")
		return
	}
	need := func(field string) string {
		s, ok := snake(field)
		if !ok || t.Column(s) == nil {
			d.problem(CodeLock, fmt.Sprintf("resource dokumen wajib punya kolom %s (sdk:document dan sdk:audit)", field))
			return ""
		}
		return s
	}
	spec := &LockSpec{
		Condition:       t.Lock,
		StatusColumn:    need("status"),
		VersionColumn:   need("version"),
		UpdatedAtColumn: need("updatedAt"),
		UpdatedByColumn: need("updatedBy"),
	}
	if spec.StatusColumn == "" || spec.VersionColumn == "" || spec.UpdatedAtColumn == "" || spec.UpdatedByColumn == "" {
		return
	}
	for _, tr := range r.Lifecycle.Transitions {
		spec.Transitions = append(spec.Transitions, [2]string{tr.From, tr.To})
	}
	for _, ax := range r.StatusAxes {
		col, ok := snake(ax.Name)
		if !ok || t.Column(col) == nil {
			d.problem(CodeLock, fmt.Sprintf("sumbu status %q tidak punya kolom", ax.Name))
			continue
		}
		la := LockAxis{Column: col}
		for _, tr := range ax.Transitions {
			la.Transitions = append(la.Transitions, [2]string{tr.From, tr.To})
		}
		spec.Axes = append(spec.Axes, la)
	}
	for _, c := range t.Columns {
		if c.MigrateFrom != "" && t.Column(c.MigrateFrom) != nil {
			spec.MigratePairs = append(spec.MigratePairs, [2]string{c.Name, c.MigrateFrom})
		}
	}
	trg, fn, ok := d.triggerNames(t, "lock", funcs)
	if !ok {
		return
	}
	t.Triggers = append(t.Triggers, &Trigger{Name: trg, Function: fn, Events: []string{"DELETE", "UPDATE"}, Lock: spec})
}

// inverseFK mencari kolom FK pada resource anak yang menunjuk resource induk lewat tepat satu belongsTo.
func (d *deriver) inverseFK(child *resolve.Resolved, parentKey string) (string, bool) {
	var names []string
	for _, rel := range child.Relations {
		if strOf(rel.Attrs["type"]) == "belongsTo" && strOf(rel.Attrs["to"]) == parentKey {
			names = append(names, rel.Name)
		}
	}
	if len(names) != 1 {
		d.problem(CodeRelation, fmt.Sprintf("anak %s harus mendeklarasikan tepat satu belongsTo ke dokumen %s agar dapat dilindungi pemicu anak (ditemukan %d)", child.Key, parentKey, len(names)))
		return "", false
	}
	col, ok := snake(names[0] + "Id")
	return col, ok
}

func (d *deriver) childLocks(r *resolve.Resolved, parent *Table, funcs *naming.Namespace) {
	add := func(child *Table, fk string) {
		if child.Column(fk) == nil {
			d.problem(CodeRelation, fmt.Sprintf("kolom FK %q tidak ada pada %s", fk, child.Name))
			return
		}
		trg, fn, ok := d.triggerNames(child, camelOf(fk)+"Lock", funcs)
		if !ok {
			return
		}
		child.Triggers = append(child.Triggers, &Trigger{Name: trg, Function: fn, Events: []string{"DELETE", "INSERT", "UPDATE"}, Child: &ChildLock{Parent: parent, FKColumn: fk}})
	}
	for _, rel := range r.Relations {
		switch strOf(rel.Attrs["type"]) {
		case "hasMany":
			if !relBool(rel, "embedded") {
				continue
			}
			of := strOf(rel.Attrs["of"])
			child, cr := d.tables[of], d.byKey[of]
			if child == nil || cr == nil {
				continue // anak lintas modul atau tidak ada pada model: dilaporkan relasi lain
			}
			if fk, ok := d.inverseFK(cr, r.Key); ok {
				add(child, fk)
			}
		case "hasOne":
			to := strOf(rel.Attrs["to"])
			child, cr := d.tables[to], d.byKey[to]
			if child == nil || cr == nil {
				continue
			}
			if fk, ok := d.inverseFK(cr, r.Key); ok {
				add(child, fk)
			}
		case "belongsToMany":
			for _, pt := range d.schema.Tables {
				if pt.Kind == KindPivot && pt.Resource == r.Key && pt.Relation == rel.Name && len(pt.PrimaryKey) > 0 {
					add(pt, pt.PrimaryKey[0])
				}
			}
		}
	}
}

// camelOf mengubah snake_case sederhana (hasil snake) kembali ke camelCase untuk penamaan pemicu.
func camelOf(s string) string {
	c, err := naming.FromSnake(s)
	if err != nil {
		return strings.ReplaceAll(s, "_", "")
	}
	return c
}
