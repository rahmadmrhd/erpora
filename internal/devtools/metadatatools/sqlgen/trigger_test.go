package sqlgen

import (
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

func lockTable(t *testing.T, cond any, spec func(*dbmodel.LockSpec)) (*dbmodel.Table, *dbmodel.Trigger) {
	t.Helper()
	tb := &dbmodel.Table{
		Name: "m_docs", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true, FieldType: metadata.TypeUUID},
			{Name: "status", Type: dbmodel.TypeText, NotNull: true, Field: "status", FieldType: metadata.TypeEnum},
			{Name: "version", Type: dbmodel.TypeBigint, NotNull: true, Field: "version", FieldType: metadata.TypeInteger},
			{Name: "updated_at", Type: dbmodel.TypeTimestamp, NotNull: true, Field: "updatedAt", FieldType: metadata.TypeDateTime},
			{Name: "updated_by", Type: dbmodel.TypeUUID, Field: "updatedBy", FieldType: metadata.TypeUUID},
			{Name: "phone_e164", Type: dbmodel.TypeText, Field: "phoneE164", FieldType: metadata.TypeString, MigrateFrom: "phone"},
			{Name: "phone", Type: dbmodel.TypeText, Field: "phone", FieldType: metadata.TypeString},
		},
		PrimaryKey: []string{"id"},
	}
	ls := &dbmodel.LockSpec{
		Condition:    parseCond(t, cond),
		StatusColumn: "status", VersionColumn: "version", UpdatedAtColumn: "updated_at", UpdatedByColumn: "updated_by",
		Transitions:  [][2]string{{"draft", "submitted"}, {"submitted", "cancelled"}},
		MigratePairs: [][2]string{{"phone_e164", "phone"}},
	}
	if spec != nil {
		spec(ls)
	}
	tr := &dbmodel.Trigger{Name: "m_docs_lock_trg", Function: "m_docs_lock_fn", Events: []string{"DELETE", "UPDATE"}, Lock: ls}
	tb.Triggers = []*dbmodel.Trigger{tr}
	return tb, tr
}

