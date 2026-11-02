//go:build integration

package sqlgen

import (
	"context"
	"database/sql"
	"fmt"
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
		{"draft", "accepted"}, {"draft", "need_approval"},
		{"need_approval", "accepted"}, {"need_approval", "rejected"},
		{"accepted", "cancelled"}, {"accepted", "done"},
	}
	for _, p := range valid {
		id := e.invoice(p[0])
		e.ok(p[0]+" -> "+p[1], `UPDATE fixturesales_invoices SET status = $2`+bump+` WHERE id = $1`, id, p[1])
	}
	invalid := [][2]string{
		{"accepted", "draft"}, {"accepted", "need_approval"}, {"accepted", "rejected"}, {"accepted", "deleted"},
		{"cancelled", "accepted"}, {"cancelled", "draft"}, {"rejected", "accepted"}, {"rejected", "draft"},
		{"done", "cancelled"}, {"done", "accepted"}, {"need_approval", "cancelled"}, {"need_approval", "done"},
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
		"UPDATE dokumen terkunci": {`UPDATE fixturesales_invoices SET customer_name = 'bypass' WHERE id = $1`, []any{inv}},
		"DELETE dokumen terkunci": {`DELETE FROM fixturesales_invoices WHERE id = $1`, []any{inv}},
		"UPDATE anak terkunci":    {`UPDATE fixturesales_invoice_lines SET description = 'bypass' WHERE id = $1`, []any{line}},
		"DELETE anak terkunci":    {`DELETE FROM fixturesales_invoice_lines WHERE id = $1`, []any{line}},
	} {
		_, err := conn.ExecContext(ctx, q.sql, q.args...)
		if sqlState(err) != "55000" {
			t.Errorf("%s: pemicu harus tetap aktif pada session_replication_role=replica, err = %v", name, err)
		}
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
