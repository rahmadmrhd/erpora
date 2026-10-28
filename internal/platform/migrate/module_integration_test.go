//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

const fxDir = "../modules/testdata/migrations/"

func TestApplyModule_AppliesAllAndCallsAfter(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	fsys := os.DirFS(fxDir + "ok")

	var got []migrate.AppliedMigration
	after := func(a migrate.AppliedMigration) error { got = append(got, a); return nil }
	if err := migrate.ApplyModule(ctx, db, "fx-mod", fsys, after); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Version != 1 || got[1].Version != 2 {
		t.Fatalf("after calls = %+v", got)
	}
	for _, a := range got {
		if len(a.Checksum) != 64 || a.Name == "" {
			t.Errorf("applied = %+v", a)
		}
	}
	var reg *string
	if err := db.Raw("SELECT to_regclass('goose_module_fx_mod')::text").Scan(&reg).Error; err != nil || reg == nil {
		t.Fatalf("goose_module_fx_mod missing (%v)", err)
	}

	got = nil
	if err := migrate.ApplyModule(ctx, db, "fx-mod", fsys, after); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("second apply called after %d times, want 0", len(got))
	}
}

func TestApplyModule_SecondFileFailsThenResumes(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))

	var first []migrate.AppliedMigration
	err := migrate.ApplyModule(ctx, db, "fx-fail", os.DirFS(fxDir+"failing"), func(a migrate.AppliedMigration) error {
		first = append(first, a)
		return nil
	})
	var me *migrate.MigrationError
	if !errors.As(err, &me) || me.Version != 2 {
		t.Fatalf("err = %v, want *MigrationError version 2", err)
	}
	if len(first) != 1 || first[0].Version != 1 {
		t.Fatalf("after calls = %+v", first)
	}
	var tbl *string
	if err := db.Raw("SELECT to_regclass('fx_items')::text").Scan(&tbl).Error; err != nil || tbl == nil {
		t.Fatalf("fx_items must remain (%v)", err)
	}
	var cols int64
	if err := db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_name = 'fx_items' AND column_name IN ('note','broken','fixed')").Scan(&cols).Error; err != nil {
		t.Fatal(err)
	}
	if cols != 0 {
		t.Errorf("migrasi 2 tidak ter-rollback: %d kolom parsial", cols)
	}
	st, err := migrate.ModuleStatus(ctx, db, "fx-fail", os.DirFS(fxDir+"failing"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Current != 1 {
		t.Errorf("Current = %d, want 1", st.Current)
	}

	var second []migrate.AppliedMigration
	err = migrate.ApplyModule(ctx, db, "fx-fail", os.DirFS(fxDir+"failing_fixed"), func(a migrate.AppliedMigration) error {
		second = append(second, a)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Version != 2 {
		t.Fatalf("resume after calls = %+v, want only version 2", second)
	}
	var n int64
	if err := db.Raw("SELECT count(*) FROM fx_counter").Scan(&n).Error; err != nil || n != 1 {
		t.Errorf("fx_counter rows = %d (%v), migrasi 1 dijalankan ulang", n, err)
	}
}

func TestModuleStatus_ReadOnly(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))
	if err := migrate.Up(ctx, db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS(fxDir + "ok")
	st, err := migrate.ModuleStatus(ctx, db, "fresh", fsys)
	if err != nil {
		t.Fatal(err)
	}
	if st.Current != 0 || st.Pending != 2 || st.Latest != 2 || st.Target != "module:fresh" {
		t.Errorf("status = %+v", st)
	}
	pending, err := migrate.PendingModuleMigrations(ctx, db, "fresh", fsys)
	if err != nil || len(pending) != 2 || pending[0] != "0001_create_fx_items.sql" {
		t.Errorf("pending = %v (%v)", pending, err)
	}
	var reg *string
	if err := db.Raw("SELECT to_regclass('goose_module_fresh')::text").Scan(&reg).Error; err != nil {
		t.Fatal(err)
	}
	if reg != nil {
		t.Error("ModuleStatus membuat tabel goose_module_fresh")
	}
}