func TestLockTrigger_Golden(t *testing.T) {
	tb, tr := lockTable(t, leaf("status", "ne", "draft"), nil)
	stmts, err := TriggerStatements(tb, tr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 || stmts[0].Kind != KindCreateFunction || stmts[1].Kind != KindCreateTrigger || stmts[2].Kind != KindEnableTrigger {
		t.Fatalf("stmts = %+v", stmts)
	}
	for _, st := range stmts {
		if n := len(splitTopLevel(t, st.SQL)); n != 1 {
			t.Errorf("pernyataan %s terpecah menjadi %d", st.Object, n)
		}
	}
	golden(t, "lock_trigger.golden.sql", joinStatements(stmts))
}

// Badan fungsi dari templat tetap: nilai berbahaya pada lockWhen hanya muncul sebagai literal ter-escape dan
// penutup kutip dolar dipilih agar tidak ada di badan.
func TestLockTrigger_DangerousConditionValue(t *testing.T) {
	bad := "x$erpora$ ; DROP TABLE m_docs; -- $erpora1$"
	tb, tr := lockTable(t, leaf("status", "ne", bad), func(ls *dbmodel.LockSpec) {
		ls.Transitions = [][2]string{{bad, "y'z"}}
	})
	stmts, err := TriggerStatements(tb, tr, Options{Schema: "s"})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range stmts {
		if n := len(splitTopLevel(t, st.SQL)); n != 1 {
			t.Fatalf("pernyataan %s terpecah menjadi %d:\n%s", st.Object, n, st.SQL)
		}
	}
	if !strings.Contains(stmts[0].SQL, "$erpora2$") {
		t.Errorf("tag dolar harus menghindari tabrakan:\n%s", stmts[0].SQL)
	}
	golden(t, "lock_trigger_dangerous.golden.sql", joinStatements(stmts))
}

func TestLockTrigger_RejectsUntranslatableCondition(t *testing.T) {
	// lockWhen untuk pemicu hanya eq, ne, in, isnull atas kolom sendiri (D-11, D-26).
	tb, tr := lockTable(t, leaf("version", "gt", 3), nil)
	if _, err := TriggerStatements(tb, tr, Options{}); err == nil {
		t.Error("gt pada lockWhen harus ditolak")
	}
	tb, tr = lockTable(t, leaf("nope", "eq", "x"), nil)
	if _, err := TriggerStatements(tb, tr, Options{}); err == nil {
		t.Error("field tak dikenal harus ditolak")
	}
}

func TestLockTrigger_NoExceptionsWhenEmpty(t *testing.T) {
	tb, tr := lockTable(t, leaf("status", "ne", "draft"), func(ls *dbmodel.LockSpec) {
		ls.Transitions = nil
		ls.MigratePairs = nil
	})
	stmts, err := TriggerStatements(tb, tr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stmts[0].SQL, "migrate") {
		t.Errorf("tanpa pasangan migrasi, tidak ada cabang pengecualian migrasi:\n%s", stmts[0].SQL)
	}
}

func axesSpec(ls *dbmodel.LockSpec) {
	ls.Axes = []dbmodel.LockAxis{
		{Column: "billing_status", Transitions: [][2]string{{"to_bill", "billed"}}},
		{Column: "delivery_status", Transitions: [][2]string{{"to_deliver", "partially_delivered"}, {"partially_delivered", "to_deliver"}}},
	}
}

func lockTableWithAxes(t *testing.T) (*dbmodel.Table, *dbmodel.Trigger) {
	tb, tr := lockTable(t, leaf("status", "ne", "draft"), axesSpec)
	tb.Columns = append(tb.Columns,
		&dbmodel.Column{Name: "billing_status", Type: dbmodel.TypeText, NotNull: true, Field: "billingStatus", FieldType: metadata.TypeEnum},
		&dbmodel.Column{Name: "delivery_status", Type: dbmodel.TypeText, NotNull: true, Field: "deliveryStatus", FieldType: metadata.TypeEnum})
	return tb, tr
}

// (a2): kolom sumbu status bersama version, updated_at, updated_by dengan pasangan sah; sekurangnya satu sumbu
// berubah; status dan kolom data lain tidak boleh berubah.
func TestLockTrigger_StatusAxesGolden(t *testing.T) {
	tb, tr := lockTableWithAxes(t)
	stmts, err := TriggerStatements(tb, tr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := stmts[0].SQL
	contains := func(s string) {
		t.Helper()
		if !strings.Contains(body, s) {
			t.Errorf("tidak memuat %q:\n%s", s, body)
		}
	}
	contains(`ARRAY['billing_status', 'delivery_status', 'updated_at', 'updated_by', 'version']`)
	contains(`(OLD."billing_status", NEW."billing_status") IN (('to_bill', 'billed'))`)
	contains(`(OLD."delivery_status", NEW."delivery_status") IN (('to_deliver', 'partially_delivered'), ('partially_delivered', 'to_deliver'))`)
	contains(`OLD."billing_status" IS DISTINCT FROM NEW."billing_status" OR OLD."delivery_status" IS DISTINCT FROM NEW."delivery_status"`)
	golden(t, "lock_trigger_axes.golden.sql", joinStatements(stmts))
}

func TestLockTrigger_AxisColumnMustExist(t *testing.T) {
	tb, tr := lockTable(t, leaf("status", "ne", "draft"), axesSpec) // kolom sumbu tidak ditambahkan
	if _, err := TriggerStatements(tb, tr, Options{}); err == nil {
		t.Error("kolom sumbu tak ada harus galat")
	}
}

// Pemicu aktif untuk semua sesi (ENABLE ALWAYS): session_replication_role=replica tidak menonaktifkannya.
func TestLockTrigger_EnabledAlways(t *testing.T) {
	tb, tr := lockTable(t, leaf("status", "ne", "draft"), nil)
	stmts, err := TriggerStatements(tb, tr, Options{Schema: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 || stmts[2].Kind != KindEnableTrigger || stmts[2].SQL != `ALTER TABLE "s"."m_docs" ENABLE ALWAYS TRIGGER "m_docs_lock_trg"` {
		t.Fatalf("stmts = %+v", stmts)
	}
}

func childFixture(t *testing.T) (*dbmodel.Table, *dbmodel.Trigger, *dbmodel.Table) {
	parent, _ := lockTable(t, leaf("status", "ne", "draft"), nil)
	parent.Name = "m_docs"
	child := &dbmodel.Table{
		Name: "m_doc_lines", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true},
			{Name: "doc_id", Type: dbmodel.TypeUUID, NotNull: true},
		},
		PrimaryKey: []string{"id"},
	}
	tr := &dbmodel.Trigger{Name: "m_doc_lines_doc_id_lock_trg", Function: "m_doc_lines_doc_id_lock_fn", Events: []string{"DELETE", "INSERT", "UPDATE"},
		Child: &dbmodel.ChildLock{Parent: parent, FKColumn: "doc_id"}}
	parent.Lock = parseCond(t, leaf("status", "ne", "draft"))
	child.Triggers = []*dbmodel.Trigger{tr}
	return child, tr, parent
}

func TestChildLockTrigger_Golden(t *testing.T) {
	child, tr, _ := childFixture(t)
	stmts, err := TriggerStatements(child, tr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 || stmts[0].Kind != KindCreateFunction || stmts[1].Kind != KindCreateTrigger || stmts[2].Kind != KindEnableTrigger {
		t.Fatalf("stmts = %+v", stmts)
	}
	if !strings.Contains(stmts[1].SQL, "BEFORE DELETE OR INSERT OR UPDATE ON") {
		t.Errorf("trigger = %s", stmts[1].SQL)
	}
	for _, st := range stmts {
		if n := len(splitTopLevel(t, st.SQL)); n != 1 {
			t.Errorf("pernyataan %s terpecah menjadi %d", st.Object, n)
		}
	}
	for _, want := range []string{"FOR SHARE OF p;", `p."status" IS DISTINCT FROM 'draft'`, `OLD."doc_id"`, `NEW."doc_id"`} {
		if !strings.Contains(stmts[0].SQL, want) {
			t.Errorf("fungsi tidak memuat %q:\n%s", want, stmts[0].SQL)
		}
	}
	golden(t, "child_lock_trigger.golden.sql", joinStatements(stmts))
}

func TestChildLockTrigger_Errors(t *testing.T) {
	child, tr, parent := childFixture(t)
	parent.Lock = nil
	if _, err := TriggerStatements(child, tr, Options{}); err == nil {
		t.Error("induk tanpa kondisi kunci harus galat")
	}
	child, tr, _ = childFixture(t)
	tr.Child.FKColumn = "nope"
	if _, err := TriggerStatements(child, tr, Options{}); err == nil {
		t.Error("kolom FK tak ada harus galat")
	}
	child, tr, _ = childFixture(t)
	tr.Child.Parent.Lock = parseCond(t, leaf("version", "gt", 3))
	if _, err := TriggerStatements(child, tr, Options{}); err == nil {
		t.Error("kondisi kunci induk yang tak dapat diterjemahkan harus galat")
	}
}

// Nilai berbahaya pada kondisi kunci induk hanya menjadi literal.
func TestChildLockTrigger_DangerousParentCondition(t *testing.T) {
	child, tr, parent := childFixture(t)
	parent.Lock = parseCond(t, leaf("status", "ne", "x$erpora$'; DROP TABLE m_docs; --"))
	stmts, err := TriggerStatements(child, tr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range stmts {
		if n := len(splitTopLevel(t, st.SQL)); n != 1 {
			t.Fatalf("pernyataan %s terpecah menjadi %d:\n%s", st.Object, n, st.SQL)
		}
	}
	if !strings.Contains(stmts[0].SQL, "$erpora1$") {
		t.Errorf("penanda dolar harus menghindari tabrakan:\n%s", stmts[0].SQL)
	}
}
