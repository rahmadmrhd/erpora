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

// TriggerSearchPath adalah search_path tetap fungsi pemicu bangkitan (keputusan pemilik produk, 2 Nov 2026):
// pg_catalog lebih dulu dan pg_temp terakhir, sehingga fungsi dan operator tak terkualifikasi pada badan
// (format, current_setting, to_jsonb, translate, strpos, operator jsonb, dsb.) selalu dari pg_catalog dan tidak dapat
// dibelokkan oleh skema yang disisipkan penyerang lewat search_path sesi. Seluruh badan hanya memakai objek
// pg_catalog; tabel induk dan anak dirujuk lewat %I.%I dengan TG_TABLE_SCHEMA. Pengaturan ini ikut proconfig dan
// dibandingkan mesin selisih (pemicu lama tanpa SET terdeteksi trigger_changed lalu diganti CREATE OR REPLACE).
const TriggerSearchPath = "pg_catalog, pg_temp"

// TriggerStatements menghasilkan CREATE FUNCTION, CREATE TRIGGER, dan ENABLE ALWAYS TRIGGER untuk satu pemicu
// bangkitan: pemicu penguncian dokumen (tr.Lock) atau pemicu anak (tr.Child). Badan fungsi berasal dari
// templat tetap di bawah; satu-satunya bagian yang berasal dari model adalah identifier (selalu dikutip),
// literal (lewat Literal), dan kondisi lockWhen yang diterjemahkan dari tata bahasa tertutup. Tidak ada
// teks berkas yang disalin. ENABLE ALWAYS membuat pemicu tetap aktif walau session_replication_role =
// replica (Requirement 13.9).
func TriggerStatements(t *dbmodel.Table, tr *dbmodel.Trigger, o Options) ([]Statement, error) {
	set := 0
	for _, ok := range []bool{tr.Lock != nil, tr.Child != nil, tr.Truncate != nil} {
		if ok {
			set++
		}
	}
	if set != 1 {
		return nil, fmt.Errorf("pemicu %s: tepat satu dari Lock, Child, dan Truncate harus diisi", tr.Name)
	}
	if len(tr.Events) == 0 {
		return nil, fmt.Errorf("pemicu %s: tanpa peristiwa", tr.Name)
	}
	allowed := map[string]bool{"UPDATE": true, "DELETE": true}
	if tr.Child != nil {
		allowed["INSERT"] = true
	}
	if tr.Truncate != nil {
		allowed = map[string]bool{"TRUNCATE": true}
	}
	for _, ev := range tr.Events {
		if !allowed[ev] {
			return nil, fmt.Errorf("pemicu %s: peristiwa %q tidak didukung templat", tr.Name, ev)
		}
	}
	var body string
	var err error
	switch {
	case tr.Lock != nil:
		body, err = lockFunctionBody(t, tr)
	case tr.Truncate != nil:
		body, err = truncateFunctionBody(t, tr)
	default:
		body, err = childFunctionBody(t, tr)
	}
	if err != nil {
		return nil, err
	}
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
	enable, err := EnableTrigger(t, tr.Name, o)
	if err != nil {
		return nil, err
	}
	granularity := "FOR EACH ROW"
	if tr.Truncate != nil {
		granularity = "FOR EACH STATEMENT" // TRUNCATE tidak memicu pemicu baris
	}
	return []Statement{
		{Kind: KindCreateFunction, Table: t.Name, Object: tr.Function, SQL: "CREATE FUNCTION " + fn + "() RETURNS trigger LANGUAGE plpgsql SET search_path = " + TriggerSearchPath + " AS " + DollarQuote(body)},
		{Kind: KindCreateTrigger, Table: t.Name, Object: tr.Name, SQL: "CREATE TRIGGER " + trg + " BEFORE " + strings.Join(tr.Events, " OR ") + " ON " + table + " " + granularity + " EXECUTE FUNCTION " + fn + "()"},
		{Kind: KindEnableTrigger, Table: t.Name, Object: tr.Name, SQL: enable},
	}, nil
}

func raiseStmt(message string) (string, error) {
	msg, err := Literal(strings.ReplaceAll(message, "%", "%%"))
	if err != nil {
		return "", err
	}
	return "RAISE EXCEPTION " + msg + " USING ERRCODE = '" + lockSQLState + "';", nil
}

