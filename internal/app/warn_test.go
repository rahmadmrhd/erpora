package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"erpora/internal/platform/config"
)

func TestWarnInsecureConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		warn bool
	}{
		{"allow di production", config.Config{Env: config.EnvProduction, AuthzStub: "allow"}, true},
		{"deny di production", config.Config{Env: config.EnvProduction, AuthzStub: "deny"}, false},
		{"allow di development", config.Config{Env: config.EnvDevelopment, AuthzStub: "allow"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			warnInsecureConfig(slog.New(slog.NewTextHandler(&buf, nil)), tc.cfg)
			got := strings.Contains(buf.String(), "level=WARN") && strings.Contains(buf.String(), "ERP_AUTHZ_STUB")
			if got != tc.warn {
				t.Errorf("warn = %v, want %v: %q", got, tc.warn, buf.String())
			}
		})
	}
}
