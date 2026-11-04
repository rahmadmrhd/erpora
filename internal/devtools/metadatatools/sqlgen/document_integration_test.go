//go:build integration

package sqlgen

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Pemicu penguncian dokumen dan pemicu anak hasil Derive atas fixturesales.invoice (task 13.1, 13.2).
// Requirement 13.2, 13.5, 13.8 (a), (a2), 13.9, 13.11, 13.14 sampai 13.16, 13.19.

type docEnv struct {
	t  *testing.T
	db *sql.DB
	n  int
}

func newDocEnv(t *testing.T) *docEnv {
	t.Helper()
	db := openDB(t)
	stmts, err := CreateSchema(fixtureSchema(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		exec(t, db, s.SQL)
	}
	return &docEnv{t: t, db: db}
}

func uid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

func (e *docEnv) id() string { e.n++; return uid(e.n) }

func (e *docEnv) invoice(status string) string {
	e.t.Helper()
	id := e.id()
	exec(e.t, e.db, `INSERT INTO fixturesales_invoices (id, number, issued_on, due_on, customer_name, total, customer_id, status)
VALUES ($1::uuid, 'INV-' || right($1::text, 12), '2026-01-01', '2026-02-01', 'Acme', 100, $2, 'draft')`, id, uid(900))
	if status != "draft" { // lewat jalur sah: draft ke status lain adalah transisi lifecycle biasa
		exec(e.t, e.db, `UPDATE fixturesales_invoices SET status = $2 WHERE id = $1`, id, status)
	}
	return id
}

func (e *docEnv) product() string {
	e.t.Helper()
	id := e.id()
	exec(e.t, e.db, `INSERT INTO fixturesales_products (id, sku, name, unit_price) VALUES ($1::uuid, 'sku-' || $1::text, 'Produk', 10)`, id)
	return id
}

func (e *docEnv) line(invoice, product string) string {
	e.t.Helper()
	id := e.id()
	exec(e.t, e.db, insertLine, id, invoice, product)
	return id
}

const insertLine = `INSERT INTO fixturesales_invoice_lines (id, line_no, product_sku, description, quantity, unit_price, invoice_id, product_id)
VALUES ($1, 1, 'sku', 'baris', 2, 10, $2, $3)`

func (e *docEnv) ok(name, q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		e.t.Errorf("%s: harus lolos: %v", name, err)
	}
}

func (e *docEnv) locked(name, q string, args ...any) {
	e.t.Helper()
	_, err := e.db.Exec(q, args...)
	if sqlState(err) != "55000" {
		e.t.Errorf("%s: harus ditolak SQLSTATE 55000, err = %v", name, err)
	}
}

// Dokumen draft bebas; draft ke deleted adalah transisi status biasa; dokumen terkunci tidak dapat diubah
// atau dihapus langsung (Requirement 13.2, 13.5, 13.8).
func TestDocumentLock_ImmutabilityAndLifecycle(t *testing.T) {
	e := newDocEnv(t)
	inv := e.invoice("draft")
	e.ok("ubah draft bebas", `UPDATE fixturesales_invoices SET approval_note = 'a', customer_name = 'B' WHERE id = $1`, inv)
	e.ok("draft -> deleted", `UPDATE fixturesales_invoices SET status = 'deleted' WHERE id = $1`, inv)
	e.locked("deleted: ubah kolom data", `UPDATE fixturesales_invoices SET approval_note = 'x' WHERE id = $1`, inv)
	e.locked("deleted: hanya version", `UPDATE fixturesales_invoices SET version = version + 1 WHERE id = $1`, inv)
	e.locked("deleted: DELETE", `DELETE FROM fixturesales_invoices WHERE id = $1`, inv)
	e.locked("deleted -> draft", `UPDATE fixturesales_invoices SET status = 'draft' WHERE id = $1`, inv)

	bump := `, version = version + 1, updated_at = now()`
	valid := [][2]string{
		{"draft", "accepted"}, {"draft", "pending_approval"},
		{"pending_approval", "accepted"}, {"pending_approval", "rejected"},
		{"accepted", "cancelled"}, {"accepted", "done"},
	}
	for _, p := range valid {
		id := e.invoice(p[0])
		e.ok(p[0]+" -> "+p[1], `UPDATE fixturesales_invoices SET status = $2`+bump+` WHERE id = $1`, id, p[1])
	}
	invalid := [][2]string{
		{"accepted", "draft"}, {"accepted", "pending_approval"}, {"accepted", "rejected"}, {"accepted", "deleted"},
		{"cancelled", "accepted"}, {"cancelled", "draft"}, {"rejected", "accepted"}, {"rejected", "draft"},
		{"done", "cancelled"}, {"done", "accepted"}, {"pending_approval", "cancelled"}, {"pending_approval", "done"},
		{"deleted", "draft"}, {"deleted", "accepted"},
	}
	for _, p := range invalid {
		id := e.invoice(p[0])
		e.locked(p[0]+" -> "+p[1]+" tak sah", `UPDATE fixturesales_invoices SET status = $2`+bump+` WHERE id = $1`, id, p[1])
	}
	acc := e.invoice("accepted")
	e.locked("transisi sah bersama kolom data", `UPDATE fixturesales_invoices SET status = 'done', approval_note = 'x' WHERE id = $1`, acc)
	e.locked("status sama dengan version naik", `UPDATE fixturesales_invoices SET status = 'accepted', version = version + 1 WHERE id = $1`, acc)
	e.locked("DELETE accepted", `DELETE FROM fixturesales_invoices WHERE id = $1`, acc)
	e.ok("DELETE draft", `DELETE FROM fixturesales_invoices WHERE id = $1`, e.invoice("draft"))
}

