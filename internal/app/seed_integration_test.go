//go:build integration

package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/lifecycle"
)

func TestRunDevSeeders_TopologicalEnabledOnly(t *testing.T) {
	ctx, a, dsn, db := setupAssembly(t)
	for _, key := range []string{"acme.example", "acme.example-addon"} {
		if _, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: key, WithDeps: true, Enable: true, Actor: "cli:test"}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, DBSlowQuery: 200 * time.Millisecond}
	count := func() int64 {
		var n int64
		if err := db.Raw("SELECT count(*) FROM example_notes").Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	if before != 3 {
		t.Fatalf("notes setelah install = %d, want 3", before)
	}

	// Hanya example yang DevSeeder; exampleaddon enabled tetapi bukan seeder.
	got, err := RunDevSeeders(ctx, cfg, testLog(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"acme.example"}) {
		t.Fatalf("seeded = %v", got)
	}
	after := count()
	if after <= before {
		t.Fatalf("notes tidak bertambah: %d -> %d", before, after)
	}
	// Idempoten: panggilan kedua tidak menambah lagi.
	if _, err := RunDevSeeders(ctx, cfg, testLog(), "example"); err != nil {
		t.Fatal(err)
	}
	if again := count(); again != after {
		t.Fatalf("seed tidak idempoten: %d -> %d", after, again)
	}
}

func TestRunDevSeeders_SkipsNonEnabledWhenAll(t *testing.T) {
	ctx, _, dsn, db := setupAssembly(t)
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, DBSlowQuery: 200 * time.Millisecond}
	got, err := RunDevSeeders(ctx, cfg, testLog(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("tidak ada modul enabled, seeded = %v", got)
	}
	var exists bool
	if err := db.Raw("SELECT to_regclass('example_notes') IS NOT NULL").Scan(&exists).Error; err != nil || exists {
		t.Fatalf("tabel example_notes tidak boleh ada: %v %v", exists, err)
	}
}

func TestRunDevSeeders_OnlyDisabledExit3(t *testing.T) {
	ctx, _, dsn, _ := setupAssembly(t)
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, DBSlowQuery: 200 * time.Millisecond}
	_, err := RunDevSeeders(ctx, cfg, testLog(), "acme.example")
	var xe *ExitError
	if !errors.As(err, &xe) || xe.Code != 3 || !strings.Contains(err.Error(), "acme.example") {
		t.Fatalf("err = %v, want exit 3", err)
	}
	_, err = RunDevSeeders(ctx, cfg, testLog(), "tidak-ada")
	if !errors.As(err, &xe) || xe.Code != 2 {
		t.Fatalf("err = %v, want exit 2 untuk modul tidak dikenal", err)
	}
}
