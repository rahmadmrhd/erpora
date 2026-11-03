//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/settings/store"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
	"erpora/internal/platform/testsupport/secretkey"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

// perTenant memetakan tiap tenant ke database terpisah (ADR-0004: isolasi per database).
type perTenant map[tenancy.TenantID]database.TenantDB

func (p perTenant) For(_ context.Context, id tenancy.TenantID) (database.TenantDB, error) {
	db, ok := p[id]
	if !ok {
		return database.TenantDB{}, database.ErrUnknownTenant
	}
	return db, nil
}

func openMigrated(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: pgtest.Start(t), DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
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
	return db
}

func sealer(t *testing.T) *crypto.Sealer {
	t.Helper()
	s, err := crypto.NewSealerFromMaterial(secretkey.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ctxOf(t tenancy.TenantID) context.Context { return tenancy.WithTenant(context.Background(), t) }

func TestStore(t *testing.T) {
	dbA, dbB := openMigrated(t), openMigrated(t)
	conn := perTenant{tenantA: {DB: dbA}, tenantB: {DB: dbB}}
	s := store.New(conn, sealer(t))
	a, b := ctxOf(tenantA), ctxOf(tenantB)

	t.Run("belum ada: ErrNotFound", func(t *testing.T) {
		if _, err := s.Get(a, store.ScopePlatform, "", "platform.public_url"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("round trip dan timpa", func(t *testing.T) {
		in := store.Setting{Scope: store.ScopePlatform, Key: "platform.public_url", Value: json.RawMessage(`"https://erp.example.com"`)}
		if err := s.Upsert(a, in, "admin@example.com"); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(a, store.ScopePlatform, "", "platform.public_url")
		if err != nil || string(got.Value) != `"https://erp.example.com"` || got.Encrypted || got.UpdatedBy != "admin@example.com" ||
			got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() || got.UpdatedAt.Location() != time.UTC {
			t.Fatalf("Get = %+v, %v", got, err)
		}
		first := got
		time.Sleep(20 * time.Millisecond)
		in.Value = json.RawMessage(`"https://erp2.example.com"`)
		if err := s.Upsert(a, in, "other"); err != nil {
			t.Fatal(err)
		}
		got, _ = s.Get(a, store.ScopePlatform, "", "platform.public_url")
		if string(got.Value) != `"https://erp2.example.com"` || got.UpdatedBy != "other" || !got.CreatedAt.Equal(first.CreatedAt) || !got.UpdatedAt.After(first.UpdatedAt) {
			t.Fatalf("setelah timpa = %+v (created_at tetap, updated_at maju)", got)
		}
		// nilai JSON apa pun (objek) tersimpan utuh
		obj := store.Setting{Scope: store.ScopeModule, ModuleKey: "erp.core", Key: "core.banner", Value: json.RawMessage(`{"a":[1,2],"b":null}`)}
		if err := s.Upsert(a, obj, "x"); err != nil {
			t.Fatal(err)
		}
		g, err := s.Get(a, store.ScopeModule, "erp.core", "core.banner")
		var back map[string]any
		if err != nil || json.Unmarshal(g.Value, &back) != nil || len(back) != 2 {
			t.Fatalf("objek = %s, %v", g.Value, err)
		}
		// modul lain dengan key sama tidak bertabrakan
		if _, err := s.Get(a, store.ScopeModule, "erp.other", "core.banner"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("modul lain: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("upsert serentak: satu baris, nilai salah satu penulis", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				v, _ := json.Marshal(i)
				if err := s.Upsert(a, store.Setting{Scope: store.ScopePlatform, Key: "race.counter", Value: v}, "r"); err != nil {
					t.Errorf("Upsert: %v", err)
				}
			}(i)
		}
		wg.Wait()
		var n int
		if err := dbA.Raw(`SELECT count(*) FROM settings WHERE key = 'race.counter'`).Scan(&n).Error; err != nil || n != 1 {
			t.Fatalf("baris = %d, %v", n, err)
		}
	})

	t.Run("nilai terenkripsi tidak tersimpan sebagai plaintext dan terikat konteks", func(t *testing.T) {
		in := store.Setting{Scope: store.ScopePlatform, Key: "smtp.password", Value: json.RawMessage(`"rahasia-sekali"`), Encrypted: true}
		if err := s.Upsert(a, in, "x"); err != nil {
			t.Fatal(err)
		}
		var raw string
		var enc bool
		row := dbA.Raw(`SELECT value::text, encrypted FROM settings WHERE key = 'smtp.password'`).Row()
		if err := row.Scan(&raw, &enc); err != nil {
			t.Fatal(err)
		}
		if !enc || strings.Contains(raw, "rahasia-sekali") {
			t.Fatalf("tersimpan %q encrypted=%v: plaintext tidak boleh ada", raw, enc)
		}
		got, err := s.Get(a, store.ScopePlatform, "", "smtp.password")
		if err != nil || string(got.Value) != `"rahasia-sekali"` || !got.Encrypted {
			t.Fatalf("Get = %+v, %v", got, err)
		}
		// menyalin blob ke key lain (SQL langsung) tidak dapat dibuka: AAD terikat
		if err := dbA.Exec(`INSERT INTO settings (scope, module_key, key, value, encrypted)
			SELECT scope, module_key, 'smtp.copy', value, true FROM settings WHERE key = 'smtp.password'`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(a, store.ScopePlatform, "", "smtp.copy"); err == nil {
			t.Error("blob yang dipindah ke key lain harus gagal dibuka")
		}
		// tanpa Sealer, nilai terenkripsi tidak dapat dibaca maupun ditulis
		noSeal := store.New(conn, nil)
		if _, err := noSeal.Get(a, store.ScopePlatform, "", "smtp.password"); !errors.Is(err, store.ErrSealerRequired) {
			t.Errorf("Get tanpa sealer: err = %v, want ErrSealerRequired", err)
		}
		if err := noSeal.Upsert(a, in, "x"); !errors.Is(err, store.ErrSealerRequired) {
			t.Errorf("Upsert tanpa sealer: err = %v, want ErrSealerRequired", err)
		}
	})

	t.Run("isolasi tenant: database terpisah", func(t *testing.T) {
		if err := s.Upsert(a, store.Setting{Scope: store.ScopePlatform, Key: "tenant.marker", Value: json.RawMessage(`"A"`)}, "x"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(b, store.ScopePlatform, "", "tenant.marker"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("tenant B melihat nilai tenant A: err = %v", err)
		}
		if err := s.Upsert(b, store.Setting{Scope: store.ScopePlatform, Key: "tenant.marker", Value: json.RawMessage(`"B"`)}, "x"); err != nil {
			t.Fatal(err)
		}
		ga, _ := s.Get(a, store.ScopePlatform, "", "tenant.marker")
		gb, _ := s.Get(b, store.ScopePlatform, "", "tenant.marker")
		if string(ga.Value) != `"A"` || string(gb.Value) != `"B"` {
			t.Errorf("A=%s B=%s", ga.Value, gb.Value)
		}
		// tenant tak dikenal connector: gagal tertutup
		if _, err := s.Get(ctxOf(tenancy.TenantID(uuid.New())), store.ScopePlatform, "", "tenant.marker"); !errors.Is(err, database.ErrUnknownTenant) {
			t.Errorf("tenant tak dikenal: err = %v, want ErrUnknownTenant", err)
		}
		// tanpa tenant di context
		if _, err := s.Get(context.Background(), store.ScopePlatform, "", "tenant.marker"); !errors.Is(err, store.ErrNoTenant) {
			t.Errorf("tanpa tenant: err = %v, want ErrNoTenant", err)
		}
	})

	t.Run("masukan tidak valid ditolak sebelum DB", func(t *testing.T) {
		for name, in := range map[string]store.Setting{
			"scope":      {Scope: "x", Key: "a.b", Value: json.RawMessage(`1`)},
			"key":        {Scope: store.ScopePlatform, Key: "A B", Value: json.RawMessage(`1`)},
			"bukan JSON": {Scope: store.ScopePlatform, Key: "a.b", Value: json.RawMessage(`{`)},
		} {
			if err := s.Upsert(a, in, "x"); !errors.Is(err, store.ErrInvalidArgument) {
				t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
			}
		}
		if _, err := s.Get(a, store.ScopePlatform, "", "A B"); !errors.Is(err, store.ErrInvalidArgument) {
			t.Errorf("Get key tidak valid: err = %v, want ErrInvalidArgument", err)
		}
	})
}