// Sumbu status: boleh berubah pada dokumen terkunci hanya dengan pasangan sah (termasuk mundur) dan tanpa
// kolom lain (Requirement 13.8 a2, 13.14 sampai 13.16, 13.19).
func TestDocumentLock_StatusAxes(t *testing.T) {
	e := newDocEnv(t)
	inv := e.invoice("accepted")
	bump := `, version = version + 1, updated_at = now()`
	e.ok("penagihan to_bill -> billed", `UPDATE fixturesales_invoices SET billing_status = 'billed'`+bump+` WHERE id = $1`, inv)
	e.locked("penagihan billed -> to_bill tak sah", `UPDATE fixturesales_invoices SET billing_status = 'to_bill'`+bump+` WHERE id = $1`, inv)
	e.ok("pengiriman to_deliver -> partially_delivered", `UPDATE fixturesales_invoices SET delivery_status = 'partially_delivered'`+bump+` WHERE id = $1`, inv)
	e.ok("pengiriman mundur partially_delivered -> to_deliver", `UPDATE fixturesales_invoices SET delivery_status = 'to_deliver'`+bump+` WHERE id = $1`, inv)
	e.ok("pengiriman to_deliver -> delivered", `UPDATE fixturesales_invoices SET delivery_status = 'delivered'`+bump+` WHERE id = $1`, inv)
	e.ok("pengiriman mundur delivered -> to_deliver", `UPDATE fixturesales_invoices SET delivery_status = 'to_deliver'`+bump+` WHERE id = $1`, inv)

	inv2 := e.invoice("accepted")
	e.ok("dua sumbu sekaligus", `UPDATE fixturesales_invoices SET billing_status = 'billed', delivery_status = 'delivered'`+bump+` WHERE id = $1`, inv2)
	e.ok("sumbu tanpa version dan updated_at", `UPDATE fixturesales_invoices SET delivery_status = 'to_deliver' WHERE id = $1`, inv2)
	e.locked("satu sumbu sah, satu tak sah", `UPDATE fixturesales_invoices SET billing_status = 'to_bill', delivery_status = 'delivered' WHERE id = $1`, inv2)

	inv3 := e.invoice("accepted")
	e.locked("sumbu bersama kolom data", `UPDATE fixturesales_invoices SET delivery_status = 'delivered', customer_name = 'Lain' WHERE id = $1`, inv3)
	e.locked("sumbu bersama status", `UPDATE fixturesales_invoices SET delivery_status = 'delivered', status = 'done' WHERE id = $1`, inv3)
	e.locked("hanya version dan updated_at (tanpa perubahan sumbu)", `UPDATE fixturesales_invoices SET version = version + 1, updated_at = now() WHERE id = $1`, inv3)
	e.locked("kolom data saja", `UPDATE fixturesales_invoices SET customer_name = 'Lain' WHERE id = $1`, inv3)
	// Pemicu BEFORE berjalan sebelum CHECK: nilai di luar daftar pada dokumen terkunci ditolak pemicu (55000);
	// pada dokumen draft CHECK-lah yang menolak (23514).
	e.locked("nilai sumbu di luar daftar pada dokumen terkunci", `UPDATE fixturesales_invoices SET delivery_status = 'tidak_ada' WHERE id = $1`, inv3)
	if _, err := e.db.Exec(`UPDATE fixturesales_invoices SET delivery_status = 'tidak_ada' WHERE id = $1`, e.invoice("draft")); sqlState(err) != "23514" {
		t.Errorf("nilai sumbu di luar daftar pada draft harus melanggar CHECK, err = %v", err)
	}
	e.locked("DELETE dokumen bersumbu", `DELETE FROM fixturesales_invoices WHERE id = $1`, inv3)

	// Dokumen draft: sumbu bebas (belum terkunci).
	d := e.invoice("draft")
	e.ok("draft: sumbu bebas", `UPDATE fixturesales_invoices SET billing_status = 'billed', delivery_status = 'delivered' WHERE id = $1`, d)
}

