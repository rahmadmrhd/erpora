// Package migrate membangkitkan berkas migrasi goose dari selisih mesin selisih (paket diff) dan model skema
// turunan metadata (paket dbmodel): spec resource-metadata Requirement 11 (task 11).
//
// Aturan yang ditegakkan:
//   - Perubahan AMAN (expand) dihasilkan otomatis: tabel baru, kolom baru yang nullable atau berdefault, indeks
//     baru, pelonggaran batasan (NOT NULL dilepas, batas panjang atau rentang dilebarkan, nilai enum extensible
//     bertambah), perubahan nilai bawaan, pemicu bangkitan yang hilang, nonaktif, atau berbeda dari templat
//     (diganti di tempat dengan CREATE OR REPLACE FUNCTION dan TRIGGER lalu ENABLE ALWAYS, tanpa DROP agar tidak
//     ada celah tanpa pemicu), salinan data rename (migrateFrom), dan constraint yang menyentuh hanya kolom yang
//     baru ditambahkan pada migrasi yang sama.
//   - Perubahan PERLU KEPUTUSAN (ubah tipe, perkecil batas, NOT NULL pada kolom ada atau kolom baru NOT NULL tanpa
//     nilai bawaan, unique atau foreign key pada data ada, menghapus apa pun, dan bentuk yang tidak dikenali)
//     hanya menghasilkan kerangka berpenanda TodoMarker, seluruhnya sebagai komentar (kecuali penjaga penghapusan
//     yang aktif dan hanya membaca); metadata:check menggagalkan selama penanda ada (FindTodos). Tidak pernah ada
//     pernyataan destruktif aktif.
//   - Down hanya berisi pembalikan yang aman (menghapus apa yang ditambahkan migrasi yang sama). Bila ada
//     langkah yang tidak dapat dibalik tanpa memeriksa data, Down ditandai eksplisit dan memuat penjaga yang
//     menggagalkan goose down.
//   - Tabel large: true yang memerlukan backfill memakai berkas berpenanda NO TRANSACTION dan templat batch
//     (komentar); BatchCopyBackfill di sqlgen membangkitkan salinan per batch yang aktif (task 12.3).
//   - Semua SQL aktif berasal dari sqlgen (identifier dikutip, literal lewat satu fungsi escape). Teks dari
//     database (nama objek berlebih, definisi) hanya muncul di komentar dan disaring (safeComment).
//   - Rename (Requirement 12.2): kolom baru dengan migrateFrom ditambahkan dan datanya disalin dari kolom lama
//     (UPDATE, atau per batch NO TRANSACTION pada tabel large: true) tanpa menghapus kolom lama; pemicu bangkitan
//     yang berubah diganti lebih dulu agar baris terkunci ikut tersalin lewat pengecualian (b).
//   - Penghapusan kolom (K18, Requirement 12.4, 12.8): hanya kerangka TODO dengan penjaga aktif (gagal bila ada
//     baris belum disalin), DROP COLUMN sebagai komentar, dan Down tidak dapat dibalik. Syarat removeAfter dan
//     rilis MAJOR ditegakkan metadata:check terhadap tag rilis terakhir (dijalankan lebih dulu oleh
//     metadata:migrate); paket ini tidak menduplikasi git.
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
	// alt (opsional) adalah varian idempoten yang dipakai pada berkas NO TRANSACTION: tanpa transaksi, kegagalan di
	// tengah berkas meninggalkan sebagian langkah terterap sehingga pengulangan harus aman (mis. ADD COLUMN IF NOT EXISTS).
	alt string
	// txOnly menandai pernyataan yang hanya bermakna dalam transaksi (mis. LOCK TABLE) dan dilewati pada berkas NO TRANSACTION.
	txOnly bool
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
	phBackfill
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
	// removed adalah kolom yang akan dihapus (temuan column_extra) per tabel.
	removed map[string]map[string]bool
}