func colRef(t *dbmodel.Table, prefix, name string) (string, error) {
	if findColumn(t, name) == nil {
		return "", fmt.Errorf("kolom %q tidak ada pada %s", name, t.Name)
	}
	q, err := QuoteIdent(name)
	if err != nil {
		return "", err
	}
	return prefix + "." + q, nil
}

func pairList(pairs [][2]string) (string, error) {
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		a, err := Literal(p[0])
		if err != nil {
			return "", err
		}
		b, err := Literal(p[1])
		if err != nil {
			return "", err
		}
		parts[i] = "(" + a + ", " + b + ")"
	}
	return strings.Join(parts, ", "), nil
}

// lockFunctionBody: perilaku (dievaluasi pada OLD; design D-19):
//   - DELETE baris terkunci selalu ditolak.
//   - UPDATE baris terkunci hanya lolos bila (a) hanya status, version, updated_at, updated_by berubah dan
//     pasangan (status lama, baru) ada di Transitions; (a2) hanya kolom sumbu status bersama version,
//     updated_at, updated_by berubah, sekurangnya satu sumbu berubah, dan tiap sumbu yang berubah dengan
//     pasangan sah; atau (b) hanya kolom tujuan migrateFrom berubah, dari NULL menjadi salinan persis asal.
//   - Tidak ada mekanisme pengecualian lain.
//
// Kolom turunan dikecualikan dari perbandingan baris karena pada pemicu BEFORE nilainya belum dihitung.
func lockFunctionBody(t *dbmodel.Table, tr *dbmodel.Trigger) (string, error) {
	ls := tr.Lock
	if ls.Condition == nil {
		return "", fmt.Errorf("pemicu %s: Lock tanpa kondisi", tr.Name)
	}
	locked, err := Condition(ls.Condition, EnvForTable(t), CondOptions{Qualifier: "OLD", Restricted: true})
	if err != nil {
		return "", fmt.Errorf("pemicu %s: lockWhen: %w", tr.Name, err)
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
	raise, err := raiseStmt("dokumen terkunci: perubahan ditolak (" + t.Name + ")")
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("\nBEGIN\n")
	sb.WriteString("  IF TG_OP = 'DELETE' THEN\n")
	sb.WriteString("    IF " + locked + " THEN\n      " + raise + "\n    END IF;\n    RETURN OLD;\n  END IF;\n")
	sb.WriteString("  IF NOT (" + locked + ") THEN\n    RETURN NEW;\n  END IF;\n")

	if len(ls.Transitions) > 0 || len(ls.Axes) > 0 {
		for _, c := range []string{ls.StatusColumn, ls.VersionColumn, ls.UpdatedAtColumn, ls.UpdatedByColumn} {
			if findColumn(t, c) == nil {
				return "", fmt.Errorf("pemicu %s: kolom %q tidak ada pada %s", tr.Name, c, t.Name)
			}
		}
	}
	audit := []string{ls.VersionColumn, ls.UpdatedAtColumn, ls.UpdatedByColumn}

	if len(ls.Transitions) > 0 {
		same, err := sameExcept(append([]string{ls.StatusColumn}, audit...))
		if err != nil {
			return "", err
		}
		oldS, _ := colRef(t, "OLD", ls.StatusColumn)
		newS, _ := colRef(t, "NEW", ls.StatusColumn)
		pairs, err := pairList(ls.Transitions)
		if err != nil {
			return "", err
		}
		sb.WriteString("  -- transisi status lifecycle yang sah\n")
		sb.WriteString("  IF " + same + " AND (" + oldS + ", " + newS + ") IN (" + pairs + ") THEN\n    RETURN NEW;\n  END IF;\n")
	}

	if len(ls.Axes) > 0 {
		var cols, ok, changed []string
		for _, ax := range ls.Axes {
			oldC, err := colRef(t, "OLD", ax.Column)
			if err != nil {
				return "", fmt.Errorf("pemicu %s: sumbu: %w", tr.Name, err)
			}
			newC, _ := colRef(t, "NEW", ax.Column)
			cols = append(cols, ax.Column)
			cond := oldC + " IS NOT DISTINCT FROM " + newC
			if len(ax.Transitions) > 0 {
				pairs, err := pairList(ax.Transitions)
				if err != nil {
					return "", err
				}
				cond += " OR (" + oldC + ", " + newC + ") IN (" + pairs + ")"
			}
			ok = append(ok, "("+cond+")")
			changed = append(changed, oldC+" IS DISTINCT FROM "+newC)
		}
		same, err := sameExcept(append(cols, audit...))
		if err != nil {
			return "", err
		}
		sb.WriteString("  -- transisi sumbu status yang sah (boleh mundur)\n")
		sb.WriteString("  IF " + same + " AND " + strings.Join(ok, " AND ") + " AND (" + strings.Join(changed, " OR ") + ") THEN\n    RETURN NEW;\n  END IF;\n")
	}

	if len(ls.MigratePairs) > 0 {
		var to, conds, changed []string
		for _, p := range ls.MigratePairs {
			to = append(to, p[0])
			if findColumn(t, p[0]) == nil {
				return "", fmt.Errorf("pemicu %s: kolom tujuan %q tidak ada pada %s", tr.Name, p[0], t.Name)
			}
			// Kolom asal dibaca lewat jsonb: boleh sudah tidak ada pada tabel (field lama dihapus pada rilis MAJOR,
			// K18) tanpa membuat fungsi rusak; kolom yang tidak ada dianggap null (tidak ada yang disalin).
			if _, err := QuoteIdent(p[1]); err != nil {
				return "", fmt.Errorf("pemicu %s: kolom asal: %w", tr.Name, err)
			}
			toLit, err := Literal(p[0])
			if err != nil {
				return "", err
			}
			fromLit, err := Literal(p[1])
			if err != nil {
				return "", err
			}
			newTo := "(to_jsonb(NEW) -> " + toLit + ")"
			oldTo := "(to_jsonb(OLD) -> " + toLit + ")"
			oldFrom := "COALESCE(to_jsonb(OLD) -> " + fromLit + ", 'null'::jsonb)"
			conds = append(conds, "("+newTo+" = "+oldTo+" OR ("+oldTo+" = 'null'::jsonb AND "+newTo+" = "+oldFrom+"))")
			changed = append(changed, newTo+" IS DISTINCT FROM "+oldTo)
		}
		same, err := sameExcept(to)
		if err != nil {
			return "", err
		}
		sb.WriteString("  -- pengisian kolom tujuan migrateFrom\n")
		sb.WriteString("  IF " + same + " AND " + strings.Join(conds, " AND ") + " AND (" + strings.Join(changed, " OR ") + ") THEN\n    RETURN NEW;\n  END IF;\n")
	}
	sb.WriteString("  " + raise + "\nEND\n")
	return sb.String(), nil
}

// qualifiedQuery menyusun literal templat untuk format(): predikat dari model disisipkan dengan % digandakan,
// sedangkan penampung %I.%I diisi saat runtime dengan TG_TABLE_SCHEMA dan nama tabel. Dengan begitu tabel selalu
// dirujuk lengkap dengan skema pemicu: tabel bayangan di pg_temp atau skema lain pada search_path tidak dapat
// membelokkan pemeriksaan, dan badan fungsi tetap identik di skema mana pun (mesin selisih membandingkan badan).
func qualifiedQuery(format string) (string, error) {
	return Literal(format)
}

// escFormat menggandakan % pada teks yang dari model agar format() tidak menafsirkannya sebagai penampung.
func escFormat(s string) string { return strings.ReplaceAll(s, "%", "%%") }

// childFunctionBody: INSERT, UPDATE, dan DELETE baris anak ditolak bila baris induk terkunci menurut kondisi
// kunci induk. Induk dibaca dengan FOR SHARE agar transisi induk ke terkunci yang bersamaan menunggu transaksi
// anak selesai (dan sebaliknya), sehingga tidak ada baris anak yang lolos setelah induk terkunci. Pada UPDATE
// yang memindahkan anak ke induk lain, kedua induk diperiksa. Tabel induk dirujuk lewat EXECUTE format() dengan
// skema tabel anak (TG_TABLE_SCHEMA; induk dan anak satu modul sehingga satu skema), bukan nama tanpa kualifikasi
// yang dapat dibelokkan tabel bayangan pg_temp atau search_path lain.
func childFunctionBody(t *dbmodel.Table, tr *dbmodel.Trigger) (string, error) {
	c := tr.Child
	if c.Parent == nil || c.Parent.Lock == nil {
		return "", fmt.Errorf("pemicu %s: tabel induk tanpa kondisi kunci", tr.Name)
	}
	if len(c.Parent.PrimaryKey) != 1 {
		return "", fmt.Errorf("pemicu %s: induk %s harus berkunci utama satu kolom", tr.Name, c.Parent.Name)
	}
	locked, err := Condition(c.Parent.Lock, EnvForTable(c.Parent), CondOptions{Qualifier: "p", Restricted: true})
	if err != nil {
		return "", fmt.Errorf("pemicu %s: kondisi kunci induk: %w", tr.Name, err)
	}
	parentName, err := Literal(c.Parent.Name)
	if err != nil {
		return "", err
	}
	if _, err := QuoteIdent(c.Parent.Name); err != nil {
		return "", err
	}
	pk, err := QuoteIdent(c.Parent.PrimaryKey[0])
	if err != nil {
		return "", err
	}
	oldFK, err := colRef(t, "OLD", c.FKColumn)
	if err != nil {
		return "", fmt.Errorf("pemicu %s: %w", tr.Name, err)
	}
	newFK, _ := colRef(t, "NEW", c.FKColumn)
	raise, err := raiseStmt("dokumen induk terkunci: perubahan anak ditolak (" + t.Name + ")")
	if err != nil {
		return "", err
	}
	// Kunci dulu baris induk tanpa predikat kunci (menunggu transaksi induk yang belum selesai; pada READ
	// COMMITTED hanya baris yang cocok pada snapshot yang ditunggu), lalu periksa keadaan terbaru dengan
	// pernyataan baru.
	lockQ, err := qualifiedQuery("SELECT 1 FROM %I.%I AS p WHERE p." + escFormat(pk) + " = $1 FOR SHARE OF p")
	if err != nil {
		return "", err
	}
	checkQ, err := qualifiedQuery("SELECT EXISTS (SELECT 1 FROM %I.%I AS p WHERE p." + escFormat(pk) + " = $1 AND " + escFormat(locked) + ")")
	if err != nil {
		return "", err
	}
	check := func(ref string) string {
		return "    EXECUTE format(" + lockQ + ", TG_TABLE_SCHEMA, " + parentName + ") USING " + ref + ";\n" +
			"    EXECUTE format(" + checkQ + ", TG_TABLE_SCHEMA, " + parentName + ") INTO parent_locked USING " + ref + ";\n" +
			"    IF parent_locked THEN\n      " + raise + "\n    END IF;\n"
	}
	var sb strings.Builder
	sb.WriteString("\nDECLARE\n  parent_locked boolean;\nBEGIN\n")
	sb.WriteString("  IF TG_OP IN ('INSERT', 'UPDATE') THEN\n" + check(newFK) + "  END IF;\n")
	sb.WriteString("  IF TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND " + oldFK + " IS DISTINCT FROM " + newFK + ") THEN\n" + check(oldFK) + "  END IF;\n")
	sb.WriteString("  IF TG_OP = 'DELETE' THEN\n    RETURN OLD;\n  END IF;\n  RETURN NEW;\nEND\n")
	return sb.String(), nil
}

// isolationGuard menolak TRUNCATE pada transaction_isolation selain READ COMMITTED. Pemeriksaan penjaga TRUNCATE
// membaca baris lewat EXECUTE; pada REPEATABLE READ atau SERIALIZABLE snapshot transaksi dapat lebih tua daripada
// penguncian baris oleh transaksi lain yang baru commit, sehingga baris terkunci tidak terlihat dan TRUNCATE lolos.
// Menolak lebih awal (gagal tertutup) menutup balapan itu: TRUNCATE tabel dokumen hanya sah pada READ COMMITTED,
// tempat tiap pernyataan EXECUTE memakai snapshot baru.
func isolationGuard(t *dbmodel.Table) (string, error) {
	raise, err := raiseStmt("TRUNCATE tabel dokumen ditolak pada transaction_isolation selain read committed (" + t.Name + "): jalankan pada READ COMMITTED")
	if err != nil {
		return "", err
	}
	return "  IF current_setting('transaction_isolation') <> 'read committed' THEN\n    " + raise + "\n  END IF;\n", nil
}

// truncateFunctionBody: TRUNCATE tidak memicu pemicu baris, sehingga pemicu statement-level BEFORE TRUNCATE
// menolak pengosongan tabel dokumen bila ada baris yang terkunci menurut predikat kunci yang sama dengan pemicu
// baris (kondisi lockWhen yang sama, kualifier t), dan pengosongan tabel anak (embedded, pivot, hasOne) bila ada
// baris anak yang induknya terkunci (predikat kunci induk yang sama dengan pemicu anak, kualifier p). TRUNCATE
// tabel yang kosong atau yang semua barisnya belum terkunci (draft) diizinkan. Tabel dirujuk lewat EXECUTE
// format() dengan TG_TABLE_SCHEMA dan TG_TABLE_NAME (tabel bayangan pg_temp atau search_path lain tidak
// berpengaruh) sehingga badan tetap identik di skema mana pun. Hanya READ COMMITTED yang diizinkan (isolationGuard).
func truncateFunctionBody(t *dbmodel.Table, tr *dbmodel.Trigger) (string, error) {
	g := tr.Truncate
	if (g.Condition == nil) == (g.Child == nil) {
		return "", fmt.Errorf("pemicu %s: tepat satu dari Condition dan Child pada Truncate harus diisi", tr.Name)
	}
	if _, err := QuoteIdent(t.Name); err != nil {
		return "", err
	}
	var q, args, what string
	if g.Condition != nil {
		locked, err := Condition(g.Condition, EnvForTable(t), CondOptions{Qualifier: "t", Restricted: true})
		if err != nil {
			return "", fmt.Errorf("pemicu %s: lockWhen: %w", tr.Name, err)
		}
		q, err = qualifiedQuery("SELECT EXISTS (SELECT 1 FROM %I.%I AS t WHERE " + escFormat(locked) + ")")
		if err != nil {
			return "", err
		}
		args = "TG_TABLE_SCHEMA, TG_TABLE_NAME"
		what = "dokumen terkunci: TRUNCATE ditolak ("
	} else {
		c := g.Child
		if c.Parent == nil || c.Parent.Lock == nil {
			return "", fmt.Errorf("pemicu %s: tabel induk tanpa kondisi kunci", tr.Name)
		}
		if len(c.Parent.PrimaryKey) != 1 {
			return "", fmt.Errorf("pemicu %s: induk %s harus berkunci utama satu kolom", tr.Name, c.Parent.Name)
		}
		locked, err := Condition(c.Parent.Lock, EnvForTable(c.Parent), CondOptions{Qualifier: "p", Restricted: true})
		if err != nil {
			return "", fmt.Errorf("pemicu %s: kondisi kunci induk: %w", tr.Name, err)
		}
		if _, err := QuoteIdent(c.Parent.Name); err != nil {
			return "", err
		}
		pk, err := QuoteIdent(c.Parent.PrimaryKey[0])
		if err != nil {
			return "", err
		}
		fk, err := colRef(t, "c", c.FKColumn)
		if err != nil {
			return "", fmt.Errorf("pemicu %s: %w", tr.Name, err)
		}
		parentName, err := Literal(c.Parent.Name)
		if err != nil {
			return "", err
		}
		q, err = qualifiedQuery("SELECT EXISTS (SELECT 1 FROM %I.%I AS c JOIN %I.%I AS p ON p." + escFormat(pk) + " = " + escFormat(fk) + " WHERE " + escFormat(locked) + ")")
		if err != nil {
			return "", err
		}
		args = "TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_TABLE_SCHEMA, " + parentName
		what = "dokumen induk terkunci: TRUNCATE anak ditolak ("
	}
	raise, err := raiseStmt(what + t.Name + ")")
	if err != nil {
		return "", err
	}
	iso, err := isolationGuard(t)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("\nDECLARE\n  is_locked boolean;\nBEGIN\n")
	sb.WriteString(iso)
	sb.WriteString("  EXECUTE format(" + q + ", " + args + ") INTO is_locked;\n")
	sb.WriteString("  IF is_locked THEN\n    " + raise + "\n  END IF;\n")
	sb.WriteString("  RETURN NULL;\nEND\n")
	return sb.String(), nil
}