// Anak embedded dan pivot ditolak bila induk terkunci; pada induk draft bebas (Requirement 13.11).
func TestDocumentLock_ChildTriggers(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()
	tag, tag2 := e.id(), e.id()
	exec(t, e.db, `INSERT INTO fixturesales_tags (id, name) VALUES ($1::uuid, 'tag-' || $1::text), ($2::uuid, 'tag-' || $2::text)`, tag, tag2)

	draft := e.invoice("draft")
	line := e.line(draft, prod)
	e.ok("draft: ubah baris", `UPDATE fixturesales_invoice_lines SET description = 'baru' WHERE id = $1`, line)
	e.ok("draft: tambah tag pivot", `INSERT INTO fixturesales_invoice_tags (invoice_id, tag_id) VALUES ($1, $2)`, draft, tag)
	e.ok("draft: ubah pivot", `UPDATE fixturesales_invoice_tags SET remark = 'x' WHERE invoice_id = $1`, draft)
	e.ok("draft: hapus pivot", `DELETE FROM fixturesales_invoice_tags WHERE invoice_id = $1`, draft)
	extra := e.line(draft, prod)
	e.ok("draft: hapus baris", `DELETE FROM fixturesales_invoice_lines WHERE id = $1`, extra)

	// Induk terkunci.
	exec(t, e.db, `INSERT INTO fixturesales_invoice_tags (invoice_id, tag_id) VALUES ($1, $2)`, draft, tag)
	exec(t, e.db, `UPDATE fixturesales_invoices SET status = 'accepted', version = version + 1 WHERE id = $1`, draft)
	e.locked("terkunci: INSERT baris", insertLine, e.id(), draft, prod)
	e.locked("terkunci: UPDATE baris", `UPDATE fixturesales_invoice_lines SET description = 'x' WHERE id = $1`, line)
	e.locked("terkunci: DELETE baris", `DELETE FROM fixturesales_invoice_lines WHERE id = $1`, line)
	e.locked("terkunci: INSERT pivot", `INSERT INTO fixturesales_invoice_tags (invoice_id, tag_id) VALUES ($1, $2)`, draft, tag2)
	e.locked("terkunci: UPDATE pivot", `UPDATE fixturesales_invoice_tags SET remark = 'x' WHERE invoice_id = $1`, draft)
	e.locked("terkunci: DELETE pivot", `DELETE FROM fixturesales_invoice_tags WHERE invoice_id = $1`, draft)

	// Memindahkan anak antar induk: kedua sisi diperiksa.
	other := e.invoice("draft")
	e.locked("pindah baris dari induk terkunci", `UPDATE fixturesales_invoice_lines SET invoice_id = $2 WHERE id = $1`, line, other)
	free := e.line(other, prod)
	e.locked("pindah baris ke induk terkunci", `UPDATE fixturesales_invoice_lines SET invoice_id = $2 WHERE id = $1`, free, draft)

	// DELETE induk draft membawa anaknya (ON DELETE CASCADE) tanpa terhalang pemicu anak.
	e.ok("hapus induk draft beserta anak", `DELETE FROM fixturesales_invoices WHERE id = $1`, other)
}

