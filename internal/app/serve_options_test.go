package app

import (
	"testing"
	"time"

	"erpora/internal/platform/config"
)

func TestServeOpenOptions_UsesConfiguredStatementTimeout(t *testing.T) {
	got := serveOpenOptions(config.Config{DBStatementTimeout: 7 * time.Second})
	if got.StatementTimeout != 7*time.Second {
		t.Errorf("StatementTimeout = %v, want 7s", got.StatementTimeout)
	}
}
