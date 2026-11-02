// Package migrate membangkitkan berkas migrasi goose dari selisih mesin selisih (paket diff) dan model skema
// turunan metadata (paket dbmodel): spec resource-metadata Requirement 11 (task 11).
//
// Aturan yang ditegakkan:
//   - Perubahan AMAN (expand) dihasilkan otomatis: tabel baru, kolom baru yang nullable atau berdefault, indeks
//     baru, pelonggaran batasan (NOT NULL dilepas, batas panjang atau rentang dilebarkan, nilai enum extensible
//     bertambah), perubahan nilai bawaan, pemicu bangkitan yang hilang atau nonaktif, dan constraint yang
//     menyentuh hanya kolom yang baru ditambahkan pada migrasi yang sama.
//   - Perubahan PERLU KEPUTUSAN (mengganti nama, ubah tipe, perkecil batas, NOT NULL pada kolom ada, unique
//     atau foreign key pada data ada, menghapus apa pun, dan bentuk yang tidak dikenali) hanya menghasilkan
//     kerangka berpenanda TodoMarker, seluruhnya sebagai komentar; metadata:check menggagalkan selama penanda
//     ada (FindTodos). Tidak pernah ada pernyataan destruktif aktif.
//   - Down hanya berisi pembalikan yang aman (menghapus apa yang ditambahkan migrasi yang sama). Bila ada
//     langkah yang tidak dapat dibalik tanpa memeriksa data, Down ditandai eksplisit dan memuat penjaga yang
//     menggagalkan goose down.
//   - Tabel large: true yang memerlukan backfill memakai berkas berpenanda NO TRANSACTION dan templat batch
//     (komentar); BatchCopyBackfill di sqlgen membangkitkan salinan per batch yang aktif (task 12.3).
//   - Semua SQL aktif berasal dari sqlgen (identifier dikutip, literal lewat satu fungsi escape). Teks dari
//     database (nama objek berlebih, definisi) hanya muncul di komentar dan disaring (safeComment).
//   - Berkas tidak pernah diterapkan oleh paket ini.
package migrate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/diff"
	"erpora/internal/devtools/metadatatools/sqlgen"
)

// TodoMarker menandai butir yang perlu keputusan manusia. metadata:check gagal bila berkas migrasi modul
// memuatnya.
const TodoMarker = "TODO(metadata:migrate)"

// DefaultBatch adalah ukuran batch templat backfill pada tabel large: true.
const DefaultBatch = 1000

// Options mengatur Generate.
type Options struct {
	// Batch adalah ukuran batch templat backfill (0 = DefaultBatch).
	Batch int
}

// Todo adalah satu butir yang perlu keputusan.
type Todo struct {
	Table, Object, Kind, Reason string
}

// Result adalah hasil Generate.
type Result struct {
	// SQL adalah isi berkas migrasi lengkap; kosong bila tidak ada selisih.
	SQL   string
	Empty bool
	// Todos adalah butir kerangka; kosong berarti seluruh perubahan otomatis.
	Todos []Todo
	// Changes menjelaskan perubahan otomatis (untuk ringkasan CLI).
	Changes []string
	// Irreversible: Down ditandai tidak dapat dibalik.
	Irreversible bool
	// NoTransaction: berkas berpenanda goose NO TRANSACTION (tabel large dengan backfill).
	NoTransaction bool
}

type stmt struct {
	sql   string
	block bool // dibungkus StatementBegin dan StatementEnd
}

type step struct {
	up, down []stmt
	// comment adalah baris komentar (tanpa awalan "-- ") untuk kerangka; backfill adalah templat komentar.
	comment      []string
	irreversible string
}

const (
	phCreate = iota
	phColumn
	phAlter
	phConstraint
	phIndex
	phForeignKey
	phTrigger
	phTodo
	phCount
)

type generator struct {
	model    *dbmodel.Schema
	rep      *diff.Report
	o        Options
	phases   [phCount][]*step
	res      *Result
	newCols  map[string]map[string]bool
	consumed map[int]bool
}