// session_replication_role = replica tidak melewati pemicu: pemicu bangkitan ENABLE ALWAYS (Requirement 13.9).
func TestDocumentLock_ReplicaRoleDoesNotBypass(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()
	inv := e.invoice("accepted")
	child := e.invoice("draft")
	line := e.line(child, prod)
	exec(t, e.db, `UPDATE fixturesales_invoices SET status = 'accepted' WHERE id = $1`, child)

	ctx := context.Background()
	conn, err := e.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var super bool
	if err := conn.QueryRowContext(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&super); err != nil || !super {
		t.Skipf("role uji bukan superuser (%v): pengaturan session_replication_role tidak dapat diuji", err)
	}
	if _, err := conn.ExecContext(ctx, `SET session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	for name, q := range map[string]struct {
		sql  string
		args []any
	}{
		"UPDATE dokumen terkunci":   {`UPDATE fixturesales_invoices SET customer_name = 'bypass' WHERE id = $1`, []any{inv}},
		"DELETE dokumen terkunci":   {`DELETE FROM fixturesales_invoices WHERE id = $1`, []any{inv}},
		"UPDATE anak terkunci":      {`UPDATE fixturesales_invoice_lines SET description = 'bypass' WHERE id = $1`, []any{line}},
		"DELETE anak terkunci":      {`DELETE FROM fixturesales_invoice_lines WHERE id = $1`, []any{line}},
		"TRUNCATE dokumen terkunci": {`TRUNCATE fixturesales_invoices CASCADE`, nil},
	} {
		_, err := conn.ExecContext(ctx, q.sql, q.args...)
		if sqlState(err) != "55000" {
			t.Errorf("%s: pemicu harus tetap aktif pada session_replication_role=replica, err = %v", name, err)
		}
	}
}

// TRUNCATE tidak memicu pemicu baris; pemicu statement-level BEFORE TRUNCATE menolaknya bila ada baris terkunci
// dan mengizinkannya bila tabel kosong atau semua baris masih draft (keputusan pemilik produk, 2 Nov 2026).
func TestDocumentLock_Truncate(t *testing.T) {
	e := newDocEnv(t)
	count := func() int {
		t.Helper()
		var n int
		if err := e.db.QueryRow(`SELECT count(*) FROM fixturesales_invoices`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	e.ok("TRUNCATE tabel kosong", `TRUNCATE fixturesales_invoices CASCADE`)
	e.invoice("draft")
	e.invoice("draft")
	e.ok("TRUNCATE semua draft", `TRUNCATE fixturesales_invoices CASCADE`)
	if n := count(); n != 0 {
		t.Fatalf("setelah TRUNCATE sah, baris = %d", n)
	}

	e.invoice("draft")
	for _, status := range []string{"accepted", "deleted", "pending_approval", "rejected", "cancelled", "done"} {
		e.invoice(status)
		e.locked("TRUNCATE dengan baris "+status, `TRUNCATE fixturesales_invoices CASCADE`)
		e.locked("TRUNCATE (tanpa CASCADE) dengan baris "+status, `TRUNCATE fixturesales_invoices, fixturesales_invoice_lines, fixturesales_invoice_tags`)
		exec(t, e.db, `ALTER TABLE fixturesales_invoices DISABLE TRIGGER fixturesales_invoices_lock_trg`)
		exec(t, e.db, `DELETE FROM fixturesales_invoices WHERE status = $1`, status)
		exec(t, e.db, `ALTER TABLE fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg`)
	}
	if n := count(); n != 1 {
		t.Fatalf("TRUNCATE yang ditolak tidak boleh menghapus data: baris = %d, ingin 1", n)
	}
}

// Migrasi tulis tangan yang harus melampaui pengecualian menonaktifkan lalu mengaktifkan pemicu secara eksplisit
// dalam transaksi yang sama; setelahnya pemicu kembali menolak (Requirement 13.10).
func TestDocumentLock_HandwrittenMigrationMayDisableAndReenable(t *testing.T) {
	e := newDocEnv(t)
	inv := e.invoice("accepted")
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`ALTER TABLE fixturesales_invoices DISABLE TRIGGER fixturesales_invoices_lock_trg`,
		`UPDATE fixturesales_invoices SET customer_name = 'dikoreksi migrasi' WHERE id = '` + inv + `'`,
		`ALTER TABLE fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg`,
	} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	e.locked("setelah migrasi pemicu aktif lagi", `UPDATE fixturesales_invoices SET customer_name = 'lagi' WHERE id = $1`, inv)
}

// Balapan: anak tidak dapat disisipkan ke dokumen yang sedang ditransisikan ke terkunci (dan sebaliknya):
// pemicu anak membaca induk dengan FOR SHARE (Requirement 13.11, design D-26).
func TestDocumentLock_ChildInsertRacesWithSubmit(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()

	// Transisi ke accepted belum commit: INSERT anak menunggu, lalu ditolak setelah commit.
	inv := e.invoice("draft")
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE fixturesales_invoices SET status = 'accepted', version = version + 1 WHERE id = $1`, inv); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := e.db.Exec(insertLine, e.id(), inv, prod)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("INSERT anak harus menunggu transisi induk, selesai dengan %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if sqlState(err) != "55000" {
			t.Errorf("INSERT anak setelah induk terkunci harus 55000, err = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("INSERT anak tidak selesai setelah commit")
	}

	// INSERT anak belum commit: transisi induk menunggu, lalu anak sudah ada saat induk terkunci.
	inv2 := e.invoice("draft")
	tx2, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx2.Exec(insertLine, e.id(), inv2, prod); err != nil {
		t.Fatal(err)
	}
	done2 := make(chan error, 1)
	go func() {
		_, err := e.db.Exec(`UPDATE fixturesales_invoices SET status = 'accepted', version = version + 1 WHERE id = $1`, inv2)
		done2 <- err
	}()
	select {
	case err := <-done2:
		t.Fatalf("transisi induk harus menunggu transaksi anak, selesai dengan %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done2:
		if err != nil {
			t.Errorf("transisi induk setelah anak commit harus lolos: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("transisi induk tidak selesai")
	}
	var n int
	if err := e.db.QueryRow(`SELECT count(*) FROM fixturesales_invoice_lines WHERE invoice_id = $1`, inv2).Scan(&n); err != nil || n != 1 {
		t.Errorf("baris anak = %d, %v", n, err)
	}
}

// Pemeriksaan pemicu merujuk tabel lewat skema pemicu sendiri (TG_TABLE_SCHEMA), bukan search_path: tabel bayangan
// di pg_temp (yang dicari lebih dulu oleh PostgreSQL) dan skema kedua dengan search_path berbeda tidak dapat
// membelokkan penjaga TRUNCATE maupun pemicu anak.
func TestDocumentLock_TempShadowAndSearchPathDoNotBypass(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()
	inv := e.invoice("accepted")
	child := e.invoice("draft")
	line := e.line(child, prod)
	exec(t, e.db, `UPDATE fixturesales_invoices SET status = 'accepted' WHERE id = $1`, child)

	ctx := context.Background()
	conn, err := e.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	connExec := func(q string, args ...any) error { _, err := conn.ExecContext(ctx, q, args...); return err }
	var schema string
	if err := conn.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	// Tabel bayangan di pg_temp: kosong, tanpa baris terkunci.
	if err := connExec(`CREATE TEMP TABLE fixturesales_invoices (id uuid, status text)`); err != nil {
		t.Fatal(err)
	}
	if err := connExec(`TRUNCATE ` + schema + `.fixturesales_invoices CASCADE`); sqlState(err) != "55000" {
		t.Errorf("TRUNCATE dengan bayangan pg_temp harus tetap ditolak, err = %v", err)
	}
	if err := connExec(`UPDATE `+schema+`.fixturesales_invoice_lines SET description = 'bypass' WHERE id = $1`, line); sqlState(err) != "55000" {
		t.Errorf("UPDATE anak dengan induk bayangan pg_temp harus tetap ditolak, err = %v", err)
	}
	if err := connExec(`DELETE FROM ` + schema + `.fixturesales_invoices WHERE id = '` + inv + `'`); sqlState(err) != "55000" {
		t.Errorf("DELETE dokumen terkunci, err = %v", err)
	}
	if err := connExec(`DROP TABLE pg_temp.fixturesales_invoices`); err != nil {
		t.Fatal(err)
	}

	// Skema kedua: baris terkunci hanya ada di skema "other"; sesi memakai search_path = skema pertama (tanpa
	// baris terkunci). Penjaga harus memeriksa tabel yang di-TRUNCATE, bukan tabel pada search_path.
	exec(t, e.db, `CREATE SCHEMA other`)
	stmts, err := CreateSchema(fixtureSchema(t), Options{Schema: "other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		exec(t, e.db, s.SQL)
	}
	exec(t, e.db, `INSERT INTO other.fixturesales_invoices (id, number, issued_on, due_on, customer_name, total, customer_id, status)
VALUES ($1::uuid, 'INV-OTHER', '2026-01-01', '2026-02-01', 'Acme', 100, $2, 'draft')`, uid(5000), uid(900))
	exec(t, e.db, `UPDATE other.fixturesales_invoices SET status = 'accepted' WHERE id = $1`, uid(5000))
	if err := connExec(`SET search_path = ` + schema); err != nil {
		t.Fatal(err)
	}
	if err := connExec(`TRUNCATE other.fixturesales_invoices CASCADE`); sqlState(err) != "55000" {
		t.Errorf("TRUNCATE skema lain dengan baris terkunci harus ditolak walau search_path menunjuk skema tanpa baris terkunci, err = %v", err)
	}
	var n int
	if err := e.db.QueryRow(`SELECT count(*) FROM other.fixturesales_invoices`).Scan(&n); err != nil || n != 1 {
		t.Errorf("data skema lain tidak boleh hilang: n=%d err=%v", n, err)
	}
	// Sebaliknya: skema lain hanya berisi draft, skema aktif punya baris terkunci; TRUNCATE skema lain diizinkan.
	exec(t, e.db, `ALTER TABLE other.fixturesales_invoices DISABLE TRIGGER fixturesales_invoices_lock_trg`)
	exec(t, e.db, `DELETE FROM other.fixturesales_invoices`)
	exec(t, e.db, `ALTER TABLE other.fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg`)
	if err := connExec(`TRUNCATE other.fixturesales_invoices CASCADE`); err != nil {
		t.Errorf("TRUNCATE skema lain yang kosong harus lolos walau skema aktif punya baris terkunci: %v", err)
	}
}

// TRUNCATE tabel anak (baris embedded dan pivot): ditolak bila ada baris anak yang induknya terkunci, diizinkan
// bila tabel kosong atau semua induknya draft; TRUNCATE ... CASCADE dari induk ikut ditolak (keputusan 2 Nov 2026).
func TestDocumentLock_ChildTruncate(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()
	tag := e.id()
	exec(t, e.db, `INSERT INTO fixturesales_tags (id, name) VALUES ($1::uuid, 'tag-' || $1::text)`, tag)
	count := func(table string) int {
		t.Helper()
		var n int
		if err := e.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	e.ok("TRUNCATE anak kosong", `TRUNCATE fixturesales_invoice_lines`)
	e.ok("TRUNCATE pivot kosong", `TRUNCATE fixturesales_invoice_tags`)

	draft := e.invoice("draft")
	e.line(draft, prod)
	exec(t, e.db, `INSERT INTO fixturesales_invoice_tags (invoice_id, tag_id) VALUES ($1, $2)`, draft, tag)
	e.ok("TRUNCATE baris dengan semua induk draft", `TRUNCATE fixturesales_invoice_lines`)
	e.ok("TRUNCATE pivot dengan semua induk draft", `TRUNCATE fixturesales_invoice_tags`)

	// Induk terkunci (accepted) di antara induk draft.
	e.line(draft, prod)
	exec(t, e.db, `INSERT INTO fixturesales_invoice_tags (invoice_id, tag_id) VALUES ($1, $2)`, draft, tag)
	locked := e.invoice("draft")
	e.line(locked, prod)
	exec(t, e.db, `INSERT INTO fixturesales_invoice_tags (invoice_id, tag_id) VALUES ($1, $2)`, locked, tag)
	exec(t, e.db, `UPDATE fixturesales_invoices SET status = 'accepted', version = version + 1 WHERE id = $1`, locked)
	e.locked("TRUNCATE baris dengan induk terkunci", `TRUNCATE fixturesales_invoice_lines`)
	e.locked("TRUNCATE pivot dengan induk terkunci", `TRUNCATE fixturesales_invoice_tags`)
	e.locked("TRUNCATE baris + pivot bersamaan", `TRUNCATE fixturesales_invoice_lines, fixturesales_invoice_tags`)
	e.locked("TRUNCATE CASCADE dari induk", `TRUNCATE fixturesales_invoices CASCADE`)
	if count("fixturesales_invoice_lines") != 2 || count("fixturesales_invoice_tags") != 2 || count("fixturesales_invoices") != 2 {
		t.Fatalf("TRUNCATE yang ditolak tidak boleh menghapus data: lines=%d tags=%d invoices=%d",
			count("fixturesales_invoice_lines"), count("fixturesales_invoice_tags"), count("fixturesales_invoices"))
	}
}

// TRUNCATE tabel dokumen dan anaknya hanya sah pada READ COMMITTED: pada REPEATABLE READ dan SERIALIZABLE,
// snapshot lama dapat tidak melihat baris yang baru terkunci (balapan), sehingga ditolak walau tabel kosong.
func TestDocumentLock_TruncateRequiresReadCommitted(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()
	truncates := []string{`TRUNCATE fixturesales_invoices CASCADE`, `TRUNCATE fixturesales_invoice_lines`, `TRUNCATE fixturesales_invoice_tags`}
	tryTx := func(level sql.IsolationLevel, q string, beforeTruncate func()) error {
		t.Helper()
		tx, err := e.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: level})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.Exec(`SELECT count(*) FROM fixturesales_invoices`); err != nil { // snapshot diambil di sini
			t.Fatal(err)
		}
		if beforeTruncate != nil {
			beforeTruncate()
		}
		_, err = tx.Exec(q)
		return err
	}
	for _, q := range truncates {
		for name, level := range map[string]sql.IsolationLevel{"REPEATABLE READ": sql.LevelRepeatableRead, "SERIALIZABLE": sql.LevelSerializable} {
			if err := tryTx(level, q, nil); sqlState(err) != "55000" || !strings.Contains(err.Error(), "read committed") {
				t.Errorf("%s pada %s (tabel kosong) harus ditolak dengan pesan isolation level, err = %v", q, name, err)
			}
		}
		if err := tryTx(sql.LevelReadCommitted, q, nil); err != nil {
			t.Errorf("%s pada READ COMMITTED (tabel kosong) harus lolos: %v", q, err)
		}
	}

	// Balapan: snapshot REPEATABLE READ diambil, lalu transaksi lain meng-commit dokumen terkunci beserta anaknya.
	// Tanpa penjaga isolation level, TRUNCATE melihat tabel kosong dan lolos; kini ditolak.
	racer := func() {
		inv := e.invoice("draft")
		e.line(inv, prod)
		exec(t, e.db, `UPDATE fixturesales_invoices SET status = 'accepted', version = version + 1 WHERE id = `+"'"+inv+"'")
	}
	for _, q := range truncates {
		err := tryTx(sql.LevelRepeatableRead, q, racer)
		if sqlState(err) != "55000" {
			t.Errorf("%s: snapshot lama pada REPEATABLE READ harus ditolak, err = %v", q, err)
		}
		// Bersihkan agar iterasi berikutnya dimulai dari tabel kosong yang sama.
		exec(t, e.db, `ALTER TABLE fixturesales_invoices DISABLE TRIGGER fixturesales_invoices_lock_trg`)
		exec(t, e.db, `ALTER TABLE fixturesales_invoice_lines DISABLE TRIGGER fixturesales_invoice_lines_invoice_id_lock_trg`)
		exec(t, e.db, `DELETE FROM fixturesales_invoice_lines`)
		exec(t, e.db, `DELETE FROM fixturesales_invoices`)
		exec(t, e.db, `ALTER TABLE fixturesales_invoices ENABLE ALWAYS TRIGGER fixturesales_invoices_lock_trg`)
		exec(t, e.db, `ALTER TABLE fixturesales_invoice_lines ENABLE ALWAYS TRIGGER fixturesales_invoice_lines_invoice_id_lock_trg`)
	}
	// Dokumen terkunci yang sudah terlihat tetap ditolak pada READ COMMITTED (perilaku normal).
	racer()
	if err := tryTx(sql.LevelReadCommitted, `TRUNCATE fixturesales_invoices CASCADE`, nil); sqlState(err) != "55000" {
		t.Errorf("READ COMMITTED dengan baris terkunci harus ditolak, err = %v", err)
	}
}

// Fungsi pemicu bangkitan memakai SET search_path = pg_catalog, pg_temp (keputusan pemilik produk, 2 Nov 2026):
// skema "evil" berisi format() dan current_setting() tiruan, dan search_path sesi menaruhnya paling depan. Tanpa
// SET pada fungsi (kontrol negatif: RESET search_path), tiruan itu membuat TRUNCATE dokumen, TRUNCATE anak, INSERT
// anak ke induk terkunci, dan penjaga isolation level lolos; dengan SET semuanya tetap ditolak. UPDATE dan DELETE baris
// terkunci ditolak di kedua keadaan (tidak bergantung pada fungsi tiruan).
func TestDocumentLock_EvilSearchPathSchemaDoesNotBypass(t *testing.T) {
	e := newDocEnv(t)
	prod := e.product()
	lockedInv := e.invoice("accepted")
	parent := e.invoice("draft")
	e.line(parent, prod)
	exec(t, e.db, `UPDATE fixturesales_invoices SET status = 'accepted' WHERE id = $1`, parent) // baris anak di bawah induk terkunci

	exec(t, e.db, `CREATE SCHEMA evil`)
	for _, sig := range []string{"name, name", "name, text", "name, name, name, text", "name, name, name, name", "name", "text", "text, text"} {
		exec(t, e.db, `CREATE FUNCTION evil.format(text, `+sig+`) RETURNS text LANGUAGE sql AS $$ SELECT 'SELECT false'::text $$`)
	}
	exec(t, e.db, `CREATE FUNCTION evil.current_setting(text) RETURNS text LANGUAGE sql AS $$ SELECT 'read committed'::text $$`)

	ctx := context.Background()
	conn, err := e.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var schema string
	if err := conn.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	// Satu pernyataan per transaksi yang selalu di-rollback (data uji tetap utuh), search_path jahat lokal transaksi.
	try := func(level sql.IsolationLevel, q string, args ...any) error {
		t.Helper()
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: level})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = evil, pg_catalog, public`); err != nil {
			t.Fatal(err)
		}
		_, err = tx.ExecContext(ctx, q, args...)
		return err
	}
	insertChild := strings.Replace(insertLine, "fixturesales_invoice_lines", schema+".fixturesales_invoice_lines", 1)
	type probe struct {
		name         string
		level        sql.IsolationLevel
		q            string
		args         []any
		bypassNoSet  bool // lolos bila fungsi tanpa SET search_path (membuktikan probe efektif)
		wantRejected bool
	}
	probes := []probe{
		{"TRUNCATE dokumen CASCADE", sql.LevelReadCommitted, `TRUNCATE ` + schema + `.fixturesales_invoices CASCADE`, nil, true, true},
		{"TRUNCATE anak", sql.LevelReadCommitted, `TRUNCATE ` + schema + `.fixturesales_invoice_lines`, nil, true, true},
		{"TRUNCATE pivot", sql.LevelReadCommitted, `TRUNCATE ` + schema + `.fixturesales_invoice_tags`, nil, true, false},
		{"TRUNCATE dokumen REPEATABLE READ", sql.LevelRepeatableRead, `TRUNCATE ` + schema + `.fixturesales_invoices CASCADE`, nil, true, true},
		{"TRUNCATE anak SERIALIZABLE", sql.LevelSerializable, `TRUNCATE ` + schema + `.fixturesales_invoice_lines`, nil, true, true},
		{"INSERT anak ke induk terkunci", sql.LevelReadCommitted, insertChild, []any{e.id(), lockedInv, prod}, true, true},
		{"UPDATE baris terkunci", sql.LevelReadCommitted, `UPDATE ` + schema + `.fixturesales_invoices SET customer_name = 'bypass' WHERE id = '` + lockedInv + `'`, nil, false, true},
		{"DELETE baris terkunci", sql.LevelReadCommitted, `DELETE FROM ` + schema + `.fixturesales_invoices WHERE id = '` + lockedInv + `'`, nil, false, true},
		{"DELETE anak di bawah induk terkunci", sql.LevelReadCommitted, `DELETE FROM ` + schema + `.fixturesales_invoice_lines`, nil, true, true},
	}
	assertAll := func(withSet bool) {
		t.Helper()
		for _, p := range probes {
			err := try(p.level, p.q, p.args...)
			switch {
			case !p.wantRejected: // tabel kosong: TRUNCATE lolos pada READ COMMITTED
				if err != nil {
					t.Errorf("%s: harus lolos: %v", p.name, err)
				}
			case withSet || !p.bypassNoSet:
				if sqlState(err) != "55000" {
					t.Errorf("%s (dengan SET search_path pada fungsi): harus ditolak SQLSTATE 55000, err = %v", p.name, err)
				}
			default: // kontrol negatif
				if err != nil {
					t.Errorf("%s (kontrol tanpa SET): probe harus berhasil membypass, err = %v", p.name, err)
				}
			}
		}
	}
	assertAll(true)

	// Setiap fungsi pemicu bangkitan membawa proconfig search_path tetap.
	rows, err := e.db.Query(`SELECT p.proname, COALESCE(array_to_string(p.proconfig, ','), '')
FROM pg_proc p JOIN pg_trigger t ON t.tgfoid = p.oid JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND NOT t.tgisinternal ORDER BY 1`, schema)
	if err != nil {
		t.Fatal(err)
	}
	var fns []string
	for rows.Next() {
		var name, cfg string
		if err := rows.Scan(&name, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg != "search_path=pg_catalog, pg_temp" {
			t.Errorf("fungsi %s: proconfig = %q", name, cfg)
		}
		fns = append(fns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(fns) < 6 {
		t.Fatalf("fungsi pemicu yang diperiksa terlalu sedikit: %v", fns)
	}

	// Kontrol negatif: tanpa SET search_path probe yang sama berhasil membypass (bukti probe efektif).
	for _, fn := range fns {
		exec(t, e.db, `ALTER FUNCTION `+schema+`.`+fn+`() RESET search_path`)
	}
	assertAll(false)
	for _, fn := range fns {
		exec(t, e.db, `ALTER FUNCTION `+schema+`.`+fn+`() SET search_path = pg_catalog, pg_temp`)
	}
	assertAll(true)
}
