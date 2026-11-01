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
	if len(stmts) != 2 || stmts[0].Kind != KindCreateFunction || stmts[1].Kind != KindCreateTrigger {
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
