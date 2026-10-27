package dbtools

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"

	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/app"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/lifecycle"
)

// FreshOptions mengatur Fresh.
type FreshOptions struct {
	Seed, Yes bool
	In        io.Reader
	Out       io.Writer
}

func openDB(ctx context.Context, cfg config.Config, log *slog.Logger) (*database.DB, func(), error) {
	db, err := database.Open(ctx, cfg, tnoop.NewTracerProvider(), log)
	if err != nil {
		return nil, nil, err
	}
	return db, func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}, nil
}

// cliActor adalah actor audit "cli:<os-user>".
func cliActor() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		name = "unknown"
	}
	return "cli:" + name
}

// Fresh membangun ulang database development dari nol. Urutan (keputusan 17): periksa ERP_ENV SEBELUM membuka
// DB -> buka DB -> periksa penanda -> konfirmasi mengetik nama database (kecuali Yes) -> hapus semua objek ->
// migrate up -> modul required -> install + enable semua modul -> seed (opsional). Setiap penolakan terjadi sebelum
// ada perubahan.
func Fresh(ctx context.Context, cfg config.Config, log *slog.Logger, o FreshOptions) error {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	if err := app.DestructiveEnvAllowed(cfg); err != nil {
		return err
	}
	db, cleanup, err := openDB(ctx, cfg, log)
	if err != nil {
		return err
	}
	if err := app.GuardDestructive(ctx, db, cfg); err != nil {
		cleanup()
		return err
	}
	if !o.Yes {
		var name string
		if err := db.WithContext(ctx).Raw("SELECT current_database()").Scan(&name).Error; err != nil {
			cleanup()
			return fmt.Errorf("baca nama database: %w", err)
		}
		in := o.In
		if in == nil {
			in = eofReader{}
		}
		if err := Confirm(in, out, name); err != nil {
			cleanup()
			return &app.ExitError{Code: 1, Err: err}
		}
	}
	if err := DropAll(ctx, db); err != nil {
		cleanup()
		return fmt.Errorf("db:fresh: hapus objek: %w", err)
	}
	cleanup() // handle baru setelah drop: cache prepared statement lama tidak boleh terbawa
	fmt.Fprintln(out, "semua objek database dihapus")

	db, cleanup, err = openDB(ctx, cfg, log)
	if err != nil {
		return err
	}
	err = migrate.UpWithTenant(ctx, db, cfg.Env, cfg.DefaultTenantID)
	cleanup()
	if err != nil {
		return fmt.Errorf("db:fresh: migrate up: %w", err)
	}
	if err := app.EnsureRequiredModules(ctx, cfg, log); err != nil {
		return err
	}
	if err := installAll(ctx, cfg, log, out); err != nil {
		return err
	}
	if !o.Seed {
		return nil
	}
	return runSeed(ctx, cfg, log, "", out)
}

// installAll meng-install dan meng-enable semua modul terkompilasi (--all --with-deps --enable).
func installAll(ctx context.Context, cfg config.Config, log *slog.Logger, out io.Writer) error {
	svc, tid, cleanup, err := app.NewLifecycle(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer cleanup()
	results, err := svc.Execute(ctx, lifecycle.Request{Tenant: tid, Op: lifecycle.OpInstall, All: true, WithDeps: true, Enable: true, Actor: cliActor()})
	for _, r := range results {
		fmt.Fprintf(out, "%s  %s  %s -> %s  %s\n", r.Module, r.Op, r.From, r.To, r.Status)
		if r.Status == "failed" && r.Error != nil {
			return fmt.Errorf("modul %s gagal pada operasi %s: %s", r.Module, r.Op, r.Error.Message)
		}
	}
	return err
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }
