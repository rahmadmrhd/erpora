package sqlgen

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"erpora/internal/devtools/metadatatools/dbmodel"
)

// Pernyataan ALTER dan DROP untuk pembangkit migrasi (task 11). Semua identifier dikutip; semua literal lewat
// Literal, NumericLiteral, atau DefaultLiteral; tidak ada teks berkas yang disalin.

func (o Options) table(t *dbmodel.Table) (string, error) { return o.qualify(t.Name) }

func (o Options) column(t *dbmodel.Table, name string) (string, error) {
	if findColumn(t, name) == nil {
		return "", fmt.Errorf("kolom %q tidak ada pada %s", name, t.Name)
	}
	return QuoteIdent(name)
}

// DropTable menghasilkan DROP TABLE.
func DropTable(t *dbmodel.Table, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	return "DROP TABLE " + n, nil
}

// DropColumn menghasilkan ALTER TABLE ... DROP COLUMN; kolom harus ada pada model t.
func DropColumn(t *dbmodel.Table, column string, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	c, err := o.column(t, column)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " DROP COLUMN " + c, nil
}

// DropIndex menghasilkan DROP INDEX (indeks berada di skema tabelnya; o.Schema mengualifikasi).
func DropIndex(name string, o Options) (string, error) {
	n, err := o.qualify(name)
	if err != nil {
		return "", err
	}
	return "DROP INDEX " + n, nil
}

// DropConstraint menghasilkan ALTER TABLE ... DROP CONSTRAINT.
func DropConstraint(t *dbmodel.Table, name string, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	c, err := QuoteIdent(name)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " DROP CONSTRAINT " + c, nil
}

// SetDefault menghasilkan ALTER COLUMN ... SET DEFAULT dari nilai bawaan model.
func SetDefault(t *dbmodel.Table, c *dbmodel.Column, o Options) (string, error) {
	if c.Default == nil {
		return "", fmt.Errorf("kolom %s tidak punya nilai bawaan di model", c.Name)
	}
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	col, err := o.column(t, c.Name)
	if err != nil {
		return "", err
	}
	lit, err := DefaultLiteral(c.Default)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " ALTER COLUMN " + col + " SET DEFAULT " + lit, nil
}

func alterColumn(t *dbmodel.Table, column, action string, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	c, err := o.column(t, column)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " ALTER COLUMN " + c + " " + action, nil
}

// DropDefault menghasilkan ALTER COLUMN ... DROP DEFAULT.
func DropDefault(t *dbmodel.Table, column string, o Options) (string, error) {
	return alterColumn(t, column, "DROP DEFAULT", o)
}

// DropNotNull menghasilkan ALTER COLUMN ... DROP NOT NULL.
func DropNotNull(t *dbmodel.Table, column string, o Options) (string, error) {
	return alterColumn(t, column, "DROP NOT NULL", o)
}

// SetNotNull menghasilkan ALTER COLUMN ... SET NOT NULL.
func SetNotNull(t *dbmodel.Table, column string, o Options) (string, error) {
	return alterColumn(t, column, "SET NOT NULL", o)
}

// AddUnique menghasilkan ALTER TABLE ... ADD CONSTRAINT ... UNIQUE.
func AddUnique(t *dbmodel.Table, u *dbmodel.Unique, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	name, err := QuoteIdent(u.Name)
	if err != nil {
		return "", err
	}
	for _, c := range u.Columns {
		if findColumn(t, c) == nil {
			return "", fmt.Errorf("unique %s merujuk kolom %q yang tidak ada", u.Name, c)
		}
	}
	cols, err := quoteList(u.Columns)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " ADD CONSTRAINT " + name + " UNIQUE (" + cols + ")", nil
}

// AddCheck menghasilkan ALTER TABLE ... ADD CONSTRAINT ... CHECK.
func AddCheck(t *dbmodel.Table, ck *dbmodel.Check, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	name, err := QuoteIdent(ck.Name)
	if err != nil {
		return "", err
	}
	e, err := CheckExpression(t, ck)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " ADD CONSTRAINT " + name + " CHECK (" + e + ")", nil
}

