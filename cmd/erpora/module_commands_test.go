package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
)

func rootWithGroups(out *bytes.Buffer) (*cobra.Command, *rootOpts) {
	opts := &rootOpts{}
	root := &cobra.Command{Use: "erpora", SilenceUsage: true, SilenceErrors: true}
	root.AddGroup(&cobra.Group{ID: groupModule, Title: "Modul"})
	root.SetOut(out)
	root.SetErr(out)
	return root, opts
}

func TestRoot_HasModuleCommandInModuleGroup(t *testing.T) {
	root := newRootCmd(&bytes.Buffer{}, &bytes.Buffer{})
	var found *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "example:recount" {
			found = c
		}
	}
	if found == nil || found.GroupID != groupModule {
		t.Fatalf("example:recount = %+v", found)
	}
	out, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Modul:") || !strings.Contains(out, "example:recount") {
		t.Errorf("list tidak memuat grup Modul:\n%s", out)
	}
}

func TestModuleCommand_FlagsDefaultsReachInvocation(t *testing.T) {
	reg := []registry.CommandInfo{{
		Module: "acme.fake", Name: "fake:count",
		Command: module.Command{
			Name: "count", Short: "Hitung", Flags: []module.Flag{{Name: "limit", Usage: "batas", Default: "5"}, {Name: "mode", Usage: "mode"}},
			Run: func(context.Context, module.Invocation) error { return nil },
		},
	}}
	var out bytes.Buffer
	root, opts := rootWithGroups(&out)
	var got app.CommandRun
	addModuleCommandsFrom(root, opts, reg, func(_ context.Context, _ *rootOpts, r app.CommandRun) error {
		got = r
		return nil
	})
	root.SetArgs([]string{"fake:count", "a", "b"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Command.Name != "fake:count" && got.Command.Name != "count" {
		t.Errorf("command = %+v", got.Command)
	}
	if got.Flags["limit"] != "5" || got.Flags["mode"] != "" {
		t.Errorf("flags = %v", got.Flags)
	}
	if len(got.Args) != 2 || got.Args[0] != "a" || !strings.HasPrefix(got.Actor, "cli:") {
		t.Errorf("run = %+v", got)
	}
	if _, ok := got.Flags["limit"]; !ok {
		t.Error("flag limit harus ada")
	}

	root, opts = rootWithGroups(&out)
	addModuleCommandsFrom(root, opts, reg, func(_ context.Context, _ *rootOpts, r app.CommandRun) error {
		got = r
		return nil
	})
	root.SetArgs([]string{"fake:count", "--limit", "9"})
	if err := root.Execute(); err != nil || got.Flags["limit"] != "9" {
		t.Fatalf("err = %v flags = %v", err, got.Flags)
	}
}
