//go:build integration

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/authz"
	"erpora/sdk/module"
)

func recountCommand(t *testing.T) registry.CommandInfo {
	t.Helper()
	reg, err := BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range reg.Commands() {
		if c.Name == "example:recount" {
			return c
		}
	}
	t.Fatal("command example:recount tidak ada")
	return registry.CommandInfo{}
}

func TestRunModuleCommand_EnabledRuns(t *testing.T) {
	ctx, a, dsn, _ := setupAssembly(t)
	if _, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: "acme.example", Enable: true, Actor: "cli:test"}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, DBSlowQuery: 200 * time.Millisecond}

	info := recountCommand(t)
	var seen context.Context
	orig := info.Command.Run
	info.Command.Run = func(c context.Context, inv module.Invocation) error {
		seen = c
		return orig(c, inv)
	}
	var out bytes.Buffer
	err := RunModuleCommand(ctx, cfg, testLog(), CommandRun{Command: info, Out: &out, Actor: "cli:tester", Flags: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "notes: 3\n" {
		t.Errorf("output = %q", out.String())
	}
	if r, ok := authz.SystemReason(seen); !ok || r != "cli:example:recount" {
		t.Errorf("system reason = %q, %v", r, ok)
	}
}

func TestRunModuleCommand_NotEnabledExit3(t *testing.T) {
	ctx, a, dsn, _ := setupAssembly(t)
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, DBSlowQuery: 200 * time.Millisecond}
	info := recountCommand(t)

	check := func(wantStatus string) {
		t.Helper()
		var out bytes.Buffer
		err := RunModuleCommand(ctx, cfg, testLog(), CommandRun{Command: info, Out: &out, Actor: "cli:tester"})
		var xe *ExitError
		if !errors.As(err, &xe) || xe.Code != 3 || !strings.Contains(err.Error(), wantStatus) || !strings.Contains(err.Error(), "acme.example") {
			t.Fatalf("err = %v, want exit 3 berisi %q", err, wantStatus)
		}
		if out.Len() != 0 {
			t.Errorf("command tidak boleh jalan: %q", out.String())
		}
	}
	check("available")

	if _, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: "acme.example", Enable: true, Actor: "cli:test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpDisable, Key: "acme.example", Actor: "cli:test"}); err != nil {
		t.Fatal(err)
	}
	check("disabled")
}
