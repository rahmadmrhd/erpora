package authorization

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"erpora/internal/platform/observability"
)

func TestStub_AllowAndDenyLogPermission(t *testing.T) {
	for mode, want := range map[StubMode]bool{"allow": true, "deny": false} {
		var buf bytes.Buffer
		s := Stub{Mode: mode, Log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
		if got := s.Allow(context.Background(), "notes.items.read"); got != want {
			t.Errorf("mode %s: Allow = %v, want %v", mode, got, want)
		}
		if !strings.Contains(buf.String(), "permission=notes.items.read") {
			t.Errorf("mode %s: log tanpa permission: %q", mode, buf.String())
		}
		wantLevel := "level=WARN"
		if want {
			wantLevel = "level=DEBUG"
		}
		if !strings.Contains(buf.String(), wantLevel) {
			t.Errorf("mode %s: log harus %s: %q", mode, wantLevel, buf.String())
		}
	}
}

func TestStub_NilLoggerAndUnknownModeDenies(t *testing.T) {
	if (Stub{Mode: "bogus"}).Allow(context.Background(), "x") {
		t.Error("mode tidak dikenal harus menolak")
	}
}

func TestStub_LogsModuleFromContext(t *testing.T) {
	var buf bytes.Buffer
	s := Stub{Mode: "deny", Log: slog.New(slog.NewTextHandler(&buf, nil))}
	s.Allow(observability.WithModule(context.Background(), "acme.notes"), "notes.items.read")
	if !strings.Contains(buf.String(), "module=acme.notes") {
		t.Errorf("log tanpa module: %q", buf.String())
	}
}
