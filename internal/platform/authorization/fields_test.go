package authorization

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestStubFieldMasker_AllowCopiesAndLogsOnce(t *testing.T) {
	log, buf := bufLogger()
	m := NewStubFieldMasker("allow", log)
	in := map[string]any{"a": 1}
	for range 2 {
		got := m.MaskFields(context.Background(), "x.note", in)
		if !reflect.DeepEqual(got, map[string]any{"a": 1}) {
			t.Fatalf("got %#v", got)
		}
	}
	got := m.MaskFields(context.Background(), "x.note", in)
	got["b"] = 2
	if _, ok := in["b"]; ok {
		t.Error("hasil harus salinan")
	}
	if n := strings.Count(buf.String(), `"level":"INFO"`); n != 1 {
		t.Errorf("log INFO = %d, want 1: %s", n, buf.String())
	}
}

func TestStubFieldMasker_DenyAndUnknownFailClosed(t *testing.T) {
	for _, mode := range []StubMode{"deny", "bogus"} {
		log, buf := bufLogger()
		got := NewStubFieldMasker(mode, log).MaskFields(context.Background(), "x.note", map[string]any{"a": 1})
		if len(got) != 0 {
			t.Errorf("%s: got %#v", mode, got)
		}
		if !strings.Contains(buf.String(), `"level":"WARN"`) {
			t.Errorf("%s: harus log WARN: %s", mode, buf.String())
		}
	}
}