// Generate membangkitkan migrasi dari laporan selisih. Temuan harus merujuk tabel dan objek yang ada pada
// model (laporan berasal dari diff.Run atas model yang sama); selain itu galat.
func Generate(rep *diff.Report, model *dbmodel.Schema, o Options) (*Result, error) {
	if o.Batch == 0 {
		o.Batch = DefaultBatch
	}
	g := &generator{model: model, rep: rep, o: o, res: &Result{}, newCols: map[string]map[string]bool{}, consumed: map[int]bool{}, removed: map[string]map[string]bool{}}
	if len(rep.Findings) == 0 {
		g.res.Empty = true
		return g.res, nil
	}
	for _, f := range rep.Findings {
		if model.Table(f.Table) == nil {
			return nil, fmt.Errorf("temuan %s: tabel %q tidak ada pada model", f.Kind, f.Table)
		}
	}
	for _, f := range rep.Findings {
		if f.Kind == diff.KindColumnExtra {
			if g.removed[f.Table] == nil {
				g.removed[f.Table] = map[string]bool{}
			}
			g.removed[f.Table][f.Object] = true
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
		if g.foldedIntoColumnRemoval(f) {
			continue
		}
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
		// Rename (migrateFrom): langkah salin dari kolom lama ikut dalam kerangka (komentar) bila kolom lama diketahui.
		copySug, copied := g.copySuggestion(t, c)
		sug = append(sug, copySug...)
		if !copied {
			sug = append(sug, "(isi nilai awal kolom untuk baris yang ada)")
		}
		if q, err := sqlgen.SetNotNull(t, c.Name, sqlgen.Options{}); err == nil {
			sug = append(sug, q)
		}
		placeholder := c.Name
		if copied {
			placeholder = "" // templat salin sudah tersedia; tanpa templat ekspresi bebas
		}
		return g.skeleton(f, "kolom NOT NULL tanpa nilai bawaan pada tabel yang sudah ada: tentukan nilai awal (expand, migrate, contract)", sug, placeholder, true)
	}
	add, err := sqlgen.AddColumn(t, c, sqlgen.Options{})
	if err != nil {
		return err
	}
	drop, err := sqlgen.DropColumn(t, c.Name, sqlgen.Options{})
	if err != nil {
		return err
	}
	addAlt, err := sqlgen.AddColumn(t, c, sqlgen.Options{IfNotExists: true})
	if err != nil {
		return err
	}
	g.add(phColumn, &step{up: []stmt{{sql: add, alt: addAlt}}, down: []stmt{{sql: drop}}})
	if g.newCols[t.Name] == nil {
		g.newCols[t.Name] = map[string]bool{}
	}
	g.newCols[t.Name][c.Name] = true
	g.change("kolom baru %s.%s", t.Name, c.Name)
	return g.copyFromOld(t, c)
}

// copyFromOld menambahkan langkah salinan data bila kolom baru punya migrateFrom (Requirement 12.2): dari kolom lama
// yang masih ada pada model, atau yang akan dihapus pada migrasi yang sama. Kolom lama tidak dihapus di sini.
func (g *generator) copyFromOld(t *dbmodel.Table, c *dbmodel.Column) error {
	if c.MigrateFrom == "" {
		return nil
	}
	if t.Column(c.MigrateFrom) == nil && !g.removed[t.Name][c.MigrateFrom] {
		return nil // kolom lama tidak ada di database ataupun model: tidak ada yang disalin
	}
	if t.Large {
		q, err := sqlgen.BatchCopyBackfill(t, c.Name, c.MigrateFrom, g.o.Batch, sqlgen.Options{})
		if err != nil {
			return err
		}
		g.add(phBackfill, &step{up: []stmt{{sql: q, block: true}}})
		g.res.NoTransaction = true
	} else {
		q, err := sqlgen.CopyColumn(t, c.Name, c.MigrateFrom, sqlgen.Options{})
		if err != nil {
			return err
		}
		g.add(phBackfill, &step{up: []stmt{{sql: q}}})
	}
	g.change("salin data %s.%s dari %s", t.Name, c.Name, c.MigrateFrom)
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
				if isLoosening(ck, old.Actual) && safeIdent(old.Object) {
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
	case diff.KindTriggerChanged:
		for _, tr := range t.Triggers {
			if tr.Name == f.Object {
				return g.triggerReplace(t, tr)
			}
		}
		return g.decision(f, t)
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
		return g.columnRemoval(f, t)
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
	_, err := g.skeletonStep(f, reason, suggestions, backfillColumn, needsValue)
	return err
}

// skeletonStep seperti skeleton dan mengembalikan butir yang ditambahkan (untuk dilengkapi pemanggil).
func (g *generator) skeletonStep(f diff.Finding, reason string, suggestions []string, backfillColumn string, needsValue bool) (*step, error) {
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
		if strings.HasPrefix(q, "\x00") {
			s.comment = append(s.comment, q)
			continue
		}
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
	return s, nil
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
	writeStmts(&sb, up, g.res.NoTransaction)
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
		writeStmts(&sb, []stmt{{sql: "DO " + sqlgen.DollarQuote(body), block: true}}, g.res.NoTransaction)
	} else {
		writeStmts(&sb, down, g.res.NoTransaction)
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

func writeStmts(sb *strings.Builder, stmts []stmt, noTx bool) {
	for _, s := range stmts {
		if noTx && s.txOnly {
			continue
		}
		if noTx && s.alt != "" {
			s.sql = s.alt
		}
		if s.block {
			sb.WriteString("-- +goose StatementBegin\n" + s.sql + ";\n-- +goose StatementEnd\n")
			continue
		}
		sb.WriteString(s.sql + ";\n")
	}
}

const reasonTriggerReplace = "badan pemicu lama tidak dapat dipulihkan otomatis"

// triggerReplace mengganti pemicu bangkitan dengan versi dari templat (parameter berasal dari metadata, mis.
// pasangan migrateFrom baru atau perubahan transisi): lepas pemicu, ganti fungsi, buat ulang pemicu, aktifkan
// ALWAYS; semuanya dalam migrasi yang sama sehingga tidak ada celah tanpa pemicu.
func (g *generator) triggerReplace(t *dbmodel.Table, tr *dbmodel.Trigger) error {
	sts, err := sqlgen.TriggerStatements(t, tr, sqlgen.Options{})
	if err != nil {
		return err
	}
	const fnPrefix, trgPrefix = "CREATE FUNCTION ", "CREATE TRIGGER "
	if len(sts) != 3 || !strings.HasPrefix(sts[0].SQL, fnPrefix) || !strings.HasPrefix(sts[1].SQL, trgPrefix) {
		return fmt.Errorf("pemicu %s: keluaran templat tak terduga", tr.Name)
	}
	// Tanpa DROP TRIGGER: berkas NO TRANSACTION (tabel large) menjalankan tiap pernyataan dengan autocommit,
	// sehingga lepas lalu buat ulang membuka celah tanpa pemicu. OR REPLACE mengganti pemicu di tempat; pemicu
	// tetap aktif (ENABLE ALWAYS menyusul dan idempoten).
	fn := "CREATE OR REPLACE FUNCTION " + strings.TrimPrefix(sts[0].SQL, fnPrefix)
	trg := "CREATE OR REPLACE TRIGGER " + strings.TrimPrefix(sts[1].SQL, trgPrefix)
	g.add(phTrigger, &step{
		up:           []stmt{{sql: fn, block: true}, {sql: trg}, {sql: sts[2].SQL}},
		irreversible: reasonTriggerReplace,
	})
	g.change("ganti pemicu %s", tr.Name)
	return nil
}

// foldedIntoColumnRemoval menyatakan temuan constraint atau indeks berlebih yang seluruh kolomnya akan dihapus:
// objek itu hilang bersama DROP COLUMN sehingga tidak perlu kerangka tersendiri.
func (g *generator) foldedIntoColumnRemoval(f diff.Finding) bool {
	switch f.Kind {
	case diff.KindUniqueExtra, diff.KindIndexExtra, diff.KindCheckExtra, diff.KindForeignKeyExtra:
	default:
		return false
	}
	if len(f.Columns) == 0 {
		return false
	}
	for _, c := range f.Columns {
		if !g.removed[f.Table][c] {
			return false
		}
	}
	return true
}

const (
	reasonDrop   = "penghapusan kolom tidak dapat dibalik: data kolom hilang"
	backupNotice = "BACKUP WAJIB sebelum migrasi MAJOR yang menghapus kolom (docs/module-sdk/lifecycle.md): penghapusan tidak dapat dibalik"
)

// columnRemoval membangkitkan kerangka penghapusan kolom (Requirement 12.4, 12.8; K18, D-35): penjaga aktif yang
// menggagalkan migrasi bila masih ada baris dengan kolom lama terisi dan kolom pengganti (field dengan
// migrateFrom ke kolom ini) kosong, DROP COLUMN hanya sebagai komentar untuk ditinjau, dan Down tidak dapat
// dibalik. Syarat removeAfter dan rilis MAJOR ditegakkan metadata:check terhadap tag rilis terakhir modul
// (pembanding yang sama; dijalankan lebih dulu oleh metadata:migrate), sehingga paket ini tidak menduplikasi git.
func (g *generator) columnRemoval(f diff.Finding, t *dbmodel.Table) error {
	var repl *dbmodel.Column
	for _, c := range t.Columns {
		if c.MigrateFrom == f.Object {
			repl = c
			break
		}
	}
	var sug []string
	if n, err := sqlgen.QuoteIdent(t.Name); err == nil {
		if cq, err := sqlgen.QuoteIdent(f.Object); err == nil {
			sug = append(sug, "ALTER TABLE "+n+" DROP COLUMN "+cq)
		}
	}
	reason := "kolom di database tidak ada di metadata: hapus hanya pada rilis MAJOR setelah deprecated dan removeAfter, atau deklarasikan di metadata"
	s, err := g.skeletonStep(f, reason, sug, "", false)
	if err != nil {
		return err
	}
	s.comment = append(s.comment, backupNotice)
	s.irreversible = reasonDrop
	if repl == nil {
		s.comment = append(s.comment, "tanpa kolom pengganti (tidak ada field dengan migrateFrom ke kolom ini): pastikan data kolom tidak diperlukan")
		return nil
	}
	// Saran salin data dari kolom lama ke pengganti sebelum DROP (komentar; hasil tinjauan manusia).
	if copySug, ok := g.copySuggestion(t, repl); ok {
		s.comment = append(s.comment, "salin data yang belum tersalin ke kolom pengganti sebelum DROP (penjaga di atas menolak bila masih ada):")
		for _, q := range copySug {
			if strings.HasPrefix(q, "\x00") {
				s.comment = append(s.comment, q)
				continue
			}
			s.comment = append(s.comment, safeSuggestion(q)+";")
		}
	}
	if !safeIdent(f.Object) {
		// Nama kolom dari katalog database tidak lolos pola identifier sah: tidak dipakai di SQL aktif (M5).
		s.comment = append(s.comment, "nama kolom tidak lolos pola identifier sah: penjaga otomatis tidak dibangkitkan; tulis tangan")
		return nil
	}
	lock, err := sqlgen.LockTable(t, sqlgen.Options{})
	if err != nil {
		return err
	}
	guard, err := sqlgen.GuardUnsyncedRows(t, f.Object, repl.Name, sqlgen.Options{})
	if err != nil {
		return err
	}
	// LOCK TABLE menahan penulis lain sampai transaksi selesai sehingga tidak ada baris baru di antara penjaga
	// dan DROP COLUMN (TOCTOU); dilewati pada berkas NO TRANSACTION (kunci akan langsung lepas).
	s.up = append(s.up, stmt{sql: lock, txOnly: true}, stmt{sql: guard, block: true})
	return nil
}

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// safeIdent menyatakan nama berpola identifier sah (snake_case, Requirement 1.4). Nama dari katalog database
// (objek berlebih) tidak dipercaya: yang tidak lolos tidak boleh masuk SQL aktif karena baris baru atau anotasi
// goose di dalam identifier terkutip tetap dapat mengacaukan pemisahan pernyataan oleh goose.
func safeIdent(s string) bool { return identRe.MatchString(s) }

// oldColumnKnown menyatakan kolom asal migrateFrom ada pada model atau akan dihapus pada migrasi yang sama.
func (g *generator) oldColumnKnown(t *dbmodel.Table, from string) bool {
	return from != "" && (t.Column(from) != nil || g.removed[t.Name][from])
}

// commentedBlock mengubah teks SQL menjadi baris komentar yang sudah berawalan "-- " (ditandai \x00 agar
// tidak diberi awalan lagi saat dirender).
func commentedBlock(sqlText string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(sqlText, "\n"), "\n") {
		if l == "" {
			out = append(out, "\x00--")
			continue
		}
		out = append(out, "\x00-- "+safeSuggestion(l))
	}
	return out
}

// copySuggestion menghasilkan langkah salin dari kolom lama (migrateFrom) sebagai saran komentar untuk kerangka:
// UPDATE biasa, atau blok batch pada tabel large: true (berkas lalu berpenanda NO TRANSACTION).
func (g *generator) copySuggestion(t *dbmodel.Table, c *dbmodel.Column) (sug []string, ok bool) {
	if !g.oldColumnKnown(t, c.MigrateFrom) {
		return nil, false
	}
	if t.Large {
		q, err := sqlgen.BatchCopyBackfill(t, c.Name, c.MigrateFrom, g.o.Batch, sqlgen.Options{})
		if err != nil {
			return nil, false
		}
		g.res.NoTransaction = true
		return append([]string{"\x00-- salin data dari kolom lama per batch (tabel large: true; berkas NO TRANSACTION):"}, commentedBlock(q)...), true
	}
	q, err := sqlgen.CopyColumn(t, c.Name, c.MigrateFrom, sqlgen.Options{})
	if err != nil {
		return nil, false
	}
	return []string{q}, true
}
