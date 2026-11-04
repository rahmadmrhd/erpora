package sqlgen

import (
	"testing"

	"erpora/internal/devtools/metadatatools/dbmodel"
)

func TestCopyColumnAndGuard(t *testing.T) {
	tb := &dbmodel.Table{
		Name: "m_things", Kind: dbmodel.KindResource,
		Columns: []*dbmodel.Column{
			{Name: "id", Type: dbmodel.TypeUUID, NotNull: true},
			{Name: "phone_e164", Type: dbmodel.TypeText},
		},
		PrimaryKey: []string{"id"},
	}
	// Kolom asal boleh sudah tidak ada pada model (akan dihapus); tujuan harus ada.
	s, err := CopyColumn(tb, "phone_e164", "phone", Options{})
	if err != nil || s != `UPDATE "m_things" SET "phone_e164" = "phone" WHERE "phone_e164" IS NULL AND "phone" IS NOT NULL` {
		t.Errorf("CopyColumn = %q, %v", s, err)
	}
	if _, err := CopyColumn(tb, "nope", "phone", Options{}); err == nil {
		t.Error("tujuan tak ada harus ditolak")
	}
	g, err := GuardUnsyncedRows(tb, "phone", "phone_e164", Options{})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "guard_unsynced.golden.sql", g+";\n")
	if n := len(splitTopLevel(t, g)); n != 1 {
		t.Errorf("harus satu pernyataan: %d", n)
	}
	if _, err := GuardUnsyncedRows(tb, "phone", "nope", Options{}); err == nil {
		t.Error("kolom pengganti tak ada harus ditolak")
	}
	if _, err := GuardUnsyncedRows(tb, "a\x00b", "phone_e164", Options{}); err == nil {
		t.Error("nama tak sah harus ditolak")
	}
}

func TestBatchCopyBackfillSourceMayBeOutsideModel(t *testing.T) {
	tb := &dbmodel.Table{
		Name: "m_things", Kind: dbmodel.KindResource,
		Columns:    []*dbmodel.Column{{Name: "id", Type: dbmodel.TypeUUID, NotNull: true}, {Name: "phone_e164", Type: dbmodel.TypeText}},
		PrimaryKey: []string{"id"},
	}
	if _, err := BatchCopyBackfill(tb, "phone_e164", "phone", 10, Options{}); err != nil {
		t.Errorf("kolom asal yang akan dihapus harus boleh: %v", err)
	}
}
