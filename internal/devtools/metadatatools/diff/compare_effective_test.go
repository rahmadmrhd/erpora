package diff

import "testing"

func lockTrg() Trigger {
	return Trigger{Name: "m_things_lock_trg", Enabled: true, Always: true, Type: 27, Function: "m_things_lock_fn", Body: "BEGIN END",
		Def: "CREATE TRIGGER m_things_lock_trg BEFORE DELETE OR UPDATE ON m_things FOR EACH ROW EXECUTE FUNCTION m_things_lock_fn()", Lang: "plpgsql"}
}

func tableWithTrigger(ts ...Trigger) *Table {
	t := baseTable()
	t.Triggers = ts
	return t
}

// Pemicu efektif (Requirement 13.6, 13.10): klausa WHEN, UPDATE OF, skema fungsi, SET fungsi, dan SECURITY DEFINER
// membuat pemicu berbeda walau nama, jenis, dan badan sama.
func TestCompare_TriggerEffectiveness(t *testing.T) {
	want := cat(tableWithTrigger(lockTrg()))
	mutations := map[string]func(*Trigger){
		"WHEN (false)": func(x *Trigger) { x.Def = x.Def + " WHEN (false)" },
		"UPDATE OF kolom": func(x *Trigger) {
			x.Def = "CREATE TRIGGER m_things_lock_trg BEFORE DELETE OR UPDATE OF note ON m_things FOR EACH ROW EXECUTE FUNCTION m_things_lock_fn()"
		},
		"fungsi di skema lain": func(x *Trigger) {
			x.Def = "CREATE TRIGGER m_things_lock_trg BEFORE DELETE OR UPDATE ON m_things FOR EACH ROW EXECUTE FUNCTION evil.m_things_lock_fn()"
		},
		"SET search_path":  func(x *Trigger) { x.Config = "search_path=pg_catalog" },
		"SECURITY DEFINER": func(x *Trigger) { x.SecDef = true },
		"bahasa fungsi":    func(x *Trigger) { x.Lang = "sql" },
	}
	for name, mut := range mutations {
		t.Run(name, func(t *testing.T) {
			x := lockTrg()
			mut(&x)
			r := Compare(cat(tableWithTrigger(x)), want, nil, CompareOptions{})
			if kinds(r) != KindTriggerChanged {
				t.Fatalf("kinds = %q\n%s", kinds(r), r)
			}
		})
	}
	if r := Compare(cat(tableWithTrigger(lockTrg())), want, nil, CompareOptions{}); !r.Empty() {
		t.Fatalf("identik harus kosong: %s", r)
	}
}

// Pemicu tulis tangan BEFORE ROW yang berjalan sesudah pemicu bangkitan dan dapat mengubah NEW adalah selisih;
// yang berjalan lebih dulu, bukan BEFORE ROW, atau tidak aktif hanya catatan.
func TestCompare_HandwrittenTriggerAfterLockIsFinding(t *testing.T) {
	want := cat(tableWithTrigger(lockTrg()))
	mk := func(name string, typ int, enabled bool) *Catalog {
		return cat(tableWithTrigger(lockTrg(), Trigger{Name: name, Enabled: enabled, Type: typ}))
	}
	r := Compare(mk("zz_modify_trg", 1|2|16, true), want, nil, CompareOptions{})
	if kinds(r) != KindTriggerShadow {
		t.Fatalf("BEFORE ROW UPDATE sesudah pemicu bangkitan: %q\n%s", kinds(r), r)
	}
	for name, c := range map[string]*Catalog{
		"nama lebih awal": mk("aa_audit_trg", 1|2|16, true),
		"AFTER ROW":       mk("zz_after_trg", 1|16, true),
		"statement-level": mk("zz_stmt_trg", 2|16, true),
		"tidak aktif":     mk("zz_off_trg", 1|2|16, false),
		"hanya DELETE":    mk("zz_del_trg", 1|2|8, true),
	} {
		r := Compare(c, want, nil, CompareOptions{})
		if !r.Empty() || len(r.Notes) != 1 || r.Notes[0].Kind != KindTriggerExtra {
			t.Errorf("%s: harus hanya catatan: %s", name, r)
		}
	}
}

func TestCompare_ColumnCollationIdentityAndInvalidIndex(t *testing.T) {
	a := baseTable()
	a.Columns[1].Collation = "C"
	a.Columns[0].Identity = "a"
	a.Indexes[0].Invalid = true
	r := Compare(cat(a), cat(baseTable()), nil, CompareOptions{})
	got := kinds(r)
	for _, want := range []string{KindColumnCollation, KindColumnIdentity, KindIndexInvalid} {
		if !contains([]string{got}, got) || !containsKind(r, want) {
			t.Errorf("tidak memuat %s: %s", want, r)
		}
	}
}

func containsKind(r *Report, kind string) bool {
	for _, f := range r.Findings {
		if f.Kind == kind {
			return true
		}
	}
	return false
}
