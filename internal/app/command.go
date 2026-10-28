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
	Actor   string
}

// RunModuleCommand menjalankan command modul hanya bila modulnya berstatus efektif enabled; selain itu
// ExitError{Code: 3, "modul <key> berstatus <status>"} (FR-22.4). ctx command = runtime.ModuleContext dengan
// hak sistem teraudit "cli:<name>" dan span "command <name>"; pemakaian dicatat di log (actor, system_reason, durasi).
func RunModuleCommand(ctx context.Context, cfg config.Config, log *slog.Logger, run CommandRun) error {
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
	err = run.Command.Command.Run(mctx, module.Invocation{Args: run.Args, Flags: flags, Out: out, Env: env})
	log.InfoContext(mctx, "command modul selesai", "command", name, "actor", run.Actor, "system_reason", reason,
		"erp.module.key", key, "duration_ms", time.Since(started).Milliseconds(), "error", err)
	if err != nil {
		span.RecordError(err)
	}
	return err
}