// Generate membangkitkan migrasi dari laporan selisih. Temuan harus merujuk tabel dan objek yang ada pada
// model (laporan berasal dari diff.Run atas model yang sama); selain itu galat.
func Generate(rep *diff.Report, model *dbmodel.Schema, o Options) (*Result, error) {
	if o.Batch == 0 {
		o.Batch = DefaultBatch
	}
	g := &generator{model: model, rep: rep, o: o, res: &Result{}, newCols: map[string]map[string]bool{}, consumed: map[int]bool{}}
	if len(rep.Findings) == 0 {
		g.res.Empty = true
		return g.res, nil
	}
	for _, f := range rep.Findings {
		if model.Table(f.Table) == nil {
			return nil, fmt.Errorf("temuan %s: tabel %q tidak ada pada model", f.Kind, f.Table)
		}
	}
	// Tiga lintasan: tabel baru, kolom baru (agar constraint pada kolom baru dikenali), lalu sisanya.
	for i, f := range rep.Findings {
		if f.Kind == diff.KindTableMissing {
			g.consumed[i] = true
			if err := g.tableMissing(f); err != nil {
				return nil, err
			}
		}
	}
	for i, f := range rep.Findings {
		if f.Kind == diff.KindColumnMissing {
			g.consumed[i] = true
			if err := g.columnMissing(f); err != nil {
				return nil, err
			}
		}
	}
	if err := g.checks(); err != nil {
		return nil, err
	}
	for i, f := range rep.Findings {
		if g.consumed[i] {
			continue
		}
		g.consumed[i] = true
		if err := g.other(f); err != nil {
			return nil, err
		}
	}
	g.render()
	return g.res, nil
}

func (g *generator) add(phase int, s *step) { g.phases[phase] = append(g.phases[phase], s) }

func (g *generator) change(format string, args ...any) {
	g.res.Changes = append(g.res.Changes, fmt.Sprintf(format, args...))
}

// ---- tabel dan kolom ----