// DropTrigger menghasilkan DROP TRIGGER ... ON ....
func DropTrigger(t *dbmodel.Table, name string, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	trg, err := QuoteIdent(name)
	if err != nil {
		return "", err
	}
	return "DROP TRIGGER " + trg + " ON " + n, nil
}

// DropFunction menghasilkan DROP FUNCTION untuk fungsi pemicu tanpa argumen.
func DropFunction(name string, o Options) (string, error) {
	n, err := o.qualify(name)
	if err != nil {
		return "", err
	}
	return "DROP FUNCTION " + n + "()", nil
}

// EnableTrigger menghasilkan ALTER TABLE ... ENABLE ALWAYS TRIGGER: pemicu aktif untuk semua sesi, juga bila
// session_replication_role = replica (Requirement 13.9).
func EnableTrigger(t *dbmodel.Table, name string, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	trg, err := QuoteIdent(name)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + n + " ENABLE ALWAYS TRIGGER " + trg, nil
}

// MaxBackfillBatch membatasi ukuran batch backfill.
const MaxBackfillBatch = 100000

func backfillLoop(table, set, where, key string, batch int) string {
	var sb strings.Builder
	sb.WriteString("\nDECLARE\n  affected bigint;\nBEGIN\n  LOOP\n")
	sb.WriteString("    UPDATE " + table + " SET " + set + " WHERE " + key + " IN (SELECT " + key + " FROM " + table + " WHERE " + where + " ORDER BY " + key + " LIMIT " + strconv.Itoa(batch) + ");\n")
	sb.WriteString("    GET DIAGNOSTICS affected = ROW_COUNT;\n    EXIT WHEN affected = 0;\n    COMMIT;\n  END LOOP;\nEND\n")
	return sb.String()
}

func backfillParts(t *dbmodel.Table, batch int, o Options) (table, key string, err error) {
	if batch < 1 || batch > MaxBackfillBatch {
		return "", "", fmt.Errorf("ukuran batch %d di luar 1..%d", batch, MaxBackfillBatch)
	}
	if len(t.PrimaryKey) != 1 {
		return "", "", errors.New("backfill per batch membutuhkan kunci utama satu kolom")
	}
	if table, err = o.table(t); err != nil {
		return "", "", err
	}
	if key, err = o.column(t, t.PrimaryKey[0]); err != nil {
		return "", "", err
	}
	return table, key, nil
}

// BatchCopyBackfill menghasilkan blok DO yang menyalin kolom asal ke kolom tujuan per batch dan melakukan
// COMMIT di antara batch, untuk tabel large: true (Requirement 11.4, 12.2). Blok memakai transaksi prosedural
// sehingga HARUS dijalankan di luar transaksi: berkas migrasi wajib berpenanda goose NO TRANSACTION, dan blok
// dibungkus StatementBegin dan StatementEnd oleh pemanggil. Hanya baris dengan tujuan NULL dan asal terisi
// yang disalin, sehingga aman diulang.
func BatchCopyBackfill(t *dbmodel.Table, to, from string, batch int, o Options) (string, error) {
	table, key, err := backfillParts(t, batch, o)
	if err != nil {
		return "", err
	}
	toQ, err := o.column(t, to)
	if err != nil {
		return "", err
	}
	// Kolom asal boleh sudah tidak ada pada model (akan dihapus pada migrasi yang sama).
	fromQ, err := QuoteIdent(from)
	if err != nil {
		return "", err
	}
	body := backfillLoop(table, toQ+" = "+fromQ, toQ+" IS NULL AND "+fromQ+" IS NOT NULL", key, batch)
	return "DO " + DollarQuote(body), nil
}

