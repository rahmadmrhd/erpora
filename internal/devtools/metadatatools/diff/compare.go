package diff

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/dbmodel"
)

// CompareOptions mengatur Compare.
type CompareOptions struct {
	// NotePrefixes: tabel aktual berawalan ini yang tidak ada di model dilaporkan sebagai catatan (mis.
	// "fixturesales_"). Tabel goose_ tidak pernah dilaporkan. Kosong = tidak ada catatan tabel berlebih.
	NotePrefixes []string
}

// Compare membandingkan katalog aktual (hasil migrasi) dengan katalog diharapkan (DDL bangkitan dari model).
// Hanya tabel yang ada di katalog diharapkan yang diperiksa. model (opsional) memberi daftar kolom tak
// terkelola (unmanagedColumns, Requirement 2.9): kolom itu dan constraint, indeks, serta pemicu yang
// menyentuhnya dikecualikan.
func Compare(actual, expected *Catalog, model *dbmodel.Schema, opts CompareOptions) *Report {
	r := &Report{}
	for _, name := range expected.Names() {
		exp := expected.Tables[name]
		act := actual.Tables[name]
		if act == nil {
			r.Findings = append(r.Findings, Finding{Kind: KindTableMissing, Table: name, Message: "tabel tidak ada di database"})
			continue
		}
		var unmanaged []string
		if model != nil {
			if mt := model.Table(name); mt != nil {
				unmanaged = mt.Unmanaged
			}
		}
		compareColumns(r, name, act, exp, unmanaged)
		compareConstraints(r, name, act, exp, unmanaged)
		compareIndexes(r, name, act, exp, unmanaged)
		compareTriggers(r, name, act, exp)
	}
	for _, name := range actual.Names() {
		if expected.Tables[name] != nil || strings.HasPrefix(name, "goose_") {
			continue
		}
		for _, p := range opts.NotePrefixes {
			if strings.HasPrefix(name, p) {
				r.Notes = append(r.Notes, Finding{Kind: KindTableExtra, Table: name, Message: "tabel di database tidak dideklarasikan sebagai resource metadata"})
				break
			}
		}
	}
	r.sort()
	return r
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func touchesUnmanaged(columns []string, def string, unmanaged []string) bool {
	for _, u := range unmanaged {
		if contains(columns, u) {
			return true
		}
		if re, err := regexp.Compile(`\b` + regexp.QuoteMeta(u) + `\b`); err == nil && re.MatchString(def) {
			return true
		}
	}
	return false
}

func compareColumns(r *Report, table string, act, exp *Table, unmanaged []string) {
	for _, e := range exp.Columns {
		a := act.Column(e.Name)
		if a == nil {
			r.Findings = append(r.Findings, Finding{Kind: KindColumnMissing, Table: table, Object: e.Name, Message: "kolom tidak ada di database", Expected: e.Type})
			continue
		}
		if a.Type != e.Type {
			r.Findings = append(r.Findings, Finding{Kind: KindColumnType, Table: table, Object: e.Name, Message: "tipe berbeda", Expected: e.Type, Actual: a.Type})
		}
		if a.NotNull != e.NotNull {
			r.Findings = append(r.Findings, Finding{Kind: KindColumnNullability, Table: table, Object: e.Name, Message: "NOT NULL berbeda",
				Expected: fmt.Sprintf("notNull=%v", e.NotNull), Actual: fmt.Sprintf("notNull=%v", a.NotNull)})
		}
		if a.Default != e.Default {
			r.Findings = append(r.Findings, Finding{Kind: KindColumnDefault, Table: table, Object: e.Name, Message: "nilai bawaan berbeda", Expected: e.Default, Actual: a.Default})
		}
		if a.Generated != e.Generated {
			r.Findings = append(r.Findings, Finding{Kind: KindColumnGenerated, Table: table, Object: e.Name, Message: "ekspresi kolom turunan berbeda", Expected: e.Generated, Actual: a.Generated})
		}
	}
	for _, a := range act.Columns {
		if exp.Column(a.Name) != nil || contains(unmanaged, a.Name) {
			continue
		}
		r.Findings = append(r.Findings, Finding{Kind: KindColumnExtra, Table: table, Object: a.Name, Message: "kolom di database tidak ada di metadata (deklarasikan, atau catat di unmanagedColumns bila highly_confidential)", Actual: a.Type})
	}
}

// multiset memetakan definisi ke jumlahnya dan satu contoh objek.
type multiset map[string]*entry

type entry struct {
	n       int
	object  string
	columns []string
}

func (m multiset) add(def, object string, columns []string) {
	if e := m[def]; e != nil {
		e.n++
		return
	}
	m[def] = &entry{n: 1, object: object, columns: columns}
}

func diffSets(r *Report, table, missingKind, extraKind string, act, exp multiset) {
	keys := make([]string, 0, len(exp)+len(act))
	seen := map[string]bool{}
	for k := range exp {
		keys = append(keys, k)
		seen[k] = true
	}
	for k := range act {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		var en, an int
		var eo, ao string
		var ec, ac []string
		if e := exp[k]; e != nil {
			en, eo, ec = e.n, e.object, e.columns
		}
		if a := act[k]; a != nil {
			an, ao, ac = a.n, a.object, a.columns
		}
		switch {
		case an < en:
			r.Findings = append(r.Findings, Finding{Kind: missingKind, Table: table, Object: eo, Message: "ada di metadata, tidak ada di database", Expected: k, Columns: ec})
		case an > en:
			r.Findings = append(r.Findings, Finding{Kind: extraKind, Table: table, Object: ao, Message: "ada di database, tidak dideklarasikan di metadata", Actual: k, Columns: ac})
		}
	}
}

func compareConstraints(r *Report, table string, act, exp *Table, unmanaged []string) {
	bucket := func(t *Table, skipUnmanaged bool) map[string]multiset {
		out := map[string]multiset{"u": {}, "c": {}, "f": {}, "p": {}}
		for _, c := range t.Constraints {
			if skipUnmanaged && touchesUnmanaged(c.Columns, c.Def, unmanaged) {
				continue
			}
			out[c.Kind].add(c.Def, c.Name, c.Columns)
		}
		return out
	}
	a, e := bucket(act, true), bucket(exp, false)
	// Kunci utama: harus identik (paling banyak satu).
	if len(a["p"]) != len(e["p"]) || !sameKeys(a["p"], e["p"]) {
		r.Findings = append(r.Findings, Finding{Kind: KindPrimaryKey, Table: table, Message: "kunci utama berbeda",
			Expected: strings.Join(keysOf(e["p"]), "; "), Actual: strings.Join(keysOf(a["p"]), "; ")})
	}
	diffSets(r, table, KindUniqueMissing, KindUniqueExtra, a["u"], e["u"])
	diffSets(r, table, KindCheckMissing, KindCheckExtra, a["c"], e["c"])
	diffSets(r, table, KindForeignKeyMissing, KindForeignKeyExtra, a["f"], e["f"])
}

func keysOf(m multiset) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sameKeys(a, b multiset) bool {
	for k := range a {
		if b[k] == nil {
			return false
		}
	}
	for k := range b {
		if a[k] == nil {
			return false
		}
	}
	return true
}

func compareIndexes(r *Report, table string, act, exp *Table, unmanaged []string) {
	build := func(t *Table, skip bool) multiset {
		m := multiset{}
		for _, ix := range t.Indexes {
			if skip && touchesUnmanaged(ix.Columns, ix.Key, unmanaged) {
				continue
			}
			m.add(ix.Key, ix.Name, ix.Columns)
		}
		return m
	}
	diffSets(r, table, KindIndexMissing, KindIndexExtra, build(act, true), build(exp, false))
}

func compareTriggers(r *Report, table string, act, exp *Table) {
	byName := func(ts []Trigger) map[string]Trigger {
		m := make(map[string]Trigger, len(ts))
		for _, t := range ts {
			m[t.Name] = t
		}
		return m
	}
	a, e := byName(act.Triggers), byName(exp.Triggers)
	names := make([]string, 0, len(e))
	for n := range e {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		want := e[n]
		got, ok := a[n]
		switch {
		case !ok:
			r.Findings = append(r.Findings, Finding{Kind: KindTriggerMissing, Table: table, Object: n, Message: "pemicu bangkitan tidak ada di database"})
		case !got.Enabled || (want.Always && !got.Always):
			r.Findings = append(r.Findings, Finding{Kind: KindTriggerDisabled, Table: table, Object: n, Message: "pemicu nonaktif atau tidak ENABLE ALWAYS (dapat dilewati session_replication_role = replica); migrasi tulis tangan wajib mengaktifkannya kembali dengan ENABLE ALWAYS, Requirement 13.9 dan 13.10"})
		case got.Type != want.Type || got.Function != want.Function || got.Body != want.Body:
			r.Findings = append(r.Findings, Finding{Kind: KindTriggerChanged, Table: table, Object: n, Message: "jenis, fungsi, atau badan pemicu berbeda dari templat bangkitan"})
		}
	}
	var extra []string
	for n := range a {
		if _, ok := e[n]; !ok {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	for _, n := range extra {
		r.Notes = append(r.Notes, Finding{Kind: KindTriggerExtra, Table: table, Object: n, Message: "pemicu di database tidak berasal dari metadata"})
	}
}
