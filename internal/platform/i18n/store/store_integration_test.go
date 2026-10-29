//go:build integration

package store_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

func setup(t *testing.T) (*gorm.DB, *store.Store) {
	t.Helper()
	dsn := pgtest.Start(t)
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	return db, store.New(database.ControlDB{DB: db})
}

func ov(ns, loc, key, msg string) store.Override {
	return store.Override{Namespace: ns, Locale: loc, Key: key, Message: msg, UpdatedBy: "api:dev"}
}

func revOf(t *testing.T, s *store.Store, tn tenancy.TenantID, ns, loc string) int64 {
	t.Helper()
	revs, err := s.Revisions(context.Background(), tn)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range revs {
		if r.Namespace == ns && r.Locale == loc {
			return r.Revision
		}
	}
	return 0
}

// P8 Idempotensi: Upsert dua kali dengan nilai sama meninggalkan keadaan sama dan tidak menaikkan revisi pada
// panggilan kedua; mengubah nilai menaikkan revisi dalam transaksi yang sama.
func TestUpsert_IdempotentAndBumpsRevisionOnlyOnChange(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	changed, rev, err := s.Upsert(ctx, tenantA, ov("example", "id", "note.title", "Catatan"), nil)
	if err != nil || !changed || rev != 1 {
		t.Fatalf("pertama: changed=%v rev=%d err=%v", changed, rev, err)
	}
	changed, rev, err = s.Upsert(ctx, tenantA, ov("example", "id", "note.title", "Catatan"), nil)
	if err != nil || changed || rev != 1 {
		t.Fatalf("nilai sama: changed=%v rev=%d err=%v, want false 1", changed, rev, err)
	}
	changed, rev, err = s.Upsert(ctx, tenantA, ov("example", "id", "note.title", "Catatan Baru"), nil)
	if err != nil || !changed || rev != 2 {
		t.Fatalf("nilai berubah: changed=%v rev=%d err=%v, want true 2", changed, rev, err)
	}
	got, err := s.Get(ctx, tenantA, "example", "id", "note.title")
	if err != nil || got.Message != "Catatan Baru" || got.UpdatedBy != "api:dev" || got.UpdatedAt.IsZero() {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	// bahasa lain: revisi sendiri
	if _, rev, _ := s.Upsert(ctx, tenantA, ov("example", "en", "note.title", "Note"), nil); rev != 1 {
		t.Errorf("revisi bahasa en = %d, want 1", rev)
	}
	if revOf(t, s, tenantA, "example", "id") != 2 {
		t.Error("revisi id tidak boleh ikut naik")
	}
}

func TestDelete_BumpsRevisionOnlyWhenRowExists(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	existed, rev, err := s.Delete(ctx, tenantA, "example", "id", "tidak.ada", nil)
	if err != nil || existed || rev != 0 {
		t.Fatalf("hapus kunci tak ada: existed=%v rev=%d err=%v", existed, rev, err)
	}
	if got := revOf(t, s, tenantA, "example", "id"); got != 0 {
		t.Fatalf("revisi setelah hapus tak ada = %d, want 0", got)
	}
	_, _, _ = s.Upsert(ctx, tenantA, ov("example", "id", "a", "x"), nil)
	existed, rev, err = s.Delete(ctx, tenantA, "example", "id", "a", nil)
	if err != nil || !existed || rev != 2 {
		t.Fatalf("hapus ada: existed=%v rev=%d err=%v, want true 2", existed, rev, err)
	}
	if _, err := s.Get(ctx, tenantA, "example", "id", "a"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get setelah hapus err = %v", err)
	}
	// hapus kedua: tidak ada perubahan
	if existed, rev, _ := s.Delete(ctx, tenantA, "example", "id", "a", nil); existed || rev != 2 {
		t.Errorf("hapus kedua: existed=%v rev=%d", existed, rev)
	}
}

// IDOR tingkat store (tenant A vs B): list, get, put, delete tidak pernah menembus tenant lain, dan revisi
// terpisah per tenant (P6).
func TestTenantIsolation(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	if _, _, err := s.Upsert(ctx, tenantA, ov("example", "id", "note.title", "Milik A"), nil); err != nil {
		t.Fatal(err)
	}
	// B tidak melihat milik A
	if _, err := s.Get(ctx, tenantB, "example", "id", "note.title"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get lintas tenant err = %v, want ErrNotFound", err)
	}
	if list, total, err := s.List(ctx, tenantB, store.Filter{}, 1, 50); err != nil || total != 0 || len(list) != 0 {
		t.Errorf("List B = %v, %d, %v", list, total, err)
	}
	if rows, err := s.ListLocale(ctx, tenantB, "example", "id"); err != nil || len(rows) != 0 {
		t.Errorf("ListLocale B = %v, %v", rows, err)
	}
	if revs, _ := s.Revisions(ctx, tenantB); len(revs) != 0 {
		t.Errorf("Revisions B = %v", revs)
	}
	// B menghapus kunci yang sama: tidak berefek pada A
	if existed, _, err := s.Delete(ctx, tenantB, "example", "id", "note.title", nil); err != nil || existed {
		t.Errorf("Delete B: existed=%v err=%v", existed, err)
	}
	if got, err := s.Get(ctx, tenantA, "example", "id", "note.title"); err != nil || got.Message != "Milik A" {
		t.Errorf("milik A rusak: %+v %v", got, err)
	}
	// B menulis kunci yang sama: baris terpisah, A tidak berubah, revisi B mulai dari 1
	if changed, rev, err := s.Upsert(ctx, tenantB, ov("example", "id", "note.title", "Milik B"), nil); err != nil || !changed || rev != 1 {
		t.Errorf("Upsert B: %v %d %v", changed, rev, err)
	}
	if got, _ := s.Get(ctx, tenantA, "example", "id", "note.title"); got.Message != "Milik A" {
		t.Errorf("Upsert B menimpa A: %+v", got)
	}
	if revOf(t, s, tenantA, "example", "id") != 1 || revOf(t, s, tenantB, "example", "id") != 1 {
		t.Error("revisi harus terpisah per tenant")
	}
}

func TestPreconditionAbortsWithoutChange(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	_, _, _ = s.Upsert(ctx, tenantA, ov("example", "id", "a", "satu"), nil)
	errStale := errors.New("basi")
	var seen *store.Override
	changed, rev, err := s.Upsert(ctx, tenantA, ov("example", "id", "a", "dua"), func(cur *store.Override) error {
		seen = cur
		return errStale
	})
	if !errors.Is(err, errStale) || changed || rev != 0 {
		t.Fatalf("Upsert dengan precondition gagal: changed=%v rev=%d err=%v", changed, rev, err)
	}
	if seen == nil || seen.Message != "satu" {
		t.Errorf("precondition harus melihat baris terkunci: %+v", seen)
	}
	if got, _ := s.Get(ctx, tenantA, "example", "id", "a"); got.Message != "satu" || revOf(t, s, tenantA, "example", "id") != 1 {
		t.Errorf("precondition gagal tidak boleh mengubah apa pun: %+v", got)
	}
	// baris belum ada: precondition menerima nil
	var gotNil bool
	_, _, _ = s.Upsert(ctx, tenantA, ov("example", "id", "baru", "x"), func(cur *store.Override) error { gotNil = cur == nil; return nil })
	if !gotNil {
		t.Error("precondition harus menerima nil untuk baris belum ada")
	}
	if _, _, err := s.Delete(ctx, tenantA, "example", "id", "a", func(*store.Override) error { return errStale }); !errors.Is(err, errStale) {
		t.Errorf("Delete precondition err = %v", err)
	}
	if _, err := s.Get(ctx, tenantA, "example", "id", "a"); err != nil {
		t.Errorf("Delete dibatalkan, baris harus tetap ada: %v", err)
	}
}

func TestListFilterAndPagination(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	for _, o := range []store.Override{
		ov("example", "id", "b", "1"), ov("example", "id", "a", "2"), ov("example", "en", "a", "3"), ov("ui", "id", "x", "4"),
	} {
		if _, _, err := s.Upsert(ctx, tenantA, o, nil); err != nil {
			t.Fatal(err)
		}
	}
	all, total, err := s.List(ctx, tenantA, store.Filter{}, 1, 50)
	if err != nil || total != 4 || len(all) != 4 {
		t.Fatalf("List = %d/%d %v", len(all), total, err)
	}
	if all[0].Namespace != "example" || all[0].Locale != "en" || all[3].Namespace != "ui" {
		t.Errorf("urutan (namespace, locale, key) salah: %+v", all)
	}
	if got, total, _ := s.List(ctx, tenantA, store.Filter{Namespace: "example"}, 1, 50); total != 3 || len(got) != 3 {
		t.Errorf("filter namespace: %d/%d", len(got), total)
	}
	if got, total, _ := s.List(ctx, tenantA, store.Filter{Namespace: "example", Locale: "id"}, 1, 50); total != 2 || len(got) != 2 {
		t.Errorf("filter namespace+locale: %d/%d", len(got), total)
	}
	p2, total, _ := s.List(ctx, tenantA, store.Filter{}, 2, 3)
	if total != 4 || len(p2) != 1 || p2[0].Namespace != "ui" {
		t.Errorf("halaman 2: %+v total=%d", p2, total)
	}
	rows, err := s.ListLocale(ctx, tenantA, "example", "id")
	if err != nil || len(rows) != 2 || rows[0].Key != "a" {
		t.Errorf("ListLocale = %+v %v", rows, err)
	}
}