// BatchBackfillTemplate menghasilkan templat backfill per batch SEBAGAI KOMENTAR (setiap baris diawali "-- ")
// untuk kerangka migrasi pada tabel large: true; nilai yang diisi adalah keputusan manusia
// (placeholder ekspresi), sehingga tidak pernah aktif tanpa ditinjau.
func BatchBackfillTemplate(t *dbmodel.Table, column string, batch int, o Options) (string, error) {
	table, key, err := backfillParts(t, batch, o)
	if err != nil {
		return "", err
	}
	col, err := o.column(t, column)
	if err != nil {
		return "", err
	}
	body := backfillLoop(table, col+" = <ISI: ekspresi nilai>", col+" IS NULL", key, batch)
	stmt := "DO " + DollarQuote(body)
	var sb strings.Builder
	for _, line := range strings.Split(strings.TrimRight(stmt, "\n"), "\n") {
		if line == "" {
			sb.WriteString("--\n")
			continue
		}
		sb.WriteString("-- " + line + "\n")
	}
	return sb.String(), nil
}

// CopyColumn menghasilkan UPDATE yang menyalin kolom asal ke kolom tujuan untuk baris yang tujuannya masih NULL
// dan asalnya terisi (rename lewat migrateFrom, Requirement 12.2). Kolom tujuan harus ada pada model; kolom asal
// boleh sudah tidak ada pada model (akan dihapus pada migrasi yang sama). Aman diulang. Untuk tabel
// large: true pakai BatchCopyBackfill.
func CopyColumn(t *dbmodel.Table, to, from string, o Options) (string, error) {
	table, err := o.table(t)
	if err != nil {
		return "", err
	}
	toQ, err := o.column(t, to)
	if err != nil {
		return "", err
	}
	fromQ, err := QuoteIdent(from)
	if err != nil {
		return "", err
	}
	return "UPDATE " + table + " SET " + toQ + " = " + fromQ + " WHERE " + toQ + " IS NULL AND " + fromQ + " IS NOT NULL", nil
}

// GuardUnsyncedRows menghasilkan blok DO penjaga penghapusan kolom (Requirement 12.8): migrasi gagal bila masih
// ada baris dengan kolom lama terisi dan kolom pengganti kosong. Kolom pengganti harus ada pada model; kolom
// lama boleh sudah tidak ada pada model. Pemanggil membungkusnya dengan StatementBegin dan StatementEnd.
func GuardUnsyncedRows(t *dbmodel.Table, oldColumn, newColumn string, o Options) (string, error) {
	table, err := o.table(t)
	if err != nil {
		return "", err
	}
	oldQ, err := QuoteIdent(oldColumn)
	if err != nil {
		return "", err
	}
	newQ, err := o.column(t, newColumn)
	if err != nil {
		return "", err
	}
	msg, err := Literal(strings.ReplaceAll("penghapusan kolom ditolak: masih ada baris dengan kolom lama terisi dan kolom pengganti kosong ("+t.Name+")", "%", "%%"))
	if err != nil {
		return "", err
	}
	body := "\nBEGIN\n  IF EXISTS (SELECT 1 FROM " + table + " WHERE " + oldQ + " IS NOT NULL AND " + newQ + " IS NULL) THEN\n    RAISE EXCEPTION " + msg + ";\n  END IF;\nEND\n"
	return "DO " + DollarQuote(body), nil
}

// LockTable menghasilkan LOCK TABLE ... IN SHARE ROW EXCLUSIVE MODE: menahan penulis lain (pembaca tetap boleh)
// sampai transaksi selesai, dipakai sebelum penjaga dan DROP COLUMN agar tidak ada baris yang masuk di antara
// pemeriksaan dan penghapusan (TOCTOU). Hanya bermakna di dalam transaksi.
func LockTable(t *dbmodel.Table, o Options) (string, error) {
	n, err := o.table(t)
	if err != nil {
		return "", err
	}
	return "LOCK TABLE " + n + " IN SHARE ROW EXCLUSIVE MODE", nil
}
