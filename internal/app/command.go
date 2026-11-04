package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/sdk/module"
)

// CommandRun adalah satu eksekusi command modul `<slug>:<name>`.
type CommandRun struct {
	Command registry.CommandInfo
	Args    []string
	Flags   map[string]string
	Out     io.Writer
	// In adalah masukan standar command (stdin proses; mis. sandi core:create-admin); nil = tidak ada.
	In    io.Reader
	Actor string
}

// RunModuleCommand menjalankan command modul hanya bila modulnya berstatus efektif enabled; selain itu
// ExitError{Code: 3, "modul <key> berstatus <status>"} (FR-22.4). ctx command = runtime.ModuleContext dengan
// hak sistem teraudit "cli:<name>" (name = "<slug>:<command>", jadi command core berawalan "cli:core:", syarat
// identity.Admin; PRD-001 Q30 butir 1) dan span "command <name>"; pemakaian dicatat di log (actor, system_reason, durasi).
//
// Command modul core (runtime.CoreModuleKey) mewajibkan ERP_SECRET_KEY valid (exit 2, diperiksa sebelum database
// dibuka) dan mendapat identity.Admin dari varian stack CLI tanpa Service/server HTTP (PRD-001 Q28); modul lain
// tidak pernah menerimanya.
func RunModuleCommand(ctx context.Context, cfg config.Config, log *slog.Logger, run CommandRun) error {
	isCore := run.Command.Module == runtime.CoreModuleKey
	if isCore {
		if err := cfg.RequireSecretKey(); err != nil {
			return &ExitError{Code: 2, Err: err}
		}
	}
	rt, err := openCLI(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	if err := rt.guardWrite(ctx); err != nil {
		return err
	}

	key, name := run.Command.Module, run.Command.Name
	st, _, err := rt.svc.Module(ctx, rt.tid, key)
	if err != nil {
		return err
	}
	if st.Status != runtime.StatusEnabled {
		return &ExitError{Code: 3, Err: fmt.Errorf("modul %s berstatus %s; aktifkan dengan `erpora modules enable %s`", key, st.Status, key)}
	}

	if isCore {
		// Stack dihentikan bersama command: ctx dibatalkan saat selesai (keyring, daftar sesi tercabut).
		ictx, stop := context.WithCancel(ctx)
		defer stop()
		idSvc := newSettingsService(cfg, rt.db, rt.tid)
		stack, err := newIdentityStack(ictx, cfg, log, rt.db, rt.tid, newSettingsURLSource(cfg, idSvc, rt.tid), identityExtras{Settings: idSvc, CLI: true})
		if err != nil {
			return &ExitError{Code: 2, Err: fmt.Errorf("mesin identitas untuk command core: %w", err)}
		}
		rt.services.Identity = stack.installCoreIdentity
	}

	reason := "cli:" + name
	ctx, span := otel.Tracer("erpora/cli").Start(ctx, "command "+name, trace.WithAttributes(attribute.String("erp.module.key", key), attribute.String("erp.actor", run.Actor)))
	defer span.End()
	mctx, env := runtime.ModuleContext(ctx, rt.tid, key, rt.conn, log, reason, rt.services)

	out := run.Out
	if out == nil {
		out = io.Discard
	}
	flags := run.Flags
	if flags == nil {
		flags = map[string]string{}
	}
	started := time.Now()
	err = run.Command.Command.Run(mctx, module.Invocation{Args: run.Args, Flags: flags, Out: out, In: run.In, Env: env})
	log.InfoContext(mctx, "command modul selesai", "command", name, "actor", run.Actor, "system_reason", reason,
		"erp.module.key", key, "duration_ms", time.Since(started).Milliseconds(), "error", err)
	if err != nil {
		span.RecordError(err)
	}
	return err
}