func (g *generator) tableMissing(f diff.Finding) error {
	t := g.model.Table(f.Table)
	create, err := sqlgen.CreateTable(t, sqlgen.Options{})
	if err != nil {
		return fmt.Errorf("tabel %s: %w", t.Name, err)
	}
	drop, err := sqlgen.DropTable(t, sqlgen.Options{})
	if err != nil {
		return err
	}
	g.add(phCreate, &step{up: []stmt{{sql: create}}, down: []stmt{{sql: drop}}})
	g.change("tabel baru %s", t.Name)
	for _, ix := range t.Indexes {
		q, err := sqlgen.CreateIndex(t, ix, sqlgen.Options{})
		if err != nil {
			return err
		}
		g.add(phIndex, &step{up: []stmt{{sql: q}}}) // dihapus bersama tabel
	}
	for _, fk := range t.ForeignKeys {
		q, err := sqlgen.AddForeignKey(t, fk, sqlgen.Options{})
		if err != nil {
			return err
		}
		d, err := sqlgen.DropConstraint(t, fk.Name, sqlgen.Options{})
		if err != nil {
			return err
		}
		g.add(phForeignKey, &step{up: []stmt{{sql: q}}, down: []stmt{{sql: d}}})
	}
	for _, tr := range t.Triggers {
		if err := g.triggerCreate(t, tr); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) columnMissing(f diff.Finding) error {
	t := g.model.Table(f.Table)
	c := t.Column(f.Object)
	if c == nil {
		return fmt.Errorf("kolom %s.%s tidak ada pada model", f.Table, f.Object)
	}
	if c.NotNull && c.Default == nil && c.Generated == nil {
		nullable := *c
		nullable.NotNull = false
		var sug []string
		if q, err := sqlgen.AddColumn(t, &nullable, sqlgen.Options{}); err == nil {
			sug = append(sug, q)
		}
		sug = append(sug, "(isi nilai awal kolom untuk baris yang ada)")
		if q, err := sqlgen.SetNotNull(t, c.Name, sqlgen.Options{}); err == nil {
			sug = append(sug, q)
		}
		return g.skeleton(f, "kolom NOT NULL tanpa nilai bawaan pada tabel yang sudah ada: tentukan nilai awal (expand, migrate, contract)", sug, c.Name, true)
	}
	add, err := sqlgen.AddColumn(t, c, sqlgen.Options{})
	if err != nil {
		return err
	}
	drop, err := sqlgen.DropColumn(t, c.Name, sqlgen.Options{})
	if err != nil {
		return err
	}
	g.add(phColumn, &step{up: []stmt{{sql: add}}, down: []stmt{{sql: drop}}})
	if g.newCols[t.Name] == nil {
		g.newCols[t.Name] = map[string]bool{}
	}
	g.newCols[t.Name][c.Name] = true
	g.change("kolom baru %s.%s", t.Name, c.Name)
	return nil
}

func (g *generator) onlyNew(table string, cols []string) bool {
	if len(cols) == 0 {
		return false
	}
	for _, c := range cols {
		if !g.newCols[table][c] {
			return false
		}
	}
	return true
}

// ---- CHECK ----

func (g *generator) checks() error {
	byTable := map[string]struct{ missing, extra []int }{}
	for i, f := range g.rep.Findings {
		if g.consumed[i] || (f.Kind != diff.KindCheckMissing && f.Kind != diff.KindCheckExtra) {
			continue
		}
		e := byTable[f.Table]
		if f.Kind == diff.KindCheckMissing {
			e.missing = append(e.missing, i)
		} else {
			e.extra = append(e.extra, i)
		}
		byTable[f.Table] = e
	}
	tables := make([]string, 0, len(byTable))
	for t := range byTable {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, tn := range tables {
		t := g.model.Table(tn)
		e := byTable[tn]
		for _, mi := range e.missing {
			m := g.rep.Findings[mi]
			ck := checkByName(t, m.Object)
			if ck == nil {
				return fmt.Errorf("CHECK %s.%s tidak ada pada model", tn, m.Object)
			}
			g.consumed[mi] = true
			if g.onlyNew(tn, []string{ck.Column}) {
				q, err := sqlgen.AddCheck(t, ck, sqlgen.Options{})
				if err != nil {
					return err
				}
				g.add(phConstraint, &step{up: []stmt{{sql: q}}}) // dihapus bersama kolom
				continue
			}
			// Pasangan: CHECK lama pada kolom yang sama.
			pair := -1
			for _, ei := range e.extra {
				x := g.rep.Findings[ei]
				if !g.consumed[ei] && len(x.Columns) == 1 && x.Columns[0] == ck.Column {
					pair = ei
					break
				}
			}
			if pair >= 0 {
				old := g.rep.Findings[pair]
				g.consumed[pair] = true
				if isLoosening(ck, old.Actual) {
					drop, err := sqlgen.DropConstraint(t, old.Object, sqlgen.Options{})
					if err != nil {
						return err
					}
					add, err := sqlgen.AddCheck(t, ck, sqlgen.Options{})
					if err != nil {
						return err
					}
					g.add(phAlter, &step{up: []stmt{{sql: drop}, {sql: add}}, irreversible: reasonLoosen})
					g.change("pelonggaran CHECK %s.%s", tn, ck.Name)
					continue
				}
				var sug []string
				if q, err := sqlgen.DropConstraint(t, old.Object, sqlgen.Options{}); err == nil {
					sug = append(sug, q)
				}
				if q, err := sqlgen.AddCheck(t, ck, sqlgen.Options{}); err == nil {
					sug = append(sug, q)
				}
				if err := g.skeleton(m, "CHECK diganti dengan batasan yang lebih ketat atau bentuk tak dikenali: periksa data yang ada", sug, "", false); err != nil {
					return err
				}
				continue
			}
			var sug []string
			if q, err := sqlgen.AddCheck(t, ck, sqlgen.Options{}); err == nil {
				sug = append(sug, q)
			}
			if err := g.skeleton(m, "CHECK baru pada kolom yang sudah ada dapat gagal pada data yang ada: periksa data", sug, "", false); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkByName(t *dbmodel.Table, name string) *dbmodel.Check {
	for _, c := range t.Checks {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ---- sisanya ----

const (
	reasonLoosen  = "pelonggaran batasan tidak dapat dibalik tanpa memeriksa data yang ada"
	reasonDefault = "nilai bawaan lama tidak dapat dipulihkan otomatis"
	reasonEnable  = "status pemicu sebelumnya tidak diketahui"
)

func (g *generator) other(f diff.Finding) error {
	t := g.model.Table(f.Table)
	opts := sqlgen.Options{}
	switch f.Kind {
	case diff.KindIndexMissing:
		var ix *dbmodel.Index
		for _, x := range t.Indexes {
			if x.Name == f.Object {
				ix = x
			}
		}
		if ix == nil {
			return fmt.Errorf("indeks %s.%s tidak ada pada model", f.Table, f.Object)
		}
		up, err := sqlgen.CreateIndex(t, ix, opts)
		if err != nil {
			return err
		}
		down, err := sqlgen.DropIndex(ix.Name, opts)
		if err != nil {
			return err
		}
		g.add(phIndex, &step{up: []stmt{{sql: up}}, down: []stmt{{sql: down}}})
		g.change("indeks baru %s", ix.Name)
		return nil
	case diff.KindUniqueMissing:
		var u *dbmodel.Unique
		for _, x := range t.Uniques {
			if x.Name == f.Object {
				u = x
			}
		}
		if u == nil {
			return fmt.Errorf("unique %s.%s tidak ada pada model", f.Table, f.Object)
		}
		up, err := sqlgen.AddUnique(t, u, opts)
		if err != nil {
			return err
		}
		if g.onlyNew(t.Name, u.Columns) {
			g.add(phConstraint, &step{up: []stmt{{sql: up}}})
			return nil
		}
		return g.skeleton(f, "unique pada data yang ada dapat gagal bila ada nilai ganda: periksa data", []string{up}, "", false)
	case diff.KindForeignKeyMissing:
		var fk *dbmodel.ForeignKey
		for _, x := range t.ForeignKeys {
			if x.Name == f.Object {
				fk = x
			}
		}
		if fk == nil {
			return fmt.Errorf("foreign key %s.%s tidak ada pada model", f.Table, f.Object)
		}
		up, err := sqlgen.AddForeignKey(t, fk, opts)
		if err != nil {
			return err
		}
		if g.onlyNew(t.Name, fk.Columns) {
			down, err := sqlgen.DropConstraint(t, fk.Name, opts)
			if err != nil {
				return err
			}
			g.add(phForeignKey, &step{up: []stmt{{sql: up}}, down: []stmt{{sql: down}}})
			return nil
		}
		return g.skeleton(f, "foreign key pada data yang ada dapat gagal bila ada baris yatim: periksa data", []string{up}, "", false)
	case diff.KindColumnNullability:
		return g.nullability(f, t)
	case diff.KindColumnDefault:
		return g.defaultChange(f, t)
	case diff.KindTriggerMissing:
		for _, tr := range t.Triggers {
			if tr.Name == f.Object {
				return g.triggerCreate(t, tr)
			}
		}
		return fmt.Errorf("pemicu %s.%s tidak ada pada model", f.Table, f.Object)
	case diff.KindTriggerDisabled:
		q, err := sqlgen.EnableTrigger(t, f.Object, opts)
		if err != nil {
			return err
		}
		g.add(phTrigger, &step{up: []stmt{{sql: q}}, irreversible: reasonEnable})
		g.change("aktifkan pemicu %s", f.Object)
		return nil
	}
	return g.decision(f, t)
}

func (g *generator) nullability(f diff.Finding, t *dbmodel.Table) error {
	c := t.Column(f.Object)
	if c == nil {
		return fmt.Errorf("kolom %s.%s tidak ada pada model", f.Table, f.Object)
	}
	if !c.NotNull { // model nullable, database NOT NULL: pelonggaran
		q, err := sqlgen.DropNotNull(t, c.Name, sqlgen.Options{})
		if err != nil {
			return err
		}
		g.add(phAlter, &step{up: []stmt{{sql: q}}, irreversible: reasonLoosen})
		g.change("lepas NOT NULL %s.%s", t.Name, c.Name)
		return nil
	}
	var sug []string
	if q, err := sqlgen.SetNotNull(t, c.Name, sqlgen.Options{}); err == nil {
		sug = append(sug, "(isi nilai untuk baris yang masih NULL)", q)
	}
	return g.skeleton(f, "NOT NULL pada kolom yang sudah ada: isi dulu baris yang masih NULL", sug, c.Name, true)
}

func (g *generator) defaultChange(f diff.Finding, t *dbmodel.Table) error {
	c := t.Column(f.Object)
	if c == nil {
		return fmt.Errorf("kolom %s.%s tidak ada pada model", f.Table, f.Object)
	}
	var up, down string
	var err error
	reason := ""
	if c.Default != nil {
		if up, err = sqlgen.SetDefault(t, c, sqlgen.Options{}); err != nil {
			return err
		}
	} else if up, err = sqlgen.DropDefault(t, c.Name, sqlgen.Options{}); err != nil {
		return err
	}
	if f.Actual == "" {
		if down, err = sqlgen.DropDefault(t, c.Name, sqlgen.Options{}); err != nil {
			return err
		}
	} else {
		reason = reasonDefault
	}
	s := &step{up: []stmt{{sql: up}}, irreversible: reason}
	if down != "" {
		s.down = []stmt{{sql: down}}
	}
	g.add(phAlter, s)
	g.change("nilai bawaan %s.%s", t.Name, c.Name)
	return nil
}

func (g *generator) triggerCreate(t *dbmodel.Table, tr *dbmodel.Trigger) error {
	sts, err := sqlgen.TriggerStatements(t, tr, sqlgen.Options{})
	if err != nil {
		return err
	}
	var up []stmt
	for _, s := range sts {
		up = append(up, stmt{sql: s.SQL, block: s.Kind == sqlgen.KindCreateFunction})
	}
	dt, err := sqlgen.DropTrigger(t, tr.Name, sqlgen.Options{})
	if err != nil {
		return err
	}
	df, err := sqlgen.DropFunction(tr.Function, sqlgen.Options{})
	if err != nil {
		return err
	}
	g.add(phTrigger, &step{up: up, down: []stmt{{sql: dt}, {sql: df}}})
	g.change("pemicu %s", tr.Name)
	return nil
}

// decision menghasilkan kerangka untuk selisih yang selalu perlu keputusan.
func (g *generator) decision(f diff.Finding, t *dbmodel.Table) error {
	opts := sqlgen.Options{}
	var sug []string
	reason := "selisih perlu keputusan"
	switch f.Kind {
	case diff.KindColumnType:
		reason = "mengubah tipe kolom dapat gagal atau menghilangkan data"
		if c := t.Column(f.Object); c != nil {
			if n, err := sqlgen.QuoteIdent(t.Name); err == nil {
				if cq, err := sqlgen.QuoteIdent(c.Name); err == nil {
					sug = append(sug, "ALTER TABLE "+n+" ALTER COLUMN "+cq+" TYPE "+c.Type.String()+" USING (tentukan konversi)")
				}
			}
		}
	case diff.KindColumnGenerated:
		reason = "mengganti ekspresi kolom turunan membutuhkan hapus dan tambah kolom"
	case diff.KindColumnExtra:
		reason = "kolom di database tidak ada di metadata: hapus hanya pada rilis MAJOR setelah deprecated, atau deklarasikan di metadata"
		if n, err := sqlgen.QuoteIdent(t.Name); err == nil {
			if cq, err := sqlgen.QuoteIdent(f.Object); err == nil {
				sug = append(sug, "ALTER TABLE "+n+" DROP COLUMN "+cq)
			}
		}
	case diff.KindUniqueExtra, diff.KindForeignKeyExtra, diff.KindCheckExtra:
		reason = "constraint di database tidak ada di metadata: hapus atau deklarasikan"
		if q, err := sqlgen.DropConstraint(t, f.Object, opts); err == nil {
			sug = append(sug, q)
		}
	case diff.KindIndexExtra:
		reason = "indeks di database tidak ada di metadata: hapus atau deklarasikan (indexes atau index: true)"
		if q, err := sqlgen.DropIndex(f.Object, opts); err == nil {
			sug = append(sug, q)
		}
	case diff.KindPrimaryKey:
		reason = "kunci utama berbeda dari metadata"
	case diff.KindTriggerChanged:
		reason = "pemicu berbeda dari templat bangkitan: tinjau sebelum mengganti"
	}
	return g.skeleton(f, reason, sug, "", false)
}

// skeleton menambahkan butir kerangka (komentar saja). backfillColumn tidak kosong pada tabel large: true
// menambahkan templat backfill per batch dan menandai berkas NO TRANSACTION.
func (g *generator) skeleton(f diff.Finding, reason string, suggestions []string, backfillColumn string, needsValue bool) error {
	t := g.model.Table(f.Table)
	obj := f.Object
	g.res.Todos = append(g.res.Todos, Todo{Table: f.Table, Object: obj, Kind: f.Kind, Reason: reason})
	s := &step{}
	head := fmt.Sprintf("%s: %s [%s tabel %s", TodoMarker, reason, safeComment(f.Kind), safeComment(f.Table))
	if obj != "" {
		head += " objek " + safeComment(obj)
	}
	s.comment = append(s.comment, head+"]")
	if f.Expected != "" || f.Actual != "" {
		s.comment = append(s.comment, fmt.Sprintf("diharapkan %s; aktual %s", safeComment(f.Expected), safeComment(f.Actual)))
	}
	for _, q := range suggestions {
		if strings.HasPrefix(q, "(") {
			s.comment = append(s.comment, safeSuggestion(q))
			continue
		}
		s.comment = append(s.comment, safeSuggestion(q)+";")
	}
	if needsValue && t != nil && t.Large && backfillColumn != "" {
		tpl, err := sqlgen.BatchBackfillTemplate(t, backfillColumn, g.o.Batch, sqlgen.Options{})
		if err == nil {
			s.comment = append(s.comment, "Tabel large: true: backfill per batch di luar transaksi (berkas ini berpenanda NO TRANSACTION):")
			for _, l := range strings.Split(strings.TrimRight(tpl, "\n"), "\n") {
				s.comment = append(s.comment, "\x00"+l) // sudah berawalan "-- "
			}
			g.res.NoTransaction = true
		}
	}
	g.add(phTodo, s)
	return nil
}

var unsafeComment = regexp.MustCompile(`[^A-Za-z0-9_.:=<>,() "'/-]`)

// safeComment menyaring teks dari database agar tidak dapat keluar dari baris komentar atau memuat penanda
// goose: karakter di luar himpunan aman (termasuk baris baru dan tanda plus) diganti "?".
func safeComment(s string) string {
	s = unsafeComment.ReplaceAllString(s, "?")
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// safeSuggestion menyaring saran SQL (dari sqlgen; nilai dari model) menjadi satu baris komentar aman.
func safeSuggestion(s string) string {
	s = strings.NewReplacer("\n", " ", "\r", " ", "+goose", "?goose").Replace(s)
	return s
}

// ---- keluaran ----

func (g *generator) render() {
	var up, down []stmt
	var commentUp []string
	var steps []*step
	for _, ph := range g.phases {
		steps = append(steps, ph...)
	}
	reasons := map[string]bool{}
	for _, s := range steps {
		up = append(up, s.up...)
		commentUp = append(commentUp, s.comment...)
		if s.irreversible != "" {
			reasons[s.irreversible] = true
		}
	}
	for i := len(steps) - 1; i >= 0; i-- {
		down = append(down, steps[i].down...)
	}

	var sb strings.Builder
	if g.res.NoTransaction {
		sb.WriteString("-- +goose NO TRANSACTION\n")
	}
	sb.WriteString("-- +goose Up\n")
	sb.WriteString("-- Dibangkitkan oleh erpora metadata:migrate dari selisih antara metadata dan database. Tinjau sebelum diterapkan;\n")
	sb.WriteString("-- berkas ini tidak pernah diterapkan otomatis. Jangan menulis penanda anotasi goose di komentar.\n")
	if len(g.res.Todos) > 0 {
		fmt.Fprintf(&sb, "-- %s: %d butir perlu keputusan di bawah; lengkapi lalu hapus penanda TODO. metadata:check gagal selama penanda ada.\n", TodoMarker, len(g.res.Todos))
	}
	writeStmts(&sb, up)
	for _, c := range commentUp {
		if strings.HasPrefix(c, "\x00") {
			sb.WriteString(c[1:] + "\n")
			continue
		}
		sb.WriteString("-- " + c + "\n")
	}

	sb.WriteString("\n-- +goose Down\n")
	if len(reasons) > 0 {
		g.res.Irreversible = true
		var rs []string
		for r := range reasons {
			rs = append(rs, r)
		}
		sort.Strings(rs)
		sb.WriteString("-- TIDAK DAPAT DIBALIK: " + strings.Join(rs, "; ") + ".\n")
		body := "BEGIN RAISE EXCEPTION " + mustLiteral("migrasi tidak dapat dibalik: "+strings.ReplaceAll(strings.Join(rs, "; "), "%", "%%")) + "; END"
		writeStmts(&sb, []stmt{{sql: "DO " + sqlgen.DollarQuote(body), block: true}})
	} else {
		writeStmts(&sb, down)
	}
	if len(g.res.Todos) > 0 {
		fmt.Fprintf(&sb, "-- %s: tulis Down untuk butir perlu keputusan di atas.\n", TodoMarker)
	}
	g.res.SQL = sb.String()
}

func mustLiteral(s string) string {
	l, err := sqlgen.Literal(s)
	if err != nil {
		panic(err) // teks konstan dari pembangkit; tidak mungkin tidak sah
	}
	return l
}

func writeStmts(sb *strings.Builder, stmts []stmt) {
	for _, s := range stmts {
		if s.block {
			sb.WriteString("-- +goose StatementBegin\n" + s.sql + ";\n-- +goose StatementEnd\n")
			continue
		}
		sb.WriteString(s.sql + ";\n")
	}
}
