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
			wantLevel = "level=INFO"
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
	if !strings.Contains(buf.String(), "erp.module.key=acme.notes") {
		t.Errorf("log tanpa module: %q", buf.String())
	}
}

// FR-05: allow-all terlihat di log pada level INFO, sekali per permission (tanpa banjir log).
func TestStub_AllowLoggedAtInfoOncePerPermission(t *testing.T) {
	var buf bytes.Buffer
	s := Stub{Mode: "allow", Log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))}
	for i := 0; i < 3; i++ {
		if !s.Allow(context.Background(), "notes.items.read") {
			t.Fatal("allow expected")
		}
	}
	if n := strings.Count(buf.String(), "permission=notes.items.read"); n != 1 {
		t.Fatalf("allow dicatat %d kali, want 1: %q", n, buf.String())
	}
	if !strings.Contains(buf.String(), "level=INFO") {
		t.Fatalf("bukan INFO: %q", buf.String())
	}
	s.Allow(context.Background(), "notes.items.write")
	if !strings.Contains(buf.String(), "permission=notes.items.write") {
		t.Fatalf("permission berbeda harus dicatat: %q", buf.String())
	}
	// deny tetap dicatat setiap kali.
	var dbuf bytes.Buffer
	d := Stub{Mode: "deny", Log: slog.New(slog.NewTextHandler(&dbuf, nil))}
	d.Allow(context.Background(), "p")
	d.Allow(context.Background(), "p")
	if n := strings.Count(dbuf.String(), "level=WARN"); n != 2 {
		t.Fatalf("deny dicatat %d kali, want 2", n)
	}
}
