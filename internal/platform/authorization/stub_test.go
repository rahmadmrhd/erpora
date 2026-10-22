package authorization

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestStub_AllowAndDenyLogPermission(t *testing.T) {
	for mode, want := range map[StubMode]bool{"allow": true, "deny": false} {
		var buf bytes.Buffer
		s := Stub{Mode: mode, Log: slog.New(slog.NewTextHandler(&buf, nil))}
		if got := s.Allow(context.Background(), "notes.items.read"); got != want {
			t.Errorf("mode %s: Allow = %v, want %v", mode, got, want)
		}
		if !strings.Contains(buf.String(), "permission=notes.items.read") {
			t.Errorf("mode %s: log tanpa permission: %q", mode, buf.String())
		}
	}
}

func TestStub_NilLoggerAndUnknownModeDenies(t *testing.T) {
	if (Stub{Mode: "bogus"}).Allow(context.Background(), "x") {
		t.Error("mode tidak dikenal harus menolak")
	}
}
