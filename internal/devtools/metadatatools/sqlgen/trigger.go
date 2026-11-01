package sqlgen

import (
	"fmt"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/dbmodel"
)

// lockSQLState adalah SQLSTATE 55000 (object_not_in_prerequisite_state): dokumen terkunci. Server memetakannya
// ke 409 invalid_state.
const lockSQLState = "55000"

// TriggerStatements menghasilkan CREATE FUNCTION dan CREATE TRIGGER untuk satu pemicu penguncian dokumen
// (Requirement 13.2, 13.8, 13.9; design D-19). Badan fungsi berasal dari templat tetap di bawah; satu-satunya
// bagian yang berasal dari model adalah identifier (selalu dikutip), literal (lewat Literal), dan kondisi
// lockWhen yang diterjemahkan dari tata bahasa tertutup. Tidak ada teks berkas yang disalin.
//
// Perilaku (dievaluasi pada OLD):
//   - DELETE baris terkunci selalu ditolak.
//   - UPDATE baris terkunci hanya lolos bila (a) hanya status, version, updated_at, dan updated_by yang
//     berubah dan pasangan (status lama, status baru) ada di Transitions, atau (b) hanya kolom tujuan
//     migrateFrom yang berubah, dari NULL menjadi salinan persis kolom asal.
//   - Tidak ada mekanisme pengecualian lain (penanda sesi, session_replication_role, dan sejenisnya).
//
// Kolom turunan dikecualikan dari perbandingan baris karena pada pemicu BEFORE nilainya belum dihitung.
func TriggerStatements(t *dbmodel.Table, tr *dbmodel.Trigger, o Options) ([]Statement, error) {
	ls := tr.Lock
	if ls == nil || ls.Condition == nil {
		return nil, fmt.Errorf("pemicu %s: hanya pemicu penguncian (Lock) yang didukung templat", tr.Name)
	}
	for _, ev := range tr.Events {
		if ev != "UPDATE" && ev != "DELETE" {
			return nil, fmt.Errorf("pemicu %s: peristiwa %q tidak didukung templat penguncian (UPDATE, DELETE)", tr.Name, ev)
		}
	}
	if len(tr.Events) == 0 {
		return nil, fmt.Errorf("pemicu %s: tanpa peristiwa", tr.Name)
	}
	env := EnvForTable(t)
	locked, err := Condition(ls.Condition, env, CondOptions{Qualifier: "OLD", Restricted: true})
	if err != nil {
		return nil, fmt.Errorf("pemicu %s: lockWhen: %w", tr.Name, err)
	}

	col := func(prefix, name string) (string, error) {
		if findColumn(t, name) == nil {
			return "", fmt.Errorf("pemicu %s: kolom %q tidak ada pada %s", tr.Name, name, t.Name)
		}
		q, err := QuoteIdent(name)
		if err != nil {
			return "", err
		}
		return prefix + "." + q, nil
	}
	arrayOf := func(names []string) (string, error) {
		sort.Strings(names)
		var uniq []string
		for i, n := range names {
			if i == 0 || n != names[i-1] {
				uniq = append(uniq, n)
			}
		}
		lits := make([]string, len(uniq))
		for i, n := range uniq {
			l, err := Literal(n)
			if err != nil {
				return "", err
			}
			lits[i] = l
		}
		return "ARRAY[" + strings.Join(lits, ", ") + "]", nil
	}
	var generated []string
	for _, c := range t.Columns {
		if c.Generated != nil {
			generated = append(generated, c.Name)
		}
	}
	sameExcept := func(except []string) (string, error) {
		arr, err := arrayOf(append(append([]string(nil), except...), generated...))
		if err != nil {
			return "", err
		}
		return "(to_jsonb(NEW) - " + arr + ") = (to_jsonb(OLD) - " + arr + ")", nil
	}

	msg, err := Literal(strings.ReplaceAll("dokumen terkunci: perubahan ditolak ("+t.Name+")", "%", "%%"))
	if err != nil {
		return nil, err
	}
	raise := "RAISE EXCEPTION " + msg + " USING ERRCODE = '" + lockSQLState + "';"

	var sb strings.Builder
	sb.WriteString("\nBEGIN\n")
	sb.WriteString("  IF TG_OP = 'DELETE' THEN\n")
	sb.WriteString("    IF " + locked + " THEN\n      " + raise + "\n    END IF;\n    RETURN OLD;\n  END IF;\n")
	sb.WriteString("  IF NOT (" + locked + ") THEN\n    RETURN NEW;\n  END IF;\n")

	if len(ls.Transitions) > 0 {
		for _, c := range []string{ls.StatusColumn, ls.VersionColumn, ls.UpdatedAtColumn, ls.UpdatedByColumn} {
			if findColumn(t, c) == nil {
				return nil, fmt.Errorf("pemicu %s: kolom %q tidak ada pada %s", tr.Name, c, t.Name)
			}
		}
		same, err := sameExcept([]string{ls.StatusColumn, ls.VersionColumn, ls.UpdatedAtColumn, ls.UpdatedByColumn})
		if err != nil {
			return nil, err
		}
		oldS, _ := col("OLD", ls.StatusColumn)
		newS, _ := col("NEW", ls.StatusColumn)
		pairs := make([]string, len(ls.Transitions))
		for i, p := range ls.Transitions {
			a, err := Literal(p[0])
			if err != nil {
				return nil, err
			}
			b, err := Literal(p[1])
			if err != nil {
				return nil, err
			}
			pairs[i] = "(" + a + ", " + b + ")"
		}
		sb.WriteString("  -- transisi status yang sah\n")
		sb.WriteString("  IF " + same + " AND (" + oldS + ", " + newS + ") IN (" + strings.Join(pairs, ", ") + ") THEN\n    RETURN NEW;\n  END IF;\n")
	}

	if len(ls.MigratePairs) > 0 {
		var to []string
		var conds []string
		for _, p := range ls.MigratePairs {
			to = append(to, p[0])
			oldTo, err := col("OLD", p[0])
			if err != nil {
				return nil, err
			}
			newTo, _ := col("NEW", p[0])
			oldFrom, err := col("OLD", p[1])
			if err != nil {
				return nil, err
			}
			conds = append(conds, "("+newTo+" IS NOT DISTINCT FROM "+oldTo+" OR ("+oldTo+" IS NULL AND "+newTo+" IS NOT DISTINCT FROM "+oldFrom+"))")
		}
		same, err := sameExcept(to)
		if err != nil {
			return nil, err
		}
		sb.WriteString("  -- pengisian kolom tujuan migrateFrom\n")
		sb.WriteString("  IF " + same + " AND " + strings.Join(conds, " AND ") + " THEN\n    RETURN NEW;\n  END IF;\n")
	}
	sb.WriteString("  " + raise + "\nEND\n")

	fn, err := o.qualify(tr.Function)
	if err != nil {
		return nil, err
	}
	trg, err := QuoteIdent(tr.Name)
	if err != nil {
		return nil, err
	}
	table, err := o.qualify(t.Name)
	if err != nil {
		return nil, err
	}
	fnSQL := "CREATE FUNCTION " + fn + "() RETURNS trigger LANGUAGE plpgsql AS " + DollarQuote(sb.String())
	trgSQL := "CREATE TRIGGER " + trg + " BEFORE " + strings.Join(tr.Events, " OR ") + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION " + fn + "()"
	return []Statement{
		{Kind: KindCreateFunction, Table: t.Name, Object: tr.Function, SQL: fnSQL},
		{Kind: KindCreateTrigger, Table: t.Name, Object: tr.Name, SQL: trgSQL},
	}, nil
}
