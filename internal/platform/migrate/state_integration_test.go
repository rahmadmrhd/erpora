//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/testsupport/pgtest"
)

func TestEnvironment_States(t *testing.T) {
	ctx := context.Background()
	db := open(t, pgtest.Start(t))

	// DB kosong: tabel platform_state belum ada.
	env, ok, err := migrate.Environment(ctx, db)
	if err != nil || ok || env != "" {
		t.Fatalf("DB kosong: env=%q ok=%v err=%v, want \"\" false nil", env, ok, err)
	}

	if err := migrate.Up(ctx, db, config.EnvDevelopment); err != nil {
		t.Fatal(err)
	}
	env, ok, err = migrate.Environment(ctx, db)
	if err != nil || !ok || env != config.EnvDevelopment {
		t.Fatalf("setelah Up: env=%q ok=%v err=%v, want development true nil", env, ok, err)
	}

	// Tabel ada tetapi baris belum ditulis.
	if err := db.Exec("DELETE FROM platform_state WHERE key = 'environment'").Error; err != nil {
		t.Fatal(err)
	}
	env, ok, err = migrate.Environment(ctx, db)
	if err != nil || ok || env != "" {
		t.Fatalf("tanpa baris: env=%q ok=%v err=%v, want \"\" false nil", env, ok, err)
	}

	// Nilai tidak dikenal adalah error.
	if err := db.Exec("INSERT INTO platform_state (key, value) VALUES ('environment', 'staging')").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = migrate.Environment(ctx, db); err == nil {
		t.Fatal("nilai staging: want error")
	}
}
